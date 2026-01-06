// Package handlers provides HTTP request handlers for the API.
package handlers

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/narvanalabs/control-plane/internal/cleanup"
	"github.com/narvanalabs/control-plane/internal/store"
)

// CleanupHandler handles cleanup-related HTTP requests.
// Requirements: 19.1
type CleanupHandler struct {
	store          store.Store
	cleanupService *cleanup.Service
	logger         *slog.Logger
}

// NewCleanupHandler creates a new cleanup handler.
func NewCleanupHandler(st store.Store, cleanupSvc *cleanup.Service, logger *slog.Logger) *CleanupHandler {
	return &CleanupHandler{
		store:          st,
		cleanupService: cleanupSvc,
		logger:         logger,
	}
}

// CleanupJobResponse represents the response for a cleanup job trigger.
// Requirements: 19.4
type CleanupJobResponse struct {
	JobID   string `json:"job_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// CleanupContainersResponse represents the response for container cleanup.
type CleanupContainersResponse struct {
	JobID        string   `json:"job_id"`
	Status       string   `json:"status"`
	ItemsRemoved int      `json:"items_removed"`
	Errors       []string `json:"errors,omitempty"`
	Duration     string   `json:"duration"`
}

// CleanupImagesResponse represents the response for image cleanup.
type CleanupImagesResponse struct {
	JobID        string   `json:"job_id"`
	Status       string   `json:"status"`
	ItemsRemoved int      `json:"items_removed"`
	Errors       []string `json:"errors,omitempty"`
	Duration     string   `json:"duration"`
}

// NixGCResponse represents the response for Nix garbage collection.
type NixGCResponse struct {
	JobID        string `json:"job_id"`
	Status       string `json:"status"`
	SpaceFreed   int64  `json:"space_freed_bytes"`
	PathsRemoved int    `json:"paths_removed"`
	Duration     string `json:"duration"`
	Error        string `json:"error,omitempty"`
}

// ArchiveDeploymentsResponse represents the response for deployment archival.
type ArchiveDeploymentsResponse struct {
	JobID               string   `json:"job_id"`
	Status              string   `json:"status"`
	DeploymentsArchived int      `json:"deployments_archived"`
	BuildsArchived      int      `json:"builds_archived"`
	LogsArchived        int      `json:"logs_archived"`
	Errors              []string `json:"errors,omitempty"`
	Duration            string   `json:"duration"`
}

// CleanupAtticResponse represents the response for Attic cache cleanup.
type CleanupAtticResponse struct {
	JobID        string   `json:"job_id"`
	Status       string   `json:"status"`
	ItemsRemoved int      `json:"items_removed"`
	SpaceFreed   int64    `json:"space_freed_bytes"`
	Errors       []string `json:"errors,omitempty"`
	Duration     string   `json:"duration"`
}

// CleanupContainers handles POST /v1/admin/cleanup/containers - triggers immediate container cleanup.
// Requirements: 19.1, 19.4
func (h *CleanupHandler) CleanupContainers(w http.ResponseWriter, r *http.Request) {
	jobID := uuid.New().String()
	h.logger.Info("triggering container cleanup", "job_id", jobID)

	result, err := h.cleanupService.CleanupContainers(r.Context())
	if err != nil {
		h.logger.Error("container cleanup failed", "error", err, "job_id", jobID)
		WriteInternalError(w, "Failed to cleanup containers: "+err.Error())
		return
	}

	response := &CleanupContainersResponse{
		JobID:        jobID,
		Status:       "completed",
		ItemsRemoved: result.ItemsRemoved,
		Errors:       result.Errors,
		Duration:     result.Duration.String(),
	}

	h.logger.Info("container cleanup completed",
		"job_id", jobID,
		"items_removed", result.ItemsRemoved,
		"errors", len(result.Errors),
		"duration", result.Duration,
	)

	WriteJSON(w, http.StatusOK, response)
}

// CleanupImages handles POST /v1/admin/cleanup/images - triggers immediate image cleanup.
// Requirements: 25.4, 19.4
func (h *CleanupHandler) CleanupImages(w http.ResponseWriter, r *http.Request) {
	jobID := uuid.New().String()
	h.logger.Info("triggering image cleanup", "job_id", jobID)

	result, err := h.cleanupService.CleanupImages(r.Context())
	if err != nil {
		h.logger.Error("image cleanup failed", "error", err, "job_id", jobID)
		WriteInternalError(w, "Failed to cleanup images: "+err.Error())
		return
	}

	response := &CleanupImagesResponse{
		JobID:        jobID,
		Status:       "completed",
		ItemsRemoved: result.ItemsRemoved,
		Errors:       result.Errors,
		Duration:     result.Duration.String(),
	}

	h.logger.Info("image cleanup completed",
		"job_id", jobID,
		"items_removed", result.ItemsRemoved,
		"errors", len(result.Errors),
		"duration", result.Duration,
	)

	WriteJSON(w, http.StatusOK, response)
}

// NixGC handles POST /v1/admin/cleanup/nix-gc - triggers immediate Nix garbage collection.
// Requirements: 19.2, 19.4
func (h *CleanupHandler) NixGC(w http.ResponseWriter, r *http.Request) {
	jobID := uuid.New().String()
	h.logger.Info("triggering Nix garbage collection", "job_id", jobID)

	// Get all nodes and trigger GC on each
	nodes, err := h.store.Nodes().List(r.Context())
	if err != nil {
		h.logger.Error("failed to list nodes for Nix GC", "error", err, "job_id", jobID)
		WriteInternalError(w, "Failed to list nodes: "+err.Error())
		return
	}

	// Check if there are any nodes registered
	if len(nodes) == 0 {
		h.logger.Warn("no nodes registered for Nix GC", "job_id", jobID)
		response := &NixGCResponse{
			JobID:        jobID,
			Status:       "completed",
			SpaceFreed:   0,
			PathsRemoved: 0,
			Error:        "",
			Duration:     "0s",
		}
		WriteJSON(w, http.StatusOK, response)
		return
	}

	var totalSpaceFreed int64
	var totalPathsRemoved int
	var lastError string

	for _, node := range nodes {
		result, err := h.cleanupService.TriggerNixGC(r.Context(), node.ID)
		if err != nil {
			h.logger.Error("Nix GC failed on node", "error", err, "node_id", node.ID, "job_id", jobID)
			lastError = err.Error()
			continue
		}
		totalSpaceFreed += result.SpaceFreed
		totalPathsRemoved += result.PathsRemoved
		if result.Error != "" {
			lastError = result.Error
		}
	}

	response := &NixGCResponse{
		JobID:        jobID,
		Status:       "completed",
		SpaceFreed:   totalSpaceFreed,
		PathsRemoved: totalPathsRemoved,
		Error:        lastError,
		Duration:     "0s", // Aggregated duration not tracked
	}

	h.logger.Info("Nix garbage collection completed",
		"job_id", jobID,
		"space_freed", totalSpaceFreed,
		"paths_removed", totalPathsRemoved,
		"nodes_processed", len(nodes),
	)

	WriteJSON(w, http.StatusOK, response)
}

// ArchiveDeployments handles POST /v1/admin/cleanup/deployments - triggers immediate deployment archival.
// Requirements: 19.3, 19.4
func (h *CleanupHandler) ArchiveDeployments(w http.ResponseWriter, r *http.Request) {
	jobID := uuid.New().String()
	h.logger.Info("triggering deployment archival", "job_id", jobID)

	result, err := h.cleanupService.ArchiveDeployments(r.Context())
	if err != nil {
		h.logger.Error("deployment archival failed", "error", err, "job_id", jobID)
		WriteInternalError(w, "Failed to archive deployments: "+err.Error())
		return
	}

	response := &ArchiveDeploymentsResponse{
		JobID:               jobID,
		Status:              "completed",
		DeploymentsArchived: result.DeploymentsArchived,
		BuildsArchived:      result.BuildsArchived,
		LogsArchived:        result.LogsArchived,
		Errors:              result.Errors,
		Duration:            result.Duration.String(),
	}

	h.logger.Info("deployment archival completed",
		"job_id", jobID,
		"deployments_archived", result.DeploymentsArchived,
		"builds_archived", result.BuildsArchived,
		"logs_archived", result.LogsArchived,
		"errors", len(result.Errors),
		"duration", result.Duration,
	)

	WriteJSON(w, http.StatusOK, response)
}

// CleanupAttic handles POST /v1/admin/cleanup/attic - triggers immediate Attic cache cleanup.
// Requirements: 26.4, 19.4
func (h *CleanupHandler) CleanupAttic(w http.ResponseWriter, r *http.Request) {
	jobID := uuid.New().String()
	h.logger.Info("triggering Attic cache cleanup", "job_id", jobID)

	result, err := h.cleanupService.CleanupAttic(r.Context())
	if err != nil {
		h.logger.Error("Attic cache cleanup failed", "error", err, "job_id", jobID)
		WriteInternalError(w, "Failed to cleanup Attic cache: "+err.Error())
		return
	}

	response := &CleanupAtticResponse{
		JobID:        jobID,
		Status:       "completed",
		ItemsRemoved: result.ItemsRemoved,
		SpaceFreed:   result.SpaceFreed,
		Errors:       result.Errors,
		Duration:     result.Duration.String(),
	}

	h.logger.Info("Attic cache cleanup completed",
		"job_id", jobID,
		"items_removed", result.ItemsRemoved,
		"space_freed", result.SpaceFreed,
		"errors", len(result.Errors),
		"duration", result.Duration,
	)

	WriteJSON(w, http.StatusOK, response)
}
