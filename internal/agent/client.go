// Package agent provides gRPC client for communicating with node agents.
package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"sync"
	"time"

	pb "github.com/narvanalabs/control-plane/api/proto"
	"github.com/narvanalabs/control-plane/internal/models"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Client defines the interface for communicating with node agents.
type Client interface {
	// Deploy instructs the agent to deploy an artifact on the node.
	Deploy(ctx context.Context, nodeID string, req *DeployRequest) error
	// Stop instructs the agent to stop a running deployment.
	Stop(ctx context.Context, nodeID string, deploymentID string) error
	// GetStatus retrieves the current status of a deployment.
	GetStatus(ctx context.Context, nodeID string, deploymentID string) (*DeploymentStatus, error)
	// StreamLogs streams logs from a deployment in real-time.
	StreamLogs(ctx context.Context, nodeID string, deploymentID string) (<-chan *models.LogEntry, error)
	// Close closes all connections.
	Close() error
}

// DeployRequest contains all information needed to deploy an artifact.
type DeployRequest struct {
	DeploymentID string
	Artifact     string
	BuildType    models.BuildType
	Config       *models.RuntimeConfig
	Secrets      map[string]string
}

// DeploymentStatus contains the current status of a deployment.
type DeploymentStatus struct {
	DeploymentID string
	Status       models.DeploymentStatus
	Message      string
	StartedAt    *time.Time
	UpdatedAt    time.Time
}

// NodeResolver resolves node IDs to their gRPC addresses.
type NodeResolver interface {
	// GetNodeAddress returns the gRPC address for a node.
	GetNodeAddress(ctx context.Context, nodeID string) (string, error)
}


// ClientConfig holds configuration for the gRPC client.
type ClientConfig struct {
	// TLSConfig for secure connections. If nil, insecure connections are used.
	TLSConfig *tls.Config
	// DialTimeout is the timeout for establishing connections.
	DialTimeout time.Duration
	// RequestTimeout is the default timeout for RPC calls.
	RequestTimeout time.Duration
	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int
	// InitialBackoff is the initial backoff duration for retries.
	InitialBackoff time.Duration
	// MaxBackoff is the maximum backoff duration for retries.
	MaxBackoff time.Duration
	// BackoffMultiplier is the multiplier for exponential backoff.
	BackoffMultiplier float64
}

// DefaultClientConfig returns a ClientConfig with sensible defaults.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		TLSConfig:         nil,
		DialTimeout:       10 * time.Second,
		RequestTimeout:    30 * time.Second,
		MaxRetries:        3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        10 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// GRPCClient implements the Client interface using gRPC.
type GRPCClient struct {
	config       *ClientConfig
	resolver     NodeResolver
	connections  map[string]*grpc.ClientConn
	mu           sync.RWMutex
	logger       *slog.Logger
}

