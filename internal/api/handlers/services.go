// Package handlers provides HTTP request handlers for the API.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/builder/detector"
	"github.com/narvanalabs/control-plane/internal/builder/templates/databases"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/podman"
	"github.com/narvanalabs/control-plane/internal/secrets"
	"github.com/narvanalabs/control-plane/internal/store"
	"github.com/narvanalabs/control-plane/internal/validation"
)

// ServiceHandler handles service-related HTTP requests.
type ServiceHandler struct {
	store               store.Store
	podman              *podman.Client
	detector            detector.Detector
	sopsService         *secrets.SOPSService
	dependencyValidator *validation.DependencyValidator
	logger              *slog.Logger
}

// NewServiceHandler creates a new service handler.
func NewServiceHandler(st store.Store, pd *podman.Client, sopsService *secrets.SOPSService, logger *slog.Logger) *ServiceHandler {
	return &ServiceHandler{
		store:               st,
		podman:              pd,
		detector:            detector.NewDetector(),
		sopsService:         sopsService,
		dependencyValidator: validation.NewDependencyValidator(logger),
		logger:              logger,
	}
}

// NewServiceHandlerWithDetector creates a new service handler with a custom detector.
func NewServiceHandlerWithDetector(st store.Store, pd *podman.Client, det detector.Detector, sopsService *secrets.SOPSService, logger *slog.Logger) *ServiceHandler {
	return &ServiceHandler{
		store:               st,
		podman:              pd,
		detector:            det,
		sopsService:         sopsService,
		dependencyValidator: validation.NewDependencyValidator(logger),
		logger:              logger,
	}
}

// CreateServiceRequest represents the request body for creating a service.
type CreateServiceRequest struct {
	Name        string                 `json:"name"`
	SourceType  models.SourceType      `json:"source_type,omitempty"`
	GitRepo     string                 `json:"git_repo,omitempty"`
	GitRef      string                 `json:"git_ref,omitempty"`      // Default: "main"
	FlakeOutput string                 `json:"flake_output,omitempty"` // Default: "packages.x86_64-linux.default"
	FlakeURI    string                 `json:"flake_uri,omitempty"`
	Image       string                 `json:"image,omitempty"`
	Database    *models.DatabaseConfig `json:"database,omitempty"`

	// Language selection for auto-detection
	// When specified, determines build strategy and build type automatically
	// Valid values: "go", "Go", "rust", "Rust", "python", "Python", "node", "nodejs", "Node.js", "dockerfile", "Dockerfile"
	Language string `json:"language,omitempty"`

	// Build strategy configuration
	BuildStrategy models.BuildStrategy `json:"build_strategy,omitempty"` // Default: "flake"
	BuildConfig   *models.BuildConfig  `json:"build_config,omitempty"`

	// Runtime
	Resources   *models.ResourceSpec      `json:"resources,omitempty"` // CPU/memory specification
	Replicas    int                       `json:"replicas,omitempty"`  // Default: 1
	Ports       []models.PortMapping      `json:"ports,omitempty"`
	HealthCheck *models.HealthCheckConfig `json:"health_check,omitempty"`
	DependsOn   []string                  `json:"depends_on,omitempty"`
	EnvVars     map[string]string         `json:"env_vars,omitempty"`
}

// UpdateServiceRequest represents the request body for updating a service.
type UpdateServiceRequest struct {
	SourceType  *models.SourceType     `json:"source_type,omitempty"`
	GitRepo     *string                `json:"git_repo,omitempty"`
	GitRef      *string                `json:"git_ref,omitempty"`
	FlakeOutput *string                `json:"flake_output,omitempty"`
	FlakeURI    *string                `json:"flake_uri,omitempty"`
	Image       *string                `json:"image,omitempty"`
	Database    *models.DatabaseConfig `json:"database,omitempty"`

	// Build strategy updates
	BuildStrategy *models.BuildStrategy `json:"build_strategy,omitempty"`
	BuildConfig   *models.BuildConfig   `json:"build_config,omitempty"`

	// Runtime updates
	Resources   *models.ResourceSpec      `json:"resources,omitempty"` // CPU/memory specification
	Replicas    *int                      `json:"replicas,omitempty"`
	Ports       []models.PortMapping      `json:"ports,omitempty"`
	HealthCheck *models.HealthCheckConfig `json:"health_check,omitempty"`
	DependsOn   []string                  `json:"depends_on,omitempty"`
	EnvVars     map[string]string         `json:"env_vars,omitempty"`
}

// ServiceResponse represents a service in API responses with inherited env vars.
type ServiceResponse struct {
	models.ServiceConfig
	InheritedEnvVars map[string]string `json:"inherited_env_vars,omitempty"`
}

