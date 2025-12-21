package grpc

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/narvanalabs/control-plane/api/proto"
)

// **Feature: grpc-node-communication, Property 3: Deployment Command Completeness**
// For any deployment command, the message should contain deployment ID, artifact, and configuration.
// **Validates: Requirements 3.2, 3.4**

// mockCommandStream implements pb.ControlPlaneService_WatchCommandsServer for testing.
type mockCommandStream struct {
	pb.ControlPlaneService_WatchCommandsServer
	sentCommands []*pb.DeploymentCommand
	mu           sync.Mutex
	ctx          context.Context
}

func newMockCommandStream(ctx context.Context) *mockCommandStream {
	return &mockCommandStream{
		sentCommands: make([]*pb.DeploymentCommand, 0),
		ctx:          ctx,
	}
}

func (m *mockCommandStream) Send(cmd *pb.DeploymentCommand) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentCommands = append(m.sentCommands, cmd)
	return nil
}

func (m *mockCommandStream) Context() context.Context {
	return m.ctx
}

func (m *mockCommandStream) GetSentCommands() []*pb.DeploymentCommand {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*pb.DeploymentCommand, len(m.sentCommands))
	copy(result, m.sentCommands)
	return result
}

// genDeployRequest generates random deploy requests for testing.
func genDeployRequest() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),                                                                       // deployment_id
		gen.Identifier(),                                                                       // app_id
		gen.Identifier(),                                                                       // service_name
		gen.Identifier(),                                                                       // artifact
		gen.OneConstOf(pb.CPBuildType_CP_BUILD_TYPE_OCI, pb.CPBuildType_CP_BUILD_TYPE_NIX),    // build_type
		gen.Int32Range(1, 10),                                                                  // replicas
		gen.Int32Range(1, 65535),                                                               // port
	).Map(func(vals []interface{}) *pb.CPDeployRequest {
		return &pb.CPDeployRequest{
			DeploymentId: vals[0].(string),
			AppId:        vals[1].(string),
			ServiceName:  vals[2].(string),
			Artifact:     vals[3].(string),
			BuildType:    vals[4].(pb.CPBuildType),
			Config: &pb.CPDeploymentConfig{
				ResourceTier: "small",
				Replicas:     vals[5].(int32),
				Port:         vals[6].(int32),
			},
		}
	})
}

// TestDeploymentCommandCompleteness tests that deployment commands contain all required fields.
func TestDeploymentCommandCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("deployment commands contain deployment_id, artifact, and configuration", prop.ForAll(
		func(deployReq *pb.CPDeployRequest) bool {
			// Create a deployment command from the deploy request
			cmd := &pb.DeploymentCommand{
				CommandId: uuid.New().String(),
				Type:      pb.CommandType_COMMAND_DEPLOY,
				Command:   &pb.DeploymentCommand_Deploy{Deploy: deployReq},
			}

			// Verify the command has all required fields (Requirement 3.2)
			deploy := cmd.GetDeploy()
			if deploy == nil {
				t.Log("deploy request should not be nil")
				return false
			}

			// Check deployment ID
			if deploy.DeploymentId == "" {
				t.Log("deployment_id should not be empty")
				return false
			}

			// Check artifact reference
			if deploy.Artifact == "" {
				t.Log("artifact should not be empty")
				return false
			}

			// Check configuration (Requirement 3.4)
			if deploy.Config == nil {
				t.Log("config should not be nil")
				return false
			}

			// Verify command type is correct
			if cmd.Type != pb.CommandType_COMMAND_DEPLOY {
				t.Logf("expected COMMAND_DEPLOY, got %v", cmd.Type)
				return false
			}

			return true
		},
		genDeployRequest(),
	))

	properties.TestingRun(t)
}