// NewGRPCClient creates a new gRPC client for agent communication.
func NewGRPCClient(resolver NodeResolver, config *ClientConfig, logger *slog.Logger) *GRPCClient {
	if config == nil {
		config = DefaultClientConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &GRPCClient{
		config:      config,
		resolver:    resolver,
		connections: make(map[string]*grpc.ClientConn),
		logger:      logger,
	}
}

// getConnection returns an existing connection or creates a new one.
func (c *GRPCClient) getConnection(ctx context.Context, nodeID string) (*grpc.ClientConn, error) {
	// Check for existing connection
	c.mu.RLock()
	conn, exists := c.connections[nodeID]
	c.mu.RUnlock()

	if exists {
		return conn, nil
	}

	// Create new connection
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if conn, exists := c.connections[nodeID]; exists {
		return conn, nil
	}

	// Resolve node address
	address, err := c.resolver.GetNodeAddress(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("resolving node address: %w", err)
	}

	// Set up dial options
	var opts []grpc.DialOption
	if c.config.TLSConfig != nil {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(c.config.TLSConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Create connection with timeout
	dialCtx, cancel := context.WithTimeout(ctx, c.config.DialTimeout)
	defer cancel()

	conn, err = grpc.DialContext(dialCtx, address, opts...)
	if err != nil {
		return nil, fmt.Errorf("dialing node %s at %s: %w", nodeID, address, err)
	}

	c.connections[nodeID] = conn
	c.logger.Info("established connection to node", "node_id", nodeID, "address", address)

	return conn, nil
}


// withRetry executes the given function with exponential backoff retry logic.
func (c *GRPCClient) withRetry(ctx context.Context, operation string, fn func() error) error {
	var lastErr error
	backoff := c.config.InitialBackoff

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			c.logger.Debug("retrying operation",
				"operation", operation,
				"attempt", attempt,
				"backoff", backoff,
			)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			// Calculate next backoff with exponential increase
			backoff = time.Duration(float64(backoff) * c.config.BackoffMultiplier)
			if backoff > c.config.MaxBackoff {
				backoff = c.config.MaxBackoff
			}
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		c.logger.Warn("operation failed",
			"operation", operation,
			"attempt", attempt,
			"error", lastErr,
		)
	}

	return fmt.Errorf("%s failed after %d attempts: %w", operation, c.config.MaxRetries+1, lastErr)
}

// Deploy instructs the agent to deploy an artifact on the node.
func (c *GRPCClient) Deploy(ctx context.Context, nodeID string, req *DeployRequest) error {
	return c.withRetry(ctx, "deploy", func() error {
		conn, err := c.getConnection(ctx, nodeID)
		if err != nil {
			return err
		}

		client := pb.NewAgentServiceClient(conn)

		// Build the protobuf request
		pbReq := &pb.DeployRequest{
			DeploymentId: req.DeploymentID,
			Artifact:     req.Artifact,
			BuildType:    string(req.BuildType),
			Secrets:      req.Secrets,
		}

		if req.Config != nil {
			pbReq.Config = convertRuntimeConfigToProto(req.Config)
		}

		// Make the RPC call with timeout
		callCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
		defer cancel()

		resp, err := client.Deploy(callCtx, pbReq)
		if err != nil {
			return fmt.Errorf("deploy RPC: %w", err)
		}

		if !resp.Success {
			return fmt.Errorf("deploy failed: %s", resp.Message)
		}

		c.logger.Info("deployment initiated",
			"node_id", nodeID,
			"deployment_id", req.DeploymentID,
		)

		return nil
	})
}

// Stop instructs the agent to stop a running deployment.
func (c *GRPCClient) Stop(ctx context.Context, nodeID string, deploymentID string) error {
	return c.withRetry(ctx, "stop", func() error {
		conn, err := c.getConnection(ctx, nodeID)
		if err != nil {
			return err
		}

		client := pb.NewAgentServiceClient(conn)

		callCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
		defer cancel()

		resp, err := client.Stop(callCtx, &pb.StopRequest{
			DeploymentId: deploymentID,
		})
		if err != nil {
			return fmt.Errorf("stop RPC: %w", err)
		}

		if !resp.Success {
			return fmt.Errorf("stop failed: %s", resp.Message)
		}

		c.logger.Info("deployment stopped",
			"node_id", nodeID,
			"deployment_id", deploymentID,
		)

		return nil
	})
}


// GetStatus retrieves the current status of a deployment.
func (c *GRPCClient) GetStatus(ctx context.Context, nodeID string, deploymentID string) (*DeploymentStatus, error) {
	var result *DeploymentStatus

	err := c.withRetry(ctx, "get_status", func() error {
		conn, err := c.getConnection(ctx, nodeID)
		if err != nil {
			return err
		}

		client := pb.NewAgentServiceClient(conn)

		callCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
		defer cancel()

		resp, err := client.GetStatus(callCtx, &pb.GetStatusRequest{
			DeploymentId: deploymentID,
		})
		if err != nil {
			return fmt.Errorf("get_status RPC: %w", err)
		}

		result = &DeploymentStatus{
			DeploymentID: resp.DeploymentId,
			Status:       models.DeploymentStatus(resp.Status),
			Message:      resp.Message,
			UpdatedAt:    time.Unix(resp.UpdatedAt, 0),
		}

		if resp.StartedAt > 0 {
			startedAt := time.Unix(resp.StartedAt, 0)
			result.StartedAt = &startedAt
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// StreamLogs streams logs from a deployment in real-time.
func (c *GRPCClient) StreamLogs(ctx context.Context, nodeID string, deploymentID string) (<-chan *models.LogEntry, error) {
	conn, err := c.getConnection(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	client := pb.NewAgentServiceClient(conn)

	stream, err := client.StreamLogs(ctx, &pb.StreamLogsRequest{
		DeploymentId: deploymentID,
		Follow:       true,
		TailLines:    100,
	})
	if err != nil {
		return nil, fmt.Errorf("stream_logs RPC: %w", err)
	}

	logChan := make(chan *models.LogEntry, 100)

	go func() {
		defer close(logChan)

		for {
			entry, err := stream.Recv()
			if err != nil {
				c.logger.Debug("log stream ended",
					"node_id", nodeID,
					"deployment_id", deploymentID,
					"error", err,
				)
				return
			}

			logEntry := &models.LogEntry{
				DeploymentID: entry.DeploymentId,
				Source:       entry.Source,
				Level:        entry.Level,
				Message:      entry.Message,
				Timestamp:    time.Unix(0, entry.Timestamp),
			}

			select {
			case logChan <- logEntry:
			case <-ctx.Done():
				return
			}
		}
	}()

	return logChan, nil
}

// Close closes all connections.
func (c *GRPCClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var lastErr error
	for nodeID, conn := range c.connections {
		if err := conn.Close(); err != nil {
			c.logger.Error("failed to close connection",
				"node_id", nodeID,
				"error", err,
			)
			lastErr = err
		}
	}

	c.connections = make(map[string]*grpc.ClientConn)
	return lastErr
}

// RemoveConnection removes a connection from the pool (e.g., when a node becomes unhealthy).
func (c *GRPCClient) RemoveConnection(nodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if conn, exists := c.connections[nodeID]; exists {
		conn.Close()
		delete(c.connections, nodeID)
		c.logger.Info("removed connection to node", "node_id", nodeID)
	}
}


// convertRuntimeConfigToProto converts a models.RuntimeConfig to protobuf.
func convertRuntimeConfigToProto(cfg *models.RuntimeConfig) *pb.RuntimeConfig {
	if cfg == nil {
		return nil
	}

	pbCfg := &pb.RuntimeConfig{
		ResourceTier: string(cfg.ResourceTier),
		EnvVars:      cfg.EnvVars,
	}

	// Convert ports
	if len(cfg.Ports) > 0 {
		pbCfg.Ports = make([]*pb.PortMapping, len(cfg.Ports))
		for i, p := range cfg.Ports {
			pbCfg.Ports[i] = &pb.PortMapping{
				ContainerPort: int32(p.ContainerPort),
				Protocol:      p.Protocol,
			}
		}
	}

	// Convert health check
	if cfg.HealthCheck != nil {
		pbCfg.HealthCheck = &pb.HealthCheckConfig{
			Path:            cfg.HealthCheck.Path,
			Port:            int32(cfg.HealthCheck.Port),
			IntervalSeconds: int32(cfg.HealthCheck.IntervalSeconds),
			TimeoutSeconds:  int32(cfg.HealthCheck.TimeoutSeconds),
			Retries:         int32(cfg.HealthCheck.Retries),
		}
	}

	return pbCfg
}

// StoreNodeResolver implements NodeResolver using the store.
type StoreNodeResolver struct {
	store interface {
		Nodes() interface {
			Get(ctx context.Context, id string) (*models.Node, error)
		}
	}
}

// NewStoreNodeResolver creates a NodeResolver that uses the store to look up nodes.
func NewStoreNodeResolver(store interface {
	Nodes() interface {
		Get(ctx context.Context, id string) (*models.Node, error)
	}
}) *StoreNodeResolver {
	return &StoreNodeResolver{store: store}
}

// GetNodeAddress returns the gRPC address for a node.
func (r *StoreNodeResolver) GetNodeAddress(ctx context.Context, nodeID string) (string, error) {
	node, err := r.store.Nodes().Get(ctx, nodeID)
	if err != nil {
		return "", fmt.Errorf("getting node: %w", err)
	}
	if node == nil {
		return "", fmt.Errorf("node not found: %s", nodeID)
	}

	return fmt.Sprintf("%s:%d", node.Address, node.GRPCPort), nil
}

// ValidateDeployRequest checks that a DeployRequest has all required fields.
// This is used for property testing to ensure deployment commands are complete.
func ValidateDeployRequest(req *DeployRequest) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if req.DeploymentID == "" {
		return fmt.Errorf("deployment_id is required")
	}
	if req.Artifact == "" {
		return fmt.Errorf("artifact is required")
	}
	if req.BuildType == "" {
		return fmt.Errorf("build_type is required")
	}
	if req.Config == nil {
		return fmt.Errorf("config is required")
	}
	return nil
}
