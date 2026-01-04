// Package handlers provides HTTP request handlers for the API.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// AppHandler handles application-related HTTP requests.
type AppHandler struct {
	store  store.Store
	logger *slog.Logger
}

// NewAppHandler creates a new app handler.
func NewAppHandler(st store.Store, logger *slog.Logger) *AppHandler {
	return &AppHandler{
		store:  st,
		logger: logger,
	}
}

// CreateAppRequest represents the request body for creating an application.
type CreateAppRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	IconURL     string                 `json:"icon_url,omitempty"`
	Services    []models.ServiceConfig `json:"services"`
}

// Validate validates the create app request.
func (r *CreateAppRequest) Validate() error {
	if r.Name == "" {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "name is required"}
	}
	if len(r.Name) > 63 {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "name must be 63 characters or less"}
	}
	return nil
}


// Create handles POST /v1/apps - creates a new application.
// Requirements: 5.1, 5.3 - Sets org_id from context, assigns default org if not specified.
func (h *AppHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

	// Get org_id from context (set by OrgContext middleware)
	// Requirements: 5.1, 5.3
	orgID := middleware.GetOrgID(r.Context())

	var req CreateAppRequest
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

	// Check for duplicate name within the organization
	existing, err := h.store.Apps().GetByName(r.Context(), userID, req.Name)
	if err == nil && existing != nil {
		WriteConflict(w, "An application with this name already exists")
		return
	}

	now := time.Now()
	
	app := &models.App{
		ID:          uuid.New().String(),
		OrgID:       orgID, // Set org_id from context (Requirements: 5.1)
		OwnerID:     userID,
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		IconURL:     strings.TrimSpace(req.IconURL),
		Services:    req.Services,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.store.Apps().Create(r.Context(), app); err != nil {
		h.logger.Error("failed to create app", "error", err)
		WriteInternalError(w, "Failed to create application")
		return
	}

	h.logger.Info("application created", "app_id", app.ID, "name", app.Name, "owner_id", userID, "org_id", orgID)
	WriteJSON(w, http.StatusCreated, app)
}

// List handles GET /v1/apps - lists all applications for the current organization.
// Requirements: 5.2 - Filter apps by org_id using ListByOrg.
func (h *AppHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

	// Get org_id from context and filter by organization (Requirements: 5.2)
	orgID := middleware.GetOrgID(r.Context())
	
	var apps []*models.App
	var err error
	
	if orgID != "" {
		// Use ListByOrg for organization-scoped filtering (Requirements: 5.2)
		apps, err = h.store.Apps().ListByOrg(r.Context(), orgID)
	} else {
		// Fallback to owner-based listing if no org context
		apps, err = h.store.Apps().List(r.Context(), userID)
	}
	
	if err != nil {
		h.logger.Error("failed to list apps", "error", err, "owner_id", userID, "org_id", orgID)
		WriteInternalError(w, "Failed to list applications")
		return
	}

	// Return empty array instead of null
	if apps == nil {
		apps = []*models.App{}
	}

	WriteJSON(w, http.StatusOK, apps)
}

// Get handles GET /v1/apps/:appID - retrieves a specific application.
// Requirements: 4.1 - Verify user owns app or is org member.
func (h *AppHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

	// Use resolved app ID from middleware (handles both UUID and name lookup)
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		// Fallback to URL param if middleware didn't resolve it
		appID = chi.URLParam(r, "appID")
	}
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	app, err := h.store.Apps().Get(r.Context(), appID)
	if err != nil {
		h.logger.Debug("failed to get app", "error", err, "app_id", appID)
		WriteNotFound(w, "Application not found")
		return
	}

	// Verify user owns app or is org member (Requirements: 4.1)
	// Note: This check is also performed by RequireOwnership middleware,
	// but we include it here for defense in depth.
	if app.OwnerID != userID {
		// Check if user is a member of the app's organization
		if app.OrgID != "" {
			isMember, err := h.store.Orgs().IsMember(r.Context(), app.OrgID, userID)
			if err != nil {
				h.logger.Error("failed to check org membership", "error", err, "org_id", app.OrgID, "user_id", userID)
				WriteInternalError(w, "Failed to verify access")
				return
			}
			if !isMember {
				h.logger.Debug("access denied to app",
					"user_id", userID,
					"owner_id", app.OwnerID,
					"org_id", app.OrgID,
					"app_id", app.ID,
				)
				WriteForbidden(w, "Access denied")
				return
			}
		} else {
			// No org, and user doesn't own the app
			h.logger.Debug("access denied to app",
				"user_id", userID,
				"owner_id", app.OwnerID,
				"app_id", app.ID,
			)
			WriteForbidden(w, "Access denied")
			return
		}
	}

	WriteJSON(w, http.StatusOK, app)
}

// UpdateAppRequest represents the request body for updating an application.
// All fields are optional - only specified fields will be updated.
type UpdateAppRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	IconURL     *string `json:"icon_url,omitempty"`
	Version     int     `json:"version"` // Required for optimistic locking
}

// Validate validates the update app request.
func (r *UpdateAppRequest) Validate() error {
	if r.Name != nil {
		name := strings.TrimSpace(*r.Name)
		if name == "" {
			return &APIError{Code: ErrCodeInvalidRequest, Message: "name cannot be empty"}
		}
		if len(name) > 63 {
			return &APIError{Code: ErrCodeInvalidRequest, Message: "name must be 63 characters or less"}
		}
	}
	return nil
}

