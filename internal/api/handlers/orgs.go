package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// OrgHandler handles organization-related HTTP requests.
type OrgHandler struct {
	store  store.Store
	logger *slog.Logger
}

// NewOrgHandler creates a new organization handler.
func NewOrgHandler(st store.Store, logger *slog.Logger) *OrgHandler {
	return &OrgHandler{
		store:  st,
		logger: logger,
	}
}

// CreateOrgRequest represents the request body for creating an organization.
type CreateOrgRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
}

// Create handles POST /v1/orgs - creates a new organization.
func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "unauthorized")
		return
	}

	var req CreateOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "name is required")
		return
	}

	org := &models.Organization{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		IconURL:     req.IconURL,
	}

	// Generate slug from name if not provided
	if org.Slug == "" {
		org.Slug = models.GenerateSlug(org.Name)
	}

	if err := h.store.Orgs().Create(r.Context(), org); err != nil {
		h.logger.Error("failed to create organization", "error", err)
		WriteInternalError(w, "failed to create organization")
		return
	}

	// Add the creator as an owner
	if err := h.store.Orgs().AddMember(r.Context(), org.ID, userID, models.RoleOwner); err != nil {
		h.logger.Error("failed to add creator as owner", "error", err)
		// Don't fail the request, org was created
	}

	WriteJSON(w, http.StatusCreated, org)
}

// List handles GET /v1/orgs - lists all organizations for the current user.
func (h *OrgHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "unauthorized")
		return
	}

	orgs, err := h.store.Orgs().List(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to list organizations", "error", err)
		WriteInternalError(w, "failed to list organizations")
		return
	}

	if orgs == nil {
		orgs = []*models.Organization{}
	}

	WriteJSON(w, http.StatusOK, orgs)
}

// Get handles GET /v1/orgs/{orgID} - gets an organization by ID.
func (h *OrgHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if orgID == "" {
		WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "organization ID is required")
		return
	}

	org, err := h.store.Orgs().Get(r.Context(), orgID)
	if err != nil {
		h.logger.Error("failed to get organization", "error", err, "org_id", orgID)
		WriteError(w, http.StatusNotFound, ErrCodeNotFound, "organization not found")
		return
	}

	WriteJSON(w, http.StatusOK, org)
}

// GetBySlug handles GET /v1/orgs/slug/{slug} - gets an organization by slug.
func (h *OrgHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "slug is required")
		return
	}

	org, err := h.store.Orgs().GetBySlug(r.Context(), slug)
	if err != nil {
		h.logger.Error("failed to get organization by slug", "error", err, "slug", slug)
		WriteError(w, http.StatusNotFound, ErrCodeNotFound, "organization not found")
		return
	}

	WriteJSON(w, http.StatusOK, org)
}

// UpdateOrgRequest represents the request body for updating an organization.
type UpdateOrgRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
}

// Update handles PATCH /v1/orgs/{orgID} - updates an organization.
func (h *OrgHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if orgID == "" {
		WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "organization ID is required")
		return
	}

	var req UpdateOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")
		return
	}

	org, err := h.store.Orgs().Get(r.Context(), orgID)
	if err != nil {
		WriteError(w, http.StatusNotFound, ErrCodeNotFound, "organization not found")
		return
	}

	if req.Name != "" {
		org.Name = req.Name
	}
	if req.Slug != "" {
		org.Slug = req.Slug
	}
	if req.Description != "" {
		org.Description = req.Description
	}
	if req.IconURL != "" {
		org.IconURL = req.IconURL
	}

	if err := h.store.Orgs().Update(r.Context(), org); err != nil {
		h.logger.Error("failed to update organization", "error", err)
		WriteInternalError(w, "failed to update organization")
		return
	}

	WriteJSON(w, http.StatusOK, org)
}

// Delete handles DELETE /v1/orgs/{orgID} - deletes an organization.
func (h *OrgHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if orgID == "" {
		WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "organization ID is required")
		return
	}

	if err := h.store.Orgs().Delete(r.Context(), orgID); err != nil {
		if err == models.ErrLastOrgDelete {
			WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "cannot delete the last organization")
			return
		}
		h.logger.Error("failed to delete organization", "error", err)
		WriteInternalError(w, "failed to delete organization")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
