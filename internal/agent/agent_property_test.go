package agent

import (
	"reflect"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: control-plane, Property 25: Deployment command completeness**
// For any deployment command sent to a node agent, the command should include
// a non-empty artifact reference and runtime configuration.
// **Validates: Requirements 11.2**

// genBuildType generates a random BuildType.
func genBuildType() gopter.Gen {
	return gen.OneConstOf(models.BuildTypeOCI, models.BuildTypePureNix)
}

// genResourceSpec generates a random ResourceSpec.
func genResourceSpec() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("0.25", "0.5", "1", "2", "4"),
		gen.OneConstOf("256Mi", "512Mi", "1Gi", "2Gi", "4Gi"),
	).Map(func(vals []interface{}) *models.ResourceSpec {
		return &models.ResourceSpec{
			CPU:    vals[0].(string),
			Memory: vals[1].(string),
		}
	})
}

// genPortMapping generates a random PortMapping.
func genPortMapping() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 65535),
		gen.OneConstOf("tcp", "udp"),
	).Map(func(vals []interface{}) models.PortMapping {
		return models.PortMapping{
			ContainerPort: vals[0].(int),
			Protocol:      vals[1].(string),
		}
	})
}

// genHealthCheckConfig generates a random HealthCheckConfig.
func genHealthCheckConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString(),
		gen.IntRange(1, 65535),
		gen.IntRange(1, 300),
		gen.IntRange(1, 60),
		gen.IntRange(1, 10),
	).Map(func(vals []interface{}) *models.HealthCheckConfig {
		return &models.HealthCheckConfig{
			Path:            vals[0].(string),
			Port:            vals[1].(int),
			IntervalSeconds: vals[2].(int),
			TimeoutSeconds:  vals[3].(int),
			Retries:         vals[4].(int),
		}
	})
}

// genOptionalHealthCheckConfig generates an optional HealthCheckConfig pointer.
func genOptionalHealthCheckConfig() gopter.Gen {
	return gen.Bool().FlatMap(func(v interface{}) gopter.Gen {
		if v.(bool) {
			return genHealthCheckConfig()
		}
		return gen.Const((*models.HealthCheckConfig)(nil))
	}, reflect.TypeOf((*models.HealthCheckConfig)(nil)))
}

// genRuntimeConfig generates a valid RuntimeConfig (always non-nil for valid requests).
func genRuntimeConfig() gopter.Gen {
	return gopter.CombineGens(
		genResourceSpec(),
		gen.MapOf(gen.AlphaString(), gen.AlphaString()),
		gen.SliceOfN(2, genPortMapping()),
		genOptionalHealthCheckConfig(),
	).Map(func(vals []interface{}) *models.RuntimeConfig {
		return &models.RuntimeConfig{
			Resources:   vals[0].(*models.ResourceSpec),
			EnvVars:     vals[1].(map[string]string),
			Ports:       vals[2].([]models.PortMapping),
			HealthCheck: vals[3].(*models.HealthCheckConfig),
		}
	})
}

// genValidDeployRequest generates a valid DeployRequest with all required fields.
func genValidDeployRequest() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(), // DeploymentID
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // Artifact
		genBuildType(),     // BuildType
		genRuntimeConfig(), // Config
		gen.MapOf(gen.AlphaString(), gen.AlphaString()), // Secrets
	).Map(func(vals []interface{}) *DeployRequest {
		return &DeployRequest{
			DeploymentID: vals[0].(string),
			Artifact:     vals[1].(string),
			BuildType:    vals[2].(models.BuildType),
			Config:       vals[3].(*models.RuntimeConfig),
			Secrets:      vals[4].(map[string]string),
		}
	})
}