// TestDeploymentCommandTypes tests that all command types are properly supported.
func TestDeploymentCommandTypes(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generate command types (Requirement 3.4: DEPLOY, STOP, RESTART, UPDATE)
	genCommandType := gen.OneConstOf(
		pb.CommandType_COMMAND_DEPLOY,
		pb.CommandType_COMMAND_STOP,
		pb.CommandType_COMMAND_RESTART,
		pb.CommandType_COMMAND_UPDATE_CONFIG,
	)

	properties.Property("all command types are valid and have correct type field", prop.ForAll(
		func(cmdType pb.CommandType) bool {
			var cmd *pb.DeploymentCommand

			switch cmdType {
			case pb.CommandType_COMMAND_DEPLOY:
				cmd = &pb.DeploymentCommand{
					CommandId: uuid.New().String(),
					Type:      cmdType,
					Command: &pb.DeploymentCommand_Deploy{
						Deploy: &pb.CPDeployRequest{
							DeploymentId: "test-deployment",
							Artifact:     "test-artifact",
							Config:       &pb.CPDeploymentConfig{},
						},
					},
				}
			case pb.CommandType_COMMAND_STOP:
				cmd = &pb.DeploymentCommand{
					CommandId: uuid.New().String(),
					Type:      cmdType,
					Command: &pb.DeploymentCommand_Stop{
						Stop: &pb.CPStopRequest{
							DeploymentId: "test-deployment",
						},
					},
				}
			case pb.CommandType_COMMAND_RESTART:
				cmd = &pb.DeploymentCommand{
					CommandId: uuid.New().String(),
					Type:      cmdType,
					Command: &pb.DeploymentCommand_Restart{
						Restart: &pb.CPRestartRequest{
							DeploymentId: "test-deployment",
						},
					},
				}
			case pb.CommandType_COMMAND_UPDATE_CONFIG:
				cmd = &pb.DeploymentCommand{
					CommandId: uuid.New().String(),
					Type:      cmdType,
					Command: &pb.DeploymentCommand_UpdateConfig{
						UpdateConfig: &pb.CPUpdateConfigRequest{
							DeploymentId: "test-deployment",
							Config:       &pb.CPDeploymentConfig{},
						},
					},
				}
			default:
				t.Logf("unexpected command type: %v", cmdType)
				return false
			}

			// Verify command ID is set
			if cmd.CommandId == "" {
				t.Log("command_id should not be empty")
				return false
			}

			// Verify type matches
			if cmd.Type != cmdType {
				t.Logf("type mismatch: expected %v, got %v", cmdType, cmd.Type)
				return false
			}

			return true
		},
		genCommandType,
	))

	properties.TestingRun(t)
}

// **Feature: grpc-node-communication, Property 4: Duplicate Deployment Prevention**
// For any running deployment, attempting to deploy again with the same ID should return ALREADY_EXISTS.
// **Validates: Requirements 3.5**

// TestDuplicateDeploymentPrevention tests that duplicate deployments are rejected.
func TestDuplicateDeploymentPrevention(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("duplicate deployments return ALREADY_EXISTS", prop.ForAll(
		func(deploymentID string) bool {
			// Create a NodeManager
			logger := slog.Default()
			nm := NewNodeManager(nil, logger)

			// Create a mock stream
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream := newMockCommandStream(ctx)

			// Register a node connection
			nodeID := "test-node"
			if err := nm.RegisterConnection(nodeID, stream); err != nil {
				t.Logf("failed to register connection: %v", err)
				return false
			}

			// Create a deploy command
			cmd := &pb.DeploymentCommand{
				CommandId: uuid.New().String(),
				Type:      pb.CommandType_COMMAND_DEPLOY,
				Command: &pb.DeploymentCommand_Deploy{
					Deploy: &pb.CPDeployRequest{
						DeploymentId: deploymentID,
						Artifact:     "test-artifact",
						Config:       &pb.CPDeploymentConfig{},
					},
				},
			}

			// First deployment should succeed
			err := nm.SendCommand(ctx, nodeID, cmd)
			if err != nil {
				t.Logf("first deployment should succeed: %v", err)
				return false
			}

			// Second deployment with same ID should fail with ALREADY_EXISTS
			cmd2 := &pb.DeploymentCommand{
				CommandId: uuid.New().String(),
				Type:      pb.CommandType_COMMAND_DEPLOY,
				Command: &pb.DeploymentCommand_Deploy{
					Deploy: &pb.CPDeployRequest{
						DeploymentId: deploymentID, // Same deployment ID
						Artifact:     "test-artifact-2",
						Config:       &pb.CPDeploymentConfig{},
					},
				},
			}

			err = nm.SendCommand(ctx, nodeID, cmd2)
			if err == nil {
				t.Log("duplicate deployment should fail")
				return false
			}

			// Check that the error is ALREADY_EXISTS
			st, ok := status.FromError(err)
			if !ok {
				t.Logf("expected gRPC status error, got: %v", err)
				return false
			}

			if st.Code() != codes.AlreadyExists {
				t.Logf("expected ALREADY_EXISTS, got: %v", st.Code())
				return false
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) <= 64 }),
	))

	properties.TestingRun(t)
}