// Create handles POST /v1/apps/{appID}/services - creates a new service.
func (h *ServiceHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Use resolved app ID from middleware (handles both UUID and name lookup)
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

	var req CreateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Validate service name (Requirements: 10.1, 10.2, 10.3)
	if err := validation.ValidateServiceName(req.Name); err != nil {
		if validationErr, ok := err.(*models.ValidationError); ok {
			WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, validationErr.Error())
			return
		}
		WriteBadRequest(w, err.Error())
		return
	}

	// Reject image source type (Requirements: 14.1)
	if req.SourceType == models.SourceTypeImage || req.Image != "" {
		WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Image source type is not supported. Please use Nix flakes or git sources instead.")
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

	// Check service count limit (Requirements: 24.1, 24.2)
	maxServices := 50 // Default limit
	if maxServicesStr, err := h.store.Settings().Get(r.Context(), "max_services_per_app"); err == nil && maxServicesStr != "" {
		if n, err := parseIntSetting(maxServicesStr); err == nil && n > 0 {
			maxServices = n
		}
	}
	if len(app.Services) >= maxServices {
		WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, fmt.Sprintf("Maximum services per app (%d) reached. Delete unused services or contact administrator.", maxServices))
		return
	}

	// Check for duplicate service name
	for _, svc := range app.Services {
		if svc.Name == req.Name {
			WriteConflict(w, "A service with this name already exists")
			return
		}
	}

	// Infer source type from provided fields (Requirements: 13.1, 13.2, 13.3, 13.4)
	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = h.inferSourceType(r.Context(), &req)
	}

	// Validate resource specification (Requirements: 12.1, 12.2)
	if req.Resources != nil {
		if err := validation.ValidateResourceSpec(req.Resources); err != nil {
			if validationErr, ok := err.(*models.ValidationError); ok {
				WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, validationErr.Error())
				return
			}
			WriteBadRequest(w, err.Error())
			return
		}
	}

	// Validate database configuration (Requirements: 29.1, 29.2)
	if sourceType == models.SourceTypeDatabase && req.Database != nil {
		if err := validation.ValidateDatabaseConfig(req.Database); err != nil {
			if validationErr, ok := err.(*models.ValidationError); ok {
				WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, validationErr.Error())
				return
			}
			WriteBadRequest(w, err.Error())
			return
		}
	}

	// Build service config
	service := models.ServiceConfig{
		Name:          req.Name,
		SourceType:    sourceType,
		GitRepo:       req.GitRepo,
		GitRef:        req.GitRef,
		FlakeOutput:   req.FlakeOutput,
		FlakeURI:      req.FlakeURI,
		Database:      req.Database,
		BuildStrategy: req.BuildStrategy,
		BuildConfig:   req.BuildConfig,
		Resources:     req.Resources,
		Ports:         req.Ports,
		HealthCheck:   req.HealthCheck,
		DependsOn:     req.DependsOn,
		EnvVars:       req.EnvVars,
	}

	// Apply default resources if not specified (Requirements: 12.3, 30.2, 30.3)
	if service.Resources == nil {
		service.Resources = h.getDefaultResources(r.Context())
	}

	if req.Replicas > 0 {
		service.Replicas = req.Replicas
	} else {
		service.Replicas = 1
	}

	// Default to port 8080 if no ports specified (common for web services)
	if len(service.Ports) == 0 {
		service.Ports = []models.PortMapping{{ContainerPort: 8080, Protocol: "tcp"}}
	}

	// Auto-detect build strategy and build type based on language selection
	// **Validates: Requirements 4.1, 4.2, 4.7, 4.8**
	if req.Language != "" {
		strategy, _ := detector.DetermineBuildTypeFromLanguage(req.Language)
		service.BuildStrategy = strategy
		h.logger.Info("build strategy determined from language",
			"language", req.Language,
			"strategy", strategy,
		)
	}

	// Apply default build strategy if not specified
	// Database services default to auto-database, others default to flake
	if service.BuildStrategy == "" {
		if sourceType == models.SourceTypeDatabase {
			service.BuildStrategy = models.BuildStrategyAutoDatabase
		} else {
			service.BuildStrategy = models.BuildStrategyFlake
		}
	}

	// Validate build strategy
	if !service.BuildStrategy.IsValid() {
		WriteBadRequest(w, "Invalid build_strategy: must be one of flake, auto-go, auto-rust, auto-node, auto-python, dockerfile, nixpacks, auto")
		return
	}

	// Validate service configuration (this also applies defaults for git_ref and flake_output)
	if err := service.Validate(); err != nil {
		if validationErr, ok := err.(*models.ValidationError); ok {
			WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, validationErr.Error())
			return
		}
		WriteBadRequest(w, err.Error())
		return
	}

	// Validate dependencies using DependencyValidator (Requirements: 9.1, 9.3, 9.4)
	if len(service.DependsOn) > 0 {
		// Check for self-dependency and circular dependencies
		if err := h.dependencyValidator.ValidateDependencies(app.Services, service.Name, service.DependsOn); err != nil {
			if validationErr, ok := err.(*models.ValidationError); ok {
				WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, validationErr.Error())
				return
			}
			WriteBadRequest(w, err.Error())
			return
		}

		// Validate dependencies exist
		existingServices := make(map[string]bool)
		for _, svc := range app.Services {
			existingServices[svc.Name] = true
		}
		for _, dep := range service.DependsOn {
			if !existingServices[dep] {
				WriteBadRequest(w, "Dependency '"+dep+"' not found in app")
				return
			}
		}
	}

	// Auto-generate database credentials for database services
	if service.SourceType == models.SourceTypeDatabase && service.Database != nil {
		if err := h.generateDatabaseCredentials(r.Context(), appID, service.Name, service.Database.Type); err != nil {
			h.logger.Error("failed to generate database credentials", "error", err, "app_id", appID, "service_name", service.Name)
			WriteInternalError(w, "Failed to generate database credentials")
			return
		}
		h.logger.Info("database credentials generated", "app_id", appID, "service_name", service.Name, "db_type", service.Database.Type)
	}

	// Add service to app
	app.Services = append(app.Services, service)
	app.UpdatedAt = time.Now()

	if err := h.store.Apps().Update(r.Context(), app); err != nil {
		h.logger.Error("failed to update app with new service", "error", err)
		WriteInternalError(w, "Failed to create service")
		return
	}

	h.logger.Info("service created", "app_id", appID, "service_name", service.Name)
	WriteJSON(w, http.StatusCreated, service)
}

// List handles GET /v1/apps/{appID}/services - lists all services for an app.
func (h *ServiceHandler) List(w http.ResponseWriter, r *http.Request) {
	// Use resolved app ID from middleware (handles both UUID and name lookup)
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
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

	// Return empty array instead of null
	services := app.Services
	if services == nil {
		services = []models.ServiceConfig{}
	}

	WriteJSON(w, http.StatusOK, services)
}

// Get handles GET /v1/apps/{appID}/services/{serviceName} - retrieves a specific service.
func (h *ServiceHandler) Get(w http.ResponseWriter, r *http.Request) {
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
	for _, svc := range app.Services {
		if svc.Name == serviceName {
			WriteJSON(w, http.StatusOK, svc)
			return
		}
	}

	WriteNotFound(w, "Service not found")
}

