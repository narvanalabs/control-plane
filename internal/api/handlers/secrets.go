package handlers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/narvanalabs/control-plane/internal/store"
)

// SecretHandler handles secret-related HTTP requests.
type SecretHandler struct {
	store         store.Store
	encryptionKey []byte
	logger        *slog.Logger
}

// NewSecretHandler creates a new secret handler.
func NewSecretHandler(st store.Store, logger *slog.Logger) *SecretHandler {
	return &SecretHandler{
		store:  st,
		logger: logger,
	}
}

// SetEncryptionKey sets the encryption key for secrets.
func (h *SecretHandler) SetEncryptionKey(key []byte) {
	h.encryptionKey = key
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
	appID := chi.URLParam(r, "appID")
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

	// Encrypt the value
	encryptedValue, err := h.encrypt([]byte(req.Value))
	if err != nil {
		h.logger.Error("failed to encrypt secret", "error", err)
		WriteInternalError(w, "Failed to encrypt secret")
		return
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
	appID := chi.URLParam(r, "appID")
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
	appID := chi.URLParam(r, "appID")
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

// encrypt encrypts data using AES-GCM.
func (h *SecretHandler) encrypt(plaintext []byte) ([]byte, error) {
	if len(h.encryptionKey) == 0 {
		// If no encryption key is set, return plaintext (for testing)
		return plaintext, nil
	}

	block, err := aes.NewCipher(h.encryptionKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}
