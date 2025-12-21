package grpc

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/narvanalabs/control-plane/api/proto"
	"github.com/narvanalabs/control-plane/internal/models"
)

// Register handles node registration requests.
// Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6
func (s *Server) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if req.NodeInfo == nil {
		return nil, status.Error(codes.InvalidArgument, "node_info is required")
	}

	info := req.NodeInfo

	// Validate required fields
	if info.Hostname == "" {
		return nil, status.Error(codes.InvalidArgument, "hostname is required")
	}
	if info.Address == "" {
		return nil, status.Error(codes.InvalidArgument, "address is required")
	}
	if info.GrpcPort <= 0 || info.GrpcPort > 65535 {
		return nil, status.Error(codes.InvalidArgument, "invalid grpc_port")
	}

	// Assign ID if not provided (Requirement 1.4)
	nodeID := info.Id
	if nodeID == "" {
		nodeID = uuid.New().String()
	}

	// Convert proto to model
	node := protoNodeInfoToModel(info)
	node.ID = nodeID
	node.Healthy = true
	node.LastHeartbeat = time.Now()
	node.RegisteredAt = time.Now()

	// Register or update node in store (Requirement 1.2, 1.6)
	if err := s.store.Nodes().Register(ctx, node); err != nil {
		s.logger.Error("failed to register node", "node_id", nodeID, "error", err)
		return nil, status.Error(codes.Internal, "failed to register node")
	}

	s.logger.Info("node registered", "node_id", nodeID, "hostname", info.Hostname)

	// Return success with config (Requirement 1.5)
	return &pb.RegisterResponse{
		Success: true,
		NodeId:  nodeID,
		Message: "registration successful",
		Config: &pb.NodeConfig{
			HeartbeatIntervalSeconds: 10,
			MaxConcurrentDeployments: 10,
			LogBufferSize:            1000,
		},
	}, nil
}

// Heartbeat handles heartbeat requests from nodes.
// Requirements: 2.1, 2.2, 2.3, 15.5
func (s *Server) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	// Verify node exists
	node, err := s.store.Nodes().Get(ctx, req.NodeId)
	if err != nil {
		s.logger.Error("failed to get node", "node_id", req.NodeId, "error", err)
		return nil, status.Error(codes.NotFound, "node not found")
	}
	if node == nil {
		return nil, status.Error(codes.NotFound, "node not found")
	}

	// Extract resource metrics (Requirement 2.2)
	var resources *models.NodeResources
	if req.NodeInfo != nil && req.NodeInfo.Resources != nil {
		resources = &models.NodeResources{
			CPUTotal:        req.NodeInfo.Resources.CpuTotal,
			CPUAvailable:    req.NodeInfo.Resources.CpuAvailable,
			MemoryTotal:     req.NodeInfo.Resources.MemoryTotal,
			MemoryAvailable: req.NodeInfo.Resources.MemoryAvailable,
			DiskTotal:       req.NodeInfo.Resources.DiskTotal,
			DiskAvailable:   req.NodeInfo.Resources.DiskAvailable,
		}
	}

	// Update heartbeat timestamp and metrics (Requirement 2.1, 2.2)
	if err := s.store.Nodes().UpdateHeartbeat(ctx, req.NodeId, resources); err != nil {
		s.logger.Error("failed to update heartbeat", "node_id", req.NodeId, "error", err)
		return nil, status.Error(codes.Internal, "failed to update heartbeat")
	}

	// Handle draining flag (Requirement 15.5)
	// When a node sends a draining heartbeat, mark it as draining in the node manager
	if req.Draining {
		s.logger.Info("node is draining", "node_id", req.NodeId)
		if s.nodeManager != nil {
			s.nodeManager.SetNodeDraining(req.NodeId, true)
		}
	}

	// Update cached paths if provided (Requirement 2.3)
	// Note: This would require extending the NodeStore interface
	// For now, we log the cached paths
	if req.NodeInfo != nil && len(req.NodeInfo.CachedPaths) > 0 {
		s.logger.Debug("node cached paths updated",
			"node_id", req.NodeId,
			"cached_paths_count", len(req.NodeInfo.CachedPaths))
	}

	return &pb.HeartbeatResponse{
		Success: true,
	}, nil
}

