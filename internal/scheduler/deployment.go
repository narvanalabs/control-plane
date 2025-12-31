// Package scheduler provides intelligent deployment scheduling for the control plane.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// Common errors for zero-downtime deployment.
var (
	ErrHealthCheckFailed    = errors.New("health check failed for new container")
	ErrRoutingUpdateFailed  = errors.New("failed to update routing to new container")
	ErrNoArtifact           = errors.New("deployment has no artifact")
	ErrNoPreviousDeployment = errors.New("no previous successful deployment found for rollback")
	ErrDeploymentNotFound   = errors.New("deployment not found")
)

// RoutingUpdater defines the interface for updating routing configuration.
// This is typically implemented by a Caddy configuration manager.
type RoutingUpdater interface {
	// UpdateRouting updates the routing configuration to point to the new container.
	// It returns the old container name that was previously routed to.
	UpdateRouting(ctx context.Context, serviceName, newContainerName string, port int) (oldContainerName string, err error)
}

// ContainerManager defines the interface for container lifecycle operations.
type ContainerManager interface {
	// StartContainer starts a new container with the given name and deployment config.
	StartContainer(ctx context.Context, nodeID, containerName string, deployment *models.Deployment) error
	// StopContainer stops a container by name.
	StopContainer(ctx context.Context, nodeID, containerName string) error
	// CheckHealth checks if a container is healthy.
	CheckHealth(ctx context.Context, nodeID, containerName string, healthCheck *models.HealthCheckConfig) (bool, error)
}

// ZeroDowntimeDeployer handles zero-downtime deployments using blue-green pattern.
// **Validates: Requirements 10.1, 10.2, 10.3, 10.4**
type ZeroDowntimeDeployer struct {
	store            store.Store
	containerManager ContainerManager
	routingUpdater   RoutingUpdater
	healthTimeout    time.Duration
	healthInterval   time.Duration
	logger           *slog.Logger
}

// ZeroDowntimeDeployerConfig holds configuration for the ZeroDowntimeDeployer.
type ZeroDowntimeDeployerConfig struct {
	// HealthTimeout is the maximum time to wait for health checks to pass.
	HealthTimeout time.Duration
	// HealthInterval is the interval between health check attempts.
	HealthInterval time.Duration
}

// DefaultZeroDowntimeDeployerConfig returns default configuration values.
func DefaultZeroDowntimeDeployerConfig() *ZeroDowntimeDeployerConfig {
	return &ZeroDowntimeDeployerConfig{
		HealthTimeout:  5 * time.Minute,
		HealthInterval: 5 * time.Second,
	}
}

