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
	"github.com/narvanalabs/control-plane/internal/logs"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// LogStreamHandler handles real-time log streaming via Server-Sent Events.
type LogStreamHandler struct {
	store  store.Store
	broker *logs.Broker
	logger *slog.Logger
}

// NewLogStreamHandler creates a new log stream handler.
func NewLogStreamHandler(st store.Store, logger *slog.Logger) *LogStreamHandler {
	return &LogStreamHandler{
		store:  st,
		broker: logs.NewBroker(logger),
		logger: logger,
	}
}

// NewLogStreamHandlerWithBroker creates a new log stream handler with a shared broker.
func NewLogStreamHandlerWithBroker(st store.Store, broker *logs.Broker, logger *slog.Logger) *LogStreamHandler {
	return &LogStreamHandler{
		store:  st,
		broker: broker,
		logger: logger,
	}
}

// Broker returns the log broker for publishing logs.
func (h *LogStreamHandler) Broker() *logs.Broker {
	return h.broker
}

// Stream handles GET /v1/apps/:appID/logs/stream - streams logs in real-time via SSE.
// Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6
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
	serviceName := r.URL.Query().Get("service_name")

	// Set SSE headers - Requirements: 8.1
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Ensure we can flush
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Flush headers immediately
	flusher.Flush()

	h.logger.Info("log stream started",
		"app_id", appID,
		"deployment_id", requestedDeploymentID,
		"source", source,
	)

	// Track deployment IDs we're watching
	currentDeploymentID := requestedDeploymentID

	// If no deployment ID, get the most recent one
	if currentDeploymentID == "" {
		currentDeploymentID = h.findLatestDeployment(r.Context(), appID, serviceName)
	}

	// Send initial connection event
	h.sendEvent(w, flusher, "connected", map[string]string{
		"app_id":        appID,
		"deployment_id": currentDeploymentID,
	})

	// Subscribe to the log broker - Requirements: 8.2
	subscriber := h.broker.Subscribe(r.Context(), appID, currentDeploymentID, source)
	defer h.broker.Unsubscribe(subscriber)

	// Track last timestamps for polling fallback
	lastTimestamps := make(map[string]time.Time)

	// Polling ticker for database fallback (when broker has no new logs)
	pollTicker := time.NewTicker(500 * time.Millisecond)
	defer pollTicker.Stop()

	// Ping ticker to keep connection alive
	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	// Deployment check ticker
	deploymentTicker := time.NewTicker(2 * time.Second)
	defer deploymentTicker.Stop()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("log stream closed by client", "app_id", appID)
			return

		case entry := <-subscriber.Ch:
			// Received log from broker - Requirements: 8.2
			if entry != nil {
				h.sendEvent(w, flusher, "log", entry)
				if entry.Timestamp.After(lastTimestamps[entry.DeploymentID]) {
					lastTimestamps[entry.DeploymentID] = entry.Timestamp
				}
			}

		case <-pingTicker.C:
			// Send ping to keep connection alive
			h.sendEvent(w, flusher, "ping", map[string]int64{"time": time.Now().Unix()})

		case <-deploymentTicker.C:
			// Check for new deployments if no specific deployment requested
			if requestedDeploymentID == "" {
				latestID := h.findLatestDeployment(ctx, appID, serviceName)
				if latestID != "" && latestID != currentDeploymentID {
					// New deployment started!
					currentDeploymentID = latestID
					h.sendEvent(w, flusher, "new_deployment", map[string]string{
						"deployment_id": currentDeploymentID,
					})

					// Update subscriber's deployment filter
					h.broker.Unsubscribe(subscriber)
					subscriber = h.broker.Subscribe(ctx, appID, currentDeploymentID, source)
				}
			}

			// Check deployment status
			if currentDeploymentID != "" {
				deployment, err := h.store.Deployments().Get(ctx, currentDeploymentID)
				if err == nil && isTerminalStatus(deployment.Status) {
					// Only send complete event once per deployment
					completeKey := currentDeploymentID + "_complete"
					if _, alreadySent := lastTimestamps[completeKey]; !alreadySent {
						h.sendEvent(w, flusher, "deployment_status", map[string]string{
							"deployment_id": currentDeploymentID,
							"status":        string(deployment.Status),
						})
						lastTimestamps[completeKey] = time.Now()
					}
				}
			}

		case <-pollTicker.C:
			// Fallback: poll database for new logs
			if currentDeploymentID == "" {
				continue
			}

			lastTs := lastTimestamps[currentDeploymentID]
			newLogs, err := h.fetchNewLogs(ctx, currentDeploymentID, source, lastTs)
			if err != nil {
				h.logger.Error("failed to fetch logs", "error", err, "deployment_id", currentDeploymentID)
				continue
			}

			// Send each new log entry in ASCENDING order (oldest first)
			for i := len(newLogs) - 1; i >= 0; i-- {
				log := newLogs[i]
				h.sendEvent(w, flusher, "log", log)
				if log.Timestamp.After(lastTs) {
					lastTs = log.Timestamp
				}
			}
			lastTimestamps[currentDeploymentID] = lastTs
		}
	}
}

// findLatestDeployment finds the latest deployment for an app/service.
func (h *LogStreamHandler) findLatestDeployment(ctx context.Context, appID, serviceName string) string {
	deployments, err := h.store.Deployments().List(ctx, appID)
	if err != nil || len(deployments) == 0 {
		return ""
	}

	if serviceName != "" {
		for _, d := range deployments {
			if d.ServiceName == serviceName {
				return d.ID
			}
		}
		return ""
	}

	return deployments[0].ID
}

// fetchNewLogs fetches logs newer than the given timestamp.
func (h *LogStreamHandler) fetchNewLogs(ctx context.Context, deploymentID, source string, since time.Time) ([]*models.LogEntry, error) {
	if deploymentID == "" {
		return nil, nil
	}

	var logEntries []*models.LogEntry
	var err error

	if source != "" {
		logEntries, err = h.store.Logs().ListBySource(ctx, deploymentID, source, 100)
	} else {
		logEntries, err = h.store.Logs().List(ctx, deploymentID, 100)
	}

	if err != nil {
		return nil, err
	}

	// Filter to only logs after the timestamp
	var newLogs []*models.LogEntry
	for _, log := range logEntries {
		if log.Timestamp.After(since) {
			newLogs = append(newLogs, log)
		}
	}

	return newLogs, nil
}

// sendEvent sends a Server-Sent Event with proper flushing.
func (h *LogStreamHandler) sendEvent(w http.ResponseWriter, flusher http.Flusher, event string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		h.logger.Error("failed to marshal event data", "error", err)
		return
	}

	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
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
