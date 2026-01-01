package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/secrets"
	"github.com/narvanalabs/control-plane/internal/store"
)

// SecretHandler handles secret-related HTTP requests.
type SecretHandler struct {
	store       store.Store
	sopsService *secrets.SOPSService
	logger      *slog.Logger
}

// NewSecretHandler creates a new secret handler.
func NewSecretHandler(st store.Store, sopsService *secrets.SOPSService, logger *slog.Logger) *SecretHandler {
	return &SecretHandler{
		store:       st,
		sopsService: sopsService,
		logger:      logger,
	}
}

// CreateSecretRequest represents the request body for creating a secret.
type CreateSecretRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Validate validates the create secret request.
func (r *CreateSecretRequest) Validate() error {
	if r.Key == "" {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "key is required"}
	}
	if r.Value == "" {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "value is required"}
	}
	// Validate key format (alphanumeric and underscores only)
	for _, c := range r.Key {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return &APIError{Code: ErrCodeInvalidRequest, Message: "key must contain only alphanumeric characters and underscores"}
		}
	}
	return nil
}

// Create handles POST /v1/apps/:appID/secrets - creates or updates a secret.
func (h *SecretHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Use resolved app ID from middleware (handles both UUID and name lookup)
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	var req CreateSecretRequest
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

	// Encrypt the value using SOPS
	var encryptedValue []byte
	var err error
	if h.sopsService != nil && h.sopsService.CanEncrypt() {
		encryptedValue, err = h.sopsService.Encrypt(r.Context(), []byte(req.Value))
		if err != nil {
			h.logger.Error("failed to encrypt secret with SOPS", "error", err)
			WriteInternalError(w, "Failed to encrypt secret")
			return
		}
	} else {
		// Fallback: store plaintext if SOPS is not configured (for testing/development)
		h.logger.Warn("SOPS not configured, storing secret without encryption")
		encryptedValue = []byte(req.Value)
	}

	// Store the secret
	if err := h.store.Secrets().Set(r.Context(), appID, strings.ToUpper(req.Key), encryptedValue); err != nil {
		h.logger.Error("failed to store secret", "error", err)
		WriteInternalError(w, "Failed to store secret")
		return
	}

	h.logger.Info("secret created", "app_id", appID, "key", req.Key)
	WriteJSON(w, http.StatusCreated, map[string]string{
		"key":    strings.ToUpper(req.Key),
		"status": "created",
	})
}

// List handles GET /v1/apps/:appID/secrets - lists secret keys for an app.
func (h *SecretHandler) List(w http.ResponseWriter, r *http.Request) {
	// Use resolved app ID from middleware (handles both UUID and name lookup)
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	keys, err := h.store.Secrets().List(r.Context(), appID)
	if err != nil {
		h.logger.Error("failed to list secrets", "error", err, "app_id", appID)
		WriteInternalError(w, "Failed to list secrets")
		return
	}

	if keys == nil {
		keys = []string{}
	}

	WriteJSON(w, http.StatusOK, map[string][]string{"keys": keys})
}

// Delete handles DELETE /v1/apps/:appID/secrets/:key - deletes a secret.
func (h *SecretHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// Use resolved app ID from middleware (handles both UUID and name lookup)
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	key := chi.URLParam(r, "key")

	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}
	if key == "" {
		WriteBadRequest(w, "Secret key is required")
		return
	}

	if err := h.store.Secrets().Delete(r.Context(), appID, strings.ToUpper(key)); err != nil {
		h.logger.Error("failed to delete secret", "error", err, "app_id", appID, "key", key)
		WriteInternalError(w, "Failed to delete secret")
		return
	}

	h.logger.Info("secret deleted", "app_id", appID, "key", key)
	w.WriteHeader(http.StatusNoContent)
}