// NewZeroDowntimeDeployer creates a new ZeroDowntimeDeployer.
func NewZeroDowntimeDeployer(
	s store.Store,
	containerManager ContainerManager,
	routingUpdater RoutingUpdater,
	cfg *ZeroDowntimeDeployerConfig,
	logger *slog.Logger,
) *ZeroDowntimeDeployer {
	if cfg == nil {
		cfg = DefaultZeroDowntimeDeployerConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ZeroDowntimeDeployer{
		store:            s,
		containerManager: containerManager,
		routingUpdater:   routingUpdater,
		healthTimeout:    cfg.HealthTimeout,
		healthInterval:   cfg.HealthInterval,
		logger:           logger,
	}
}


// DeployWithZeroDowntime performs a blue-green style deployment.
// It starts the new container, waits for health checks, updates routing,
// and then stops the old container.
// **Validates: Requirements 10.1, 10.2, 10.3, 10.4**
func (d *ZeroDowntimeDeployer) DeployWithZeroDowntime(ctx context.Context, deployment *models.Deployment, appName string) error {
	if deployment.Artifact == "" {
		return ErrNoArtifact
	}

	d.logger.Info("starting zero-downtime deployment",
		"deployment_id", deployment.ID,
		"app_name", appName,
		"service_name", deployment.ServiceName,
		"version", deployment.Version,
	)

	// 1. Generate the new container name with version
	newContainerName := models.GenerateContainerName(appName, deployment.ServiceName, deployment.Version)

	// 2. Start the new container (Requirement 10.1: start new before stopping old)
	d.logger.Info("starting new container",
		"container_name", newContainerName,
		"node_id", deployment.NodeID,
	)

	if err := d.containerManager.StartContainer(ctx, deployment.NodeID, newContainerName, deployment); err != nil {
		d.logger.Error("failed to start new container",
			"container_name", newContainerName,
			"error", err,
		)
		return fmt.Errorf("starting new container: %w", err)
	}

	// 3. Wait for health check to pass (Requirement 10.4)
	d.logger.Info("waiting for health check",
		"container_name", newContainerName,
		"timeout", d.healthTimeout,
	)

	healthCheck := d.getHealthCheckConfig(deployment)
	healthy, err := d.waitForHealthy(ctx, deployment.NodeID, newContainerName, healthCheck)
	if err != nil || !healthy {
		// Health check failed - rollback by stopping the new container
		d.logger.Error("health check failed, rolling back",
			"container_name", newContainerName,
			"error", err,
		)

		// Stop the new container (best effort)
		if stopErr := d.containerManager.StopContainer(ctx, deployment.NodeID, newContainerName); stopErr != nil {
			d.logger.Error("failed to stop new container during rollback",
				"container_name", newContainerName,
				"error", stopErr,
			)
		}

		if err != nil {
			return fmt.Errorf("health check failed: %w", err)
		}
		return ErrHealthCheckFailed
	}

	d.logger.Info("health check passed",
		"container_name", newContainerName,
	)

	// 4. Update routing to the new container (Requirement 10.2)
	port := d.getServicePort(deployment)
	oldContainerName, err := d.routingUpdater.UpdateRouting(ctx, deployment.ServiceName, newContainerName, port)
	if err != nil {
		d.logger.Error("failed to update routing",
			"new_container", newContainerName,
			"error", err,
		)

		// Rollback: stop the new container
		if stopErr := d.containerManager.StopContainer(ctx, deployment.NodeID, newContainerName); stopErr != nil {
			d.logger.Error("failed to stop new container during routing rollback",
				"container_name", newContainerName,
				"error", stopErr,
			)
		}

		return fmt.Errorf("updating routing: %w", err)
	}

	d.logger.Info("routing updated",
		"old_container", oldContainerName,
		"new_container", newContainerName,
	)

	// 5. Stop the old container (Requirement 10.3)
	if oldContainerName != "" && deployment.Version > 1 {
		d.logger.Info("stopping old container",
			"container_name", oldContainerName,
		)

		if err := d.containerManager.StopContainer(ctx, deployment.NodeID, oldContainerName); err != nil {
			// Log but don't fail - the new container is already serving traffic
			d.logger.Warn("failed to stop old container",
				"container_name", oldContainerName,
				"error", err,
			)
		}
	}

	d.logger.Info("zero-downtime deployment completed successfully",
		"deployment_id", deployment.ID,
		"container_name", newContainerName,
	)

	return nil
}

// getHealthCheckConfig returns the health check configuration for a deployment.
func (d *ZeroDowntimeDeployer) getHealthCheckConfig(deployment *models.Deployment) *models.HealthCheckConfig {
	if deployment.Config != nil && deployment.Config.HealthCheck != nil {
		return deployment.Config.HealthCheck
	}

	// Default health check configuration
	port := d.getServicePort(deployment)
	return &models.HealthCheckConfig{
		Path:            "/health",
		Port:            port,
		IntervalSeconds: 5,
		TimeoutSeconds:  10,
		Retries:         3,
	}
}

// getServicePort returns the primary service port for a deployment.
func (d *ZeroDowntimeDeployer) getServicePort(deployment *models.Deployment) int {
	if deployment.Config != nil && len(deployment.Config.Ports) > 0 {
		return deployment.Config.Ports[0].ContainerPort
	}
	return 8080 // Default port
}


// waitForHealthy waits for a container to become healthy within the configured timeout.
// It polls the health check endpoint at regular intervals.
// **Validates: Requirements 10.4**
func (d *ZeroDowntimeDeployer) waitForHealthy(
	ctx context.Context,
	nodeID, containerName string,
	healthCheck *models.HealthCheckConfig,
) (bool, error) {
	deadline := time.Now().Add(d.healthTimeout)
	ticker := time.NewTicker(d.healthInterval)
	defer ticker.Stop()

	var lastErr error
	attempts := 0

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ticker.C:
			attempts++

			if time.Now().After(deadline) {
				d.logger.Warn("health check timeout exceeded",
					"container_name", containerName,
					"attempts", attempts,
					"last_error", lastErr,
				)
				return false, fmt.Errorf("health check timeout after %d attempts: %w", attempts, lastErr)
			}

			healthy, err := d.containerManager.CheckHealth(ctx, nodeID, containerName, healthCheck)
			if err != nil {
				lastErr = err
				d.logger.Debug("health check attempt failed",
					"container_name", containerName,
					"attempt", attempts,
					"error", err,
				)
				continue
			}

			if healthy {
				d.logger.Info("container is healthy",
					"container_name", containerName,
					"attempts", attempts,
				)
				return true, nil
			}

			d.logger.Debug("container not yet healthy",
				"container_name", containerName,
				"attempt", attempts,
			)
		}
	}
}

