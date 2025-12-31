package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/queue"
	"github.com/narvanalabs/control-plane/internal/store"
)

// DeploymentHandler handles deployment-related HTTP requests.
type DeploymentHandler struct {
	store  store.Store
	queue  queue.Queue
	logger *slog.Logger
}

// NewDeploymentHandler creates a new deployment handler.
func NewDeploymentHandler(st store.Store, q queue.Queue, logger *slog.Logger) *DeploymentHandler {
	return &DeploymentHandler{
		store:  st,
		queue:  q,
		logger: logger,
	}
}

// CreateDeploymentRequest represents the request body for creating a deployment.
type CreateDeploymentRequest struct {
	GitRef      string `json:"git_ref,omitempty"`      // Optional, uses service's git_ref if not specified
	ServiceName string `json:"service_name,omitempty"` // Optional for app-level deploy, required for per-service deploy
}

// Validate validates the create deployment request.
func (r *CreateDeploymentRequest) Validate() error {
	// git_ref is now optional - will use service's configured git_ref if not specified
	return nil
}

// ServiceDeployRequest represents the request body for deploying a specific service.
type ServiceDeployRequest struct {
	GitRef string `json:"git_ref,omitempty"` // Optional, overrides service's git_ref if specified
}

// Validate validates the service deploy request.
func (r *ServiceDeployRequest) Validate() error {
	// git_ref is optional - will use service's configured git_ref if not specified
	return nil
}


