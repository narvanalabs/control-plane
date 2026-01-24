// Package handlers provides HTTP handlers for the update system.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/narvanalabs/control-plane/internal/updater"
)

// UpdatesHandler handles update-related requests.
type UpdatesHandler struct {
	updater *updater.Service
	logger  *slog.Logger
}

// NewUpdatesHandler creates a new updates handler.
func NewUpdatesHandler(updaterService *updater.Service, logger *slog.Logger) *UpdatesHandler {
	return &UpdatesHandler{
		updater: updaterService,
		logger:  logger,
	}
}

// CheckForUpdates handles GET /v1/updates/check
func (h *UpdatesHandler) CheckForUpdates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	info, err := h.updater.CheckForUpdates(ctx)
	if err != nil {
		h.logger.Error("failed to check for updates", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to check for updates",
		})
		return
	}

	WriteJSON(w, http.StatusOK, info)
}

// TriggerUpdate handles POST /v1/updates/apply
func (h *UpdatesHandler) TriggerUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Version string `json:"version"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	if req.Version == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Version is required",
		})
		return
	}

	// Start the update process
	if err := h.updater.ApplyUpdate(ctx, req.Version); err != nil {
		h.logger.Error("failed to apply update", "error", err, "version", req.Version)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	h.logger.Info("update initiated successfully", "version", req.Version)
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Update initiated. Services will restart shortly.",
		"version": req.Version,
	})
}