// Update handles PATCH /v1/apps/{appID}/services/{serviceName} - updates a service.
func (h *ServiceHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	var req UpdateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Reject image source type (Requirements: 14.1)
	if req.SourceType != nil && *req.SourceType == models.SourceTypeImage {
		WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Image source type is not supported. Please use Nix flakes or git sources instead.")
		return
	}
	if req.Image != nil && *req.Image != "" {
		WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Image source type is not supported. Please use Nix flakes or git sources instead.")
		return
	}

	// Validate resource specification if provided (Requirements: 12.1, 12.2)
	if req.Resources != nil {
		if err := validation.ValidateResourceSpec(req.Resources); err != nil {
			if validationErr, ok := err.(*models.ValidationError); ok {
				WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, validationErr.Error())
				return
			}
			WriteBadRequest(w, err.Error())
			return
		}
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

	// Find and update the service
	serviceIndex := -1
	for i, svc := range app.Services {
		if svc.Name == serviceName {
			serviceIndex = i
			break
		}
	}

	if serviceIndex == -1 {
		WriteNotFound(w, "Service not found")
		return
	}

	service := &app.Services[serviceIndex]

	// Apply updates (preserve unspecified fields)
	if req.SourceType != nil {
		service.SourceType = *req.SourceType
	}
	if req.GitRepo != nil {
		service.GitRepo = *req.GitRepo
		service.FlakeURI = ""
		service.Image = ""
		service.Database = nil
	}
	if req.GitRef != nil {
		service.GitRef = *req.GitRef
	}
	if req.FlakeOutput != nil {
		service.FlakeOutput = *req.FlakeOutput
	}
	if req.FlakeURI != nil {
		service.FlakeURI = *req.FlakeURI
		service.GitRepo = ""
		service.Image = ""
		service.Database = nil
	}
	// Image field is no longer supported - skip updating it
	if req.Database != nil {
		// Validate database configuration (Requirements: 29.1, 29.2)
		if err := validation.ValidateDatabaseConfig(req.Database); err != nil {
			if validationErr, ok := err.(*models.ValidationError); ok {
				WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, validationErr.Error())
				return
			}
			WriteBadRequest(w, err.Error())
			return
		}
		service.Database = req.Database
		service.GitRepo = ""
		service.FlakeURI = ""
		service.Image = ""
	}
	if req.BuildStrategy != nil {
		if !req.BuildStrategy.IsValid() {
			WriteBadRequest(w, "Invalid build_strategy: must be one of flake, auto-go, auto-rust, auto-node, auto-python, dockerfile, nixpacks, auto")
			return
		}
		service.BuildStrategy = *req.BuildStrategy
	}
	if req.BuildConfig != nil {
		service.BuildConfig = req.BuildConfig
	}
	// Direct resource specification (Requirements: 12.1, 12.5)
	if req.Resources != nil {
		service.Resources = req.Resources
	}
	if req.Replicas != nil {
		service.Replicas = *req.Replicas
	}
	if req.Ports != nil {
		service.Ports = req.Ports
	}
	if req.HealthCheck != nil {
		service.HealthCheck = req.HealthCheck
	}
	if req.DependsOn != nil {
		service.DependsOn = req.DependsOn
	}
	if req.EnvVars != nil {
		service.EnvVars = req.EnvVars
	}

	// Re-validate after updates
	if err := service.Validate(); err != nil {
		if validationErr, ok := err.(*models.ValidationError); ok {
			WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, validationErr.Error())
			return
		}
		WriteBadRequest(w, err.Error())
		return
	}

	// Validate dependencies using DependencyValidator (Requirements: 9.1)
	if len(service.DependsOn) > 0 {
		// Check for self-dependency and circular dependencies
		if err := h.dependencyValidator.ValidateDependencies(app.Services, service.Name, service.DependsOn); err != nil {
			if validationErr, ok := err.(*models.ValidationError); ok {
				WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, validationErr.Error())
				return
			}
			WriteBadRequest(w, err.Error())
			return
		}

		// Validate dependencies exist
		existingServices := make(map[string]bool)
		for _, svc := range app.Services {
			existingServices[svc.Name] = true
		}
		for _, dep := range service.DependsOn {
			if !existingServices[dep] {
				WriteBadRequest(w, "Dependency '"+dep+"' not found in app")
				return
			}
		}
	}

	// Update app's updated_at timestamp
	app.UpdatedAt = time.Now()

	if err := h.store.Apps().Update(r.Context(), app); err != nil {
		h.logger.Error("failed to update service", "error", err)
		WriteInternalError(w, "Failed to update service")
		return
	}

	h.logger.Info("service updated", "app_id", appID, "service_name", serviceName)
	WriteJSON(w, http.StatusOK, service)
}

// Delete handles DELETE /v1/apps/{appID}/services/{serviceName} - deletes a service.
func (h *ServiceHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
	serviceIndex := -1
	var serviceToDelete *models.ServiceConfig
	for i, svc := range app.Services {
		if svc.Name == serviceName {
			serviceIndex = i
			serviceToDelete = &app.Services[i]
			break
		}
	}

	if serviceIndex == -1 {
		WriteNotFound(w, "Service not found")
		return
	}

	// Check for dependency violations
	var dependents []string
	for _, svc := range app.Services {
		if svc.Name == serviceName {
			continue
		}
		for _, dep := range svc.DependsOn {
			if dep == serviceName {
				dependents = append(dependents, svc.Name)
				break
			}
		}
	}

	if len(dependents) > 0 {
		WriteError(w, http.StatusConflict, ErrCodeConflict,
			"Cannot delete service '"+serviceName+"': it is a dependency of "+formatDependents(dependents))
		return
	}

	// Use transaction for atomicity (Requirements: 21.1, 21.2, 21.3, 21.4)
	err = h.store.WithTx(r.Context(), func(txStore store.Store) error {
		// Delete associated secrets (database credentials) (Requirements: 21.1)
		if serviceToDelete.SourceType == models.SourceTypeDatabase {
			secretKeys, err := txStore.Secrets().List(r.Context(), appID)
			if err == nil {
				// Delete secrets that match this service's naming pattern
				servicePrefix := strings.ToUpper(serviceName)
				for _, key := range secretKeys {
					if strings.HasPrefix(key, servicePrefix+"_") {
						if err := txStore.Secrets().Delete(r.Context(), appID, key); err != nil {
							h.logger.Error("failed to delete secret", "error", err, "key", key)
						}
					}
				}
			}
		}

		// Remove domain mappings for this service (Requirements: 21.2)
		domains, err := txStore.Domains().List(r.Context(), appID)
		if err == nil {
			for _, domain := range domains {
				if domain.Service == serviceName {
					if err := txStore.Domains().Delete(r.Context(), domain.ID); err != nil {
						h.logger.Error("failed to delete domain mapping", "error", err, "domain_id", domain.ID)
					}
				}
			}
		}

		// Cancel pending builds for this service (Requirements: 21.3)
		builds, err := txStore.Builds().List(r.Context(), appID)
		if err == nil {
			for _, build := range builds {
				if build.ServiceName == serviceName && (build.Status == models.BuildStatusQueued || build.Status == models.BuildStatusRunning) {
					build.Status = models.BuildStatusFailed
					build.FinishedAt = timePtr(time.Now())
					if err := txStore.Builds().Update(r.Context(), build); err != nil {
						h.logger.Error("failed to cancel build", "error", err, "build_id", build.ID)
					}
				}
			}
		}

		// Stop running deployments and schedule container cleanup (Requirements: 21.4)
		deployments, err := txStore.Deployments().List(r.Context(), appID)
		if err == nil {
			for _, d := range deployments {
				if d.ServiceName == serviceName && isActiveDeployment(d.Status) {
					d.Status = models.DeploymentStatusFailed
					d.UpdatedAt = time.Now()
					if err := txStore.Deployments().Update(r.Context(), d); err != nil {
						h.logger.Error("failed to update deployment status", "error", err, "deployment_id", d.ID)
					}
				}
			}
		}

		// Remove the service from the app
		app.Services = append(app.Services[:serviceIndex], app.Services[serviceIndex+1:]...)
		app.UpdatedAt = time.Now()

		return txStore.Apps().Update(r.Context(), app)
	})

	if err != nil {
		h.logger.Error("failed to delete service", "error", err)
		WriteInternalError(w, "Failed to delete service")
		return
	}

	h.logger.Info("service deleted", "app_id", appID, "service_name", serviceName)
	w.WriteHeader(http.StatusNoContent)
}

