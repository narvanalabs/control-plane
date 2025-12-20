package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

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

// HeartbeatRequest represents the request body for a node heartbeat.
type HeartbeatRequest struct {
	NodeID    string                `json:"node_id"`
	Resources *models.NodeResources `json:"resources"`
}

// Validate validates the heartbeat request.
func (r *HeartbeatRequest) Validate() error {
	if r.NodeID == "" {
		return &APIError{Code: ErrCodeInvalidRequest, Message: "node_id is required"}
	}
	return nil
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

	// Update the node's heartbeat
	if err := h.store.Nodes().UpdateHeartbeat(r.Context(), req.NodeID, req.Resources); err != nil {
		h.logger.Error("failed to update heartbeat", "error", err, "node_id", req.NodeID)
		WriteInternalError(w, "Failed to update heartbeat")
		return
	}

	h.logger.Debug("heartbeat received", "node_id", req.NodeID)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
