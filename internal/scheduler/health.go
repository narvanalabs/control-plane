package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/narvanalabs/control-plane/internal/store"
)

// HealthMonitor periodically checks node health and triggers rescheduling for unhealthy nodes.
type HealthMonitor struct {
	store           store.Store
	scheduler       *Scheduler
	healthThreshold time.Duration
	checkInterval   time.Duration
	logger          *slog.Logger

	mu       sync.Mutex
	running  bool
	stopChan chan struct{}
}

// NewHealthMonitor creates a new HealthMonitor instance.
func NewHealthMonitor(s store.Store, scheduler *Scheduler, healthThreshold, checkInterval time.Duration, logger *slog.Logger) *HealthMonitor {
	if logger == nil {
		logger = slog.Default()
	}
	return &HealthMonitor{
		store:           s,
		scheduler:       scheduler,
		healthThreshold: healthThreshold,
		checkInterval:   checkInterval,
		logger:          logger,
		stopChan:        make(chan struct{}),
	}
}

// Start begins the periodic health check loop.
func (h *HealthMonitor) Start(ctx context.Context) error {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return nil
	}
	h.running = true
	h.stopChan = make(chan struct{})
	h.mu.Unlock()

	h.logger.Info("starting health monitor",
		"health_threshold", h.healthThreshold,
		"check_interval", h.checkInterval,
	)

	ticker := time.NewTicker(h.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("health monitor stopped by context")
			return ctx.Err()
		case <-h.stopChan:
			h.logger.Info("health monitor stopped")
			return nil
		case <-ticker.C:
			if err := h.checkNodes(ctx); err != nil {
				h.logger.Error("health check failed", "error", err)
			}
		}
	}
}

// Stop stops the health monitor.
func (h *HealthMonitor) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running {
		close(h.stopChan)
		h.running = false
	}
}


// checkNodes checks all nodes and marks stale ones as unhealthy.
func (h *HealthMonitor) checkNodes(ctx context.Context) error {
	nodes, err := h.store.Nodes().List(ctx)
	if err != nil {
		return err
	}

	threshold := time.Now().Add(-h.healthThreshold)
	var unhealthyNodes []string

	for _, node := range nodes {
		isStale := node.LastHeartbeat.Before(threshold)

		if node.Healthy && isStale {
			// Node was healthy but heartbeat is stale - mark as unhealthy
			h.logger.Warn("marking node as unhealthy due to stale heartbeat",
				"node_id", node.ID,
				"last_heartbeat", node.LastHeartbeat,
				"threshold", threshold,
			)

			if err := h.store.Nodes().UpdateHealth(ctx, node.ID, false); err != nil {
				h.logger.Error("failed to update node health",
					"node_id", node.ID,
					"error", err,
				)
				continue
			}

			unhealthyNodes = append(unhealthyNodes, node.ID)
		}
	}

	// Trigger rescheduling for deployments on unhealthy nodes
	for _, nodeID := range unhealthyNodes {
		if h.scheduler != nil {
			if err := h.scheduler.Reschedule(ctx, nodeID); err != nil {
				h.logger.Error("failed to reschedule deployments",
					"node_id", nodeID,
					"error", err,
				)
			}
		}
	}

	return nil
}

// CheckNodeHealth checks if a specific node should be marked as unhealthy.
// Returns true if the node is stale (heartbeat older than threshold).
func (h *HealthMonitor) CheckNodeHealth(node *NodeHealthInfo) bool {
	threshold := time.Now().Add(-h.healthThreshold)
	return node.LastHeartbeat.Before(threshold)
}

// NodeHealthInfo contains the information needed to check node health.
type NodeHealthInfo struct {
	ID            string
	Healthy       bool
	LastHeartbeat time.Time
}

// IsStale checks if a node's heartbeat is older than the given threshold.
func IsStale(lastHeartbeat time.Time, threshold time.Duration) bool {
	cutoff := time.Now().Add(-threshold)
	return lastHeartbeat.Before(cutoff)
}

// MarkUnhealthyIfStale marks a node as unhealthy if its heartbeat is stale.
// Returns true if the node was marked unhealthy.
func MarkUnhealthyIfStale(ctx context.Context, nodeStore store.NodeStore, nodeID string, lastHeartbeat time.Time, threshold time.Duration) (bool, error) {
	if IsStale(lastHeartbeat, threshold) {
		if err := nodeStore.UpdateHealth(ctx, nodeID, false); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}