// formatDependents formats a list of dependent service names for error messages.
func formatDependents(dependents []string) string {
	if len(dependents) == 1 {
		return dependents[0]
	}
	result := ""
	for i, d := range dependents {
		if i > 0 {
			if i == len(dependents)-1 {
				result += " and "
			} else {
				result += ", "
			}
		}
		result += d
	}
	return result
}

// isActiveDeployment returns true if the deployment is in an active state.
func isActiveDeployment(status models.DeploymentStatus) bool {
	return status == models.DeploymentStatusPending ||
		status == models.DeploymentStatusBuilding ||
		status == models.DeploymentStatusBuilt ||
		status == models.DeploymentStatusScheduled ||
		status == models.DeploymentStatusRunning
}

// TerminalWS handles GET /v1/apps/{appID}/services/{serviceName}/terminal/ws - WebSocket terminal bridge.
func (h *ServiceHandler) TerminalWS(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	serviceName := chi.URLParam(r, "serviceName")

	h.logger.Info("service terminal websocket request received", "app_id", appID, "service", serviceName)

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("failed to upgrade websocket", "error", err)
		return
	}
	defer conn.Close()

	// Find the latest running deployment to get the deployment ID
	ctx := r.Context()
	deployments, err := h.store.Deployments().List(ctx, appID)
	if err != nil {
		h.logger.Error("failed to list deployments", "error", err)
		return
	}

	var runningDeployment *models.Deployment
	for _, d := range deployments {
		if d.ServiceName == serviceName && d.Status == models.DeploymentStatusRunning {
			runningDeployment = d
			break
		}
	}

	if runningDeployment == nil {
		h.logger.Error("no running deployment found", "app_id", appID, "service", serviceName)
		return
	}

	// Container name format: narvana-<deployment-id>
	// This matches the node-agent's naming convention
	containerName := fmt.Sprintf("narvana-%s", runningDeployment.ID)

	// Spawn shell with PTY
	h.logger.Info("starting podman exec pty", "container", containerName, "deployment_id", runningDeployment.ID)
	c := h.podman.Exec(containerName, []string{"/bin/sh"})
	c.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	)

	f, err := pty.Start(c)
	if err != nil {
		h.logger.Error("failed to start pty", "error", err, "container", containerName)
		// Try legacy format as fallback
		containerName = fmt.Sprintf("%s-%s", appID, serviceName)
		h.logger.Info("trying legacy container name", "container", containerName)
		c = h.podman.Exec(containerName, []string{"/bin/sh"})
		f, err = pty.Start(c)
		if err != nil {
			h.logger.Error("failed to start pty (second attempt)", "error", err, "container", containerName)
			return
		}
	}

	h.logger.Info("service pty started successfully", "container", containerName)
	defer f.Close()

	// Set initial size
	_ = pty.Setsize(f, &pty.Winsize{Rows: 24, Cols: 80})

	// Clean up process on exit
	defer func() {
		if c.Process != nil {
			c.Process.Kill()
		}
	}()

	// Copy PTY output to WebSocket
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := f.Read(buf)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	// Handle WebSocket input
	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		if mt == websocket.TextMessage {
			var ctrl struct {
				Type string `json:"type"`
				Rows uint16 `json:"rows"`
				Cols uint16 `json:"cols"`
			}
			if err := json.Unmarshal(msg, &ctrl); err == nil {
				if ctrl.Type == "resize" {
					_ = pty.Setsize(f, &pty.Winsize{Rows: ctrl.Rows, Cols: ctrl.Cols})
					continue
				}
				if ctrl.Type == "terminate" {
					h.logger.Info("service terminal termination requested")
					if c.Process != nil {
						c.Process.Kill()
					}
					return
				}
			}
		}

		if mt == websocket.BinaryMessage || mt == websocket.TextMessage {
			f.Write(msg)
		}
	}
}

// ServiceDetectRequest represents the request body for detecting service configuration.
type ServiceDetectRequest struct {
	GitURL string `json:"git_url"`
	GitRef string `json:"git_ref,omitempty"`
}

