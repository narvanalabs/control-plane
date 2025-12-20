// Package scheduler provides intelligent deployment scheduling for the control plane.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
	"github.com/narvanalabs/control-plane/pkg/config"
)

// Common errors returned by the scheduler.
var (
	ErrNoHealthyNodes        = errors.New("no healthy nodes available")
	ErrInsufficientResources = errors.New("no nodes with sufficient resources")
)

// AgentClient defines the interface for communicating with node agents.
type AgentClient interface {
	Deploy(ctx context.Context, nodeID string, deployment *models.Deployment) error
	Stop(ctx context.Context, nodeID string, deploymentID string) error
}

// Scheduler determines optimal node placement for deployments.
type Scheduler struct {
	store           store.Store
	agentClient     AgentClient
	healthThreshold time.Duration
	maxRetries      int
	retryBackoff    time.Duration
	logger          *slog.Logger
}

// NewScheduler creates a new Scheduler instance.
func NewScheduler(s store.Store, agentClient AgentClient, cfg *config.SchedulerConfig, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		store:           s,
		agentClient:     agentClient,
		healthThreshold: cfg.HealthThreshold,
		maxRetries:      cfg.MaxRetries,
		retryBackoff:    cfg.RetryBackoff,
		logger:          logger,
	}
}


// Schedule assigns a deployment to an appropriate node.
// It filters nodes by health, resources, and cache locality, then selects the best candidate.
func (s *Scheduler) Schedule(ctx context.Context, deployment *models.Deployment) (*models.Node, error) {
	s.logger.Info("scheduling deployment",
		"deployment_id", deployment.ID,
		"app_id", deployment.AppID,
		"build_type", deployment.BuildType,
		"resource_tier", deployment.ResourceTier,
	)

	// 1. Get all nodes
	nodes, err := s.store.Nodes().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	if len(nodes) == 0 {
		return nil, ErrNoHealthyNodes
	}

	// 2. Filter healthy nodes (heartbeat within threshold)
	healthyNodes := s.filterHealthy(nodes)
	if len(healthyNodes) == 0 {
		s.logger.Warn("no healthy nodes available", "total_nodes", len(nodes))
		return nil, ErrNoHealthyNodes
	}

	// 3. Filter by resource capacity
	capableNodes := s.filterByCapacity(healthyNodes, deployment.ResourceTier)
	if len(capableNodes) == 0 {
		s.logger.Warn("no nodes with sufficient resources",
			"healthy_nodes", len(healthyNodes),
			"resource_tier", deployment.ResourceTier,
		)
		return nil, ErrInsufficientResources
	}

	// 4. For pure Nix: prefer nodes with cached closure
	var selectedNode *models.Node
	if deployment.BuildType == models.BuildTypePureNix && deployment.Artifact != "" {
		selectedNode = s.findNodeWithClosure(capableNodes, deployment.Artifact)
		if selectedNode != nil {
			s.logger.Info("selected node with cached closure",
				"node_id", selectedNode.ID,
				"artifact", deployment.Artifact,
			)
		}
	}

	// 5. Fallback: select node with most available capacity
	if selectedNode == nil {
		selectedNode = s.selectByCapacity(capableNodes)
		s.logger.Info("selected node by capacity",
			"node_id", selectedNode.ID,
		)
	}

	return selectedNode, nil
}

// filterHealthy returns nodes that have sent a heartbeat within the health threshold.
func (s *Scheduler) filterHealthy(nodes []*models.Node) []*models.Node {
	threshold := time.Now().Add(-s.healthThreshold)
	var healthy []*models.Node

	for _, node := range nodes {
		if node.Healthy && node.LastHeartbeat.After(threshold) {
			healthy = append(healthy, node)
		}
	}

	return healthy
}

// IsNodeHealthy checks if a node is considered healthy based on its heartbeat.
func (s *Scheduler) IsNodeHealthy(node *models.Node) bool {
	threshold := time.Now().Add(-s.healthThreshold)
	return node.Healthy && node.LastHeartbeat.After(threshold)
}

// GetHealthThreshold returns the configured health threshold duration.
func (s *Scheduler) GetHealthThreshold() time.Duration {
	return s.healthThreshold
}


// ScheduleAndAssign schedules a deployment and updates the deployment record with the placement.
func (s *Scheduler) ScheduleAndAssign(ctx context.Context, deployment *models.Deployment) error {
	node, err := s.Schedule(ctx, deployment)
	if err != nil {
		return err
	}

	// Update deployment with placement
	deployment.NodeID = node.ID
	deployment.Status = models.DeploymentStatusScheduled
	now := time.Now()
	deployment.UpdatedAt = now

	if err := s.store.Deployments().Update(ctx, deployment); err != nil {
		return fmt.Errorf("updating deployment with placement: %w", err)
	}

	// Notify the node agent if available
	if s.agentClient != nil {
		if err := s.agentClient.Deploy(ctx, node.ID, deployment); err != nil {
			s.logger.Error("failed to notify agent",
				"node_id", node.ID,
				"deployment_id", deployment.ID,
				"error", err,
			)
			// Don't fail the scheduling - the deployment is recorded
		}
	}

	s.logger.Info("deployment scheduled",
		"deployment_id", deployment.ID,
		"node_id", node.ID,
	)

	return nil
}

// Reschedule moves all deployments from a node to other healthy nodes.
func (s *Scheduler) Reschedule(ctx context.Context, nodeID string) error {
	s.logger.Info("rescheduling deployments from node", "node_id", nodeID)

	// Get all deployments on this node
	deployments, err := s.store.Deployments().ListByNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("listing deployments for node: %w", err)
	}

	var rescheduled, failed int
	for _, deployment := range deployments {
		// Only reschedule running or starting deployments
		if deployment.Status != models.DeploymentStatusRunning &&
			deployment.Status != models.DeploymentStatusStarting &&
			deployment.Status != models.DeploymentStatusScheduled {
			continue
		}

		// Clear the node assignment and reschedule
		deployment.NodeID = ""
		deployment.Status = models.DeploymentStatusBuilt

		if err := s.ScheduleAndAssign(ctx, deployment); err != nil {
			s.logger.Error("failed to reschedule deployment",
				"deployment_id", deployment.ID,
				"error", err,
			)
			failed++
			continue
		}
		rescheduled++
	}

	s.logger.Info("rescheduling complete",
		"node_id", nodeID,
		"rescheduled", rescheduled,
		"failed", failed,
	)

	return nil
}