// Update handles PATCH /v1/apps/:appID - updates an application.
// Requirements: 8.1, 8.2, 8.3, 8.4 - Partial updates with optimistic locking.
func (h *AppHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

	// Use resolved app ID from middleware (handles both UUID and name lookup)
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		// Fallback to URL param if middleware didn't resolve it
		appID = chi.URLParam(r, "appID")
	}
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	var req UpdateAppRequest
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

	// Get the current app
	app, err := h.store.Apps().Get(r.Context(), appID)
	if err != nil {
		h.logger.Debug("failed to get app for update", "error", err, "app_id", appID)
		WriteNotFound(w, "Application not found")
		return
	}

	// Set the version for optimistic locking (Requirements: 8.4)
	app.Version = req.Version

	// Apply partial updates - preserve unspecified fields (Requirements: 8.3)
	if req.Name != nil {
		newName := strings.TrimSpace(*req.Name)
		
		// Validate name uniqueness within org if name is changing (Requirements: 8.2)
		if newName != app.Name {
			// Check if another app with this name exists in the same org
			existing, err := h.store.Apps().GetByName(r.Context(), app.OwnerID, newName)
			if err == nil && existing != nil && existing.ID != app.ID {
				WriteConflict(w, "An application with this name already exists in this organization")
				return
			}
		}
		app.Name = newName
	}

	if req.Description != nil {
		app.Description = strings.TrimSpace(*req.Description)
	}

	if req.IconURL != nil {
		app.IconURL = strings.TrimSpace(*req.IconURL)
	}

	// Update the app with optimistic locking
	if err := h.store.Apps().Update(r.Context(), app); err != nil {
		// Check for concurrent modification error
		if err.Error() == "resource was modified by another request" {
			WriteConflict(w, "Resource was modified by another request. Please refresh and try again.")
			return
		}
		// Check for duplicate name error
		if err.Error() == "duplicate name" {
			WriteConflict(w, "An application with this name already exists in this organization")
			return
		}
		h.logger.Error("failed to update app", "error", err, "app_id", appID)
		WriteInternalError(w, "Failed to update application")
		return
	}

	h.logger.Info("application updated", "app_id", appID, "name", app.Name, "owner_id", userID)
	WriteJSON(w, http.StatusOK, app)
}

// Delete handles DELETE /v1/apps/:appID - soft-deletes an application.
// Requirements: 11.1, 11.2, 11.3, 11.4 - Safe deletion with deployment cleanup.
func (h *AppHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// Use resolved app ID from middleware (handles both UUID and name lookup)
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		// Fallback to URL param if middleware didn't resolve it
		appID = chi.URLParam(r, "appID")
	}
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	// Verify app exists
	app, err := h.store.Apps().Get(r.Context(), appID)
	if err != nil {
		WriteNotFound(w, "Application not found")
		return
	}

	// Use transaction for atomicity (Requirements: 11.4)
	err = h.store.WithTx(r.Context(), func(txStore store.Store) error {
		// Get all deployments for this app
		deployments, err := txStore.Deployments().List(r.Context(), appID)
		if err != nil {
			return err
		}

		// Stop running deployments and cancel pending/building ones (Requirements: 11.1, 11.2)
		for _, deployment := range deployments {
			switch deployment.Status {
			case models.DeploymentStatusRunning:
				// Stop running deployments (Requirements: 11.1)
				deployment.Status = models.DeploymentStatusStopped
				if err := txStore.Deployments().Update(r.Context(), deployment); err != nil {
					h.logger.Error("failed to stop deployment", "error", err, "deployment_id", deployment.ID)
					return err
				}
				h.logger.Info("stopped deployment for app deletion", "deployment_id", deployment.ID, "app_id", appID)

			case models.DeploymentStatusPending, models.DeploymentStatusBuilding:
				// Cancel pending/building deployments (Requirements: 11.2)
				deployment.Status = models.DeploymentStatusFailed
				if err := txStore.Deployments().Update(r.Context(), deployment); err != nil {
					h.logger.Error("failed to cancel deployment", "error", err, "deployment_id", deployment.ID)
					return err
				}
				h.logger.Info("cancelled deployment for app deletion", "deployment_id", deployment.ID, "app_id", appID)
			}
		}

		// Mark secrets for cleanup (Requirements: 11.3)
		// Get all secret keys for this app and delete them
		secretKeys, err := txStore.Secrets().List(r.Context(), appID)
		if err != nil {
			h.logger.Debug("failed to list secrets for cleanup", "error", err, "app_id", appID)
			// Continue even if secrets listing fails - not critical
		} else {
			for _, key := range secretKeys {
				if err := txStore.Secrets().Delete(r.Context(), appID, key); err != nil {
					h.logger.Debug("failed to delete secret", "error", err, "app_id", appID, "key", key)
					// Continue even if individual secret deletion fails
				}
			}
			h.logger.Info("marked secrets for cleanup", "app_id", appID, "count", len(secretKeys))
		}

		// Soft delete the app
		if err := txStore.Apps().Delete(r.Context(), appID); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		h.logger.Error("failed to delete app", "error", err, "app_id", appID)
		WriteInternalError(w, "Failed to delete application")
		return
	}

	h.logger.Info("application deleted", "app_id", appID, "name", app.Name)
	w.WriteHeader(http.StatusNoContent)
}