// ServiceDetectResponse represents the response for service detection.
// **Validates: Requirements 4.3, 4.4, 4.5**
type ServiceDetectResponse struct {
	Strategy        models.BuildStrategy   `json:"strategy"`
	BuildType       models.BuildType       `json:"build_type"`
	Framework       models.Framework       `json:"framework"`
	Version         string                 `json:"version,omitempty"`
	EntryPoint      string                 `json:"entry_point,omitempty"`
	EntryPoints     []string               `json:"entry_points,omitempty"`
	BuildCommand    string                 `json:"build_command,omitempty"`
	StartCommand    string                 `json:"start_command,omitempty"`
	SuggestedConfig map[string]interface{} `json:"suggested_config,omitempty"`
	Confidence      float64                `json:"confidence"`
	Warnings        []string               `json:"warnings,omitempty"`
}

// DetectForService handles POST /v1/apps/{appID}/services/detect - detects service configuration from git URL.
// This endpoint is called when a user enters a git URL during service creation to auto-populate fields.
// **Validates: Requirements 4.3, 4.4, 4.5**
func (h *ServiceHandler) DetectForService(w http.ResponseWriter, r *http.Request) {
	var req ServiceDetectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Validate git URL
	if req.GitURL == "" {
		WriteBadRequest(w, "git_url is required")
		return
	}

	// Default git ref to main
	gitRef := req.GitRef
	if gitRef == "" {
		gitRef = "main"
	}

	// Create a detect handler to leverage existing detection logic
	detectHandler := NewDetectHandler(h.logger)

	// Use a timeout context for detection
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	// Clone and detect
	result, err := detectHandler.CloneAndDetect(ctx, req.GitURL, gitRef)
	if err != nil {
		h.logger.Error("detection failed", "error", err, "url", req.GitURL)
		WriteError(w, http.StatusUnprocessableEntity, "detection_failed", "Failed to detect repository: "+err.Error())
		return
	}

	// Determine build type based on strategy
	// **Validates: Requirements 4.7, 4.8**
	buildType := detector.DetermineBuildType(result.Strategy)

	// Extract entry point and build command from suggested config
	var entryPoint, buildCommand, startCommand string
	if result.SuggestedConfig != nil {
		if ep, ok := result.SuggestedConfig["entry_point"].(string); ok {
			entryPoint = ep
		}
		if bc, ok := result.SuggestedConfig["build_command"].(string); ok {
			buildCommand = bc
		}
		if sc, ok := result.SuggestedConfig["start_command"].(string); ok {
			startCommand = sc
		}
	}

	// If entry points are detected, use the first one as the default entry point
	if entryPoint == "" && len(result.EntryPoints) > 0 {
		entryPoint = result.EntryPoints[0]
	}

	// Build response
	response := ServiceDetectResponse{
		Strategy:        result.Strategy,
		BuildType:       buildType,
		Framework:       result.Framework,
		Version:         result.Version,
		EntryPoint:      entryPoint,
		EntryPoints:     result.EntryPoints,
		BuildCommand:    buildCommand,
		StartCommand:    startCommand,
		SuggestedConfig: result.SuggestedConfig,
		Confidence:      result.Confidence,
		Warnings:        result.Warnings,
	}

	h.logger.Info("service detection completed",
		"url", req.GitURL,
		"strategy", result.Strategy,
		"build_type", buildType,
		"framework", result.Framework,
	)

	WriteJSON(w, http.StatusOK, response)
}

// StopService handles POST /v1/apps/{appID}/services/{serviceName}/stop - stops a running service.
// **Validates: Requirements 7.7**
func (h *ServiceHandler) StopService(w http.ResponseWriter, r *http.Request) {
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

	// Find the latest running deployment for this service
	deployments, err := h.store.Deployments().List(r.Context(), appID)
	if err != nil {
		h.logger.Error("failed to list deployments", "error", err)
		WriteInternalError(w, "Failed to list deployments")
		return
	}

	var runningDeployment *models.Deployment
	for _, d := range deployments {
		if d.ServiceName == serviceName && d.Status == models.DeploymentStatusRunning {
			runningDeployment = d
			break
		}
	}

	if runningDeployment == nil {
		WriteBadRequest(w, "No running deployment found for this service")
		return
	}

	// Update deployment status to stopping, then stopped
	runningDeployment.Status = models.DeploymentStatusStopped
	runningDeployment.UpdatedAt = time.Now()

	if err := h.store.Deployments().Update(r.Context(), runningDeployment); err != nil {
		h.logger.Error("failed to update deployment status", "error", err)
		WriteInternalError(w, "Failed to stop service")
		return
	}

	h.logger.Info("service stopped", "app_id", appID, "service_name", serviceName, "deployment_id", runningDeployment.ID)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "stopped", "deployment_id": runningDeployment.ID})
}

// StartService handles POST /v1/apps/{appID}/services/{serviceName}/start - starts a stopped service.
// **Validates: Requirements 7.7**
func (h *ServiceHandler) StartService(w http.ResponseWriter, r *http.Request) {
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

	// Find the latest stopped deployment for this service
	deployments, err := h.store.Deployments().List(r.Context(), appID)
	if err != nil {
		h.logger.Error("failed to list deployments", "error", err)
		WriteInternalError(w, "Failed to list deployments")
		return
	}

	var stoppedDeployment *models.Deployment
	for _, d := range deployments {
		if d.ServiceName == serviceName && d.Status == models.DeploymentStatusStopped {
			stoppedDeployment = d
			break
		}
	}

	if stoppedDeployment == nil {
		WriteBadRequest(w, "No stopped deployment found for this service")
		return
	}

	// Update deployment status to running
	stoppedDeployment.Status = models.DeploymentStatusRunning
	stoppedDeployment.UpdatedAt = time.Now()

	if err := h.store.Deployments().Update(r.Context(), stoppedDeployment); err != nil {
		h.logger.Error("failed to update deployment status", "error", err)
		WriteInternalError(w, "Failed to start service")
		return
	}

	h.logger.Info("service started", "app_id", appID, "service_name", serviceName, "deployment_id", stoppedDeployment.ID)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "running", "deployment_id": stoppedDeployment.ID})
}

