package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
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

	// Check if systemd services exist (production mode)
	// Try to check if narvana-api service exists
	checkCmd := exec.Command("systemctl", "is-active", "narvana-api")
	if err := checkCmd.Run(); err != nil {
		// Systemd services not available, use dev mode
		h.streamDevLogs(w, r, service, lines, level)
		return
	}

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

// streamDevLogs streams logs in development mode using journalctl for current user session
func (h *ServerLogsHandler) streamDevLogs(w http.ResponseWriter, r *http.Request, service, lines, level string) {
	h.sendEvent(w, "connected", map[string]string{"status": "streaming (dev mode)"})

	// Send initial message
	h.sendEvent(w, "log", map[string]interface{}{
		"timestamp": time.Now(),
		"level":     "info",
		"message":   "[dev] Server log streaming started",
	})

	// In dev mode, try to tail journalctl for the current user session
	// This captures logs from processes started in the current session
	journalArgs := []string{"-f", "--output", "short-iso", "--user"}

	if lines != "" {
		journalArgs = append(journalArgs, "-n", lines)
	} else {
		journalArgs = append(journalArgs, "-n", "100")
	}

	cmd := exec.CommandContext(r.Context(), "journalctl", journalArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		h.logger.Error("failed to get stdout pipe for journalctl", "error", err)
		// Fallback to heartbeat mode
		h.streamHeartbeat(w, r)
		return
	}

	if err := cmd.Start(); err != nil {
		h.logger.Error("failed to start journalctl --user", "error", err)
		// Fallback to heartbeat mode
		h.streamHeartbeat(w, r)
		return
	}

	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		logEntry := map[string]interface{}{
			"timestamp": time.Now(),
			"level":     "info",
			"message":   line,
		}
		h.sendEvent(w, "log", logEntry)
	}
}

// streamHeartbeat keeps the connection alive when no log source is available
func (h *ServerLogsHandler) streamHeartbeat(w http.ResponseWriter, r *http.Request) {
	h.sendEvent(w, "log", map[string]interface{}{
		"timestamp": time.Now(),
		"level":     "info",
		"message":   "[dev] No log source available. Logs will appear when services are running as systemd units.",
	})

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			h.sendEvent(w, "log", map[string]interface{}{
				"timestamp": time.Now(),
				"level":     "debug",
				"message":   "[heartbeat] Connection alive",
			})
		}
	}
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

// Restart handles POST /v1/server/restart - restarts systemd services.
func (h *ServerLogsHandler) Restart(w http.ResponseWriter, r *http.Request) {
	services := []string{"narvana-api", "narvana-web", "narvana-worker"}

	for _, svc := range services {
		cmd := exec.Command("sudo", "systemctl", "restart", svc)
		if err := cmd.Run(); err != nil {
			h.logger.Error("failed to restart service", "service", svc, "error", err)
			http.Error(w, fmt.Sprintf("failed to restart %s", svc), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "restarted"})
}

// TerminalWS handles GET /v1/server/console/ws - WebSocket terminal bridge.
func (h *ServerLogsHandler) TerminalWS(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("terminal websocket request received")
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("failed to upgrade websocket", "error", err)
		return
	}
	defer conn.Close()

	// Spawn shell with PTY
	h.logger.Info("starting bash pty")
	c := exec.Command("bash", "--norc") // Use --norc to avoid local bashrc pollution
	c.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"PS1=\\u@\\h:\\w\\$ ",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	)

	f, err := pty.Start(c)
	if err != nil {
		h.logger.Error("failed to start pty", "error", err)
		return
	}
	h.logger.Info("pty started successfully")
	defer f.Close()

	// Set initial size
	_ = pty.Setsize(f, &pty.Winsize{Rows: 24, Cols: 80})

	// Clean up process on exit
	defer func() {
		c.Process.Kill()
	}()

	// Copy PTY output to WebSocket
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := f.Read(buf)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	// Handle WebSocket input (both data and control messages)
	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		if mt == websocket.TextMessage {
			// Check for control messages (e.g., resize, terminate)
			var ctrl struct {
				Type string `json:"type"`
				Rows uint16 `json:"rows"`
				Cols uint16 `json:"cols"`
			}
			if err := json.Unmarshal(msg, &ctrl); err == nil {
				if ctrl.Type == "resize" {
					_ = pty.Setsize(f, &pty.Winsize{Rows: ctrl.Rows, Cols: ctrl.Cols})
					continue
				}
				if ctrl.Type == "terminate" {
					h.logger.Info("terminal termination requested")
					c.Process.Kill()
					return
				}
			}
		}

		// Otherwise treat as raw terminal input
		if mt == websocket.BinaryMessage || mt == websocket.TextMessage {
			f.Write(msg)
		}
	}
}

func (h *ServerLogsHandler) sendEvent(w http.ResponseWriter, event string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		h.logger.Error("failed to marshal event data", "error", err)
		return
	}

	// Use default message event format for compatibility with EventSource.onmessage
	fmt.Fprintf(w, "data: %s\n\n", jsonData)

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
