// Package terminal provides WebSocket-based terminal access to deployed service containers.
// **Validates: Requirements 23.1, 23.2**
package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/podman"
	"github.com/narvanalabs/control-plane/internal/store"
)

var (
	// ErrServiceNotRunning is returned when attempting to connect to a non-running service.
	ErrServiceNotRunning = errors.New("service is not running")
	// ErrContainerNotFound is returned when the container cannot be found.
	ErrContainerNotFound = errors.New("container not found")
	// ErrSessionClosed is returned when the terminal session is closed.
	ErrSessionClosed = errors.New("terminal session closed")
)

// ControlMessage represents a control message from the WebSocket client.
type ControlMessage struct {
	Type string `json:"type"` // "resize" or "terminate"
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
}

// Session represents an active terminal session.
type Session struct {
	ID           string
	DeploymentID string
	ContainerName string
	PTY          *os.File
	Conn         *websocket.Conn
	mu           sync.Mutex
	closed       bool
	closeCh      chan struct{}
}

// Close closes the terminal session and cleans up resources.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	close(s.closeCh)

	var errs []error
	if s.PTY != nil {
		if err := s.PTY.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing pty: %w", err))
		}
	}
	if s.Conn != nil {
		if err := s.Conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing websocket: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing session: %v", errs)
	}
	return nil
}

// IsClosed returns true if the session is closed.
func (s *Session) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// Service provides terminal access to deployed containers.
// **Validates: Requirements 23.1, 23.2**
type Service struct {
	store       store.Store
	podman      *podman.Client
	logger      *slog.Logger
	sessions    map[string]*Session
	sessionsMu  sync.RWMutex
	defaultShell string
}

// Config holds configuration for the terminal service.
type Config struct {
	// DefaultShell is the shell to use when connecting to containers.
	// Defaults to "/bin/sh" for BusyBox compatibility.
	// **Validates: Requirements 23.6**
	DefaultShell string
}

// DefaultConfig returns the default terminal service configuration.
func DefaultConfig() *Config {
	return &Config{
		DefaultShell: "/bin/sh", // BusyBox compatible
	}
}

// NewService creates a new terminal service.
func NewService(st store.Store, pd *podman.Client, cfg *Config, logger *slog.Logger) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.DefaultShell == "" {
		cfg.DefaultShell = "/bin/sh"
	}

	return &Service{
		store:        st,
		podman:       pd,
		logger:       logger,
		sessions:     make(map[string]*Session),
		defaultShell: cfg.DefaultShell,
	}
}


// Connect establishes a terminal session to a container.
// **Validates: Requirements 23.1, 23.2**
func (s *Service) Connect(ctx context.Context, appID, serviceName string, conn *websocket.Conn) (*Session, error) {
	s.logger.Info("connecting to service terminal",
		"app_id", appID,
		"service", serviceName,
	)

	// Get the app to verify it exists
	app, err := s.store.Apps().Get(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("getting app: %w", err)
	}

	// Find the service
	var service *models.ServiceConfig
	for i := range app.Services {
		if app.Services[i].Name == serviceName {
			service = &app.Services[i]
			break
		}
	}
	if service == nil {
		return nil, fmt.Errorf("service not found: %s", serviceName)
	}

	// Find the latest running deployment
	deployments, err := s.store.Deployments().List(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("listing deployments: %w", err)
	}

	var runningDeployment *models.Deployment
	for _, d := range deployments {
		if d.ServiceName == serviceName && d.Status == models.DeploymentStatusRunning {
			runningDeployment = d
			break
		}
	}

	if runningDeployment == nil {
		return nil, ErrServiceNotRunning
	}

	// Generate container name
	containerName := GenerateContainerName(appID, serviceName, runningDeployment.Version)

	// Create the terminal session
	session, err := s.createSession(ctx, containerName, conn)
	if err != nil {
		// Try alternative container name format
		containerName = fmt.Sprintf("%s-%s", appID, serviceName)
		session, err = s.createSession(ctx, containerName, conn)
		if err != nil {
			return nil, fmt.Errorf("creating session: %w", err)
		}
	}

	session.DeploymentID = runningDeployment.ID
	session.ContainerName = containerName

	// Store the session
	s.sessionsMu.Lock()
	s.sessions[session.ID] = session
	s.sessionsMu.Unlock()

	s.logger.Info("terminal session created",
		"session_id", session.ID,
		"container", containerName,
		"deployment_id", runningDeployment.ID,
	)

	return session, nil
}

