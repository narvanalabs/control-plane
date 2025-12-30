package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"time"
)

// ServerLogsHandler handles real-time server log streaming via SSE.
type ServerLogsHandler struct {
	logger *slog.Logger
}

// NewServerLogsHandler creates a new server logs handler.
func NewServerLogsHandler(logger *slog.Logger) *ServerLogsHandler {
	return &ServerLogsHandler{
		logger: logger,
	}
}

// Stream handles GET /v1/server/logs/stream - streams system logs in real-time.
func (h *ServerLogsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no")

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	h.logger.Info("server log stream started")

	// Parse parameters
	service := r.URL.Query().Get("service")
	lines := r.URL.Query().Get("lines")
	since := r.URL.Query().Get("since")
	level := r.URL.Query().Get("level")

	journalArgs := []string{"-f", "--output", "short-iso"}

	if lines != "" {
		journalArgs = append(journalArgs, "-n", lines)
	} else {
		journalArgs = append(journalArgs, "-n", "100")
	}

	if since != "" {
		journalArgs = append(journalArgs, "-S", since)
	}

	if level != "" && level != "all" {
		priority := level
		if level == "error" {
			priority = "err"
		}
		journalArgs = append(journalArgs, "-p", priority)
	}

	switch service {
	case "api":
		journalArgs = append(journalArgs, "-u", "narvana-api")
	case "web":
		journalArgs = append(journalArgs, "-u", "narvana-web")
	case "worker":
		journalArgs = append(journalArgs, "-u", "narvana-worker")
	default:
		journalArgs = append(journalArgs, "-u", "narvana-api", "-u", "narvana-web", "-u", "narvana-worker")
	}

	cmd := exec.CommandContext(r.Context(), "journalctl", journalArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		h.logger.Error("failed to get stdout pipe", "error", err)
		return
	}

	if err := cmd.Start(); err != nil {
		h.logger.Error("failed to start journalctl", "error", err)
		return
	}

	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	h.sendEvent(w, "connected", map[string]string{"status": "streaming"})

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		logEntry := map[string]interface{}{
			"timestamp": time.Now(),
			"level":     "info", // Default for now
			"message":   line,
		}
		h.sendEvent(w, "log", logEntry)
	}

	if err := scanner.Err(); err != nil {
		h.logger.Error("scanner error", "error", err)
	}

	h.logger.Info("server log stream ended")
}

// Download handles GET /v1/server/logs/download - downloads logs as a file.
func (h *ServerLogsHandler) Download(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	level := r.URL.Query().Get("level")
	since := r.URL.Query().Get("since")

	journalArgs := []string{"--output", "short-iso", "--no-pager"}

	if since != "" {
		journalArgs = append(journalArgs, "-S", since)
	} else {
		// Default to last 24h for download if not specified
		journalArgs = append(journalArgs, "-S", "-24h")
	}

	if level != "" && level != "all" {
		priority := level
		if level == "error" {
			priority = "err"
		}
		journalArgs = append(journalArgs, "-p", priority)
	}

	switch service {
	case "api":
		journalArgs = append(journalArgs, "-u", "narvana-api")
	case "web":
		journalArgs = append(journalArgs, "-u", "narvana-web")
	case "worker":
		journalArgs = append(journalArgs, "-u", "narvana-worker")
	default:
		journalArgs = append(journalArgs, "-u", "narvana-api", "-u", "narvana-web", "-u", "narvana-worker")
	}

	filename := fmt.Sprintf("server_logs_%s.txt", time.Now().Format("20060102_150405"))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("Content-Type", "text/plain")

	cmd := exec.CommandContext(r.Context(), "journalctl", journalArgs...)
	cmd.Stdout = w
	cmd.Stderr = w

	if err := cmd.Run(); err != nil {
		h.logger.Error("failed to download logs", "error", err)
	}
}

func (h *ServerLogsHandler) sendEvent(w http.ResponseWriter, event string, data interface{}) {
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
