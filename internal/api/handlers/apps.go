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
func (h *AppHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

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

	// Check for duplicate name
	existing, err := h.store.Apps().GetByName(r.Context(), userID, req.Name)
	if err == nil && existing != nil {
		WriteConflict(w, "An application with this name already exists")
		return
	}

	now := time.Now()
	
	app := &models.App{
		ID:          uuid.New().String(),
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

	h.logger.Info("application created", "app_id", app.ID, "name", app.Name, "owner_id", userID)
	WriteJSON(w, http.StatusCreated, app)
}

// List handles GET /v1/apps - lists all applications for the authenticated user.
func (h *AppHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

	apps, err := h.store.Apps().List(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to list apps", "error", err, "owner_id", userID)
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
func (h *AppHandler) Get(w http.ResponseWriter, r *http.Request) {
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

	WriteJSON(w, http.StatusOK, app)
}

// Delete handles DELETE /v1/apps/:appID - soft-deletes an application.
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

	// Delete the app (soft delete)
	if err := h.store.Apps().Delete(r.Context(), appID); err != nil {
		h.logger.Error("failed to delete app", "error", err, "app_id", appID)
		WriteInternalError(w, "Failed to delete application")
		return
	}

	h.logger.Info("application deleted", "app_id", appID, "name", app.Name)
	w.WriteHeader(http.StatusNoContent)
}
