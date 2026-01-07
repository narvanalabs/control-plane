// Package e2e provides end-to-end testing framework for the control-plane.
// It simulates complete user workflows including app creation, service configuration,
// deployment, and verification.
// **Validates: Requirements 12.2**
package e2e

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

// TestEnvironment represents a complete test environment with all components.
// **Validates: Requirements 12.2**
type TestEnvironment struct {
	mu sync.RWMutex

	// In-memory stores
	Apps        map[string]*models.App
	Deployments map[string]*models.Deployment
	Nodes       map[string]*models.Node
	Builds      map[string]*models.BuildJob
	Secrets     map[string]map[string]string // appID -> key -> value

	// Event tracking
	Events []Event

	// Configuration
	Config *TestConfig
}

// TestConfig holds configuration for the test environment.
type TestConfig struct {
	// SimulateBuildDelay adds artificial delay to build operations
	SimulateBuildDelay time.Duration
	// SimulateDeployDelay adds artificial delay to deploy operations
	SimulateDeployDelay time.Duration
	// FailBuildProbability is the probability (0-1) that a build will fail
	FailBuildProbability float64
	// FailDeployProbability is the probability (0-1) that a deploy will fail
	FailDeployProbability float64
}

// DefaultTestConfig returns a default test configuration.
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		SimulateBuildDelay:    0,
		SimulateDeployDelay:   0,
		FailBuildProbability:  0,
		FailDeployProbability: 0,
	}
}

// Event represents an event that occurred during testing.
type Event struct {
	Type      EventType
	Timestamp time.Time
	EntityID  string
	Details   map[string]interface{}
}

// EventType represents the type of event.
type EventType string

const (
	EventAppCreated        EventType = "app_created"
	EventServiceAdded      EventType = "service_added"
	EventDeploymentCreated EventType = "deployment_created"
	EventBuildStarted      EventType = "build_started"
	EventBuildCompleted    EventType = "build_completed"
	EventBuildFailed       EventType = "build_failed"
	EventDeploymentStarted EventType = "deployment_started"
	EventDeploymentRunning EventType = "deployment_running"
	EventDeploymentFailed  EventType = "deployment_failed"
	EventSecretSet         EventType = "secret_set"
	EventNodeRegistered    EventType = "node_registered"
)

// NewTestEnvironment creates a new test environment.
// **Validates: Requirements 12.2**
func NewTestEnvironment(config *TestConfig) *TestEnvironment {
	if config == nil {
		config = DefaultTestConfig()
	}
	return &TestEnvironment{
		Apps:        make(map[string]*models.App),
		Deployments: make(map[string]*models.Deployment),
		Nodes:       make(map[string]*models.Node),
		Builds:      make(map[string]*models.BuildJob),
		Secrets:     make(map[string]map[string]string),
		Events:      make([]Event, 0),
		Config:      config,
	}
}

// recordEvent records an event in the test environment.
func (env *TestEnvironment) recordEvent(eventType EventType, entityID string, details map[string]interface{}) {
	env.Events = append(env.Events, Event{
		Type:      eventType,
		Timestamp: time.Now(),
		EntityID:  entityID,
		Details:   details,
	})
}

// GetEvents returns all recorded events.
func (env *TestEnvironment) GetEvents() []Event {
	env.mu.RLock()
	defer env.mu.RUnlock()
	
	events := make([]Event, len(env.Events))
	copy(events, env.Events)
	return events
}