// createSession creates a new terminal session for a container.
func (s *Service) createSession(ctx context.Context, containerName string, conn *websocket.Conn) (*Session, error) {
	// Create the exec command with PTY
	// **Validates: Requirements 23.6** - Using /bin/sh for BusyBox compatibility
	cmd := s.podman.Exec(containerName, []string{s.defaultShell})
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	)

	// Start the PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("starting pty: %w", err)
	}

	// Set initial terminal size
	if err := pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		ptmx.Close()
		return nil, fmt.Errorf("setting initial size: %w", err)
	}

	session := &Session{
		ID:      generateSessionID(),
		PTY:     ptmx,
		Conn:    conn,
		closeCh: make(chan struct{}),
	}

	return session, nil
}

// HandleSession manages the bidirectional communication for a terminal session.
// **Validates: Requirements 23.1, 23.2, 23.5**
func (s *Service) HandleSession(session *Session) error {
	defer func() {
		s.sessionsMu.Lock()
		delete(s.sessions, session.ID)
		s.sessionsMu.Unlock()
		session.Close()
	}()

	// Start goroutine to copy PTY output to WebSocket
	go s.copyPTYToWebSocket(session)

	// Handle WebSocket input
	return s.handleWebSocketInput(session)
}

// copyPTYToWebSocket copies output from the PTY to the WebSocket connection.
func (s *Service) copyPTYToWebSocket(session *Session) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-session.closeCh:
			return
		default:
			n, err := session.PTY.Read(buf)
			if err != nil {
				if err != io.EOF && !session.IsClosed() {
					s.logger.Debug("pty read error", "error", err, "session_id", session.ID)
				}
				return
			}
			if n > 0 {
				if err := session.Conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
					if !session.IsClosed() {
						s.logger.Debug("websocket write error", "error", err, "session_id", session.ID)
					}
					return
				}
			}
		}
	}
}

// handleWebSocketInput handles input from the WebSocket connection.
// **Validates: Requirements 23.5** - Handles reconnection by detecting disconnect
func (s *Service) handleWebSocketInput(session *Session) error {
	for {
		select {
		case <-session.closeCh:
			return ErrSessionClosed
		default:
			mt, msg, err := session.Conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					return nil
				}
				return fmt.Errorf("reading websocket: %w", err)
			}

			// Handle control messages (resize, terminate)
			if mt == websocket.TextMessage {
				var ctrl ControlMessage
				if err := json.Unmarshal(msg, &ctrl); err == nil {
					if err := s.handleControlMessage(session, &ctrl); err != nil {
						if err == ErrSessionClosed {
							return nil
						}
						s.logger.Warn("control message error", "error", err, "session_id", session.ID)
					}
					continue
				}
			}

			// Write input to PTY
			if mt == websocket.BinaryMessage || mt == websocket.TextMessage {
				if _, err := session.PTY.Write(msg); err != nil {
					return fmt.Errorf("writing to pty: %w", err)
				}
			}
		}
	}
}

// handleControlMessage processes control messages from the client.
func (s *Service) handleControlMessage(session *Session, ctrl *ControlMessage) error {
	switch ctrl.Type {
	case "resize":
		if ctrl.Rows > 0 && ctrl.Cols > 0 {
			if err := pty.Setsize(session.PTY, &pty.Winsize{
				Rows: ctrl.Rows,
				Cols: ctrl.Cols,
			}); err != nil {
				return fmt.Errorf("resizing pty: %w", err)
			}
			s.logger.Debug("terminal resized",
				"session_id", session.ID,
				"rows", ctrl.Rows,
				"cols", ctrl.Cols,
			)
		}
	case "terminate":
		s.logger.Info("terminal termination requested", "session_id", session.ID)
		return ErrSessionClosed
	}
	return nil
}

// GetSession returns an active session by ID.
func (s *Service) GetSession(sessionID string) (*Session, bool) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	session, ok := s.sessions[sessionID]
	return session, ok
}

// CloseSession closes a session by ID.
func (s *Service) CloseSession(sessionID string) error {
	s.sessionsMu.Lock()
	session, ok := s.sessions[sessionID]
	if ok {
		delete(s.sessions, sessionID)
	}
	s.sessionsMu.Unlock()

	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return session.Close()
}

// ActiveSessions returns the number of active terminal sessions.
func (s *Service) ActiveSessions() int {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	return len(s.sessions)
}

// GenerateContainerName creates a container name from app, service, and version.
// Format: {app}-{service}-v{version}
// **Validates: Requirements 9.3, 9.4, 9.5**
func GenerateContainerName(appName, serviceName string, version int) string {
	return fmt.Sprintf("%s-%s-v%d", appName, serviceName, version)
}

// generateSessionID creates a unique session identifier.
func generateSessionID() string {
	return fmt.Sprintf("term-%d", time.Now().UnixNano())
}
