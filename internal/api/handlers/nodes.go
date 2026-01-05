package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/narvanalabs/control-plane/internal/cleanup"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// NodeHandler handles node-related HTTP requests.
type NodeHandler struct {
	store          store.Store
	cleanupService *cleanup.Service
	logger         *slog.Logger
}

// NewNodeHandler creates a new node handler.
func NewNodeHandler(st store.Store, logger *slog.Logger) *NodeHandler {
	return &NodeHandler{
		store:  st,
		logger: logger,
	}
}

// NewNodeHandlerWithCleanup creates a new node handler with cleanup service for automatic cleanup.
// **Validates: Requirements 20.3**
func NewNodeHandlerWithCleanup(st store.Store, cleanupSvc *cleanup.Service, logger *slog.Logger) *NodeHandler {
	return &NodeHandler{
		store:          st,
		cleanupService: cleanupSvc,
		logger:         logger,
	}
}

// NodeInfo contains information about a node from registration/heartbeat.
type NodeInfo struct {
	ID          string                `json:"id"`
	Hostname    string                `json:"hostname"`
	Address     string                `json:"address"`
	GRPCPort    int                   `json:"grpc_port"`
	Resources   *models.NodeResources `json:"resources"`
	DiskMetrics *models.NodeDiskMetrics `json:"disk_metrics,omitempty"`
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
	NodeID      string                  `json:"node_id"`
	Resources   *models.NodeResources   `json:"resources"`
	DiskMetrics *models.NodeDiskMetrics `json:"disk_metrics,omitempty"`
	NodeInfo    *NodeInfo               `json:"node_info,omitempty"`
	Timestamp   int64                   `json:"timestamp,omitempty"`
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

// GetDiskMetrics returns disk metrics from either format.
// **Validates: Requirements 20.1**
func (r *HeartbeatRequest) GetDiskMetrics() *models.NodeDiskMetrics {
	if r.DiskMetrics != nil {
		return r.DiskMetrics
	}
	if r.NodeInfo != nil {
		return r.NodeInfo.DiskMetrics
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
	diskMetrics := req.GetDiskMetrics()

	// Update the node's heartbeat with disk metrics if provided
	// **Validates: Requirements 20.1**
	var err error
	if diskMetrics != nil {
		err = h.store.Nodes().UpdateHeartbeatWithDiskMetrics(r.Context(), nodeID, resources, diskMetrics)
		
		// Check disk usage and log warnings
		// **Validates: Requirements 20.2**
		h.checkDiskUsageWarnings(nodeID, diskMetrics)
	} else {
		err = h.store.Nodes().UpdateHeartbeat(r.Context(), nodeID, resources)
	}

	if err != nil {
		h.logger.Error("failed to update heartbeat", "error", err, "node_id", nodeID)
		WriteInternalError(w, "Failed to update heartbeat")
		return
	}

	h.logger.Debug("heartbeat received", "node_id", nodeID)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DiskWarningThreshold is the percentage at which a warning is logged.
// **Validates: Requirements 20.2**
const DiskWarningThreshold = 80.0

// DiskCriticalThreshold is the percentage at which automatic cleanup should be triggered.
// **Validates: Requirements 20.3**
const DiskCriticalThreshold = 90.0

// checkDiskUsageWarnings checks disk usage and logs warnings if thresholds are exceeded.
// **Validates: Requirements 20.2**
func (h *NodeHandler) checkDiskUsageWarnings(nodeID string, diskMetrics *models.NodeDiskMetrics) {
	if diskMetrics == nil {
		return
	}

	// Check Nix store usage
	if diskMetrics.NixStore != nil {
		h.checkPathUsageWarning(nodeID, "nix_store", diskMetrics.NixStore)
	}

	// Check container storage usage
	if diskMetrics.ContainerStorage != nil {
		h.checkPathUsageWarning(nodeID, "container_storage", diskMetrics.ContainerStorage)
	}
}

// checkPathUsageWarning checks disk usage for a specific path and logs warnings.
// **Validates: Requirements 20.2, 20.3**
func (h *NodeHandler) checkPathUsageWarning(nodeID, pathType string, stats *models.DiskStats) {
	if stats == nil {
		return
	}

	usagePercent := stats.UsagePercent

	// Log warning at 80% usage
	if usagePercent >= DiskWarningThreshold && usagePercent < DiskCriticalThreshold {
		h.logger.Warn("disk usage warning",
			"node_id", nodeID,
			"path_type", pathType,
			"path", stats.Path,
			"usage_percent", usagePercent,
			"total_bytes", stats.Total,
			"used_bytes", stats.Used,
			"available_bytes", stats.Available,
			"threshold", DiskWarningThreshold,
		)
	}

	// Log critical and trigger cleanup at 90% usage
	// **Validates: Requirements 20.3**
	if usagePercent >= DiskCriticalThreshold {
		h.logger.Error("disk usage critical - triggering automatic cleanup",
			"node_id", nodeID,
			"path_type", pathType,
			"path", stats.Path,
			"usage_percent", usagePercent,
			"total_bytes", stats.Total,
			"used_bytes", stats.Used,
			"available_bytes", stats.Available,
			"threshold", DiskCriticalThreshold,
		)

		// Trigger automatic cleanup
		h.triggerAutomaticCleanup(nodeID, pathType)
	}
}

// triggerAutomaticCleanup triggers the appropriate cleanup based on path type.
// **Validates: Requirements 20.3**
func (h *NodeHandler) triggerAutomaticCleanup(nodeID, pathType string) {
	if h.cleanupService == nil {
		h.logger.Warn("cleanup service not available, skipping automatic cleanup",
			"node_id", nodeID,
			"path_type", pathType,
		)
		return
	}

	h.logger.Info("triggering automatic cleanup due to high disk usage",
		"node_id", nodeID,
		"path_type", pathType,
	)

	// Run cleanup in a goroutine to not block the heartbeat response
	go func() {
		ctx := context.Background()

		switch pathType {
		case "nix_store":
			// Trigger Nix garbage collection
			result, err := h.cleanupService.TriggerNixGC(ctx, nodeID)
			if err != nil {
				h.logger.Error("automatic Nix GC failed",
					"node_id", nodeID,
					"error", err,
				)
				return
			}
			h.logger.Info("automatic Nix GC completed",
				"node_id", nodeID,
				"space_freed", result.SpaceFreed,
				"paths_removed", result.PathsRemoved,
			)

		case "container_storage":
			// Trigger container and image cleanup
			containerResult, err := h.cleanupService.CleanupContainers(ctx)
			if err != nil {
				h.logger.Error("automatic container cleanup failed",
					"node_id", nodeID,
					"error", err,
				)
			} else {
				h.logger.Info("automatic container cleanup completed",
					"node_id", nodeID,
					"items_removed", containerResult.ItemsRemoved,
				)
			}

			// Then clean up images
			imageResult, err := h.cleanupService.CleanupImages(ctx)
			if err != nil {
				h.logger.Error("automatic image cleanup failed",
					"node_id", nodeID,
					"error", err,
				)
			} else {
				h.logger.Info("automatic image cleanup completed",
					"node_id", nodeID,
					"items_removed", imageResult.ItemsRemoved,
				)
			}
		}
	}()
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
	diskMetrics := req.GetDiskMetrics()

	// Update the node's heartbeat with disk metrics if provided
	var err error
	if diskMetrics != nil {
		err = h.store.Nodes().UpdateHeartbeatWithDiskMetrics(r.Context(), nodeID, resources, diskMetrics)
		
		// Check disk usage and log warnings
		h.checkDiskUsageWarnings(nodeID, diskMetrics)
	} else {
		err = h.store.Nodes().UpdateHeartbeat(r.Context(), nodeID, resources)
	}

	if err != nil {
		h.logger.Error("failed to update heartbeat", "error", err, "node_id", nodeID)
		WriteInternalError(w, "Failed to update heartbeat")
		return
	}

	h.logger.Debug("heartbeat received", "node_id", nodeID)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Get handles GET /v1/nodes/{nodeID} - retrieves a specific node with disk stats.
// **Validates: Requirements 20.4**
func (h *NodeHandler) Get(w http.ResponseWriter, r *http.Request, nodeID string) {
	node, err := h.store.Nodes().Get(r.Context(), nodeID)
	if err != nil {
		h.logger.Error("failed to get node", "error", err, "node_id", nodeID)
		WriteNotFound(w, "Node not found")
		return
	}

	// Return node with disk stats included
	WriteJSON(w, http.StatusOK, node)
}

// NodeDetailsResponse represents the response for node details with disk stats.
// **Validates: Requirements 20.4**
type NodeDetailsResponse struct {
	*models.Node
	DiskUsageSummary *DiskUsageSummary `json:"disk_usage_summary,omitempty"`
}

// DiskUsageSummary provides a summary of disk usage across all monitored paths.
// **Validates: Requirements 20.4**
type DiskUsageSummary struct {
	NixStoreUsagePercent         float64 `json:"nix_store_usage_percent"`
	ContainerStorageUsagePercent float64 `json:"container_storage_usage_percent"`
	WarningThreshold             float64 `json:"warning_threshold"`
	CriticalThreshold            float64 `json:"critical_threshold"`
	NixStoreStatus               string  `json:"nix_store_status"`
	ContainerStorageStatus       string  `json:"container_storage_status"`
}

// GetDetails handles GET /v1/nodes/{nodeID}/details - retrieves detailed node info with disk stats.
// **Validates: Requirements 20.4**
func (h *NodeHandler) GetDetails(w http.ResponseWriter, r *http.Request, nodeID string) {
	node, err := h.store.Nodes().Get(r.Context(), nodeID)
	if err != nil {
		h.logger.Error("failed to get node", "error", err, "node_id", nodeID)
		WriteNotFound(w, "Node not found")
		return
	}

	// Build disk usage summary
	summary := &DiskUsageSummary{
		WarningThreshold:  DiskWarningThreshold,
		CriticalThreshold: DiskCriticalThreshold,
	}

	if node.DiskMetrics != nil {
		if node.DiskMetrics.NixStore != nil {
			summary.NixStoreUsagePercent = node.DiskMetrics.NixStore.UsagePercent
			summary.NixStoreStatus = getDiskStatus(node.DiskMetrics.NixStore.UsagePercent)
		}
		if node.DiskMetrics.ContainerStorage != nil {
			summary.ContainerStorageUsagePercent = node.DiskMetrics.ContainerStorage.UsagePercent
			summary.ContainerStorageStatus = getDiskStatus(node.DiskMetrics.ContainerStorage.UsagePercent)
		}
	}

	response := &NodeDetailsResponse{
		Node:             node,
		DiskUsageSummary: summary,
	}

	WriteJSON(w, http.StatusOK, response)
}

// getDiskStatus returns a status string based on disk usage percentage.
func getDiskStatus(usagePercent float64) string {
	if usagePercent >= DiskCriticalThreshold {
		return "critical"
	}
	if usagePercent >= DiskWarningThreshold {
		return "warning"
	}
	return "healthy"
}