// TestPropertyDeploymentCommandCompleteness tests that all valid deploy requests
// have non-empty artifact references and runtime configuration.
// **Feature: control-plane, Property 25: Deployment command completeness**
// **Validates: Requirements 11.2**
func TestPropertyDeploymentCommandCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Valid deploy requests have non-empty artifact and config", prop.ForAll(
		func(req *DeployRequest) bool {
			// Validate the request using our validation function
			err := ValidateDeployRequest(req)
			if err != nil {
				t.Logf("Validation failed: %v", err)
				return false
			}

			// Additional checks for completeness
			if req.Artifact == "" {
				t.Log("Artifact is empty")
				return false
			}

			if req.Config == nil {
				t.Log("Config is nil")
				return false
			}

			if req.DeploymentID == "" {
				t.Log("DeploymentID is empty")
				return false
			}

			if req.BuildType == "" {
				t.Log("BuildType is empty")
				return false
			}

			return true
		},
		genValidDeployRequest(),
	))

	properties.TestingRun(t)
}

// TestPropertyInvalidDeployRequestsRejected tests that deploy requests with
// missing required fields are properly rejected.
func TestPropertyInvalidDeployRequestsRejected(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Test that nil requests are rejected
	properties.Property("Nil requests are rejected", prop.ForAll(
		func(_ bool) bool {
			err := ValidateDeployRequest(nil)
			return err != nil
		},
		gen.Bool(),
	))

	// Test that requests with empty deployment ID are rejected
	properties.Property("Empty deployment ID is rejected", prop.ForAll(
		func(artifact string, buildType models.BuildType) bool {
			req := &DeployRequest{
				DeploymentID: "", // Empty
				Artifact:     artifact,
				BuildType:    buildType,
				Config:       &models.RuntimeConfig{Resources: models.DefaultResourceSpec()},
			}
			err := ValidateDeployRequest(req)
			return err != nil
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		genBuildType(),
	))

	// Test that requests with empty artifact are rejected
	properties.Property("Empty artifact is rejected", prop.ForAll(
		func(deploymentID string, buildType models.BuildType) bool {
			req := &DeployRequest{
				DeploymentID: deploymentID,
				Artifact:     "", // Empty
				BuildType:    buildType,
				Config:       &models.RuntimeConfig{Resources: models.DefaultResourceSpec()},
			}
			err := ValidateDeployRequest(req)
			return err != nil
		},
		gen.Identifier(),
		genBuildType(),
	))

	// Test that requests with nil config are rejected
	properties.Property("Nil config is rejected", prop.ForAll(
		func(deploymentID, artifact string, buildType models.BuildType) bool {
			req := &DeployRequest{
				DeploymentID: deploymentID,
				Artifact:     artifact,
				BuildType:    buildType,
				Config:       nil, // Nil
			}
			err := ValidateDeployRequest(req)
			return err != nil
		},
		gen.Identifier(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		genBuildType(),
	))

	properties.TestingRun(t)
}

// **Feature: control-plane, Property 26: Status report synchronization**
// For any deployment status reported by a node agent, the deployment record
// should be updated to reflect that status.
// **Validates: Requirements 11.3**

// genDeploymentStatus generates a random DeploymentStatus.
func genDeploymentStatus() gopter.Gen {
	return gen.OneConstOf(
		models.DeploymentStatusPending,
		models.DeploymentStatusBuilding,
		models.DeploymentStatusBuilt,
		models.DeploymentStatusScheduled,
		models.DeploymentStatusStarting,
		models.DeploymentStatusRunning,
		models.DeploymentStatusStopping,
		models.DeploymentStatusStopped,
		models.DeploymentStatusFailed,
	)
}

// StatusReportHandler handles status reports from agents and updates deployments.
// This is the interface that the control plane implements to handle status updates.
type StatusReportHandler struct {
	deployments map[string]*models.Deployment
}

// NewStatusReportHandler creates a new handler for testing.
func NewStatusReportHandler() *StatusReportHandler {
	return &StatusReportHandler{
		deployments: make(map[string]*models.Deployment),
	}
}

// SetDeployment adds a deployment to the handler's store.
func (h *StatusReportHandler) SetDeployment(d *models.Deployment) {
	h.deployments[d.ID] = d
}

// GetDeployment retrieves a deployment from the handler's store.
func (h *StatusReportHandler) GetDeployment(id string) *models.Deployment {
	return h.deployments[id]
}

// HandleStatusReport processes a status report from an agent and updates the deployment.
// This simulates what the control plane does when it receives a status update.
func (h *StatusReportHandler) HandleStatusReport(status *DeploymentStatus) error {
	deployment, exists := h.deployments[status.DeploymentID]
	if !exists {
		return nil // Deployment not found, nothing to update
	}

	// Update the deployment status
	deployment.Status = status.Status
	deployment.UpdatedAt = status.UpdatedAt

	if status.StartedAt != nil {
		deployment.StartedAt = status.StartedAt
	}

	return nil
}

// TestPropertyStatusReportSynchronization tests that status reports from agents
// correctly update deployment records.
// **Feature: control-plane, Property 26: Status report synchronization**
// **Validates: Requirements 11.3**
func TestPropertyStatusReportSynchronization(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Status reports update deployment records", prop.ForAll(
		func(deploymentID string, initialStatus, reportedStatus models.DeploymentStatus) bool {
			// Create a handler with an initial deployment
			handler := NewStatusReportHandler()

			initialDeployment := &models.Deployment{
				ID:     deploymentID,
				Status: initialStatus,
			}
			handler.SetDeployment(initialDeployment)

			// Simulate receiving a status report from the agent
			statusReport := &DeploymentStatus{
				DeploymentID: deploymentID,
				Status:       reportedStatus,
				Message:      "Status update from agent",
			}

			// Handle the status report
			err := handler.HandleStatusReport(statusReport)
			if err != nil {
				t.Logf("HandleStatusReport failed: %v", err)
				return false
			}

			// Verify the deployment was updated
			updatedDeployment := handler.GetDeployment(deploymentID)
			if updatedDeployment == nil {
				t.Log("Deployment not found after update")
				return false
			}

			// The deployment status should match the reported status
			if updatedDeployment.Status != reportedStatus {
				t.Logf("Status mismatch: expected %s, got %s", reportedStatus, updatedDeployment.Status)
				return false
			}

			return true
		},
		gen.Identifier(),
		genDeploymentStatus(),
		genDeploymentStatus(),
	))

	properties.TestingRun(t)
}