// Create handles POST /v1/apps/:appID/deploy - triggers a new deployment.
func (h *DeploymentHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Use resolved app ID from middleware (handles both UUID and name lookup)
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	var req CreateDeploymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	if err := req.Validate(); err != nil {
		if apiErr, ok := err.(*APIError); ok {
			WriteError(w, http.StatusBadRequest, apiErr.Code, apiErr.Message)
			return
		}
		WriteBadRequest(w, err.Error())
		return
	}

	// Get the app
	app, err := h.store.Apps().Get(r.Context(), appID)
	if err != nil {
		WriteNotFound(w, "Application not found")
		return
	}

	// Determine which services to deploy
	servicesToDeploy := app.Services
	if req.ServiceName != "" {
		// Deploy only the specified service
		servicesToDeploy = nil
		for _, svc := range app.Services {
			if svc.Name == req.ServiceName {
				servicesToDeploy = []models.ServiceConfig{svc}
				break
			}
		}
		if len(servicesToDeploy) == 0 {
			WriteNotFound(w, "Service not found")
			return
		}
	}

	// If no services defined, create a default deployment
	if len(servicesToDeploy) == 0 {
		servicesToDeploy = []models.ServiceConfig{{
			Name:         "default",
			ResourceTier: models.ResourceTierSmall,
		}}
	}

	// Sort services by dependency order (services with no dependencies first)
	sortedServices := sortServicesByDependency(servicesToDeploy)

	now := time.Now()
	var deployments []*models.Deployment

	// Create a deployment for each service
	for _, svc := range sortedServices {
		// Determine git_ref: use request override or service's configured git_ref
		gitRef := req.GitRef
		if gitRef == "" {
			gitRef = svc.GitRef
		}

		// Determine build type from service source type
		buildType := determineBuildType(&svc)

		// Get next version for this service
		// **Validates: Requirements 9.1, 9.2**
		version, err := h.store.Deployments().GetNextVersion(r.Context(), appID, svc.Name)
		if err != nil {
			h.logger.Error("failed to get next version", "error", err, "service", svc.Name)
			WriteInternalError(w, "Failed to determine deployment version")
			return
		}

		deployment := &models.Deployment{
			ID:           uuid.New().String(),
			AppID:        appID,
			ServiceName:  svc.Name,
			Version:      version,
			GitRef:       gitRef,
			BuildType:    buildType,
			Status:       models.DeploymentStatusPending,
			ResourceTier: svc.ResourceTier,
			Config: &models.RuntimeConfig{
				ResourceTier: svc.ResourceTier,
				EnvVars:      svc.EnvVars,
				Ports:        svc.Ports,
				HealthCheck:  svc.HealthCheck,
			},
			DependsOn: svc.DependsOn, // Track service dependencies
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := h.store.Deployments().Create(r.Context(), deployment); err != nil {
			h.logger.Error("failed to create deployment", "error", err)
			WriteInternalError(w, "Failed to create deployment")
			return
		}

		// Create and enqueue build job based on source type
		buildJob := h.createBuildJobForService(r.Context(), deployment.ID, appID, &svc, gitRef, buildType, now)

		if buildJob != nil && h.queue != nil {
			if err := h.queue.Enqueue(r.Context(), buildJob); err != nil {
				h.logger.Error("failed to enqueue build job", "error", err)
				// Don't fail the request, the deployment is created
			}
		}

		deployments = append(deployments, deployment)
	}

	h.logger.Info("deployment triggered",
		"app_id", appID,
		"git_ref", req.GitRef,
		"deployment_count", len(deployments),
	)

	// Return single deployment or array based on count
	if len(deployments) == 1 {
		WriteJSON(w, http.StatusAccepted, deployments[0])
	} else {
		WriteJSON(w, http.StatusAccepted, deployments)
	}
}

// determineBuildType determines the build type based on service source type.
// Image sources use OCI, git/flake/database sources use pure-nix (they generate Nix closures).
func determineBuildType(svc *models.ServiceConfig) models.BuildType {
	switch svc.SourceType {
	case models.SourceTypeImage:
		return models.BuildTypeOCI
	case models.SourceTypeGit, models.SourceTypeFlake, models.SourceTypeDatabase:
		return models.BuildTypePureNix
	default:
		return models.BuildTypePureNix // Default to pure-nix for generated flakes
	}
}

// sortServicesByDependency sorts services so that services with no dependencies come first,
// followed by services whose dependencies appear earlier in the list.
// This implements a topological sort for service dependencies.
func sortServicesByDependency(services []models.ServiceConfig) []models.ServiceConfig {
	if len(services) <= 1 {
		return services
	}

	// Build a map of service names to their configs
	serviceMap := make(map[string]models.ServiceConfig)
	for _, svc := range services {
		serviceMap[svc.Name] = svc
	}

	// Track which services have been added to the result
	added := make(map[string]bool)
	var result []models.ServiceConfig

	// Keep iterating until all services are added
	for len(result) < len(services) {
		progress := false
		for _, svc := range services {
			if added[svc.Name] {
				continue
			}

			// Check if all dependencies are satisfied
			allDepsAdded := true
			for _, dep := range svc.DependsOn {
				// Only check dependencies that are in the current deployment set
				if _, exists := serviceMap[dep]; exists && !added[dep] {
					allDepsAdded = false
					break
				}
			}

			if allDepsAdded {
				result = append(result, svc)
				added[svc.Name] = true
				progress = true
			}
		}

		// If no progress was made, there's a circular dependency
		// Add remaining services in original order to avoid infinite loop
		if !progress {
			for _, svc := range services {
				if !added[svc.Name] {
					result = append(result, svc)
					added[svc.Name] = true
				}
			}
		}
	}

	return result
}

// List handles GET /v1/apps/:appID/deployments - lists deployments for an app.
func (h *DeploymentHandler) List(w http.ResponseWriter, r *http.Request) {
	// Use resolved app ID from middleware (handles both UUID and name lookup)
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	deployments, err := h.store.Deployments().List(r.Context(), appID)
	if err != nil {
		h.logger.Error("failed to list deployments", "error", err, "app_id", appID)
		WriteInternalError(w, "Failed to list deployments")
		return
	}

	if deployments == nil {
		deployments = []*models.Deployment{}
	}

	WriteJSON(w, http.StatusOK, deployments)
}

// ListAll handles GET /v1/deployments - lists all deployments for the authenticated user.
func (h *DeploymentHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

	deployments, err := h.store.Deployments().ListByUser(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to list deployments for user", "error", err, "user_id", userID)
		WriteInternalError(w, "Failed to list deployments")
		return
	}

	if deployments == nil {
		deployments = []*models.Deployment{}
	}

	WriteJSON(w, http.StatusOK, deployments)
}

// Get handles GET /v1/deployments/:deploymentID - retrieves a specific deployment.
func (h *DeploymentHandler) Get(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "deploymentID")
	if deploymentID == "" {
		WriteBadRequest(w, "Deployment ID is required")
		return
	}

	deployment, err := h.store.Deployments().Get(r.Context(), deploymentID)
	if err != nil {
		h.logger.Debug("failed to get deployment", "error", err, "deployment_id", deploymentID)
		WriteNotFound(w, "Deployment not found")
		return
	}

	// Verify ownership through the app
	userID := middleware.GetUserID(r.Context())
	app, err := h.store.Apps().Get(r.Context(), deployment.AppID)
	if err != nil || app.OwnerID != userID {
		WriteForbidden(w, "Access denied")
		return
	}

	WriteJSON(w, http.StatusOK, deployment)
}

