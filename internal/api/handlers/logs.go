package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// LogHandler handles log-related HTTP requests.
type LogHandler struct {
	store  store.Store
	logger *slog.Logger
}

// NewLogHandler creates a new log handler.
func NewLogHandler(st store.Store, logger *slog.Logger) *LogHandler {
	return &LogHandler{
		store:  st,
		logger: logger,
	}
}

// Get handles GET /v1/apps/:appID/logs - retrieves logs for the most recent deployment.
func (h *LogHandler) Get(w http.ResponseWriter, r *http.Request) {
	// Use resolved app ID from middleware (handles both UUID and name lookup)
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	// Parse query parameters
	limit := 100 // Default limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	source := r.URL.Query().Get("source") // "build" or "runtime"
	deploymentID := r.URL.Query().Get("deployment_id")

	// If no deployment ID specified, get the most recent deployment
	if deploymentID == "" {
		deployments, err := h.store.Deployments().List(r.Context(), appID)
		if err != nil {
			h.logger.Error("failed to list deployments", "error", err, "app_id", appID)
			WriteInternalError(w, "Failed to retrieve logs")
			return
		}
		if len(deployments) == 0 {
			WriteJSON(w, http.StatusOK, map[string][]*models.LogEntry{"logs": {}})
			return
		}
		// Deployments are ordered by created_at DESC, so first is most recent
		deploymentID = deployments[0].ID
	}

	// Get logs
	var logs []*models.LogEntry
	var err error

	if source != "" {
		logs, err = h.store.Logs().ListBySource(r.Context(), deploymentID, source, limit)
	} else {
		logs, err = h.store.Logs().List(r.Context(), deploymentID, limit)
	}

	if err != nil {
		h.logger.Error("failed to get logs", "error", err, "deployment_id", deploymentID)
		WriteInternalError(w, "Failed to retrieve logs")
		return
	}

	if logs == nil {
		logs = []*models.LogEntry{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"deployment_id": deploymentID,
		"logs":          logs,
	})
}