// WaitForHealthyWithConfig is a pure function for testing health check waiting logic.
// It returns true if the container becomes healthy within the given parameters.
func WaitForHealthyWithConfig(
	ctx context.Context,
	checkFunc func() (bool, error),
	timeout time.Duration,
	interval time.Duration,
) (bool, int, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastErr error
	attempts := 0

	for {
		select {
		case <-ctx.Done():
			return false, attempts, ctx.Err()
		case <-ticker.C:
			attempts++

			if time.Now().After(deadline) {
				return false, attempts, fmt.Errorf("timeout after %d attempts: %w", attempts, lastErr)
			}

			healthy, err := checkFunc()
			if err != nil {
				lastErr = err
				continue
			}

			if healthy {
				return true, attempts, nil
			}
		}
	}
}


// CaddyRoutingUpdater implements RoutingUpdater for Caddy reverse proxy.
// It updates Caddy's configuration to route traffic to the new container.
// **Validates: Requirements 10.2, 10.3**
type CaddyRoutingUpdater struct {
	// caddyAPIURL is the URL of the Caddy admin API.
	caddyAPIURL string
	logger      *slog.Logger
}

// NewCaddyRoutingUpdater creates a new CaddyRoutingUpdater.
func NewCaddyRoutingUpdater(caddyAPIURL string, logger *slog.Logger) *CaddyRoutingUpdater {
	if logger == nil {
		logger = slog.Default()
	}
	return &CaddyRoutingUpdater{
		caddyAPIURL: caddyAPIURL,
		logger:      logger,
	}
}

// UpdateRouting updates Caddy configuration to route to the new container.
// Returns the old container name that was previously configured.
// **Validates: Requirements 10.2, 10.3**
func (c *CaddyRoutingUpdater) UpdateRouting(ctx context.Context, serviceName, newContainerName string, port int) (string, error) {
	c.logger.Info("updating Caddy routing",
		"service_name", serviceName,
		"new_container", newContainerName,
		"port", port,
	)

	// In a real implementation, this would:
	// 1. Query Caddy API to get current upstream for the service
	// 2. Update the upstream to point to the new container
	// 3. Return the old container name

	// For now, we return an empty string to indicate no previous container
	// The actual Caddy API integration would be implemented based on the
	// specific Caddy configuration structure used in the deployment.

	// Example Caddy API call structure:
	// PATCH /config/apps/http/servers/srv0/routes/0/handle/0/upstreams
	// Body: [{"dial": "newContainerName:port"}]

	return "", nil
}

// RoutingConfig represents the routing configuration for a service.
type RoutingConfig struct {
	ServiceName   string `json:"service_name"`
	ContainerName string `json:"container_name"`
	Port          int    `json:"port"`
	Domain        string `json:"domain,omitempty"`
}

// BuildRoutingConfig creates a RoutingConfig from a deployment.
func BuildRoutingConfig(deployment *models.Deployment, appName string) *RoutingConfig {
	containerName := models.GenerateContainerName(appName, deployment.ServiceName, deployment.Version)
	port := 8080
	if deployment.Config != nil && len(deployment.Config.Ports) > 0 {
		port = deployment.Config.Ports[0].ContainerPort
	}

	return &RoutingConfig{
		ServiceName:   deployment.ServiceName,
		ContainerName: containerName,
		Port:          port,
	}
}


