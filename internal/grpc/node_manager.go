// Package grpc provides the gRPC server implementation for the control plane.
package grpc

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/narvanalabs/control-plane/api/proto"
	"github.com/narvanalabs/control-plane/internal/store"
)

// NodeStatus represents the health status of a node.
type NodeStatus int

const (
	// NodeStatusHealthy indicates the node is responding normally.
	NodeStatusHealthy NodeStatus = iota
	// NodeStatusDegraded indicates the node has missed some heartbeats.
	NodeStatusDegraded
	// NodeStatusDown indicates the node is unresponsive.
	NodeStatusDown
)

// String returns the string representation of NodeStatus.
func (s NodeStatus) String() string {
	switch s {
	case NodeStatusHealthy:
		return "healthy"
	case NodeStatusDegraded:
		return "degraded"
	case NodeStatusDown:
		return "down"
	default:
		return "unknown"
	}
}

// NodeConnection represents an active connection to a node agent.
type NodeConnection struct {
	NodeID        string
	Info          *pb.NodeInfo
	CommandStream pb.ControlPlaneService_WatchCommandsServer
	LastHeartbeat time.Time
	Status        NodeStatus
	// activeDeployments tracks deployment IDs currently running on this node
	activeDeployments map[string]bool
	mu                sync.RWMutex
}

// UpdateHeartbeat updates the last heartbeat time and node info.
func (nc *NodeConnection) UpdateHeartbeat(info *pb.NodeInfo) {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.LastHeartbeat = time.Now()
	nc.Info = info
	// If node was degraded or down and sends heartbeat, mark as healthy
	if nc.Status != NodeStatusHealthy {
		nc.Status = NodeStatusHealthy
	}
}

// SetStatus sets the node status.
func (nc *NodeConnection) SetStatus(status NodeStatus) {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.Status = status
}

// GetStatus returns the current node status.
func (nc *NodeConnection) GetStatus() NodeStatus {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	return nc.Status
}

// GetLastHeartbeat returns the last heartbeat time.
func (nc *NodeConnection) GetLastHeartbeat() time.Time {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	return nc.LastHeartbeat
}

// HasDeployment checks if a deployment is active on this node.
func (nc *NodeConnection) HasDeployment(deploymentID string) bool {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	return nc.activeDeployments[deploymentID]
}

// AddDeployment marks a deployment as active on this node.
func (nc *NodeConnection) AddDeployment(deploymentID string) {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.activeDeployments[deploymentID] = true
}

// RemoveDeployment removes a deployment from the active set.
func (nc *NodeConnection) RemoveDeployment(deploymentID string) {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	delete(nc.activeDeployments, deploymentID)
}

