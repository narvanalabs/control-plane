package scheduler

import (
	"context"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/narvanalabs/control-plane/api/proto"
	"github.com/narvanalabs/control-plane/internal/models"
)

// mockCommandSender is a mock implementation of CommandSender for testing.
type mockCommandSender struct {
	sentCommands []*pb.DeploymentCommand
	returnError  error
}

func (m *mockCommandSender) SendCommand(ctx context.Context, nodeID string, cmd *pb.DeploymentCommand) error {
	m.sentCommands = append(m.sentCommands, cmd)
	return m.returnError
}

// genDeploymentForGRPC generates a random Deployment for gRPC testing.
func genDeploymentForGRPC() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),
		gen.Identifier(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(models.BuildTypeOCI, models.BuildTypePureNix),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			models.ResourceTierNano,
			models.ResourceTierSmall,
			models.ResourceTierMedium,
			models.ResourceTierLarge,
		),
	).Map(func(vals []interface{}) *models.Deployment {
		return &models.Deployment{
			ID:           vals[0].(string),
			AppID:        vals[1].(string),
			ServiceName:  vals[2].(string),
			BuildType:    vals[3].(models.BuildType),
			Artifact:     vals[4].(string),
			ResourceTier: vals[5].(models.ResourceTier),
			Status:       models.DeploymentStatusBuilt,
		}
	})
}

// TestGRPCAgentClientDeployCommandCompleteness tests that deployment commands
// contain all required fields.
// **Feature: grpc-node-communication, Property 3: Deployment Command Completeness**
// **Validates: Requirements 3.2, 3.4**
func TestGRPCAgentClientDeployCommandCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("deployment commands contain deployment_id, artifact, and configuration", prop.ForAll(
		func(deployment *models.Deployment) bool {
			// Build the command
			cmd := BuildDeployCommand(deployment)

			// Verify command completeness (Requirement 3.2)
			if cmd.CommandId == "" {
				return false
			}
			if cmd.Type != pb.CommandType_COMMAND_DEPLOY {
				return false
			}

			deploy := cmd.GetDeploy()
			if deploy == nil {
				return false
			}

			// Verify deployment ID is present
			if deploy.DeploymentId != deployment.ID {
				return false
			}

			// Verify artifact is present
			if deploy.Artifact != deployment.Artifact {
				return false
			}

			// Verify configuration is present (Requirement 3.4)
			if deploy.Config == nil {
				return false
			}
			if deploy.Config.ResourceTier != string(deployment.ResourceTier) {
				return false
			}

			return true
		},
		genDeploymentForGRPC(),
	))

	properties.TestingRun(t)
}

// TestGRPCAgentClientRetryClassification tests that errors are correctly classified
// as retryable or non-retryable.
// **Feature: grpc-node-communication, Property 10: Retry Classification**
// **Validates: Requirements 9.1, 9.2**
func TestGRPCAgentClientRetryClassification(t *testing.T) {
	// Test retryable errors (Requirement 9.1)
	retryableCodes := []codes.Code{
		codes.Unavailable,
		codes.DeadlineExceeded,
	}

	for _, code := range retryableCodes {
		err := status.Error(code, "test error")
		if !IsRetryableError(err) {
			t.Errorf("expected %v to be retryable", code)
		}
	}

	// Test non-retryable errors (Requirement 9.2)
	nonRetryableCodes := []codes.Code{
		codes.InvalidArgument,
		codes.NotFound,
		codes.AlreadyExists,
		codes.PermissionDenied,
		codes.Unauthenticated,
	}

	for _, code := range nonRetryableCodes {
		err := status.Error(code, "test error")
		if IsRetryableError(err) {
			t.Errorf("expected %v to be non-retryable", code)
		}
	}
}

// TestGRPCAgentClientDeployWithNonRetryableError tests that the Deploy method
// does not retry on non-retryable errors.
// **Feature: grpc-node-communication, Property 10: Retry Classification**
// **Validates: Requirements 9.1, 9.2, 9.3**
func TestGRPCAgentClientDeployWithNonRetryableError(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("non-retryable errors fail immediately without retry", prop.ForAll(
		func(deployment *models.Deployment) bool {
			// Test with non-retryable error - should fail immediately
			sender := &mockCommandSender{
				returnError: status.Error(codes.InvalidArgument, "invalid argument"),
			}
			client := NewGRPCAgentClient(sender, &GRPCAgentClientConfig{
				MaxRetries: 3,
			})

			err := client.Deploy(context.Background(), "test-node", deployment)
			if err == nil {
				return false // Expected error for non-retryable error
			}

			// Should only have tried once (no retries for non-retryable errors)
			return len(sender.sentCommands) == 1
		},
		genDeploymentForGRPC(),
	))

	properties.TestingRun(t)
}

// TestGRPCAgentClientBuildTypeMapping tests that build types are correctly mapped
// to proto enums.
func TestGRPCAgentClientBuildTypeMapping(t *testing.T) {
	testCases := []struct {
		modelType models.BuildType
		protoType pb.CPBuildType
	}{
		{models.BuildTypeOCI, pb.CPBuildType_CP_BUILD_TYPE_OCI},
		{models.BuildTypePureNix, pb.CPBuildType_CP_BUILD_TYPE_NIX},
		{"unknown", pb.CPBuildType_CP_BUILD_TYPE_UNKNOWN},
	}

	for _, tc := range testCases {
		deployment := &models.Deployment{
			ID:           "test-id",
			AppID:        "test-app",
			ServiceName:  "test-service",
			Artifact:     "test-artifact",
			BuildType:    tc.modelType,
			ResourceTier: models.ResourceTierSmall,
		}

		cmd := BuildDeployCommand(deployment)
		deploy := cmd.GetDeploy()

		if deploy.BuildType != tc.protoType {
			t.Errorf("build type mismatch for %s: expected %v, got %v",
				tc.modelType, tc.protoType, deploy.BuildType)
		}
	}
}

// TestGRPCAgentClientDeploySuccess tests that successful deployments work correctly.
func TestGRPCAgentClientDeploySuccess(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("successful deploy sends command once", prop.ForAll(
		func(deployment *models.Deployment) bool {
			sender := &mockCommandSender{
				returnError: nil, // Success
			}
			client := NewGRPCAgentClient(sender, &GRPCAgentClientConfig{
				MaxRetries: 3,
			})

			err := client.Deploy(context.Background(), "test-node", deployment)
			if err != nil {
				return false
			}

			// Should have sent exactly one command
			if len(sender.sentCommands) != 1 {
				return false
			}

			// Verify the command was built correctly
			cmd := sender.sentCommands[0]
			deploy := cmd.GetDeploy()
			return deploy != nil && deploy.DeploymentId == deployment.ID
		},
		genDeploymentForGRPC(),
	))

	properties.TestingRun(t)
}
