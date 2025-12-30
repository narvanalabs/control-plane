package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/narvanalabs/control-plane/internal/store"
)

// SettingsHandler handles global settings endpoints.
type SettingsHandler struct {
	store  store.Store
	logger *slog.Logger
}

// NewSettingsHandler creates a new settings handler.
func NewSettingsHandler(st store.Store, logger *slog.Logger) *SettingsHandler {
	return &SettingsHandler{
		store:  st,
		logger: logger,
	}
}

// Get retrieves all global settings.
func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.Settings().GetAll(r.Context())
	if err != nil {
		h.logger.Error("failed to get settings", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	WriteJSON(w, http.StatusOK, settings)
}

// Update updates global settings.
func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	ctx := r.Context()
	for key, value := range req {
		if err := h.store.Settings().Set(ctx, key, value); err != nil {
			h.logger.Error("failed to set setting", "key", key, "error", err)
			WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
			return
		}
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "success"})
}
