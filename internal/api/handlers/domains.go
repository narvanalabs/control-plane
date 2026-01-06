package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
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

// ValidateDomain validates a domain string, supporting both standard and wildcard domains.
// Standard domain: example.com, sub.example.com
// Wildcard domain: *.example.com
func ValidateDomain(domain string) bool {
	domainRegex := regexp.MustCompile(`^(\*\.)?(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	return domainRegex.MatchString(domain)
}

// IsWildcardDomain checks if a domain is a wildcard domain (starts with *.)
func IsWildcardDomain(domain string) bool {
	return strings.HasPrefix(domain, "*.")
}

// Validate validates the create domain request.
func (r *CreateDomainRequest) Validate() error {
	if r.Service == "" {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "service is required"}
	}
	if r.Domain == "" {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "domain is required"}
	}

	if !ValidateDomain(r.Domain) {
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
		AppID:      appID,
		Service:    req.Service,
		Domain:     req.Domain,
		IsWildcard: IsWildcardDomain(req.Domain),
	}

	if err := h.store.Domains().Create(r.Context(), domain); err != nil {
		h.logger.Error("failed to create domain", "error", err)
		WriteInternalError(w, "Failed to create domain")
		return
	}

	h.logger.Info("domain created", "app_id", appID, "domain", req.Domain, "is_wildcard", domain.IsWildcard)
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
	domainID := chi.URLParam(r, "domainID")
	if domainID == "" {
		WriteBadRequest(w, "Domain ID is required")
		return
	}

	if err := h.store.Domains().Delete(r.Context(), domainID); err != nil {
		h.logger.Error("failed to delete domain", "error", err, "domain_id", domainID)
		WriteInternalError(w, "Failed to delete domain")
		return
	}

	h.logger.Info("domain deleted", "domain_id", domainID)
	w.WriteHeader(http.StatusNoContent)
}

// ListAll handles GET /v1/domains - lists all domains across all applications.
func (h *DomainHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	domains, err := h.store.Domains().ListAll(r.Context())
	if err != nil {
		h.logger.Error("failed to list all domains", "error", err)
		WriteInternalError(w, "Failed to list domains")
		return
	}

	if domains == nil {
		domains = []*models.Domain{}
	}

	WriteJSON(w, http.StatusOK, domains)
}

// CreateGlobal handles POST /v1/domains - creates a domain with app_id in body.
func (h *DomainHandler) CreateGlobal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AppID      string `json:"app_id"`
		Service    string `json:"service"`
		Domain     string `json:"domain"`
		IsWildcard bool   `json:"is_wildcard"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	if req.AppID == "" {
		WriteBadRequest(w, "app_id is required")
		return
	}
	if req.Service == "" {
		WriteBadRequest(w, "service is required")
		return
	}
	if req.Domain == "" {
		WriteBadRequest(w, "domain is required")
		return
	}

	// Enforce lowercase
	req.Domain = strings.ToLower(req.Domain)

	// Validate domain format
	if !ValidateDomain(req.Domain) {
		WriteBadRequest(w, "invalid domain format")
		return
	}

	// Auto-detect wildcard if not explicitly set
	isWildcard := req.IsWildcard || IsWildcardDomain(req.Domain)

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

	// Verify app exists
	app, err := h.store.Apps().Get(r.Context(), req.AppID)
	if err != nil {
		h.logger.Error("failed to get app", "error", err)
		WriteInternalError(w, "Failed to verify application")
		return
	}
	if app == nil {
		WriteNotFound(w, "Application not found")
		return
	}

	// Verify service exists in app
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
		AppID:      req.AppID,
		Service:    req.Service,
		Domain:     req.Domain,
		IsWildcard: isWildcard,
		Verified:   false,
	}

	if err := h.store.Domains().Create(r.Context(), domain); err != nil {
		h.logger.Error("failed to create domain", "error", err)
		WriteInternalError(w, "Failed to create domain")
		return
	}

	h.logger.Info("domain created", "app_id", req.AppID, "domain", req.Domain, "is_wildcard", isWildcard)
	WriteJSON(w, http.StatusCreated, domain)
}

// DeleteGlobal handles DELETE /v1/domains/:domainID - removes a domain by ID.
func (h *DomainHandler) DeleteGlobal(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "domainID")
	if domainID == "" {
		WriteBadRequest(w, "Domain ID is required")
		return
	}

	if err := h.store.Domains().Delete(r.Context(), domainID); err != nil {
		h.logger.Error("failed to delete domain", "error", err, "domain_id", domainID)
		WriteInternalError(w, "Failed to delete domain")
		return
	}

	h.logger.Info("domain deleted", "domain_id", domainID)
	w.WriteHeader(http.StatusNoContent)
}