// NodeManager manages active node connections and their command streams.
type NodeManager struct {
	store       store.Store
	connections map[string]*NodeConnection
	mu          sync.RWMutex
	logger      *slog.Logger

	// Health check configuration
	healthCheckInterval time.Duration
	degradedThreshold   time.Duration // 30s (3 missed heartbeats)
	downThreshold       time.Duration // 60s (6 missed heartbeats)

	// Shutdown coordination
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NodeManagerConfig holds configuration for the NodeManager.
type NodeManagerConfig struct {
	HealthCheckInterval time.Duration
	DegradedThreshold   time.Duration
	DownThreshold       time.Duration
}

// DefaultNodeManagerConfig returns default configuration values.
func DefaultNodeManagerConfig() *NodeManagerConfig {
	return &NodeManagerConfig{
		HealthCheckInterval: 10 * time.Second,
		DegradedThreshold:   30 * time.Second,
		DownThreshold:       60 * time.Second,
	}
}

// NewNodeManager creates a new NodeManager instance.
func NewNodeManager(st store.Store, logger *slog.Logger) *NodeManager {
	return NewNodeManagerWithConfig(st, logger, DefaultNodeManagerConfig())
}

// NewNodeManagerWithConfig creates a new NodeManager with custom configuration.
func NewNodeManagerWithConfig(st store.Store, logger *slog.Logger, cfg *NodeManagerConfig) *NodeManager {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg == nil {
		cfg = DefaultNodeManagerConfig()
	}

	return &NodeManager{
		store:               st,
		connections:         make(map[string]*NodeConnection),
		logger:              logger,
		healthCheckInterval: cfg.HealthCheckInterval,
		degradedThreshold:   cfg.DegradedThreshold,
		downThreshold:       cfg.DownThreshold,
		stopChan:            make(chan struct{}),
	}
}

// RegisterConnection registers a node's command stream.
func (m *NodeManager) RegisterConnection(nodeID string, stream pb.ControlPlaneService_WatchCommandsServer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if there's an existing connection
	if existing, ok := m.connections[nodeID]; ok {
		m.logger.Info("replacing existing connection for node", "node_id", nodeID)
		// The old stream will be closed when the old goroutine detects context cancellation
		existing.CommandStream = stream
		existing.LastHeartbeat = time.Now()
		existing.Status = NodeStatusHealthy
		return nil
	}

	// Create new connection
	m.connections[nodeID] = &NodeConnection{
		NodeID:            nodeID,
		CommandStream:     stream,
		LastHeartbeat:     time.Now(),
		Status:            NodeStatusHealthy,
		activeDeployments: make(map[string]bool),
	}

	m.logger.Info("registered node connection", "node_id", nodeID)
	return nil
}

// UnregisterConnection removes a node's connection.
func (m *NodeManager) UnregisterConnection(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.connections[nodeID]; ok {
		delete(m.connections, nodeID)
		m.logger.Info("unregistered node connection", "node_id", nodeID)
	}
}

// GetConnection returns a node's connection if it exists.
func (m *NodeManager) GetConnection(nodeID string) (*NodeConnection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.connections[nodeID]
	return conn, ok
}

// SendCommand sends a deployment command to a specific node.
// Returns ALREADY_EXISTS if trying to deploy a deployment that's already running.
func (m *NodeManager) SendCommand(ctx context.Context, nodeID string, cmd *pb.DeploymentCommand) error {
	m.mu.RLock()
	conn, ok := m.connections[nodeID]
	m.mu.RUnlock()

	if !ok {
		return status.Errorf(codes.NotFound, "node %s not connected", nodeID)
	}

	// Check for duplicate deployment (Requirement 3.5)
	if cmd.Type == pb.CommandType_COMMAND_DEPLOY && cmd.GetDeploy() != nil {
		deploymentID := cmd.GetDeploy().DeploymentId
		if conn.HasDeployment(deploymentID) {
			return status.Errorf(codes.AlreadyExists, "deployment %s already running on node %s", deploymentID, nodeID)
		}
	}

	// Check if node is down
	if conn.GetStatus() == NodeStatusDown {
		return status.Errorf(codes.Unavailable, "node %s is down", nodeID)
	}

	// Send the command via the stream
	if err := conn.CommandStream.Send(cmd); err != nil {
		m.logger.Error("failed to send command to node",
			"node_id", nodeID,
			"command_id", cmd.CommandId,
			"error", err,
		)
		return status.Errorf(codes.Internal, "failed to send command: %v", err)
	}

	// Track the deployment if it's a deploy command
	if cmd.Type == pb.CommandType_COMMAND_DEPLOY && cmd.GetDeploy() != nil {
		conn.AddDeployment(cmd.GetDeploy().DeploymentId)
	}

	m.logger.Info("sent command to node",
		"node_id", nodeID,
		"command_id", cmd.CommandId,
		"command_type", cmd.Type.String(),
	)

	return nil
}

// BroadcastCommand sends a command to all connected nodes.
func (m *NodeManager) BroadcastCommand(ctx context.Context, cmd *pb.DeploymentCommand) error {
	m.mu.RLock()
	nodeIDs := make([]string, 0, len(m.connections))
	for nodeID := range m.connections {
		nodeIDs = append(nodeIDs, nodeID)
	}
	m.mu.RUnlock()

	var lastErr error
	for _, nodeID := range nodeIDs {
		if err := m.SendCommand(ctx, nodeID, cmd); err != nil {
			m.logger.Error("failed to broadcast command to node",
				"node_id", nodeID,
				"command_id", cmd.CommandId,
				"error", err,
			)
			lastErr = err
		}
	}

	return lastErr
}

// UpdateHeartbeat updates a node's heartbeat timestamp and info.
func (m *NodeManager) UpdateHeartbeat(nodeID string, info *pb.NodeInfo) {
	m.mu.RLock()
	conn, ok := m.connections[nodeID]
	m.mu.RUnlock()

	if ok {
		conn.UpdateHeartbeat(info)
		m.logger.Debug("updated heartbeat for node", "node_id", nodeID)
	}
}

// StartHealthChecker starts the background health checker goroutine.
func (m *NodeManager) StartHealthChecker(ctx context.Context) {
	m.wg.Add(1)
	go m.healthCheckerLoop(ctx)
}

// Stop stops the NodeManager and waits for goroutines to finish.
func (m *NodeManager) Stop() {
	close(m.stopChan)
	m.wg.Wait()
}

// healthCheckerLoop runs the periodic health check.
func (m *NodeManager) healthCheckerLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("health checker stopped by context")
			return
		case <-m.stopChan:
			m.logger.Info("health checker stopped")
			return
		case <-ticker.C:
			m.checkNodeHealth()
		}
	}
}