// TestPropertyStatusReportWithStartTime tests that status reports with start times
// correctly update the deployment's StartedAt field.
func TestPropertyStatusReportWithStartTime(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Status reports with start time update StartedAt", prop.ForAll(
		func(deploymentID string, startedAtUnix int64) bool {
			// Create a handler with an initial deployment
			handler := NewStatusReportHandler()

			initialDeployment := &models.Deployment{
				ID:        deploymentID,
				Status:    models.DeploymentStatusScheduled,
				StartedAt: nil, // Not started yet
			}
			handler.SetDeployment(initialDeployment)

			// Simulate receiving a status report with a start time
			startedAt := time.Unix(startedAtUnix, 0)
			statusReport := &DeploymentStatus{
				DeploymentID: deploymentID,
				Status:       models.DeploymentStatusRunning,
				StartedAt:    &startedAt,
			}

			// Handle the status report
			err := handler.HandleStatusReport(statusReport)
			if err != nil {
				t.Logf("HandleStatusReport failed: %v", err)
				return false
			}

			// Verify the deployment was updated with the start time
			updatedDeployment := handler.GetDeployment(deploymentID)
			if updatedDeployment == nil {
				t.Log("Deployment not found after update")
				return false
			}

			if updatedDeployment.StartedAt == nil {
				t.Log("StartedAt is nil after update")
				return false
			}

			if !updatedDeployment.StartedAt.Equal(startedAt) {
				t.Logf("StartedAt mismatch: expected %v, got %v", startedAt, *updatedDeployment.StartedAt)
				return false
			}

			return true
		},
		gen.Identifier(),
		gen.Int64Range(1000000000, 2000000000), // Unix timestamps
	))

	properties.TestingRun(t)
}