// CreateForService handles POST /v1/apps/{appID}/services/{serviceName}/deploy - deploys a specific service.
func (h *DeploymentHandler) CreateForService(w http.ResponseWriter, r *http.Request) {
	// Use resolved app ID from middleware (handles both UUID and name lookup)
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	serviceName := chi.URLParam(r, "serviceName")

	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}
	if serviceName == "" {
		WriteBadRequest(w, "Service name is required")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

	var req ServiceDeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Get the app
	app, err := h.store.Apps().Get(r.Context(), appID)
	if err != nil {
		WriteNotFound(w, "Application not found")
		return
	}

	// Verify ownership
	if app.OwnerID != userID {
		WriteForbidden(w, "Access denied")
		return
	}

	// Find the service
	var service *models.ServiceConfig
	for i := range app.Services {
		if app.Services[i].Name == serviceName {
			service = &app.Services[i]
			break
		}
	}

	if service == nil {
		WriteNotFound(w, "Service not found")
		return
	}

	// Determine git_ref: use request override or service's configured git_ref
	gitRef := req.GitRef
	if gitRef == "" {
		gitRef = service.GitRef
	}

	// Verify dependencies are running before starting (for services with dependencies)
	if len(service.DependsOn) > 0 {
		deployments, err := h.store.Deployments().List(r.Context(), appID)
		if err == nil {
			runningServices := make(map[string]bool)
			for _, d := range deployments {
				if d.Status == models.DeploymentStatusRunning {
					runningServices[d.ServiceName] = true
				}
			}

			for _, dep := range service.DependsOn {
				if !runningServices[dep] {
					WriteBadRequest(w, "Dependency '"+dep+"' is not running")
					return
				}
			}
		}
	}

	now := time.Now()

	// Determine build type from service source type
	buildType := determineBuildType(service)

	// Get next version for this service
	// **Validates: Requirements 9.1, 9.2**
	version, err := h.store.Deployments().GetNextVersion(r.Context(), appID, service.Name)
	if err != nil {
		h.logger.Error("failed to get next version", "error", err, "service", service.Name)
		WriteInternalError(w, "Failed to determine deployment version")
		return
	}

	// Create deployment for this service only
	deployment := &models.Deployment{
		ID:           uuid.New().String(),
		AppID:        appID,
		ServiceName:  service.Name,
		Version:      version,
		GitRef:       gitRef,
		BuildType:    buildType,
		Status:       models.DeploymentStatusPending,
		ResourceTier: service.ResourceTier,
		Config: &models.RuntimeConfig{
			ResourceTier: service.ResourceTier,
			EnvVars:      service.EnvVars,
			Ports:        service.Ports,
			HealthCheck:  service.HealthCheck,
		},
		DependsOn: service.DependsOn,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := h.store.Deployments().Create(r.Context(), deployment); err != nil {
		h.logger.Error("failed to create deployment", "error", err)
		WriteInternalError(w, "Failed to create deployment")
		return
	}

	// Create and enqueue build job based on source type
	buildJob := h.createBuildJobForService(r.Context(), deployment.ID, appID, service, gitRef, buildType, now)

	if buildJob == nil {
		h.logger.Warn("no build job created for deployment",
			"deployment_id", deployment.ID,
			"service_name", serviceName,
			"source_type", service.SourceType,
		)
	} else if h.queue != nil {
		if err := h.queue.Enqueue(r.Context(), buildJob); err != nil {
			h.logger.Error("failed to enqueue build job",
				"error", err,
				"job_id", buildJob.ID,
				"deployment_id", deployment.ID,
			)
			// Don't fail the request, the deployment is created
		} else {
			h.logger.Info("build job enqueued",
				"job_id", buildJob.ID,
				"deployment_id", deployment.ID,
				"build_strategy", buildJob.BuildStrategy,
			)
		}
	} else {
		h.logger.Error("queue is nil, cannot enqueue build job",
			"job_id", buildJob.ID,
			"deployment_id", deployment.ID,
		)
	}

	h.logger.Info("per-service deployment triggered",
		"app_id", appID,
		"service_name", serviceName,
		"git_ref", gitRef,
		"deployment_id", deployment.ID,
	)

	WriteJSON(w, http.StatusAccepted, deployment)
}

// createBuildJobForService creates a build job based on the service's source type.
// Returns nil for image sources (no build needed).
// Also creates the build record in the database.
func (h *DeploymentHandler) createBuildJobForService(ctx context.Context, deploymentID, appID string, service *models.ServiceConfig, gitRef string, buildType models.BuildType, now time.Time) *models.BuildJob {
	var buildJob *models.BuildJob

	switch service.SourceType {
	case models.SourceTypeGit:
		buildJob = &models.BuildJob{
			ID:            uuid.New().String(),
			DeploymentID:  deploymentID,
			AppID:         appID,
			GitURL:        service.GitRepo,
			GitRef:        gitRef,
			FlakeOutput:   service.FlakeOutput,
			BuildType:     buildType,
			BuildStrategy: service.BuildStrategy,
			BuildConfig:   service.BuildConfig,
			Status:        models.BuildStatusQueued,
			CreatedAt:     now,
		}
	case models.SourceTypeFlake, models.SourceTypeDatabase:
		// For flake/database sources, use the flake URI directly
		// Note: Database sources often use a predefined flake for the engine
		url := service.FlakeURI
		if url == "" && service.Database != nil {
			// Fallback: generate a simple flake URI if possible or leave empty for worker to resolve
		}

		buildConfig := service.BuildConfig
		if buildConfig == nil {
			buildConfig = &models.BuildConfig{}
		}
		if service.Database != nil {
			buildConfig.DatabaseOptions = &models.DatabaseOptions{
				Type:    service.Database.Type,
				Version: service.Database.Version,
			}
		}

		buildJob = &models.BuildJob{
			ID:            uuid.New().String(),
			DeploymentID:  deploymentID,
			AppID:         appID,
			GitURL:        url, // Store flake URI in GitURL field
			GitRef:        "",  // Not applicable for flake sources
			FlakeOutput:   "",  // Output is part of the flake URI
			BuildType:     buildType,
			BuildStrategy: service.BuildStrategy,
			BuildConfig:   buildConfig,
			Status:        models.BuildStatusQueued,
			CreatedAt:     now,
		}

		// Ensure we have a valid strategy
		if service.SourceType == models.SourceTypeDatabase {
			// Database services always use auto-database strategy (unless explicitly overridden)
			if buildJob.BuildStrategy == "" || url == "" {
				buildJob.BuildStrategy = models.BuildStrategyAutoDatabase
			}
		} else if buildJob.BuildStrategy == "" {
			if service.SourceType == models.SourceTypeFlake {
				buildJob.BuildStrategy = models.BuildStrategyFlake
			}
		}
	case models.SourceTypeImage:
		// No build job needed for image sources - skip build phase
		return nil
	default:
		return nil
	}

	// Create the build record in the database
	if buildJob != nil {
		if err := h.store.Builds().Create(ctx, buildJob); err != nil {
			h.logger.Error("failed to create build record",
				"error", err,
				"job_id", buildJob.ID,
				"deployment_id", deploymentID,
				"service_name", service.Name,
				"source_type", service.SourceType,
				"build_strategy", buildJob.BuildStrategy,
			)
			// Return nil to prevent enqueueing a job without a database record
			return nil
		}
		h.logger.Info("created build job",
			"job_id", buildJob.ID,
			"deployment_id", deploymentID,
			"service_name", service.Name,
			"source_type", service.SourceType,
			"build_strategy", buildJob.BuildStrategy,
		)
	} else {
		h.logger.Warn("build job is nil, skipping creation",
			"deployment_id", deploymentID,
			"service_name", service.Name,
			"source_type", service.SourceType,
		)
	}

	return buildJob
}

// Rollback handles POST /v1/deployments/:deploymentID/rollback - rolls back to a previous deployment.
func (h *DeploymentHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "deploymentID")
	if deploymentID == "" {
		WriteBadRequest(w, "Deployment ID is required")
		return
	}

	// Get the deployment to rollback to
	targetDeployment, err := h.store.Deployments().Get(r.Context(), deploymentID)
	if err != nil {
		WriteNotFound(w, "Deployment not found")
		return
	}

	// Verify ownership
	userID := middleware.GetUserID(r.Context())
	app, err := h.store.Apps().Get(r.Context(), targetDeployment.AppID)
	if err != nil || app.OwnerID != userID {
		WriteForbidden(w, "Access denied")
		return
	}

	// Verify the target deployment has an artifact
	if targetDeployment.Artifact == "" {
		WriteBadRequest(w, "Cannot rollback to a deployment without an artifact")
		return
	}

	// Get next version for this service
	// **Validates: Requirements 9.1, 9.2**
	version, err := h.store.Deployments().GetNextVersion(r.Context(), targetDeployment.AppID, targetDeployment.ServiceName)
	if err != nil {
		h.logger.Error("failed to get next version", "error", err, "service", targetDeployment.ServiceName)
		WriteInternalError(w, "Failed to determine deployment version")
		return
	}

	// Create a new deployment using the artifact from the target
	now := time.Now()
	newDeployment := &models.Deployment{
		ID:           uuid.New().String(),
		AppID:        targetDeployment.AppID,
		ServiceName:  targetDeployment.ServiceName,
		Version:      version,
		GitRef:       targetDeployment.GitRef,
		GitCommit:    targetDeployment.GitCommit,
		BuildType:    targetDeployment.BuildType,
		Artifact:     targetDeployment.Artifact, // Use the same artifact
		Status:       models.DeploymentStatusBuilt, // Skip build phase
		ResourceTier: targetDeployment.ResourceTier,
		Config:       targetDeployment.Config,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := h.store.Deployments().Create(r.Context(), newDeployment); err != nil {
		h.logger.Error("failed to create rollback deployment", "error", err)
		WriteInternalError(w, "Failed to create rollback deployment")
		return
	}

	h.logger.Info("rollback deployment created",
		"new_deployment_id", newDeployment.ID,
		"target_deployment_id", deploymentID,
		"artifact", newDeployment.Artifact,
	)

	WriteJSON(w, http.StatusAccepted, newDeployment)
}
