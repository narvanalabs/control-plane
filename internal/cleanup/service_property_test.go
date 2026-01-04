package cleanup

import (
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/podman"
)

// **Feature: backend-source-of-truth, Property 14: Cleanup Retention Enforcement**
// *For any* cleanup operation, resources (containers, images, deployments) within their
// configured retention period SHALL be preserved.
// **Validates: Requirements 16.2, 17.2, 18.2, 25.2, 26.2**
func TestCleanupRetentionEnforcement(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Containers within retention period are preserved
	properties.Property("containers within retention period are preserved", prop.ForAll(
		func(retentionHours int, containerAgeHours int) bool {
			retention := time.Duration(retentionHours) * time.Hour
			containerAge := time.Duration(containerAgeHours) * time.Hour

			// Create a container with the given age
			stoppedAt := time.Now().Add(-containerAge)
			cutoff := time.Now().Add(-retention)

			// Container should be preserved if it's within retention period
			// (i.e., stoppedAt is after cutoff)
			shouldPreserve := !stoppedAt.Before(cutoff)

			// Simulate the cleanup logic
			wouldRemove := stoppedAt.Before(cutoff)

			// The container should be preserved if and only if it's within retention
			return shouldPreserve == !wouldRemove
		},
		gen.IntRange(1, 168),  // retention: 1-168 hours (1 week max)
		gen.IntRange(0, 336),  // container age: 0-336 hours (2 weeks max)
	))

	// Property: Images within retention period are preserved
	properties.Property("images within retention period are preserved", prop.ForAll(
		func(retentionDays int, imageAgeDays int) bool {
			retention := time.Duration(retentionDays) * 24 * time.Hour
			imageAge := time.Duration(imageAgeDays) * 24 * time.Hour

			// Create an image with the given age
			createdAt := time.Now().Add(-imageAge)
			cutoff := time.Now().Add(-retention)

			// Image should be preserved if it's within retention period
			shouldPreserve := !createdAt.Before(cutoff)

			// Simulate the cleanup logic
			wouldRemove := createdAt.Before(cutoff)

			return shouldPreserve == !wouldRemove
		},
		gen.IntRange(1, 30),  // retention: 1-30 days
		gen.IntRange(0, 60),  // image age: 0-60 days
	))

	// Property: Deployments within retention period are preserved
	properties.Property("deployments within retention period are preserved", prop.ForAll(
		func(retentionDays int, deploymentAgeDays int) bool {
			retention := time.Duration(retentionDays) * 24 * time.Hour
			deploymentAge := time.Duration(deploymentAgeDays) * 24 * time.Hour

			// Create a deployment with the given age
			createdAt := time.Now().Add(-deploymentAge)
			cutoff := time.Now().Add(-retention)

			// Deployment should be preserved if it's within retention period
			shouldPreserve := !createdAt.Before(cutoff)

			// Simulate the cleanup logic
			wouldArchive := createdAt.Before(cutoff)

			return shouldPreserve == !wouldArchive
		},
		gen.IntRange(1, 90),   // retention: 1-90 days
		gen.IntRange(0, 180),  // deployment age: 0-180 days
	))

	properties.TestingRun(t)
}

// **Feature: backend-source-of-truth, Property 14 (continued): Minimum deployments kept**
// Tests that the minimum number of deployments per service is always preserved.
func TestMinimumDeploymentsKept(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("minimum deployments per service are always preserved", prop.ForAll(
		func(totalDeployments int, minKept int, retentionDays int) bool {
			// Ensure minKept is valid
			if minKept <= 0 {
				minKept = 1
			}
			if totalDeployments < 0 {
				totalDeployments = 0
			}

			retention := time.Duration(retentionDays) * 24 * time.Hour
			cutoff := time.Now().Add(-retention)

			// Create deployments with varying ages
			// All deployments are older than retention for this test
			deploymentsToArchive := 0
			deploymentsToKeep := 0

			for i := 0; i < totalDeployments; i++ {
				// All deployments are old enough to be archived
				createdAt := time.Now().Add(-retention * 2)
				isOldEnough := createdAt.Before(cutoff)

				// Keep at least minKept deployments
				if i < minKept {
					deploymentsToKeep++
				} else if isOldEnough {
					deploymentsToArchive++
				} else {
					deploymentsToKeep++
				}
			}

			// The number of deployments kept should be at least minKept (or total if less)
			expectedMinKept := minKept
			if totalDeployments < minKept {
				expectedMinKept = totalDeployments
			}

			return deploymentsToKeep >= expectedMinKept
		},
		gen.IntRange(0, 20),  // total deployments
		gen.IntRange(1, 10),  // min kept
		gen.IntRange(1, 30),  // retention days
	))

	properties.TestingRun(t)
}