// ReloadService handles POST /v1/apps/{appID}/services/{serviceName}/reload - restarts a service without rebuilding.
// **Validates: Requirements 7.8**
func (h *ServiceHandler) ReloadService(w http.ResponseWriter, r *http.Request) {
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

	// Find the latest running deployment for this service
	deployments, err := h.store.Deployments().List(r.Context(), appID)
	if err != nil {
		h.logger.Error("failed to list deployments", "error", err)
		WriteInternalError(w, "Failed to list deployments")
		return
	}

	var runningDeployment *models.Deployment
	for _, d := range deployments {
		if d.ServiceName == serviceName && d.Status == models.DeploymentStatusRunning {
			runningDeployment = d
			break
		}
	}

	if runningDeployment == nil {
		WriteBadRequest(w, "No running deployment found for this service")
		return
	}

	// For reload, we mark the deployment as restarting (using starting status)
	// The scheduler will handle the actual restart
	runningDeployment.Status = models.DeploymentStatusStarting
	runningDeployment.UpdatedAt = time.Now()

	if err := h.store.Deployments().Update(r.Context(), runningDeployment); err != nil {
		h.logger.Error("failed to update deployment status", "error", err)
		WriteInternalError(w, "Failed to reload service")
		return
	}

	h.logger.Info("service reload initiated", "app_id", appID, "service_name", serviceName, "deployment_id", runningDeployment.ID)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "reloading", "deployment_id": runningDeployment.ID})
}

// RetryService handles POST /v1/apps/{appID}/services/{serviceName}/retry - retries a failed deployment.
// **Validates: Requirements 7.6**
func (h *ServiceHandler) RetryService(w http.ResponseWriter, r *http.Request) {
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

	// Find the latest failed deployment for this service
	deployments, err := h.store.Deployments().List(r.Context(), appID)
	if err != nil {
		h.logger.Error("failed to list deployments", "error", err)
		WriteInternalError(w, "Failed to list deployments")
		return
	}

	var failedDeployment *models.Deployment
	for _, d := range deployments {
		if d.ServiceName == serviceName && d.Status == models.DeploymentStatusFailed {
			failedDeployment = d
			break
		}
	}

	if failedDeployment == nil {
		WriteBadRequest(w, "No failed deployment found for this service")
		return
	}

	// Reset the deployment status to pending to retry
	failedDeployment.Status = models.DeploymentStatusPending
	failedDeployment.UpdatedAt = time.Now()

	if err := h.store.Deployments().Update(r.Context(), failedDeployment); err != nil {
		h.logger.Error("failed to update deployment status", "error", err)
		WriteInternalError(w, "Failed to retry service")
		return
	}

	h.logger.Info("service retry initiated", "app_id", appID, "service_name", serviceName, "deployment_id", failedDeployment.ID)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "retrying", "deployment_id": failedDeployment.ID})
}

// generateDatabaseCredentials generates and stores database credentials for a database service.
// This is called automatically when a database service is created.
func (h *ServiceHandler) generateDatabaseCredentials(ctx context.Context, appID, serviceName, dbType string) error {
	// Generate credentials using the database registry
	creds, err := databases.GenerateCredentials(databases.DatabaseType(dbType), serviceName)
	if err != nil {
		return fmt.Errorf("generating credentials: %w", err)
	}

	// Get the secret keys and values
	secretsMap := creds.GetSecretKeys(databases.DatabaseType(dbType), serviceName)

	// Store each secret
	for key, value := range secretsMap {
		var encryptedValue []byte

		// Encrypt the value if SOPS is configured
		if h.sopsService != nil && h.sopsService.CanEncrypt() {
			encryptedValue, err = h.sopsService.Encrypt(ctx, []byte(value))
			if err != nil {
				h.logger.Error("failed to encrypt secret with SOPS", "error", err, "key", key)
				// Fall back to storing plaintext
				encryptedValue = []byte(value)
			}
		} else {
			// No encryption configured, store as plaintext
			h.logger.Warn("SOPS not configured, storing database secret without encryption", "key", key)
			encryptedValue = []byte(value)
		}

		// Store the secret
		if err := h.store.Secrets().Set(ctx, appID, strings.ToUpper(key), encryptedValue); err != nil {
			return fmt.Errorf("storing secret %s: %w", key, err)
		}

		h.logger.Debug("database secret stored", "app_id", appID, "key", key)
	}

	return nil
}

// RenameRequest represents the request body for renaming a service.
type RenameRequest struct {
	NewName string `json:"new_name"`
}

// Rename handles POST /v1/apps/{appID}/services/{serviceName}/rename - renames a service.
// **Validates: Requirements 23.1, 23.2, 23.3, 23.4, 23.5**
func (h *ServiceHandler) Rename(w http.ResponseWriter, r *http.Request) {
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	oldName := chi.URLParam(r, "serviceName")

	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}
	if oldName == "" {
		WriteBadRequest(w, "Service name is required")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

	var req RenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Validate new name (Requirements: 23.1)
	if err := validation.ValidateServiceName(req.NewName); err != nil {
		if validationErr, ok := err.(*models.ValidationError); ok {
			WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, validationErr.Error())
			return
		}
		WriteBadRequest(w, err.Error())
		return
	}

	// Use transaction for atomicity (Requirements: 23.5)
	err := h.store.WithTx(r.Context(), func(txStore store.Store) error {
		app, err := txStore.Apps().Get(r.Context(), appID)
		if err != nil {
			return &APIError{Code: ErrCodeNotFound, Message: "Application not found"}
		}

		// Verify ownership
		if app.OwnerID != userID {
			return &APIError{Code: ErrCodeForbidden, Message: "Access denied"}
		}

		// Find the service to rename
		serviceIndex := -1
		for i, svc := range app.Services {
			if svc.Name == oldName {
				serviceIndex = i
				break
			}
		}

		if serviceIndex == -1 {
			return &APIError{Code: ErrCodeNotFound, Message: "Service not found"}
		}

		// Check new name doesn't exist (Requirements: 23.1)
		for _, svc := range app.Services {
			if svc.Name == req.NewName {
				return &APIError{Code: ErrCodeConflict, Message: "Service name already exists"}
			}
		}

		// Update service name
		app.Services[serviceIndex].Name = req.NewName

		// Update DependsOn references in other services (Requirements: 23.2)
		for i := range app.Services {
			for j, dep := range app.Services[i].DependsOn {
				if dep == oldName {
					app.Services[i].DependsOn[j] = req.NewName
				}
			}
		}

		// Update deployment records (Requirements: 23.3)
		deployments, err := txStore.Deployments().List(r.Context(), appID)
		if err != nil {
			return fmt.Errorf("failed to list deployments: %w", err)
		}
		for _, d := range deployments {
			if d.ServiceName == oldName {
				d.ServiceName = req.NewName
				if err := txStore.Deployments().Update(r.Context(), d); err != nil {
					return fmt.Errorf("failed to update deployment: %w", err)
				}
			}
		}

		// Update build records
		builds, err := txStore.Builds().List(r.Context(), appID)
		if err != nil {
			return fmt.Errorf("failed to list builds: %w", err)
		}
		for _, b := range builds {
			if b.ServiceName == oldName {
				b.ServiceName = req.NewName
				if err := txStore.Builds().Update(r.Context(), b); err != nil {
					return fmt.Errorf("failed to update build: %w", err)
				}
			}
		}

		// Update domain mappings
		domains, err := txStore.Domains().List(r.Context(), appID)
		if err == nil {
			for _, domain := range domains {
				if domain.Service == oldName {
					domain.Service = req.NewName
					// Note: Domain store doesn't have Update, so we'd need to delete and recreate
					// For now, we'll log this as a limitation
					h.logger.Warn("domain mapping service name not updated (requires manual update)", "domain_id", domain.ID)
				}
			}
		}

		app.UpdatedAt = time.Now()
		return txStore.Apps().Update(r.Context(), app)
	})

	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			switch apiErr.Code {
			case ErrCodeNotFound:
				WriteNotFound(w, apiErr.Message)
			case ErrCodeForbidden:
				WriteForbidden(w, apiErr.Message)
			case ErrCodeConflict:
				WriteConflict(w, apiErr.Message)
			default:
				WriteInternalError(w, apiErr.Message)
			}
			return
		}
		h.logger.Error("failed to rename service", "error", err)
		WriteInternalError(w, "Failed to rename service")
		return
	}

	h.logger.Info("service renamed", "app_id", appID, "old_name", oldName, "new_name", req.NewName)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "renamed", "old_name": oldName, "new_name": req.NewName})
}