// ReportStatus handles deployment status reports from nodes.
// Requirements: 5.1, 5.2, 5.3, 5.4
func (s *Server) ReportStatus(ctx context.Context, req *pb.StatusReport) (*pb.StatusResponse, error) {
	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}
	if req.DeploymentId == "" {
		return nil, status.Error(codes.InvalidArgument, "deployment_id is required")
	}

	// Validate status is a known value (Requirement 5.4)
	if !isValidDeploymentStatus(req.Status) {
		return nil, status.Error(codes.InvalidArgument, "invalid deployment status")
	}

	// Get the deployment
	deployment, err := s.store.Deployments().Get(ctx, req.DeploymentId)
	if err != nil {
		s.logger.Error("failed to get deployment", "deployment_id", req.DeploymentId, "error", err)
		return nil, status.Error(codes.NotFound, "deployment not found")
	}
	if deployment == nil {
		return nil, status.Error(codes.NotFound, "deployment not found")
	}

	// Map proto status to model status
	statusStr := mapProtoStatusToModel(req.Status)
	deployment.Status = models.DeploymentStatus(statusStr)

	// Update started_at timestamp when deployment starts running (Requirement 5.3)
	if req.Status == pb.DeploymentStatus_STATUS_RUNNING && deployment.StartedAt == nil {
		now := time.Now()
		// Use the timestamp from the request if provided, otherwise use current time
		if req.StartedAt != nil && req.StartedAt.IsValid() {
			startTime := req.StartedAt.AsTime()
			deployment.StartedAt = &startTime
		} else {
			deployment.StartedAt = &now
		}
	}

	// Update finished_at timestamp when deployment stops or fails
	if req.Status == pb.DeploymentStatus_STATUS_STOPPED || req.Status == pb.DeploymentStatus_STATUS_FAILED {
		if deployment.FinishedAt == nil {
			now := time.Now()
			deployment.FinishedAt = &now
		}
	}

	// Log container ID if provided (Requirement 5.3)
	if req.ContainerId != "" {
		s.logger.Debug("container ID reported",
			"deployment_id", req.DeploymentId,
			"container_id", req.ContainerId)
	}

	// Log error message if status is FAILED (Requirement 5.2, 5.5)
	if req.Status == pb.DeploymentStatus_STATUS_FAILED {
		s.logger.Error("deployment failed",
			"deployment_id", req.DeploymentId,
			"error_message", req.ErrorMessage,
			"exit_code", req.ExitCode,
			"node_id", req.NodeId)
	}

	// Update the deployment in store
	if err := s.store.Deployments().Update(ctx, deployment); err != nil {
		s.logger.Error("failed to update deployment status",
			"deployment_id", req.DeploymentId,
			"status", statusStr,
			"error", err)
		return nil, status.Error(codes.Internal, "failed to update deployment status")
	}

	s.logger.Info("deployment status updated",
		"deployment_id", req.DeploymentId,
		"node_id", req.NodeId,
		"status", statusStr)

	return &pb.StatusResponse{
		Acknowledged: true,
	}, nil
}

// isValidDeploymentStatus checks if the status is a valid deployment status.
// Supports: PENDING, PULLING, STARTING, RUNNING, STOPPING, STOPPED, FAILED, UNKNOWN
// (Requirement 5.4)
func isValidDeploymentStatus(status pb.DeploymentStatus) bool {
	switch status {
	case pb.DeploymentStatus_STATUS_UNKNOWN,
		pb.DeploymentStatus_STATUS_PENDING,
		pb.DeploymentStatus_STATUS_PULLING,
		pb.DeploymentStatus_STATUS_STARTING,
		pb.DeploymentStatus_STATUS_RUNNING,
		pb.DeploymentStatus_STATUS_STOPPING,
		pb.DeploymentStatus_STATUS_STOPPED,
		pb.DeploymentStatus_STATUS_FAILED:
		return true
	default:
		return false
	}
}

