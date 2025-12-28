package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// LogStreamHandler handles real-time log streaming via Server-Sent Events.
type LogStreamHandler struct {
	store  store.Store
	logger *slog.Logger
}

// NewLogStreamHandler creates a new log stream handler.
func NewLogStreamHandler(st store.Store, logger *slog.Logger) *LogStreamHandler {
	return &LogStreamHandler{
		store:  st,
		logger: logger,
	}
}

// Stream handles GET /v1/apps/:appID/logs/stream - streams logs in real-time via SSE.
func (h *LogStreamHandler) Stream(w http.ResponseWriter, r *http.Request) {
	// Use resolved app ID from middleware
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}

	// Parse query parameters
	source := r.URL.Query().Get("source") // "build" or "runtime"
	requestedDeploymentID := r.URL.Query().Get("deployment_id")

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Flush headers immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	h.logger.Info("log stream started",
		"app_id", appID,
		"deployment_id", requestedDeploymentID,
		"source", source,
	)

	// Track deployment IDs we're watching
	currentDeploymentID := requestedDeploymentID
	
	// If no deployment ID, get the most recent one
	if currentDeploymentID == "" {
		deployments, err := h.store.Deployments().List(r.Context(), appID)
		if err == nil && len(deployments) > 0 {
			currentDeploymentID = deployments[0].ID
		}
	}

	// Send initial connection event
	h.sendEvent(w, "connected", map[string]string{
		"app_id":        appID,
		"deployment_id": currentDeploymentID,
	})

	// Track the last log timestamp we've seen per deployment
	lastTimestamps := make(map[string]time.Time)
	ticker := time.NewTicker(300 * time.Millisecond) // Poll every 300ms for responsiveness
	defer ticker.Stop()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("log stream closed by client", "app_id", appID)
			return
		case <-ticker.C:
			// If no specific deployment requested, check for new deployments
			if requestedDeploymentID == "" {
				deployments, err := h.store.Deployments().List(ctx, appID)
				if err == nil && len(deployments) > 0 {
					latestID := deployments[0].ID
					if latestID != currentDeploymentID {
						// New deployment started!
						currentDeploymentID = latestID
						h.sendEvent(w, "new_deployment", map[string]string{
							"deployment_id": currentDeploymentID,
						})
					}
				}
			}

			// Fetch new logs since last timestamp
			if currentDeploymentID == "" {
				continue
			}

			lastTs := lastTimestamps[currentDeploymentID]
			newLogs, err := h.fetchNewLogs(ctx, currentDeploymentID, source, lastTs)
			if err != nil {
				h.logger.Error("failed to fetch logs", "error", err, "deployment_id", currentDeploymentID)
				continue
			}

			// Send each new log entry
			for _, log := range newLogs {
				h.sendEvent(w, "log", log)
				if log.Timestamp.After(lastTs) {
					lastTs = log.Timestamp
				}
			}
			lastTimestamps[currentDeploymentID] = lastTs

			// Check deployment status and notify on completion (but don't close)
			deployment, err := h.store.Deployments().Get(ctx, currentDeploymentID)
			if err == nil && isTerminalStatus(deployment.Status) {
				// Only send complete event once per deployment
				if _, alreadySent := lastTimestamps[currentDeploymentID+"_complete"]; !alreadySent {
					h.sendEvent(w, "deployment_status", map[string]string{
						"deployment_id": currentDeploymentID,
						"status":        string(deployment.Status),
					})
					lastTimestamps[currentDeploymentID+"_complete"] = time.Now()
				}
			}
		}
	}
}

// fetchNewLogs fetches logs newer than the given timestamp.
func (h *LogStreamHandler) fetchNewLogs(ctx context.Context, deploymentID, source string, since time.Time) ([]*models.LogEntry, error) {
	if deploymentID == "" {
		return nil, nil
	}

	var logs []*models.LogEntry
	var err error

	if source != "" {
		logs, err = h.store.Logs().ListBySource(ctx, deploymentID, source, 100)
	} else {
		logs, err = h.store.Logs().List(ctx, deploymentID, 100)
	}

	if err != nil {
		return nil, err
	}

	// Filter to only logs after the timestamp
	var newLogs []*models.LogEntry
	for _, log := range logs {
		if log.Timestamp.After(since) {
			newLogs = append(newLogs, log)
		}
	}

	return newLogs, nil
}

// sendEvent sends a Server-Sent Event.
func (h *LogStreamHandler) sendEvent(w http.ResponseWriter, event string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		h.logger.Error("failed to marshal event data", "error", err)
		return
	}

	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// isTerminalStatus returns true if the deployment is in a terminal state.
func isTerminalStatus(status models.DeploymentStatus) bool {
	switch status {
	case models.DeploymentStatusRunning,
		models.DeploymentStatusStopped,
		models.DeploymentStatusFailed:
		return true
	default:
		return false
	}
}

