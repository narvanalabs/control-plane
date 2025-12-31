package scheduler

import (
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

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

// genSuccessfulDeployment generates a deployment that could be rolled back to.
// It has a valid artifact and is in a successful state.
func genSuccessfulDeployment() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),                                                    // ID
		gen.Identifier(),                                                    // AppID
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // ServiceName
		gen.IntRange(1, 100),                                                // Version
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // GitRef
		gen.AlphaString(),                                                   // GitCommit
		genBuildType(),                                                      // BuildType
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // Artifact (non-empty)
		genResourceTier(),                                                   // ResourceTier
	).Map(func(vals []interface{}) *models.Deployment {
		return &models.Deployment{
			ID:           vals[0].(string),
			AppID:        vals[1].(string),
			ServiceName:  vals[2].(string),
			Version:      vals[3].(int),
			GitRef:       vals[4].(string),
			GitCommit:    vals[5].(string),
			BuildType:    vals[6].(models.BuildType),
			Artifact:     vals[7].(string),
			Status:       models.DeploymentStatusRunning,
			ResourceTier: vals[8].(models.ResourceTier),
			CreatedAt:    time.Now().Add(-time.Hour),
			UpdatedAt:    time.Now().Add(-time.Hour),
		}
	})
}


// **Feature: platform-enhancements, Property 11: Rollback Creates New Deployment**
// *For any* rollback operation on a previous successful deployment, a new deployment
// SHALL be created with an incremented version number and the same artifact as the
// rolled-back-to deployment.
// **Validates: Requirements 10.5, 20.3, 20.4**
func TestRollbackCreatesNewDeployment(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("rollback creates deployment with incremented version and same artifact", prop.ForAll(
		func(source *models.Deployment, versionIncrement int) bool {
			// Calculate the new version (must be greater than source)
			newVersion := source.Version + versionIncrement
			newID := "rollback-" + source.ID

			// Create the rollback deployment using the pure function
			rollback := CreateRollbackDeployment(source, newVersion, newID)

			// Validate the rollback deployment
			return ValidateRollbackDeployment(source, rollback)
		},
		genSuccessfulDeployment(),
		gen.IntRange(1, 100), // Version increment (always positive)
	))

	properties.TestingRun(t)
}

// **Feature: platform-enhancements, Property 11 (continued): Rollback version monotonicity**
// Tests that rollback deployments always have a higher version than the source.
func TestRollbackVersionMonotonicity(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("rollback version is always greater than source version", prop.ForAll(
		func(source *models.Deployment, versionIncrement int) bool {
			newVersion := source.Version + versionIncrement
			newID := "rollback-" + source.ID

			rollback := CreateRollbackDeployment(source, newVersion, newID)

			// Version must be strictly greater
			return rollback.Version > source.Version
		},
		genSuccessfulDeployment(),
		gen.IntRange(1, 100),
	))

	properties.TestingRun(t)
}

// **Feature: platform-enhancements, Property 11 (continued): Rollback artifact preservation**
// Tests that rollback deployments preserve the exact artifact from the source.
func TestRollbackArtifactPreservation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("rollback preserves the exact artifact from source", prop.ForAll(
		func(source *models.Deployment) bool {
			newVersion := source.Version + 1
			newID := "rollback-" + source.ID

			rollback := CreateRollbackDeployment(source, newVersion, newID)

			// Artifact must be exactly the same
			return rollback.Artifact == source.Artifact
		},
		genSuccessfulDeployment(),
	))

	properties.TestingRun(t)
}

// **Feature: platform-enhancements, Property 11 (continued): Rollback status is built**
// Tests that rollback deployments skip the build phase by starting in "built" status.
func TestRollbackStatusIsBuilt(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("rollback deployment starts in 'built' status", prop.ForAll(
		func(source *models.Deployment) bool {
			newVersion := source.Version + 1
			newID := "rollback-" + source.ID

			rollback := CreateRollbackDeployment(source, newVersion, newID)

			// Status must be "built" to skip the build phase
			return rollback.Status == models.DeploymentStatusBuilt
		},
		genSuccessfulDeployment(),
	))

	properties.TestingRun(t)
}