// PushLogs handles log streaming from nodes.
// Requirements: 4.2, 4.4
func (s *Server) PushLogs(stream pb.ControlPlaneService_PushLogsServer) error {
	var entriesReceived int64
	var lastDeploymentID string
	var lastStreamID string

	s.logger.Debug("starting log stream")

	for {
		entry, err := stream.Recv()
		if err == io.EOF {
			// End of stream (Requirement 4.3 - graceful cleanup)
			s.logger.Debug("log stream ended",
				"entries_received", entriesReceived,
				"deployment_id", lastDeploymentID,
				"stream_id", lastStreamID)
			break
		}
		if err != nil {
			s.logger.Error("error receiving log entry", "error", err)
			return status.Error(codes.Internal, "error receiving log entry")
		}

		// Track the deployment and stream for logging
		lastDeploymentID = entry.DeploymentId
		lastStreamID = entry.StreamId

		// Validate required fields
		if entry.DeploymentId == "" {
			s.logger.Warn("log entry missing deployment_id, skipping")
			continue
		}

		// Determine timestamp - use entry timestamp if provided, otherwise current time
		var timestamp time.Time
		if entry.Timestamp != nil && entry.Timestamp.IsValid() {
			timestamp = entry.Timestamp.AsTime()
		} else {
			timestamp = time.Now()
		}

		// Map log level from proto to string
		level := mapProtoLogLevelToString(entry.Level)

		// Generate a unique ID for the log entry
		logID := uuid.New().String()

		// Create the log entry model (Requirement 4.2)
		logEntry := &models.LogEntry{
			ID:           logID,
			DeploymentID: entry.DeploymentId,
			Source:       "runtime",
			Level:        level,
			Message:      entry.Message,
			Timestamp:    timestamp,
		}

		// Store the log entry in the database (Requirement 4.4)
		if err := s.store.Logs().Create(stream.Context(), logEntry); err != nil {
			s.logger.Error("failed to store log entry",
				"deployment_id", entry.DeploymentId,
				"stream_id", entry.StreamId,
				"error", err)
			// Continue processing other entries - don't fail the entire stream
			continue
		}

		entriesReceived++

		// Log periodically for debugging
		if entriesReceived%100 == 0 {
			s.logger.Debug("log entries received",
				"count", entriesReceived,
				"deployment_id", entry.DeploymentId)
		}
	}

	s.logger.Info("log stream completed",
		"entries_received", entriesReceived,
		"deployment_id", lastDeploymentID)

	return stream.SendAndClose(&pb.PushLogsResponse{
		EntriesReceived: entriesReceived,
	})
}

// mapProtoLogLevelToString converts a proto log level to a string.
func mapProtoLogLevelToString(level pb.CPLogLevel) string {
	switch level {
	case pb.CPLogLevel_CP_LOG_DEBUG:
		return "debug"
	case pb.CPLogLevel_CP_LOG_INFO:
		return "info"
	case pb.CPLogLevel_CP_LOG_WARN:
		return "warn"
	case pb.CPLogLevel_CP_LOG_ERROR:
		return "error"
	default:
		return "unknown"
	}
}

// WatchCommands handles the command streaming to nodes.
// Requirements: 3.1, 3.2, 3.3, 3.4
func (s *Server) WatchCommands(req *pb.WatchCommandsRequest, stream pb.ControlPlaneService_WatchCommandsServer) error {
	if req.NodeId == "" {
		return status.Error(codes.InvalidArgument, "node_id is required")
	}

	nodeID := req.NodeId
	s.logger.Info("node watching for commands", "node_id", nodeID)

	// Register the node's command stream with the NodeManager
	if s.nodeManager != nil {
		if err := s.nodeManager.RegisterConnection(nodeID, stream); err != nil {
			s.logger.Error("failed to register node connection",
				"node_id", nodeID,
				"error", err,
			)
			return status.Error(codes.Internal, "failed to register connection")
		}

		// Ensure we unregister when the stream closes
		defer func() {
			s.nodeManager.UnregisterConnection(nodeID)
			s.logger.Info("node command stream closed", "node_id", nodeID)
		}()
	}

	// Keep the stream open until context is cancelled
	// Commands are sent via NodeManager.SendCommand()
	<-stream.Context().Done()

	return nil
}

// mapProtoStatusToModel converts a proto DeploymentStatus to a string.
func mapProtoStatusToModel(status pb.DeploymentStatus) string {
	switch status {
	case pb.DeploymentStatus_STATUS_PENDING:
		return "pending"
	case pb.DeploymentStatus_STATUS_PULLING:
		return "pulling"
	case pb.DeploymentStatus_STATUS_STARTING:
		return "starting"
	case pb.DeploymentStatus_STATUS_RUNNING:
		return "running"
	case pb.DeploymentStatus_STATUS_STOPPING:
		return "stopping"
	case pb.DeploymentStatus_STATUS_STOPPED:
		return "stopped"
	case pb.DeploymentStatus_STATUS_FAILED:
		return "failed"
	default:
		return "unknown"
	}
}