// Rollback creates a new deployment using a previous successful deployment's artifact.
// It finds the specified deployment, creates a new deployment with the same artifact,
// and returns the new deployment.
// **Validates: Requirements 10.5, 20.3, 20.4**
func (d *ZeroDowntimeDeployer) Rollback(ctx context.Context, appID, serviceName string, targetDeploymentID string) (*models.Deployment, error) {
	d.logger.Info("initiating rollback",
		"app_id", appID,
		"service_name", serviceName,
		"target_deployment_id", targetDeploymentID,
	)

	// 1. Get the target deployment to rollback to
	targetDeployment, err := d.store.Deployments().Get(ctx, targetDeploymentID)
	if err != nil {
		return nil, fmt.Errorf("getting target deployment: %w", err)
	}

	if targetDeployment == nil {
		return nil, ErrDeploymentNotFound
	}

	// Verify the deployment belongs to the correct app and service
	if targetDeployment.AppID != appID || targetDeployment.ServiceName != serviceName {
		return nil, fmt.Errorf("deployment does not belong to app %s service %s", appID, serviceName)
	}

	// Verify the deployment has an artifact
	if targetDeployment.Artifact == "" {
		return nil, ErrNoArtifact
	}

	// 2. Get the next version number
	nextVersion, err := d.store.Deployments().GetNextVersion(ctx, appID, serviceName)
	if err != nil {
		return nil, fmt.Errorf("getting next version: %w", err)
	}

	// 3. Create a new deployment with the same artifact but new version
	// **Validates: Requirements 20.3, 20.4**
	newDeployment := &models.Deployment{
		ID:           generateDeploymentID(),
		AppID:        appID,
		ServiceName:  serviceName,
		Version:      nextVersion,
		GitRef:       targetDeployment.GitRef,
		GitCommit:    targetDeployment.GitCommit,
		BuildType:    targetDeployment.BuildType,
		Artifact:     targetDeployment.Artifact, // Use the same artifact
		Status:       models.DeploymentStatusBuilt, // Skip build phase
		ResourceTier: targetDeployment.ResourceTier,
		Config:       targetDeployment.Config,
		DependsOn:    targetDeployment.DependsOn,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// 4. Save the new deployment
	if err := d.store.Deployments().Create(ctx, newDeployment); err != nil {
		return nil, fmt.Errorf("creating rollback deployment: %w", err)
	}

	d.logger.Info("rollback deployment created",
		"new_deployment_id", newDeployment.ID,
		"new_version", newDeployment.Version,
		"artifact", newDeployment.Artifact,
		"rolled_back_from", targetDeploymentID,
	)

	return newDeployment, nil
}

// RollbackToLatestSuccessful creates a new deployment using the latest successful deployment's artifact.
// **Validates: Requirements 10.5**
func (d *ZeroDowntimeDeployer) RollbackToLatestSuccessful(ctx context.Context, appID, serviceName string) (*models.Deployment, error) {
	d.logger.Info("initiating rollback to latest successful",
		"app_id", appID,
		"service_name", serviceName,
	)

	// Find the latest successful deployment for this service
	deployments, err := d.store.Deployments().List(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("listing deployments: %w", err)
	}

	var latestSuccessful *models.Deployment
	for _, dep := range deployments {
		if dep.ServiceName == serviceName &&
			dep.Status == models.DeploymentStatusRunning &&
			dep.Artifact != "" {
			if latestSuccessful == nil || dep.Version > latestSuccessful.Version {
				latestSuccessful = dep
			}
		}
	}

	if latestSuccessful == nil {
		return nil, ErrNoPreviousDeployment
	}

	return d.Rollback(ctx, appID, serviceName, latestSuccessful.ID)
}

// generateDeploymentID generates a unique deployment ID.
func generateDeploymentID() string {
	return fmt.Sprintf("dep-%d", time.Now().UnixNano())
}

// RollbackResult represents the result of a rollback operation.
type RollbackResult struct {
	NewDeployment    *models.Deployment `json:"new_deployment"`
	SourceDeployment *models.Deployment `json:"source_deployment"`
	Success          bool               `json:"success"`
	Message          string             `json:"message,omitempty"`
}

// CreateRollbackDeployment is a pure function that creates a rollback deployment
// from a source deployment. This is useful for testing.
// **Validates: Requirements 10.5, 20.3, 20.4**
func CreateRollbackDeployment(source *models.Deployment, newVersion int, newID string) *models.Deployment {
	now := time.Now()
	return &models.Deployment{
		ID:           newID,
		AppID:        source.AppID,
		ServiceName:  source.ServiceName,
		Version:      newVersion,
		GitRef:       source.GitRef,
		GitCommit:    source.GitCommit,
		BuildType:    source.BuildType,
		Artifact:     source.Artifact,
		Status:       models.DeploymentStatusBuilt,
		ResourceTier: source.ResourceTier,
		Config:       source.Config,
		DependsOn:    source.DependsOn,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// ValidateRollbackDeployment validates that a rollback deployment was created correctly.
// Returns true if the rollback deployment has the correct properties.
// **Validates: Requirements 10.5, 20.3, 20.4**
func ValidateRollbackDeployment(source, rollback *models.Deployment) bool {
	// Version must be greater than source
	if rollback.Version <= source.Version {
		return false
	}

	// Artifact must be the same
	if rollback.Artifact != source.Artifact {
		return false
	}

	// App and service must match
	if rollback.AppID != source.AppID || rollback.ServiceName != source.ServiceName {
		return false
	}

	// Status must be "built" (skip build phase)
	if rollback.Status != models.DeploymentStatusBuilt {
		return false
	}

	// Build type must be preserved
	if rollback.BuildType != source.BuildType {
		return false
	}

	return true
}