// **Feature: platform-enhancements, Property 11 (continued): Rollback preserves app and service**
// Tests that rollback deployments belong to the same app and service.
func TestRollbackPreservesAppAndService(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("rollback preserves app ID and service name", prop.ForAll(
		func(source *models.Deployment) bool {
			newVersion := source.Version + 1
			newID := "rollback-" + source.ID

			rollback := CreateRollbackDeployment(source, newVersion, newID)

			// App ID and service name must match
			return rollback.AppID == source.AppID && rollback.ServiceName == source.ServiceName
		},
		genSuccessfulDeployment(),
	))

	properties.TestingRun(t)
}

// **Feature: platform-enhancements, Property 11 (continued): Rollback preserves build type**
// Tests that rollback deployments preserve the build type from the source.
func TestRollbackPreservesBuildType(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("rollback preserves build type from source", prop.ForAll(
		func(source *models.Deployment) bool {
			newVersion := source.Version + 1
			newID := "rollback-" + source.ID

			rollback := CreateRollbackDeployment(source, newVersion, newID)

			// Build type must be preserved
			return rollback.BuildType == source.BuildType
		},
		genSuccessfulDeployment(),
	))

	properties.TestingRun(t)
}

// TestValidateRollbackDeploymentRejectsInvalid tests that validation correctly rejects invalid rollbacks.
func TestValidateRollbackDeploymentRejectsInvalid(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("validation rejects rollback with lower or equal version", prop.ForAll(
		func(source *models.Deployment, versionOffset int) bool {
			// Create a rollback with version <= source version
			invalidVersion := source.Version - versionOffset
			if invalidVersion < 1 {
				invalidVersion = 1
			}
			// Only test when invalidVersion <= source.Version
			if invalidVersion > source.Version {
				return true // Skip this case
			}

			rollback := &models.Deployment{
				ID:           "invalid-rollback",
				AppID:        source.AppID,
				ServiceName:  source.ServiceName,
				Version:      invalidVersion,
				Artifact:     source.Artifact,
				Status:       models.DeploymentStatusBuilt,
				BuildType:    source.BuildType,
			}

			// Validation should fail for version <= source
			return !ValidateRollbackDeployment(source, rollback)
		},
		genSuccessfulDeployment(),
		gen.IntRange(0, 50),
	))

	properties.Property("validation rejects rollback with different artifact", prop.ForAll(
		func(source *models.Deployment, differentArtifact string) bool {
			// Skip if artifacts happen to be the same
			if differentArtifact == source.Artifact {
				return true
			}

			rollback := &models.Deployment{
				ID:           "invalid-rollback",
				AppID:        source.AppID,
				ServiceName:  source.ServiceName,
				Version:      source.Version + 1,
				Artifact:     differentArtifact, // Different artifact
				Status:       models.DeploymentStatusBuilt,
				BuildType:    source.BuildType,
			}

			// Validation should fail for different artifact
			return !ValidateRollbackDeployment(source, rollback)
		},
		genSuccessfulDeployment(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	properties.Property("validation rejects rollback with wrong status", prop.ForAll(
		func(source *models.Deployment, status models.DeploymentStatus) bool {
			// Skip if status is "built" (the correct status)
			if status == models.DeploymentStatusBuilt {
				return true
			}

			rollback := &models.Deployment{
				ID:           "invalid-rollback",
				AppID:        source.AppID,
				ServiceName:  source.ServiceName,
				Version:      source.Version + 1,
				Artifact:     source.Artifact,
				Status:       status, // Wrong status
				BuildType:    source.BuildType,
			}

			// Validation should fail for wrong status
			return !ValidateRollbackDeployment(source, rollback)
		},
		genSuccessfulDeployment(),
		genDeploymentStatus(),
	))

	properties.TestingRun(t)
}