// inferSourceType infers the source type from the provided request fields.
// Requirements: 13.1, 13.2, 13.3, 13.4
func (h *ServiceHandler) inferSourceType(ctx context.Context, req *CreateServiceRequest) models.SourceType {
	// If database config is provided, it's a database service
	if req.Database != nil {
		return models.SourceTypeDatabase
	}

	// If flake URI is provided, it's a flake source
	if req.FlakeURI != "" {
		return models.SourceTypeFlake
	}

	// If git repo is provided, it's a git source
	// The build worker will auto-detect if it contains flake.nix
	if req.GitRepo != "" {
		return models.SourceTypeGit
	}

	// Default to git if nothing is specified
	return models.SourceTypeGit
}

// getDefaultResources returns the default resource specification from settings.
// Requirements: 30.2, 30.3
func (h *ServiceHandler) getDefaultResources(ctx context.Context) *models.ResourceSpec {
	defaultCPU := "0.5"
	defaultMemory := "512Mi"

	if cpuStr, err := h.store.Settings().Get(ctx, "default_resource_cpu"); err == nil && cpuStr != "" {
		defaultCPU = cpuStr
	}
	if memStr, err := h.store.Settings().Get(ctx, "default_resource_memory"); err == nil && memStr != "" {
		defaultMemory = memStr
	}

	return &models.ResourceSpec{
		CPU:    defaultCPU,
		Memory: defaultMemory,
	}
}

