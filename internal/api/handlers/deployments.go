package handlers

import (
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
	GitRef      string `json:"git_ref"`
	ServiceName string `json:"service_name,omitempty"` // Optional, deploys all services if empty
}

// Validate validates the create deployment request.
func (r *CreateDeploymentRequest) Validate() error {
	if r.GitRef == "" {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "git_ref is required"}
	}
	return nil
}


// Create handles POST /v1/apps/:appID/deploy - triggers a new deployment.
func (h *DeploymentHandler) Create(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	var req CreateDeploymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

	// Get the app to inherit build type
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
		deployment := &models.Deployment{
			ID:           uuid.New().String(),
			AppID:        appID,
			ServiceName:  svc.Name,
			GitRef:       req.GitRef,
			BuildType:    app.BuildType, // Inherit from app
			Status:       models.DeploymentStatusPending,
			ResourceTier: svc.ResourceTier,
			Config: &models.RuntimeConfig{
				ResourceTier: svc.ResourceTier,
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

		// Enqueue build job
		buildJob := &models.BuildJob{
			ID:           uuid.New().String(),
			DeploymentID: deployment.ID,
			AppID:        appID,
			GitRef:       req.GitRef,
			FlakeOutput:  svc.FlakeOutput,
			BuildType:    app.BuildType,
			Status:       models.BuildStatusQueued,
			CreatedAt:    now,
		}

		if h.queue != nil {
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
	appID := chi.URLParam(r, "appID")
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

	// Create a new deployment using the artifact from the target
	now := time.Now()
	newDeployment := &models.Deployment{
		ID:           uuid.New().String(),
		AppID:        targetDeployment.AppID,
		ServiceName:  targetDeployment.ServiceName,
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
