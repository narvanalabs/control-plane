package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
	"log/slog"
)

// DomainHandler handles domain-related HTTP requests.
type DomainHandler struct {
	store  store.Store
	logger *slog.Logger
}

// NewDomainHandler creates a new domain handler.
func NewDomainHandler(st store.Store, logger *slog.Logger) *DomainHandler {
	return &DomainHandler{
		store:  st,
		logger: logger,
	}
}

// CreateDomainRequest represents the request body for adding a domain.
type CreateDomainRequest struct {
	Service string `json:"service"`
	Domain  string `json:"domain"`
}

// Validate validates the create domain request.
func (r *CreateDomainRequest) Validate() error {
	if r.Service == "" {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "service is required"}
	}
	if r.Domain == "" {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "domain is required"}
	}
	
	// Basic domain validation regex
	domainRegex := regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	if !domainRegex.MatchString(r.Domain) {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "invalid domain format"}
	}
	
	// Enforce lowercase
	r.Domain = strings.ToLower(r.Domain)
	return nil
}

// Create handles POST /v1/apps/:appID/domains - adds a custom domain.
func (h *DomainHandler) Create(w http.ResponseWriter, r *http.Request) {
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	var req CreateDomainRequest
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

	// Check if domain is already in use
	existing, err := h.store.Domains().GetByDomain(r.Context(), req.Domain)
	if err != nil {
		h.logger.Error("failed to check existing domain", "error", err)
		WriteInternalError(w, "Failed to check domain availability")
		return
	}
	if existing != nil {
		WriteError(w, http.StatusConflict, ErrCodeConflict, "Domain is already in use")
		return
	}

	// Verify service exists in app
	app, err := h.store.Apps().Get(r.Context(), appID)
	if err != nil {
		h.logger.Error("failed to get app", "error", err)
		WriteInternalError(w, "Failed to verify application")
		return
	}
	if app == nil {
		WriteNotFound(w, "Application not found")
		return
	}
	
	serviceFound := false
	for _, svc := range app.Services {
		if svc.Name == req.Service {
			serviceFound = true
			break
		}
	}
	if !serviceFound {
		WriteBadRequest(w, "Service not found in application")
		return
	}

	domain := &models.Domain{
		AppID:   appID,
		Service: req.Service,
		Domain:  req.Domain,
	}

	if err := h.store.Domains().Create(r.Context(), domain); err != nil {
		h.logger.Error("failed to create domain", "error", err)
		WriteInternalError(w, "Failed to create domain")
		return
	}

	h.logger.Info("domain created", "app_id", appID, "domain", req.Domain)
	WriteJSON(w, http.StatusCreated, domain)
}

// List handles GET /v1/apps/:appID/domains - lists domains for an app.
func (h *DomainHandler) List(w http.ResponseWriter, r *http.Request) {
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	domains, err := h.store.Domains().List(r.Context(), appID)
	if err != nil {
		h.logger.Error("failed to list domains", "error", err, "app_id", appID)
		WriteInternalError(w, "Failed to list domains")
		return
	}

	if domains == nil {
		domains = []*models.Domain{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{"domains": domains})
}

// Delete handles DELETE /v1/apps/:appID/domains/:domainID - removes a domain.
func (h *DomainHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// We might receive domain ID or the domain name itself as param?
	// Standard CRUD usually uses ID. Let's assume ID for now.
	// But the UI might want to delete by name.
	// Let's implement delete by ID as it is safer.
	
	domainID := chi.URLParam(r, "domainID")
	if domainID == "" {
		WriteBadRequest(w, "Domain ID is required")
		return
	}

	if err := h.store.Domains().Delete(r.Context(), domainID); err != nil {
		h.logger.Error("failed to delete domain", "error", err, "domain_id", domainID)
		// Check if not found error? store typically returns error if not found
		WriteInternalError(w, "Failed to delete domain")
		return
	}

	h.logger.Info("domain deleted", "domain_id", domainID)
	w.WriteHeader(http.StatusNoContent)
}
