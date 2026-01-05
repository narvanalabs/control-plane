// Package cleanup provides services for automatic cleanup and disk monitoring.
package cleanup

import (
	"context"
	"log/slog"

	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// DiskUsageThresholds defines the thresholds for disk usage warnings and actions.
const (
	// DiskWarningThreshold is the percentage at which a warning is logged.
	// **Validates: Requirements 20.2**
	DiskWarningThreshold = 80.0

	// DiskCriticalThreshold is the percentage at which automatic cleanup is triggered.
	// **Validates: Requirements 20.3**
	DiskCriticalThreshold = 90.0
)

// DiskMonitor monitors disk usage on nodes and triggers warnings/cleanup.
type DiskMonitor struct {
	store          store.Store
	cleanupService *Service
	logger         *slog.Logger
}

// NewDiskMonitor creates a new disk monitor.
func NewDiskMonitor(st store.Store, cleanupSvc *Service, logger *slog.Logger) *DiskMonitor {
	if logger == nil {
		logger = slog.Default()
	}
	return &DiskMonitor{
		store:          st,
		cleanupService: cleanupSvc,
		logger:         logger,
	}
}

// CheckDiskUsage checks disk usage for a node and logs warnings if thresholds are exceeded.
// Returns true if cleanup was triggered due to critical threshold.
// **Validates: Requirements 20.2, 20.3**
func (m *DiskMonitor) CheckDiskUsage(ctx context.Context, nodeID string, diskMetrics *models.NodeDiskMetrics) bool {
	if diskMetrics == nil {
		return false
	}

	cleanupTriggered := false

	// Check Nix store usage
	if diskMetrics.NixStore != nil {
		cleanupTriggered = m.checkPathUsage(ctx, nodeID, "nix_store", diskMetrics.NixStore) || cleanupTriggered
	}

	// Check container storage usage
	if diskMetrics.ContainerStorage != nil {
		cleanupTriggered = m.checkPathUsage(ctx, nodeID, "container_storage", diskMetrics.ContainerStorage) || cleanupTriggered
	}

	return cleanupTriggered
}

// checkPathUsage checks disk usage for a specific path and logs warnings.
// Returns true if cleanup was triggered.
func (m *DiskMonitor) checkPathUsage(ctx context.Context, nodeID, pathType string, stats *models.DiskStats) bool {
	if stats == nil {
		return false
	}

	usagePercent := stats.UsagePercent

	// Log warning at 80% usage
	// **Validates: Requirements 20.2**
	if usagePercent >= DiskWarningThreshold && usagePercent < DiskCriticalThreshold {
		m.logger.Warn("disk usage warning",
			"node_id", nodeID,
			"path_type", pathType,
			"path", stats.Path,
			"usage_percent", usagePercent,
			"total_bytes", stats.Total,
			"used_bytes", stats.Used,
			"available_bytes", stats.Available,
			"threshold", DiskWarningThreshold,
		)
		return false
	}

	// Trigger cleanup at 90% usage
	// **Validates: Requirements 20.3**
	if usagePercent >= DiskCriticalThreshold {
		m.logger.Error("disk usage critical - triggering cleanup",
			"node_id", nodeID,
			"path_type", pathType,
			"path", stats.Path,
			"usage_percent", usagePercent,
			"total_bytes", stats.Total,
			"used_bytes", stats.Used,
			"available_bytes", stats.Available,
			"threshold", DiskCriticalThreshold,
		)

		// Trigger appropriate cleanup based on path type
		m.triggerCleanup(ctx, nodeID, pathType)
		return true
	}

	return false
}

// triggerCleanup triggers the appropriate cleanup based on path type.
// **Validates: Requirements 20.3**
func (m *DiskMonitor) triggerCleanup(ctx context.Context, nodeID, pathType string) {
	if m.cleanupService == nil {
		m.logger.Warn("cleanup service not available, skipping automatic cleanup",
			"node_id", nodeID,
			"path_type", pathType,
		)
		return
	}

	m.logger.Info("triggering automatic cleanup due to high disk usage",
		"node_id", nodeID,
		"path_type", pathType,
	)

	switch pathType {
	case "nix_store":
		// Trigger Nix garbage collection
		go func() {
			result, err := m.cleanupService.TriggerNixGC(ctx, nodeID)
			if err != nil {
				m.logger.Error("automatic Nix GC failed",
					"node_id", nodeID,
					"error", err,
				)
				return
			}
			m.logger.Info("automatic Nix GC completed",
				"node_id", nodeID,
				"space_freed", result.SpaceFreed,
				"paths_removed", result.PathsRemoved,
			)
		}()

	case "container_storage":
		// Trigger container and image cleanup
		go func() {
			// Clean up containers first
			containerResult, err := m.cleanupService.CleanupContainers(ctx)
			if err != nil {
				m.logger.Error("automatic container cleanup failed",
					"node_id", nodeID,
					"error", err,
				)
			} else {
				m.logger.Info("automatic container cleanup completed",
					"node_id", nodeID,
					"items_removed", containerResult.ItemsRemoved,
				)
			}

			// Then clean up images
			imageResult, err := m.cleanupService.CleanupImages(ctx)
			if err != nil {
				m.logger.Error("automatic image cleanup failed",
					"node_id", nodeID,
					"error", err,
				)
			} else {
				m.logger.Info("automatic image cleanup completed",
					"node_id", nodeID,
					"items_removed", imageResult.ItemsRemoved,
				)
			}
		}()
	}
}

// CheckAllNodes checks disk usage for all nodes and logs warnings.
// **Validates: Requirements 20.2, 20.3**
func (m *DiskMonitor) CheckAllNodes(ctx context.Context) error {
	nodes, err := m.store.Nodes().List(ctx)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		if node.DiskMetrics != nil {
			m.CheckDiskUsage(ctx, node.ID, node.DiskMetrics)
		}
	}

	return nil
}
