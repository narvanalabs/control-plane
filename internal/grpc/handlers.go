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
// Requirements: 2.1, 2.2, 2.3
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

	// Log container ID if provided (Requirement 5.3)
	if req.ContainerId != "" {
		s.logger.Debug("container ID reported",
			"deployment_id", req.DeploymentId,
			"container_id", req.ContainerId)
	}

	// Log error message if status is FAILED (Requirement 5.2, 5.5)
	if req.Status == pb.DeploymentStatus_STATUS_FAILED && req.ErrorMessage != "" {
		s.logger.Error("deployment failed",
			"deployment_id", req.DeploymentId,
			"error_message", req.ErrorMessage,
			"exit_code", req.ExitCode)
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

// PushLogs handles log streaming from nodes.
// Requirements: 4.2, 4.4
func (s *Server) PushLogs(stream pb.ControlPlaneService_PushLogsServer) error {
	var entriesReceived int64

	for {
		entry, err := stream.Recv()
		if err == io.EOF {
			// End of stream
			break
		}
		if err != nil {
			s.logger.Error("error receiving log entry", "error", err)
			return status.Error(codes.Internal, "error receiving log entry")
		}

		// Store the log entry
		logEntry := &models.LogEntry{
			DeploymentID: entry.DeploymentId,
			Source:       "runtime",
			Message:      entry.Message,
			Timestamp:    time.Now(),
		}

		if err := s.store.Logs().Create(stream.Context(), logEntry); err != nil {
			s.logger.Error("failed to store log entry",
				"deployment_id", entry.DeploymentId,
				"error", err)
			// Continue processing other entries
		}

		entriesReceived++
	}

	return stream.SendAndClose(&pb.PushLogsResponse{
		EntriesReceived: entriesReceived,
	})
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