// parseIntSetting parses an integer from a settings string.
func parseIntSetting(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// timePtr returns a pointer to the given time.
func timePtr(t time.Time) *time.Time {
	return &t
}

// AddEnvVarRequest represents the request body for adding an environment variable.
type AddEnvVarRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// EnvVarResponse represents an environment variable in API responses.
type EnvVarResponse struct {
	Key       string    `json:"key"`
	Value     string    `json:"value,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// AddEnvVar handles POST /v1/apps/{appID}/services/{serviceName}/env - adds an environment variable.
// **Validates: Requirements 1.2, 5.1, 5.2, 5.3**
func (h *ServiceHandler) AddEnvVar(w http.ResponseWriter, r *http.Request) {
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

	var req AddEnvVarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Validate key (Requirements: 5.1, 5.2)
	if err := validation.ValidateEnvKey(req.Key); err != nil {
		if validationErr, ok := err.(*models.ValidationError); ok {
			WriteErrorWithDetails(w, http.StatusBadRequest, "INVALID_KEY", validationErr.Message, map[string]string{
				"field":      "key",
				"constraint": "pattern",
				"expected":   "^[A-Za-z_][A-Za-z0-9_]*$",
			})
			return
		}
		WriteBadRequest(w, err.Error())
		return
	}

	// Validate value (Requirements: 5.1, 5.3)
	if err := validation.ValidateEnvValue(req.Value); err != nil {
		if validationErr, ok := err.(*models.ValidationError); ok {
			WriteErrorWithDetails(w, http.StatusBadRequest, "INVALID_VALUE", validationErr.Message, map[string]string{
				"field":      "value",
				"constraint": "max_length",
				"expected":   "32KB",
			})
			return
		}
		WriteBadRequest(w, err.Error())
		return
	}

	// Get the app
	app, err := h.store.Apps().Get(r.Context(), appID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "APP_NOT_FOUND", "Application not found")
		return
	}

	// Verify ownership
	if app.OwnerID != userID {
		WriteForbidden(w, "Access denied")
		return
	}

	// Find the service
	serviceIndex := -1
	for i := range app.Services {
		if app.Services[i].Name == serviceName {
			serviceIndex = i
			break
		}
	}

	if serviceIndex == -1 {
		WriteError(w, http.StatusNotFound, "SERVICE_NOT_FOUND", "Service not found")
		return
	}

	// Initialize env vars map if nil
	if app.Services[serviceIndex].EnvVars == nil {
		app.Services[serviceIndex].EnvVars = make(map[string]string)
	}

	// Check if key already exists
	if _, exists := app.Services[serviceIndex].EnvVars[req.Key]; exists {
		WriteError(w, http.StatusConflict, "KEY_EXISTS", "Environment variable with this key already exists")
		return
	}

	// Add the env var
	app.Services[serviceIndex].EnvVars[req.Key] = req.Value
	app.UpdatedAt = time.Now()

	if err := h.store.Apps().Update(r.Context(), app); err != nil {
		h.logger.Error("failed to add env var", "error", err)
		WriteInternalError(w, "Failed to add environment variable")
		return
	}

	h.logger.Info("env var added", "app_id", appID, "service_name", serviceName, "key", req.Key)
	WriteJSON(w, http.StatusCreated, EnvVarResponse{
		Key:       req.Key,
		Value:     req.Value,
		CreatedAt: app.UpdatedAt,
	})
}

// DeleteEnvVar handles DELETE /v1/apps/{appID}/services/{serviceName}/env/{key} - deletes an environment variable.
// **Validates: Requirements 1.4**
func (h *ServiceHandler) DeleteEnvVar(w http.ResponseWriter, r *http.Request) {
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	serviceName := chi.URLParam(r, "serviceName")
	key := chi.URLParam(r, "key")

	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}
	if serviceName == "" {
		WriteBadRequest(w, "Service name is required")
		return
	}
	if key == "" {
		WriteBadRequest(w, "Key is required")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

	// Get the app
	app, err := h.store.Apps().Get(r.Context(), appID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "APP_NOT_FOUND", "Application not found")
		return
	}

	// Verify ownership
	if app.OwnerID != userID {
		WriteForbidden(w, "Access denied")
		return
	}

	// Find the service
	serviceIndex := -1
	for i := range app.Services {
		if app.Services[i].Name == serviceName {
			serviceIndex = i
			break
		}
	}

	if serviceIndex == -1 {
		WriteError(w, http.StatusNotFound, "SERVICE_NOT_FOUND", "Service not found")
		return
	}

	// Check if env var exists
	if app.Services[serviceIndex].EnvVars == nil {
		WriteError(w, http.StatusNotFound, "KEY_NOT_FOUND", "Environment variable not found")
		return
	}

	if _, exists := app.Services[serviceIndex].EnvVars[key]; !exists {
		WriteError(w, http.StatusNotFound, "KEY_NOT_FOUND", "Environment variable not found")
		return
	}

	// Delete the env var
	delete(app.Services[serviceIndex].EnvVars, key)
	app.UpdatedAt = time.Now()

	if err := h.store.Apps().Update(r.Context(), app); err != nil {
		h.logger.Error("failed to delete env var", "error", err)
		WriteInternalError(w, "Failed to delete environment variable")
		return
	}

	h.logger.Info("env var deleted", "app_id", appID, "service_name", serviceName, "key", key)
	w.WriteHeader(http.StatusNoContent)
}

// UpdateEnvVarRequest represents the request body for updating an environment variable.
type UpdateEnvVarRequest struct {
	Value string `json:"value"`
}

// UpdateEnvVar handles PUT /v1/apps/{appID}/services/{serviceName}/env/{key} - updates an environment variable.
// **Validates: Requirements 1.3, 5.1, 5.3**
func (h *ServiceHandler) UpdateEnvVar(w http.ResponseWriter, r *http.Request) {
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	serviceName := chi.URLParam(r, "serviceName")
	key := chi.URLParam(r, "key")

	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}
	if serviceName == "" {
		WriteBadRequest(w, "Service name is required")
		return
	}
	if key == "" {
		WriteBadRequest(w, "Key is required")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

	var req UpdateEnvVarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Validate value (Requirements: 5.1, 5.3)
	if err := validation.ValidateEnvValue(req.Value); err != nil {
		if validationErr, ok := err.(*models.ValidationError); ok {
			WriteErrorWithDetails(w, http.StatusBadRequest, "INVALID_VALUE", validationErr.Message, map[string]string{
				"field":      "value",
				"constraint": "max_length",
				"expected":   "32KB",
			})
			return
		}
		WriteBadRequest(w, err.Error())
		return
	}

	// Get the app
	app, err := h.store.Apps().Get(r.Context(), appID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "APP_NOT_FOUND", "Application not found")
		return
	}

	// Verify ownership
	if app.OwnerID != userID {
		WriteForbidden(w, "Access denied")
		return
	}

	// Find the service
	serviceIndex := -1
	for i := range app.Services {
		if app.Services[i].Name == serviceName {
			serviceIndex = i
			break
		}
	}

	if serviceIndex == -1 {
		WriteError(w, http.StatusNotFound, "SERVICE_NOT_FOUND", "Service not found")
		return
	}

	// Check if env var exists
	if app.Services[serviceIndex].EnvVars == nil {
		WriteError(w, http.StatusNotFound, "KEY_NOT_FOUND", "Environment variable not found")
		return
	}

	if _, exists := app.Services[serviceIndex].EnvVars[key]; !exists {
		WriteError(w, http.StatusNotFound, "KEY_NOT_FOUND", "Environment variable not found")
		return
	}

	// Update the env var
	app.Services[serviceIndex].EnvVars[key] = req.Value
	app.UpdatedAt = time.Now()

	if err := h.store.Apps().Update(r.Context(), app); err != nil {
		h.logger.Error("failed to update env var", "error", err)
		WriteInternalError(w, "Failed to update environment variable")
		return
	}

	h.logger.Info("env var updated", "app_id", appID, "service_name", serviceName, "key", key)
	WriteJSON(w, http.StatusOK, EnvVarResponse{
		Key:       key,
		Value:     req.Value,
		CreatedAt: app.UpdatedAt,
	})
}

// ListEnvVars handles GET /v1/apps/{appID}/services/{serviceName}/env - lists all environment variables.
// **Validates: Requirements 5.4**
func (h *ServiceHandler) ListEnvVars(w http.ResponseWriter, r *http.Request) {
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

	// Get the app
	app, err := h.store.Apps().Get(r.Context(), appID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "APP_NOT_FOUND", "Application not found")
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
		WriteError(w, http.StatusNotFound, "SERVICE_NOT_FOUND", "Service not found")
		return
	}

	// Build response
	variables := make([]EnvVarResponse, 0)
	if service.EnvVars != nil {
		for k, v := range service.EnvVars {
			variables = append(variables, EnvVarResponse{
				Key:   k,
				Value: v,
			})
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"variables": variables,
	})
}