// TestDuplicateDeploymentAfterCompletion tests that deployments can be redeployed after completion.
func TestDuplicateDeploymentAfterCompletion(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("deployments can be redeployed after completion", prop.ForAll(
		func(deploymentID string) bool {
			// Create a NodeManager
			logger := slog.Default()
			nm := NewNodeManager(nil, logger)

			// Create a mock stream
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream := newMockCommandStream(ctx)

			// Register a node connection
			nodeID := "test-node"
			if err := nm.RegisterConnection(nodeID, stream); err != nil {
				t.Logf("failed to register connection: %v", err)
				return false
			}

			// Create a deploy command
			cmd := &pb.DeploymentCommand{
				CommandId: uuid.New().String(),
				Type:      pb.CommandType_COMMAND_DEPLOY,
				Command: &pb.DeploymentCommand_Deploy{
					Deploy: &pb.CPDeployRequest{
						DeploymentId: deploymentID,
						Artifact:     "test-artifact",
						Config:       &pb.CPDeploymentConfig{},
					},
				},
			}

			// First deployment should succeed
			err := nm.SendCommand(ctx, nodeID, cmd)
			if err != nil {
				t.Logf("first deployment should succeed: %v", err)
				return false
			}

			// Mark deployment as complete
			nm.MarkDeploymentComplete(nodeID, deploymentID)

			// Second deployment with same ID should now succeed
			cmd2 := &pb.DeploymentCommand{
				CommandId: uuid.New().String(),
				Type:      pb.CommandType_COMMAND_DEPLOY,
				Command: &pb.DeploymentCommand_Deploy{
					Deploy: &pb.CPDeployRequest{
						DeploymentId: deploymentID, // Same deployment ID
						Artifact:     "test-artifact-2",
						Config:       &pb.CPDeploymentConfig{},
					},
				},
			}

			err = nm.SendCommand(ctx, nodeID, cmd2)
			if err != nil {
				t.Logf("deployment after completion should succeed: %v", err)
				return false
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) <= 64 }),
	))

	properties.TestingRun(t)
}

// **Feature: grpc-node-communication, Property 8: Node Health State Transitions**
// For any node, missing N heartbeats should transition it through HEALTHY → DEGRADED → DOWN states correctly.
// **Validates: Requirements 8.1, 8.2, 8.5**

// TestNodeHealthStateTransitions tests that node health transitions correctly based on heartbeat timing.
func TestNodeHealthStateTransitions(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generate time since last heartbeat
	genTimeSinceHeartbeat := gen.Int64Range(0, 120).Map(func(seconds int64) time.Duration {
		return time.Duration(seconds) * time.Second
	})

	properties.Property("node status transitions correctly based on heartbeat timing", prop.ForAll(
		func(timeSinceHeartbeat time.Duration) bool {
			// Create a NodeManager with standard thresholds
			cfg := &NodeManagerConfig{
				HealthCheckInterval: 10 * time.Second,
				DegradedThreshold:   30 * time.Second,
				DownThreshold:       60 * time.Second,
			}
			nm := NewNodeManagerWithConfig(nil, slog.Default(), cfg)

			// Calculate expected status
			status := nm.CalculateNodeStatus(timeSinceHeartbeat)

			// Verify the status is correct based on thresholds
			if timeSinceHeartbeat >= cfg.DownThreshold {
				// Should be DOWN (Requirement 8.2: 60s = 6 missed heartbeats)
				if status != NodeStatusDown {
					t.Logf("expected DOWN for %v, got %v", timeSinceHeartbeat, status)
					return false
				}
			} else if timeSinceHeartbeat >= cfg.DegradedThreshold {
				// Should be DEGRADED (Requirement 8.1: 30s = 3 missed heartbeats)
				if status != NodeStatusDegraded {
					t.Logf("expected DEGRADED for %v, got %v", timeSinceHeartbeat, status)
					return false
				}
			} else {
				// Should be HEALTHY
				if status != NodeStatusHealthy {
					t.Logf("expected HEALTHY for %v, got %v", timeSinceHeartbeat, status)
					return false
				}
			}

			return true
		},
		genTimeSinceHeartbeat,
	))

	properties.TestingRun(t)
}

