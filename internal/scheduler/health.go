package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// HealthMonitor periodically checks node health and triggers rescheduling for unhealthy nodes.
// It also watches for nodes becoming healthy and processes pending deployments.
// **Validates: Requirements 16.2**
type HealthMonitor struct {
	store             store.Store
	scheduler         *Scheduler
	healthThreshold   time.Duration
	checkInterval     time.Duration
	deploymentTimeout time.Duration // Timeout for deployments waiting to be scheduled
	logger            *slog.Logger

	mu       sync.Mutex
	running  bool
	stopChan chan struct{}
}

// NewHealthMonitor creates a new HealthMonitor instance.
func NewHealthMonitor(s store.Store, scheduler *Scheduler, healthThreshold, checkInterval time.Duration, logger *slog.Logger) *HealthMonitor {
	return NewHealthMonitorWithTimeout(s, scheduler, healthThreshold, checkInterval, 30*time.Minute, logger)
}

// NewHealthMonitorWithTimeout creates a new HealthMonitor instance with a custom deployment timeout.
// **Validates: Requirements 16.3**
func NewHealthMonitorWithTimeout(s store.Store, scheduler *Scheduler, healthThreshold, checkInterval, deploymentTimeout time.Duration, logger *slog.Logger) *HealthMonitor {
	if logger == nil {
		logger = slog.Default()
	}
	return &HealthMonitor{
		store:             s,
		scheduler:         scheduler,
		healthThreshold:   healthThreshold,
		checkInterval:     checkInterval,
		deploymentTimeout: deploymentTimeout,
		logger:            logger,
		stopChan:          make(chan struct{}),
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
// It also processes pending deployments when healthy nodes are available.
// **Validates: Requirements 16.2, 16.4**
func (h *HealthMonitor) checkNodes(ctx context.Context) error {
	nodes, err := h.store.Nodes().List(ctx)
	if err != nil {
		return err
	}

	threshold := time.Now().Add(-h.healthThreshold)
	var unhealthyNodes []string
	var healthyNodeCount int
	var totalAvailableResources ResourceAvailability

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
		} else if node.Healthy && !isStale {
			healthyNodeCount++
			// Track available resources for resource constraint retry
			// **Validates: Requirements 16.4**
			if node.Resources != nil {
				totalAvailableResources.CPUAvailable += node.Resources.CPUAvailable
				totalAvailableResources.MemoryAvailable += node.Resources.MemoryAvailable
			}
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

	// Process pending deployments when healthy nodes are available or resources have changed
	// **Validates: Requirements 16.2, 16.4**
	if healthyNodeCount > 0 && h.scheduler != nil {
		if err := h.processPendingDeployments(ctx); err != nil {
			h.logger.Error("failed to process pending deployments",
				"error", err,
			)
		}
	}

	return nil
}

// ResourceAvailability tracks the total available resources across all healthy nodes.
// **Validates: Requirements 16.4**
type ResourceAvailability struct {
	CPUAvailable    float64
	MemoryAvailable int64
}

// processPendingDeployments attempts to schedule deployments that are waiting for nodes.
// It also marks deployments as failed if they have exceeded the timeout.
// **Validates: Requirements 16.2, 16.3**
func (h *HealthMonitor) processPendingDeployments(ctx context.Context) error {
	// Get all deployments in "built" status (waiting to be scheduled)
	pendingDeployments, err := h.store.Deployments().ListByStatus(ctx, models.DeploymentStatusBuilt)
	if err != nil {
		return err
	}

	if len(pendingDeployments) == 0 {
		return nil
	}

	h.logger.Info("processing pending deployments",
		"count", len(pendingDeployments),
	)

	var scheduled, failed, timedOut int
	now := time.Now()

	for _, deployment := range pendingDeployments {
		// Check if deployment has timed out
		// **Validates: Requirements 16.3**
		if h.deploymentTimeout > 0 && now.Sub(deployment.CreatedAt) > h.deploymentTimeout {
			h.logger.Warn("deployment timed out waiting for scheduling",
				"deployment_id", deployment.ID,
				"created_at", deployment.CreatedAt,
				"timeout", h.deploymentTimeout,
			)

			deployment.Status = models.DeploymentStatusFailed
			deployment.UpdatedAt = now
			finishedAt := now
			deployment.FinishedAt = &finishedAt

			if err := h.store.Deployments().Update(ctx, deployment); err != nil {
				h.logger.Error("failed to mark deployment as timed out",
					"deployment_id", deployment.ID,
					"error", err,
				)
			} else {
				timedOut++
			}
			continue
		}

		err := h.scheduler.ScheduleAndAssign(ctx, deployment)
		if err != nil {
			if errors.Is(err, ErrDeploymentQueued) {
				// Still no resources available, skip
				continue
			}
			if errors.Is(err, ErrDependenciesNotRunning) {
				// Dependencies not ready, skip
				continue
			}
			h.logger.Error("failed to schedule pending deployment",
				"deployment_id", deployment.ID,
				"error", err,
			)
			failed++
			continue
		}
		scheduled++
	}

	if scheduled > 0 || failed > 0 || timedOut > 0 {
		h.logger.Info("pending deployments processed",
			"scheduled", scheduled,
			"failed", failed,
			"timed_out", timedOut,
			"remaining", len(pendingDeployments)-scheduled-failed-timedOut,
		)
	}

	return nil
}

// CheckDeploymentTimeout checks if a deployment has exceeded the timeout.
// Returns true if the deployment should be marked as failed.
// **Validates: Requirements 16.3**
func CheckDeploymentTimeout(deployment *models.Deployment, timeout time.Duration) bool {
	if timeout <= 0 {
		return false
	}
	return time.Since(deployment.CreatedAt) > timeout
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

// HasResourcesForSpec checks if the total available resources can accommodate a deployment.
// **Validates: Requirements 16.4**
func (r *ResourceAvailability) HasResourcesForSpec(spec *models.ResourceSpec) bool {
	requirements := GetResourceRequirements(spec)
	return r.CPUAvailable >= requirements.CPU && r.MemoryAvailable >= requirements.Memory
}

// CalculateTotalResources calculates the total available resources across all healthy nodes.
// **Validates: Requirements 16.4**
func CalculateTotalResources(nodes []*models.Node, healthThreshold time.Duration) ResourceAvailability {
	var total ResourceAvailability
	threshold := time.Now().Add(-healthThreshold)

	for _, node := range nodes {
		if node.Healthy && node.LastHeartbeat.After(threshold) && node.Resources != nil {
			total.CPUAvailable += node.Resources.CPUAvailable
			total.MemoryAvailable += node.Resources.MemoryAvailable
		}
	}

	return total
}
