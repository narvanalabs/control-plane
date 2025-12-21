// Package handlers provides HTTP request handlers for the API.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// ServiceHandler handles service-related HTTP requests.
type ServiceHandler struct {
	store  store.Store
	logger *slog.Logger
}

// NewServiceHandler creates a new service handler.
func NewServiceHandler(st store.Store, logger *slog.Logger) *ServiceHandler {
	return &ServiceHandler{
		store:  st,
		logger: logger,
	}
}

// CreateServiceRequest represents the request body for creating a service.
type CreateServiceRequest struct {
	Name string `json:"name"`

	// Source (exactly one required)
	GitRepo     string `json:"git_repo,omitempty"`
	GitRef      string `json:"git_ref,omitempty"`      // Default: "main"
	FlakeOutput string `json:"flake_output,omitempty"` // Default: "packages.x86_64-linux.default"
	FlakeURI    string `json:"flake_uri,omitempty"`
	Image       string `json:"image,omitempty"`

	// Runtime
	ResourceTier models.ResourceTier       `json:"resource_tier,omitempty"` // Default: "small"
	Replicas     int                       `json:"replicas,omitempty"`      // Default: 1
	Ports        []models.PortMapping      `json:"ports,omitempty"`
	HealthCheck  *models.HealthCheckConfig `json:"health_check,omitempty"`
	DependsOn    []string                  `json:"depends_on,omitempty"`
	EnvVars      map[string]string         `json:"env_vars,omitempty"`
}

// UpdateServiceRequest represents the request body for updating a service.
type UpdateServiceRequest struct {
	// Source updates (can change source type)
	GitRepo     *string `json:"git_repo,omitempty"`
	GitRef      *string `json:"git_ref,omitempty"`
	FlakeOutput *string `json:"flake_output,omitempty"`
	FlakeURI    *string `json:"flake_uri,omitempty"`
	Image       *string `json:"image,omitempty"`

	// Runtime updates
	ResourceTier *models.ResourceTier      `json:"resource_tier,omitempty"`
	Replicas     *int                      `json:"replicas,omitempty"`
	Ports        []models.PortMapping      `json:"ports,omitempty"`
	HealthCheck  *models.HealthCheckConfig `json:"health_check,omitempty"`
	DependsOn    []string                  `json:"depends_on,omitempty"`
	EnvVars      map[string]string         `json:"env_vars,omitempty"`
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

	// Check for duplicate service name
	for _, svc := range app.Services {
		if svc.Name == req.Name {
			WriteConflict(w, "A service with this name already exists")
			return
		}
	}

	// Build service config
	service := models.ServiceConfig{
		Name:        req.Name,
		GitRepo:     req.GitRepo,
		GitRef:      req.GitRef,
		FlakeOutput: req.FlakeOutput,
		FlakeURI:    req.FlakeURI,
		Image:       req.Image,
		Ports:       req.Ports,
		HealthCheck: req.HealthCheck,
		DependsOn:   req.DependsOn,
		EnvVars:     req.EnvVars,
	}

	// Apply defaults for resource tier and replicas
	if req.ResourceTier != "" {
		service.ResourceTier = req.ResourceTier
	} else {
		service.ResourceTier = models.ResourceTierSmall
	}

	if req.Replicas > 0 {
		service.Replicas = req.Replicas
	} else {
		service.Replicas = 1
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

	// Validate dependencies exist
	if len(service.DependsOn) > 0 {
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
	if req.GitRepo != nil {
		service.GitRepo = *req.GitRepo
		service.FlakeURI = ""
		service.Image = ""
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
	}
	if req.Image != nil {
		service.Image = *req.Image
		service.GitRepo = ""
		service.FlakeURI = ""
	}
	if req.ResourceTier != nil {
		service.ResourceTier = *req.ResourceTier
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

	// Validate dependencies exist
	if len(service.DependsOn) > 0 {
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

	// Stop running deployments for this service
	deployments, err := h.store.Deployments().List(r.Context(), appID)
	if err == nil {
		for _, d := range deployments {
			if d.ServiceName == serviceName && isActiveDeployment(d.Status) {
				d.Status = models.DeploymentStatusFailed
				d.UpdatedAt = time.Now()
				h.store.Deployments().Update(r.Context(), d)
			}
		}
	}

	// Remove the service
	app.Services = append(app.Services[:serviceIndex], app.Services[serviceIndex+1:]...)
	app.UpdatedAt = time.Now()

	if err := h.store.Apps().Update(r.Context(), app); err != nil {
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