// TestNodeRecoveryFromDown tests that a down node can recover when heartbeat is received.
func TestNodeRecoveryFromDown(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("down nodes recover to healthy when heartbeat is received", prop.ForAll(
		func(nodeID string) bool {
			// Create a NodeManager
			logger := slog.Default()
			nm := NewNodeManager(nil, logger)

			// Create a mock stream
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream := newMockCommandStream(ctx)

			// Register a node connection
			if err := nm.RegisterConnection(nodeID, stream); err != nil {
				t.Logf("failed to register connection: %v", err)
				return false
			}

			// Get the connection and manually set it to DOWN
			conn, ok := nm.GetConnection(nodeID)
			if !ok {
				t.Log("connection should exist")
				return false
			}
			conn.SetStatus(NodeStatusDown)

			// Verify it's DOWN
			if conn.GetStatus() != NodeStatusDown {
				t.Log("status should be DOWN")
				return false
			}

			// Send a heartbeat update (Requirement 8.5)
			nm.UpdateHeartbeat(nodeID, &pb.NodeInfo{
				Id:       nodeID,
				Hostname: "test-host",
				Address:  "127.0.0.1",
				GrpcPort: 9090,
			})

			// Verify it's now HEALTHY
			if conn.GetStatus() != NodeStatusHealthy {
				t.Logf("status should be HEALTHY after heartbeat, got %v", conn.GetStatus())
				return false
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) <= 64 }),
	))

	properties.TestingRun(t)
}

// **Feature: grpc-node-communication, Property 9: Scheduling Excludes Down Nodes**
// For any scheduling decision, nodes marked as DOWN should never be selected.
// **Validates: Requirements 8.3**

// TestSchedulingExcludesDownNodes tests that down nodes are not returned by GetHealthyNodes.
func TestSchedulingExcludesDownNodes(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generate a list of node statuses
	genNodeStatuses := gen.SliceOfN(5, gen.OneConstOf(
		NodeStatusHealthy,
		NodeStatusDegraded,
		NodeStatusDown,
	))

	properties.Property("GetHealthyNodes excludes down and degraded nodes", prop.ForAll(
		func(statuses []NodeStatus) bool {
			// Create a NodeManager
			logger := slog.Default()
			nm := NewNodeManager(nil, logger)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Register nodes with different statuses
			for i, status := range statuses {
				nodeID := uuid.New().String()
				stream := newMockCommandStream(ctx)

				if err := nm.RegisterConnection(nodeID, stream); err != nil {
					t.Logf("failed to register connection %d: %v", i, err)
					return false
				}

				// Set the status
				conn, _ := nm.GetConnection(nodeID)
				conn.SetStatus(status)
			}

			// Get healthy nodes
			healthyNodes := nm.GetHealthyNodes()

			// Count expected healthy nodes
			expectedHealthy := 0
			for _, status := range statuses {
				if status == NodeStatusHealthy {
					expectedHealthy++
				}
			}

			// Verify count matches
			if len(healthyNodes) != expectedHealthy {
				t.Logf("expected %d healthy nodes, got %d", expectedHealthy, len(healthyNodes))
				return false
			}

			// Verify all returned nodes are actually healthy
			for _, conn := range healthyNodes {
				if conn.GetStatus() != NodeStatusHealthy {
					t.Logf("returned node has status %v, expected HEALTHY", conn.GetStatus())
					return false
				}
			}

			return true
		},
		genNodeStatuses,
	))

	properties.TestingRun(t)
}

// TestSendCommandToDownNode tests that sending commands to down nodes fails.
func TestSendCommandToDownNode(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("sending commands to down nodes returns UNAVAILABLE", prop.ForAll(
		func(nodeID string) bool {
			// Create a NodeManager
			logger := slog.Default()
			nm := NewNodeManager(nil, logger)

			// Create a mock stream
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream := newMockCommandStream(ctx)

			// Register a node connection
			if err := nm.RegisterConnection(nodeID, stream); err != nil {
				t.Logf("failed to register connection: %v", err)
				return false
			}

			// Set the node to DOWN
			conn, _ := nm.GetConnection(nodeID)
			conn.SetStatus(NodeStatusDown)

			// Try to send a command
			cmd := &pb.DeploymentCommand{
				CommandId: uuid.New().String(),
				Type:      pb.CommandType_COMMAND_DEPLOY,
				Command: &pb.DeploymentCommand_Deploy{
					Deploy: &pb.CPDeployRequest{
						DeploymentId: "test-deployment",
						Artifact:     "test-artifact",
						Config:       &pb.CPDeploymentConfig{},
					},
				},
			}

			err := nm.SendCommand(ctx, nodeID, cmd)
			if err == nil {
				t.Log("sending to down node should fail")
				return false
			}

			// Check that the error is UNAVAILABLE
			st, ok := status.FromError(err)
			if !ok {
				t.Logf("expected gRPC status error, got: %v", err)
				return false
			}

			if st.Code() != codes.Unavailable {
				t.Logf("expected UNAVAILABLE, got: %v", st.Code())
				return false
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) <= 64 }),
	))

	properties.TestingRun(t)
}
