// Package scheduler provides intelligent deployment scheduling for the control plane.
package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/narvanalabs/control-plane/api/proto"
	"github.com/narvanalabs/control-plane/internal/models"
)

// CommandSender defines the interface for sending commands to nodes.
// This is implemented by NodeManager.
type CommandSender interface {
	SendCommand(ctx context.Context, nodeID string, cmd *pb.DeploymentCommand) error
}

// GRPCAgentClient implements the AgentClient interface using gRPC via NodeManager.
// It sends deployment commands through the persistent WatchCommands stream.
// Requirements: 3.1, 11.1, 11.2, 11.3
type GRPCAgentClient struct {
	commandSender CommandSender
	// commandTimeout is the deadline for command acknowledgment (Requirement 11.3)
	commandTimeout time.Duration
	// maxRetries is the maximum number of retry attempts (Requirement 9.3)
	maxRetries int
}

// GRPCAgentClientConfig holds configuration for the GRPCAgentClient.
type GRPCAgentClientConfig struct {
	// CommandTimeout is the deadline for command acknowledgment (default: 10s)
	CommandTimeout time.Duration
	// MaxRetries is the maximum number of retry attempts (default: 3)
	MaxRetries int
}

// DefaultGRPCAgentClientConfig returns default configuration values.
func DefaultGRPCAgentClientConfig() *GRPCAgentClientConfig {
	return &GRPCAgentClientConfig{
		CommandTimeout: 10 * time.Second,
		MaxRetries:     3,
	}
}

// NewGRPCAgentClient creates a new GRPCAgentClient.
func NewGRPCAgentClient(sender CommandSender, cfg *GRPCAgentClientConfig) *GRPCAgentClient {
	if cfg == nil {
		cfg = DefaultGRPCAgentClientConfig()
	}
	return &GRPCAgentClient{
		commandSender:  sender,
		commandTimeout: cfg.CommandTimeout,
		maxRetries:     cfg.MaxRetries,
	}
}

// Deploy sends a deployment command to the specified node via gRPC.
// Requirements: 3.1, 3.2, 11.1, 11.2, 11.3
func (c *GRPCAgentClient) Deploy(ctx context.Context, nodeID string, deployment *models.Deployment) error {
	if deployment == nil {
		return fmt.Errorf("deployment is nil")
	}

	// Build the deployment command (Requirement 3.2)
	cmd := c.buildDeployCommand(deployment)

	// Set deadline for acknowledgment (Requirement 11.3)
	deadline := time.Now().Add(c.commandTimeout)
	cmd.Deadline = timestamppb.New(deadline)

	// Send command with retry logic (Requirement 9.3)
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		err := c.commandSender.SendCommand(ctx, nodeID, cmd)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable (Requirement 9.1, 9.2)
		if !isRetryableError(err) {
			return fmt.Errorf("deploy command failed: %w", err)
		}

		// Don't retry if context is cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	// Mark deployment as failed after exhausting retries (Requirement 9.3)
	return fmt.Errorf("deploy command failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// Stop sends a stop command to the specified node via gRPC.
// Requirements: 3.3
func (c *GRPCAgentClient) Stop(ctx context.Context, nodeID string, deploymentID string) error {
	if deploymentID == "" {
		return fmt.Errorf("deployment_id is required")
	}

	cmd := &pb.DeploymentCommand{
		CommandId: uuid.New().String(),
		Type:      pb.CommandType_COMMAND_STOP,
		Deadline:  timestamppb.New(time.Now().Add(c.commandTimeout)),
		Command: &pb.DeploymentCommand_Stop{
			Stop: &pb.CPStopRequest{
				DeploymentId:   deploymentID,
				Force:          false,
				TimeoutSeconds: 30,
			},
		},
	}

	// Send command with retry logic
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		err := c.commandSender.SendCommand(ctx, nodeID, cmd)
		if err == nil {
			return nil
		}

		lastErr = err

		if !isRetryableError(err) {
			return fmt.Errorf("stop command failed: %w", err)
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return fmt.Errorf("stop command failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// buildDeployCommand creates a DeploymentCommand from a Deployment model.
// Requirements: 3.2, 3.4
func (c *GRPCAgentClient) buildDeployCommand(deployment *models.Deployment) *pb.DeploymentCommand {
	// Map build type to proto enum
	buildType := pb.CPBuildType_CP_BUILD_TYPE_UNKNOWN
	switch deployment.BuildType {
	case models.BuildTypeOCI:
		buildType = pb.CPBuildType_CP_BUILD_TYPE_OCI
	case models.BuildTypePureNix:
		buildType = pb.CPBuildType_CP_BUILD_TYPE_NIX
	}

	// Build deployment config
	config := &pb.CPDeploymentConfig{
		DependsOn: deployment.DependsOn,
	}

	// Add resources if available
	if deployment.Resources != nil {
		config.Resources = &pb.CPResourceSpec{
			Cpu:    deployment.Resources.CPU,
			Memory: deployment.Resources.Memory,
		}
	}

	// Add runtime config if available
	if deployment.Config != nil {
		config.EnvVars = deployment.Config.EnvVars
		if deployment.Config.HealthCheck != nil {
			config.HealthCheck = &pb.CPHealthCheckConfig{
				Path:            deployment.Config.HealthCheck.Path,
				Port:            int32(deployment.Config.HealthCheck.Port),
				IntervalSeconds: int32(deployment.Config.HealthCheck.IntervalSeconds),
				TimeoutSeconds:  int32(deployment.Config.HealthCheck.TimeoutSeconds),
			}
		}
		if len(deployment.Config.Ports) > 0 {
			config.Port = int32(deployment.Config.Ports[0].ContainerPort)
		}
	}

	return &pb.DeploymentCommand{
		CommandId: uuid.New().String(),
		Type:      pb.CommandType_COMMAND_DEPLOY,
		Command: &pb.DeploymentCommand_Deploy{
			Deploy: &pb.CPDeployRequest{
				DeploymentId: deployment.ID,
				AppId:        deployment.AppID,
				ServiceName:  deployment.ServiceName,
				Artifact:     deployment.Artifact,
				BuildType:    buildType,
				Config:       config,
				Version:      int32(deployment.Version),
			},
		},
	}
}

// isRetryableError checks if an error should trigger a retry.
// Requirements: 9.1, 9.2
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	st, ok := status.FromError(err)
	if !ok {
		// Non-gRPC errors are generally retryable
		return true
	}

	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded:
		// Retryable errors (Requirement 9.1)
		return true
	case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists, codes.PermissionDenied, codes.Unauthenticated:
		// Non-retryable errors (Requirement 9.2)
		return false
	default:
		// Other errors may be retryable
		return true
	}
}

// BuildDeployCommand is exported for testing purposes.
// It creates a DeploymentCommand from a Deployment model.
func BuildDeployCommand(deployment *models.Deployment) *pb.DeploymentCommand {
	client := &GRPCAgentClient{}
	return client.buildDeployCommand(deployment)
}

// IsRetryableError is exported for testing purposes.
func IsRetryableError(err error) bool {
	return isRetryableError(err)
}