// GetEventsByType returns events filtered by type.
func (env *TestEnvironment) GetEventsByType(eventType EventType) []Event {
	env.mu.RLock()
	defer env.mu.RUnlock()
	
	var filtered []Event
	for _, e := range env.Events {
		if e.Type == eventType {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// UserSimulator provides helper functions for simulating user workflows.
// **Validates: Requirements 12.2**
type UserSimulator struct {
	env *TestEnvironment
}

// NewUserSimulator creates a new user simulator.
func NewUserSimulator(env *TestEnvironment) *UserSimulator {
	return &UserSimulator{env: env}
}

// CreateApp simulates a user creating a new application.
// **Validates: Requirements 12.2**
func (s *UserSimulator) CreateApp(ctx context.Context, name, description string) (*models.App, error) {
	s.env.mu.Lock()
	defer s.env.mu.Unlock()

	// Check for duplicate name
	for _, app := range s.env.Apps {
		if app.Name == name {
			return nil, fmt.Errorf("app with name %s already exists", name)
		}
	}

	app := &models.App{
		ID:          fmt.Sprintf("app-%d", time.Now().UnixNano()),
		Name:        name,
		Description: description,
		Services:    make([]models.ServiceConfig, 0),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	s.env.Apps[app.ID] = app
	s.env.recordEvent(EventAppCreated, app.ID, map[string]interface{}{
		"name":        name,
		"description": description,
	})

	return app, nil
}

// AddService simulates a user adding a service to an application.
// **Validates: Requirements 12.2**
func (s *UserSimulator) AddService(ctx context.Context, appID string, service models.ServiceConfig) error {
	s.env.mu.Lock()
	defer s.env.mu.Unlock()

	app, ok := s.env.Apps[appID]
	if !ok {
		return fmt.Errorf("app %s not found", appID)
	}

	// Check for duplicate service name
	for _, svc := range app.Services {
		if svc.Name == service.Name {
			return fmt.Errorf("service %s already exists in app %s", service.Name, appID)
		}
	}

	// Apply defaults
	if service.Replicas == 0 {
		service.Replicas = 1
	}

	app.Services = append(app.Services, service)
	app.UpdatedAt = time.Now()

	s.env.recordEvent(EventServiceAdded, appID, map[string]interface{}{
		"service_name": service.Name,
		"source_type":  service.SourceType,
	})

	return nil
}

// SetSecret simulates a user setting a secret for an application.
// **Validates: Requirements 12.2**
func (s *UserSimulator) SetSecret(ctx context.Context, appID, key, value string) error {
	s.env.mu.Lock()
	defer s.env.mu.Unlock()

	if _, ok := s.env.Apps[appID]; !ok {
		return fmt.Errorf("app %s not found", appID)
	}

	if s.env.Secrets[appID] == nil {
		s.env.Secrets[appID] = make(map[string]string)
	}
	s.env.Secrets[appID][key] = value

	s.env.recordEvent(EventSecretSet, appID, map[string]interface{}{
		"key": key,
	})

	return nil
}

// Deploy simulates a user triggering a deployment.
// **Validates: Requirements 12.2**
func (s *UserSimulator) Deploy(ctx context.Context, appID, serviceName, gitRef string) (*models.Deployment, error) {
	s.env.mu.Lock()
	defer s.env.mu.Unlock()

	app, ok := s.env.Apps[appID]
	if !ok {
		return nil, fmt.Errorf("app %s not found", appID)
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
		return nil, fmt.Errorf("service %s not found in app %s", serviceName, appID)
	}

	// Determine build type from service configuration
	buildType := models.BuildTypePureNix
	if service.SourceType == models.SourceTypeImage {
		buildType = models.BuildTypeOCI
	}

	// Get next version
	version := 1
	for _, dep := range s.env.Deployments {
		if dep.AppID == appID && dep.ServiceName == serviceName {
			if dep.Version >= version {
				version = dep.Version + 1
			}
		}
	}

	deployment := &models.Deployment{
		ID:          fmt.Sprintf("dep-%d", time.Now().UnixNano()),
		AppID:       appID,
		ServiceName: serviceName,
		Version:     version,
		GitRef:      gitRef,
		BuildType:   buildType,
		Status:      models.DeploymentStatusPending,
		Resources:   service.Resources,
		DependsOn:   service.DependsOn,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	s.env.Deployments[deployment.ID] = deployment
	s.env.recordEvent(EventDeploymentCreated, deployment.ID, map[string]interface{}{
		"app_id":       appID,
		"service_name": serviceName,
		"version":      version,
		"git_ref":      gitRef,
	})

	return deployment, nil
}

// RegisterNode simulates registering a node in the cluster.
// **Validates: Requirements 12.2**
func (s *UserSimulator) RegisterNode(ctx context.Context, hostname, address string, resources *models.NodeResources) (*models.Node, error) {
	s.env.mu.Lock()
	defer s.env.mu.Unlock()

	node := &models.Node{
		ID:            fmt.Sprintf("node-%d", time.Now().UnixNano()),
		Hostname:      hostname,
		Address:       address,
		GRPCPort:      9090,
		Healthy:       true,
		Resources:     resources,
		LastHeartbeat: time.Now(),
		RegisteredAt:  time.Now(),
	}

	s.env.Nodes[node.ID] = node
	s.env.recordEvent(EventNodeRegistered, node.ID, map[string]interface{}{
		"hostname": hostname,
		"address":  address,
	})

	return node, nil
}

// GetApp retrieves an app by ID.
func (s *UserSimulator) GetApp(ctx context.Context, appID string) (*models.App, error) {
	s.env.mu.RLock()
	defer s.env.mu.RUnlock()

	app, ok := s.env.Apps[appID]
	if !ok {
		return nil, fmt.Errorf("app %s not found", appID)
	}
	return app, nil
}

// GetDeployment retrieves a deployment by ID.
func (s *UserSimulator) GetDeployment(ctx context.Context, deploymentID string) (*models.Deployment, error) {
	s.env.mu.RLock()
	defer s.env.mu.RUnlock()

	dep, ok := s.env.Deployments[deploymentID]
	if !ok {
		return nil, fmt.Errorf("deployment %s not found", deploymentID)
	}
	return dep, nil
}

// ListDeployments lists all deployments for an app.
func (s *UserSimulator) ListDeployments(ctx context.Context, appID string) ([]*models.Deployment, error) {
	s.env.mu.RLock()
	defer s.env.mu.RUnlock()

	var deployments []*models.Deployment
	for _, dep := range s.env.Deployments {
		if dep.AppID == appID {
			deployments = append(deployments, dep)
		}
	}
	return deployments, nil
}
