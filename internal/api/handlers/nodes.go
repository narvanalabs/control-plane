package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// NodeHandler handles node-related HTTP requests.
type NodeHandler struct {
	store  store.Store
	logger *slog.Logger
}

// NewNodeHandler creates a new node handler.
func NewNodeHandler(st store.Store, logger *slog.Logger) *NodeHandler {
	return &NodeHandler{
		store:  st,
		logger: logger,
	}
}

// NodeInfo contains information about a node from registration/heartbeat.
type NodeInfo struct {
	ID          string                `json:"id"`
	Hostname    string                `json:"hostname"`
	Address     string                `json:"address"`
	GRPCPort    int                   `json:"grpc_port"`
	Resources   *models.NodeResources `json:"resources"`
	CachedPaths []string              `json:"cached_paths,omitempty"`
}

// RegisterRequest represents the request body for node registration.
type RegisterRequest struct {
	NodeInfo *NodeInfo `json:"node_info"`
}

// Validate validates the registration request.
func (r *RegisterRequest) Validate() error {
	if r.NodeInfo == nil {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "node_info is required"}
	}
	if r.NodeInfo.ID == "" {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "node_info.id is required"}
	}
	return nil
}

// HeartbeatRequest represents the request body for a node heartbeat.
type HeartbeatRequest struct {
	NodeID    string                `json:"node_id"`
	Resources *models.NodeResources `json:"resources"`
	NodeInfo  *NodeInfo             `json:"node_info,omitempty"`
	Timestamp int64                 `json:"timestamp,omitempty"`
}

// Validate validates the heartbeat request.
func (r *HeartbeatRequest) Validate() error {
	// Support both old format (node_id) and new format (node_info)
	if r.NodeID == "" && (r.NodeInfo == nil || r.NodeInfo.ID == "") {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "node_id or node_info.id is required"}
	}
	return nil
}

// GetNodeID returns the node ID from either format.
func (r *HeartbeatRequest) GetNodeID() string {
	if r.NodeID != "" {
		return r.NodeID
	}
	if r.NodeInfo != nil {
		return r.NodeInfo.ID
	}
	return ""
}

// GetResources returns resources from either format.
func (r *HeartbeatRequest) GetResources() *models.NodeResources {
	if r.Resources != nil {
		return r.Resources
	}
	if r.NodeInfo != nil {
		return r.NodeInfo.Resources
	}
	return nil
}

// Register handles POST /v1/nodes/register - registers a new node.
func (h *NodeHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
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

	node := &models.Node{
		ID:            req.NodeInfo.ID,
		Hostname:      req.NodeInfo.Hostname,
		Address:       req.NodeInfo.Address,
		GRPCPort:      req.NodeInfo.GRPCPort,
		Healthy:       true,
		LastHeartbeat: time.Now(),
		Resources:     req.NodeInfo.Resources,
	}

	// Register creates or updates the node
	if err := h.store.Nodes().Register(r.Context(), node); err != nil {
		h.logger.Error("failed to register node", "error", err, "node_id", req.NodeInfo.ID)
		WriteInternalError(w, "Failed to register node")
		return
	}

	h.logger.Info("node registered", "node_id", req.NodeInfo.ID, "hostname", req.NodeInfo.Hostname)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Node registered successfully",
		"node_id": req.NodeInfo.ID,
	})
}

// List handles GET /v1/nodes - lists all registered nodes.
func (h *NodeHandler) List(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.store.Nodes().List(r.Context())
	if err != nil {
		h.logger.Error("failed to list nodes", "error", err)
		WriteInternalError(w, "Failed to list nodes")
		return
	}

	if nodes == nil {
		nodes = []*models.Node{}
	}

	WriteJSON(w, http.StatusOK, nodes)
}

// Heartbeat handles POST /v1/nodes/heartbeat - updates node health status.
func (h *NodeHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	var req HeartbeatRequest
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

	nodeID := req.GetNodeID()
	resources := req.GetResources()

	// Update the node's heartbeat
	if err := h.store.Nodes().UpdateHeartbeat(r.Context(), nodeID, resources); err != nil {
		h.logger.Error("failed to update heartbeat", "error", err, "node_id", nodeID)
		WriteInternalError(w, "Failed to update heartbeat")
		return
	}

	h.logger.Debug("heartbeat received", "node_id", nodeID)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HeartbeatByID handles POST /v1/nodes/{nodeID}/heartbeat - updates specific node health.
func (h *NodeHandler) HeartbeatByID(w http.ResponseWriter, r *http.Request, nodeID string) {
	var req HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Use nodeID from URL path
	resources := req.GetResources()

	// Update the node's heartbeat
	if err := h.store.Nodes().UpdateHeartbeat(r.Context(), nodeID, resources); err != nil {
		h.logger.Error("failed to update heartbeat", "error", err, "node_id", nodeID)
		WriteInternalError(w, "Failed to update heartbeat")
		return
	}

	h.logger.Debug("heartbeat received", "node_id", nodeID)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
