// Package handlers provides HTTP request handlers for the API.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/builder/detector"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
	"github.com/narvanalabs/control-plane/internal/podman"
	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// ServiceHandler handles service-related HTTP requests.
type ServiceHandler struct {
	store    store.Store
	podman   *podman.Client
	detector detector.Detector
	logger   *slog.Logger
}

// NewServiceHandler creates a new service handler.
func NewServiceHandler(st store.Store, pd *podman.Client, logger *slog.Logger) *ServiceHandler {
	return &ServiceHandler{
		store:    st,
		podman:   pd,
		detector: detector.NewDetector(),
		logger:   logger,
	}
}

// NewServiceHandlerWithDetector creates a new service handler with a custom detector.
func NewServiceHandlerWithDetector(st store.Store, pd *podman.Client, det detector.Detector, logger *slog.Logger) *ServiceHandler {
	return &ServiceHandler{
		store:    st,
		podman:   pd,
		detector: det,
		logger:   logger,
	}
}

// CreateServiceRequest represents the request body for creating a service.
type CreateServiceRequest struct {
	Name        string            `json:"name"`
	SourceType  models.SourceType `json:"source_type,omitempty"`
	GitRepo     string            `json:"git_repo,omitempty"`
	GitRef      string            `json:"git_ref,omitempty"`      // Default: "main"
	FlakeOutput string            `json:"flake_output,omitempty"` // Default: "packages.x86_64-linux.default"
	FlakeURI    string            `json:"flake_uri,omitempty"`
	Image       string            `json:"image,omitempty"`
	Database    *models.DatabaseConfig `json:"database,omitempty"`

	// Language selection for auto-detection
	// When specified, determines build strategy and build type automatically
	// Valid values: "go", "Go", "rust", "Rust", "python", "Python", "node", "nodejs", "Node.js", "dockerfile", "Dockerfile"
	Language string `json:"language,omitempty"`

	// Build strategy configuration
	BuildStrategy models.BuildStrategy `json:"build_strategy,omitempty"` // Default: "flake"
	BuildConfig   *models.BuildConfig  `json:"build_config,omitempty"`

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
	SourceType  *models.SourceType `json:"source_type,omitempty"`
	GitRepo     *string            `json:"git_repo,omitempty"`
	GitRef      *string            `json:"git_ref,omitempty"`
	FlakeOutput *string            `json:"flake_output,omitempty"`
	FlakeURI    *string            `json:"flake_uri,omitempty"`
	Image       *string            `json:"image,omitempty"`
	Database    *models.DatabaseConfig `json:"database,omitempty"`

	// Build strategy updates
	BuildStrategy *models.BuildStrategy `json:"build_strategy,omitempty"`
	BuildConfig   *models.BuildConfig   `json:"build_config,omitempty"`

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
		Name:          req.Name,
		SourceType:    req.SourceType,
		GitRepo:       req.GitRepo,
		GitRef:        req.GitRef,
		FlakeOutput:   req.FlakeOutput,
		FlakeURI:      req.FlakeURI,
		Image:         req.Image,
		Database:      req.Database,
		BuildStrategy: req.BuildStrategy,
		BuildConfig:   req.BuildConfig,
		Ports:         req.Ports,
		HealthCheck:   req.HealthCheck,
		DependsOn:     req.DependsOn,
		EnvVars:       req.EnvVars,
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
		if req.SourceType == models.SourceTypeDatabase {
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
	if req.Image != nil {
		service.Image = *req.Image
		service.GitRepo = ""
		service.FlakeURI = ""
		service.Database = nil
	}
	if req.Database != nil {
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

	// Container name format: narvana-<app-id>-<service-name>
	// We'll try a few variations if needed, but this is the standard.
	containerName := fmt.Sprintf("%s-%s", appID, serviceName)
	
	// Spawn shell with PTY
	h.logger.Info("starting podman exec pty", "container", containerName)
	c := h.podman.Exec(containerName, []string{"/bin/sh"})
	c.Env = append(os.Environ(), 
		"TERM=xterm-256color", 
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	)
	
	f, err := pty.Start(c)
	if err != nil {
		h.logger.Error("failed to start pty", "error", err, "container", containerName)
		// Try without appID prefix just in case some services use simple names
		containerName = serviceName
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