// checkNodeHealth checks all nodes and updates their health status.
func (m *NodeManager) checkNodeHealth() {
	m.mu.RLock()
	nodeIDs := make([]string, 0, len(m.connections))
	for nodeID := range m.connections {
		nodeIDs = append(nodeIDs, nodeID)
	}
	m.mu.RUnlock()

	now := time.Now()

	for _, nodeID := range nodeIDs {
		m.mu.RLock()
		conn, ok := m.connections[nodeID]
		m.mu.RUnlock()

		if !ok {
			continue
		}

		lastHeartbeat := conn.GetLastHeartbeat()
		currentStatus := conn.GetStatus()
		timeSinceHeartbeat := now.Sub(lastHeartbeat)

		newStatus := m.CalculateNodeStatus(timeSinceHeartbeat)

		if newStatus != currentStatus {
			conn.SetStatus(newStatus)
			m.logger.Info("node status changed",
				"node_id", nodeID,
				"old_status", currentStatus.String(),
				"new_status", newStatus.String(),
				"time_since_heartbeat", timeSinceHeartbeat,
			)

			// Update health in store if available
			if m.store != nil {
				healthy := newStatus == NodeStatusHealthy
				if err := m.store.Nodes().UpdateHealth(context.Background(), nodeID, healthy); err != nil {
					m.logger.Error("failed to update node health in store",
						"node_id", nodeID,
						"error", err,
					)
				}
			}
		}
	}
}

// CalculateNodeStatus determines the node status based on time since last heartbeat.
// This is exported for testing purposes.
func (m *NodeManager) CalculateNodeStatus(timeSinceHeartbeat time.Duration) NodeStatus {
	if timeSinceHeartbeat >= m.downThreshold {
		return NodeStatusDown
	}
	if timeSinceHeartbeat >= m.degradedThreshold {
		return NodeStatusDegraded
	}
	return NodeStatusHealthy
}

// GetHealthyNodes returns all nodes that are in healthy status.
func (m *NodeManager) GetHealthyNodes() []*NodeConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var healthy []*NodeConnection
	for _, conn := range m.connections {
		if conn.GetStatus() == NodeStatusHealthy {
			healthy = append(healthy, conn)
		}
	}
	return healthy
}

// GetNodeStatus returns the status of a specific node.
func (m *NodeManager) GetNodeStatus(nodeID string) NodeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if conn, ok := m.connections[nodeID]; ok {
		return conn.GetStatus()
	}
	return NodeStatusDown // Unknown nodes are considered down
}

// GetConnectedNodeIDs returns a list of all connected node IDs.
func (m *NodeManager) GetConnectedNodeIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodeIDs := make([]string, 0, len(m.connections))
	for nodeID := range m.connections {
		nodeIDs = append(nodeIDs, nodeID)
	}
	return nodeIDs
}

// ConnectionCount returns the number of active connections.
func (m *NodeManager) ConnectionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connections)
}

// MarkDeploymentComplete marks a deployment as no longer active on a node.
func (m *NodeManager) MarkDeploymentComplete(nodeID, deploymentID string) {
	m.mu.RLock()
	conn, ok := m.connections[nodeID]
	m.mu.RUnlock()

	if ok {
		conn.RemoveDeployment(deploymentID)
	}
}
