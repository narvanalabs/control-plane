// Package grpc provides the gRPC server implementation for the control plane.
package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/narvanalabs/control-plane/api/proto"
	"github.com/narvanalabs/control-plane/internal/auth"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// contextKey is a type for context keys used in this package.
type contextKey string

const (
	// nodeIDKey is the context key for the authenticated node ID.
	nodeIDKey contextKey = "node_id"
)

// Config holds the gRPC server configuration.
type Config struct {
	Port                 int
	TLSCertFile          string
	TLSKeyFile           string
	MaxConcurrentStreams uint32
	KeepaliveTime        time.Duration
	KeepaliveTimeout     time.Duration
	MaxRecvMsgSize       int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Port:                 9090,
		MaxConcurrentStreams: 1000,
		KeepaliveTime:        30 * time.Second,
		KeepaliveTimeout:     10 * time.Second,
		MaxRecvMsgSize:       16 * 1024 * 1024, // 16MB
	}
}

// AuthService defines the interface for authentication operations.
type AuthService interface {
	ValidateToken(tokenString string) (*auth.Claims, error)
}

// HealthChecker defines the interface for checking service health.
type HealthChecker interface {
	CheckPostgres(ctx context.Context) error
}

// Server implements the gRPC server for the control plane.
type Server struct {
	pb.UnimplementedControlPlaneServiceServer
	pb.UnimplementedHealthServer

	config      *Config
	store       store.Store
	authService AuthService
	logger      *slog.Logger

	grpcServer    *grpc.Server
	healthChecker HealthChecker

	// Server state
	serving atomic.Bool
	mu      sync.RWMutex
}

// NewServer creates a new gRPC server instance.
func NewServer(cfg *Config, st store.Store, authSvc AuthService, logger *slog.Logger) (*Server, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		config:      cfg,
		store:       st,
		authService: authSvc,
		logger:      logger,
	}

	return s, nil
}

// SetHealthChecker sets the health checker for the server.
func (s *Server) SetHealthChecker(hc HealthChecker) {
	s.healthChecker = hc
}

// buildServerOptions constructs the gRPC server options.
func (s *Server) buildServerOptions() ([]grpc.ServerOption, error) {
	opts := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(s.config.MaxConcurrentStreams),
		grpc.MaxRecvMsgSize(s.config.MaxRecvMsgSize),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    s.config.KeepaliveTime,
			Timeout: s.config.KeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.ChainUnaryInterceptor(
			s.loggingInterceptor(),
			s.authInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			s.streamLoggingInterceptor(),
			s.streamAuthInterceptor(),
		),
	}

	// Add TLS if configured
	if s.config.TLSCertFile != "" && s.config.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.config.TLSCertFile, s.config.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("loading TLS credentials: %w", err)
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}

	return opts, nil
}

// Start starts the gRPC server.
func (s *Server) Start(ctx context.Context) error {
	opts, err := s.buildServerOptions()
	if err != nil {
		return fmt.Errorf("building server options: %w", err)
	}

	s.grpcServer = grpc.NewServer(opts...)
	pb.RegisterControlPlaneServiceServer(s.grpcServer, s)
	pb.RegisterHealthServer(s.grpcServer, s)

	addr := fmt.Sprintf(":%d", s.config.Port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}

	s.serving.Store(true)
	s.logger.Info("gRPC server starting", "address", addr)

	go func() {
		<-ctx.Done()
		s.Stop(context.Background())
	}()

	if err := s.grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("serving gRPC: %w", err)
	}

	return nil
}

// Stop gracefully stops the gRPC server.
func (s *Server) Stop(ctx context.Context) error {
	s.serving.Store(false)
	s.logger.Info("gRPC server stopping")

	if s.grpcServer == nil {
		return nil
	}

	// Create a channel to signal when graceful stop completes
	done := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(done)
	}()

	// Wait for graceful stop or timeout (30 seconds as per requirements)
	select {
	case <-done:
		s.logger.Info("gRPC server stopped gracefully")
	case <-time.After(30 * time.Second):
		s.logger.Warn("gRPC server graceful stop timed out, forcing stop")
		s.grpcServer.Stop()
	case <-ctx.Done():
		s.logger.Warn("context cancelled, forcing stop")
		s.grpcServer.Stop()
	}

	return nil
}

// IsServing returns whether the server is currently serving requests.
func (s *Server) IsServing() bool {
	return s.serving.Load()
}

// extractToken extracts the auth token from gRPC metadata.
func extractToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing metadata")
	}

	tokens := md.Get("authorization")
	if len(tokens) == 0 {
		return "", status.Error(codes.Unauthenticated, "missing authorization header")
	}

	return auth.ExtractBearerToken(tokens[0]), nil
}

// NodeIDFromContext extracts the node ID from the context.
func NodeIDFromContext(ctx context.Context) (string, bool) {
	nodeID, ok := ctx.Value(nodeIDKey).(string)
	return nodeID, ok
}

// protoNodeInfoToModel converts a proto NodeInfo to a models.Node.
func protoNodeInfoToModel(info *pb.NodeInfo) *models.Node {
	node := &models.Node{
		ID:       info.Id,
		Hostname: info.Hostname,
		Address:  info.Address,
		GRPCPort: int(info.GrpcPort),
	}

	if info.Resources != nil {
		node.Resources = &models.NodeResources{
			CPUTotal:        info.Resources.CpuTotal,
			CPUAvailable:    info.Resources.CpuAvailable,
			MemoryTotal:     info.Resources.MemoryTotal,
			MemoryAvailable: info.Resources.MemoryAvailable,
			DiskTotal:       info.Resources.DiskTotal,
			DiskAvailable:   info.Resources.DiskAvailable,
		}
	}

	node.CachedPaths = info.CachedPaths
	return node
}
