package grpc

import (
	"context"

	pb "github.com/narvanalabs/control-plane/api/proto"
)

// Check implements the gRPC Health Check protocol.
// Requirements: 13.1, 13.2, 13.3, 13.4, 13.5
func (s *Server) Check(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	// Check if server is serving (Requirement 13.2, 13.3, 13.4)
	if !s.serving.Load() {
		return &pb.HealthCheckResponse{
			Status: pb.HealthCheckResponse_NOT_SERVING,
		}, nil
	}

	// Check PostgreSQL connectivity if health checker is configured (Requirement 13.5)
	if s.healthChecker != nil {
		if err := s.healthChecker.CheckPostgres(ctx); err != nil {
			s.logger.Warn("health check failed: postgres unavailable", "error", err)
			return &pb.HealthCheckResponse{
				Status: pb.HealthCheckResponse_NOT_SERVING,
			}, nil
		}
	}

	return &pb.HealthCheckResponse{
		Status: pb.HealthCheckResponse_SERVING,
	}, nil
}

// Watch implements the streaming health check protocol.
// Requirements: 13.1
func (s *Server) Watch(req *pb.HealthCheckRequest, stream pb.Health_WatchServer) error {
	// Send initial status
	status := pb.HealthCheckResponse_SERVING
	if !s.serving.Load() {
		status = pb.HealthCheckResponse_NOT_SERVING
	}

	if err := stream.Send(&pb.HealthCheckResponse{Status: status}); err != nil {
		return err
	}

	// Keep stream open until context is cancelled
	<-stream.Context().Done()
	return nil
}