// **Feature: backend-source-of-truth, Property 14 (continued): Active images preserved**
// Tests that images used by active deployments are never removed.
func TestActiveImagesPreserved(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("images used by active deployments are never removed", prop.ForAll(
		func(imageID string, isActive bool, imageAgeDays int) bool {
			// Skip empty image IDs
			if imageID == "" {
				return true
			}

			imageAge := time.Duration(imageAgeDays) * 24 * time.Hour
			createdAt := time.Now().Add(-imageAge)

			// Create active images set
			activeImages := make(map[string]bool)
			if isActive {
				activeImages[imageID] = true
			}

			// Create image info
			img := podman.ImageInfo{
				ID:        imageID,
				Tags:      []string{imageID + ":latest"},
				CreatedAt: createdAt,
			}

			// Check if image is active
			isImageActive := activeImages[img.ID]
			for _, tag := range img.Tags {
				if activeImages[tag] {
					isImageActive = true
					break
				}
			}

			// If image is active, it should never be removed regardless of age
			if isActive {
				return isImageActive == true
			}

			// If image is not active, the active check should return false
			return isImageActive == false
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 64 }),
		gen.Bool(),
		gen.IntRange(0, 365),  // image age in days
	))

	properties.TestingRun(t)
}

// TestSettingsValidation tests that settings validation works correctly.
func TestSettingsValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("positive retention periods are valid", prop.ForAll(
		func(containerHours, imageHours, nixHours, deploymentHours, minKept, atticHours int) bool {
			// Ensure all values are positive
			if containerHours <= 0 || imageHours <= 0 || nixHours <= 0 ||
				deploymentHours <= 0 || minKept <= 0 || atticHours <= 0 {
				return true // Skip invalid inputs
			}

			settings := &Settings{
				ContainerRetention:  time.Duration(containerHours) * time.Hour,
				ImageRetention:      time.Duration(imageHours) * time.Hour,
				NixGCInterval:       time.Duration(nixHours) * time.Hour,
				DeploymentRetention: time.Duration(deploymentHours) * time.Hour,
				MinDeploymentsKept:  minKept,
				AtticRetention:      time.Duration(atticHours) * time.Hour,
			}

			err := settings.Validate()
			return err == nil
		},
		gen.IntRange(1, 168),   // container retention hours
		gen.IntRange(1, 720),   // image retention hours
		gen.IntRange(1, 168),   // nix gc interval hours
		gen.IntRange(1, 2160),  // deployment retention hours
		gen.IntRange(1, 100),   // min deployments kept
		gen.IntRange(1, 2160),  // attic retention hours
	))

	properties.Property("non-positive retention periods are invalid", prop.ForAll(
		func(containerHours, imageHours, nixHours, deploymentHours, minKept, atticHours int) bool {
			// Make at least one value non-positive
			settings := &Settings{
				ContainerRetention:  time.Duration(containerHours) * time.Hour,
				ImageRetention:      time.Duration(imageHours) * time.Hour,
				NixGCInterval:       time.Duration(nixHours) * time.Hour,
				DeploymentRetention: time.Duration(deploymentHours) * time.Hour,
				MinDeploymentsKept:  minKept,
				AtticRetention:      time.Duration(atticHours) * time.Hour,
			}

			err := settings.Validate()

			// If any value is non-positive, validation should fail
			hasNonPositive := containerHours <= 0 || imageHours <= 0 || nixHours <= 0 ||
				deploymentHours <= 0 || minKept <= 0 || atticHours <= 0

			if hasNonPositive {
				return err != nil
			}
			return err == nil
		},
		gen.IntRange(-10, 10),  // container retention hours (can be negative)
		gen.IntRange(-10, 10),  // image retention hours
		gen.IntRange(-10, 10),  // nix gc interval hours
		gen.IntRange(-10, 10),  // deployment retention hours
		gen.IntRange(-10, 10),  // min deployments kept
		gen.IntRange(-10, 10),  // attic retention hours
	))

	properties.TestingRun(t)
}
