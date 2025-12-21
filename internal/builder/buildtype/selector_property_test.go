package buildtype

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: flexible-build-strategies, Property 4: Dockerfile Strategy Forces OCI**
// For any service with build_strategy: dockerfile, the resulting build_type SHALL always be oci.
// **Validates: Requirements 7.3**

// genDetectionResult generates random DetectionResult values for testing.
func genDetectionResult() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf(
			models.FrameworkGeneric,
			models.FrameworkNextJS,
			models.FrameworkExpress,
			models.FrameworkReact,
			models.FrameworkFastify,
			models.FrameworkDjango,
			models.FrameworkFastAPI,
			models.FrameworkFlask,
		),
		gen.OneConstOf("1.21", "1.22", "18.0.0", "20.0.0", "3.11", "3.12"),
		gen.Float64Range(0.0, 1.0),
	).Map(func(vals []interface{}) *models.DetectionResult {
		return &models.DetectionResult{
			Strategy:             models.BuildStrategyAuto,
			Framework:            vals[0].(models.Framework),
			Version:              vals[1].(string),
			Confidence:           vals[2].(float64),
			RecommendedBuildType: models.BuildTypePureNix,
		}
	})
}

// genOptionalBuildType generates optional BuildType values (including nil).
func genOptionalBuildType() gopter.Gen {
	return gen.OneConstOf(
		(*models.BuildType)(nil),
		ptrBuildType(models.BuildTypeOCI),
		ptrBuildType(models.BuildTypePureNix),
	)
}

// ptrBuildType returns a pointer to a BuildType.
func ptrBuildType(bt models.BuildType) *models.BuildType {
	return &bt
}

func TestDockerfileStrategyForcesOCI(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	selector := NewSelector()

	properties.Property("dockerfile strategy always returns OCI regardless of user preference", prop.ForAll(
		func(detection *models.DetectionResult, userPref *models.BuildType) bool {
			result := selector.SelectBuildType(models.BuildStrategyDockerfile, detection, userPref)
			return result == models.BuildTypeOCI
		},
		genDetectionResult(),
		genOptionalBuildType(),
	))

	properties.Property("dockerfile strategy recommendation is always OCI", prop.ForAll(
		func(detection *models.DetectionResult) bool {
			recommendation := selector.GetRecommendation(models.BuildStrategyDockerfile, detection)
			return recommendation.Recommended == models.BuildTypeOCI
		},
		genDetectionResult(),
	))

	properties.TestingRun(t)
}

// **Feature: flexible-build-strategies, Property 5: Nixpacks Strategy Forces OCI**
// For any service with build_strategy: nixpacks, the resulting build_type SHALL always be oci.
// **Validates: Requirements 8.3**

func TestNixpacksStrategyForcesOCI(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	selector := NewSelector()

	properties.Property("nixpacks strategy always returns OCI regardless of user preference", prop.ForAll(
		func(detection *models.DetectionResult, userPref *models.BuildType) bool {
			result := selector.SelectBuildType(models.BuildStrategyNixpacks, detection, userPref)
			return result == models.BuildTypeOCI
		},
		genDetectionResult(),
		genOptionalBuildType(),
	))

	properties.Property("nixpacks strategy recommendation is always OCI", prop.ForAll(
		func(detection *models.DetectionResult) bool {
			recommendation := selector.GetRecommendation(models.BuildStrategyNixpacks, detection)
			return recommendation.Recommended == models.BuildTypeOCI
		},
		genDetectionResult(),
	))

	properties.TestingRun(t)
}

// **Feature: flexible-build-strategies, Property 18: Build Type Selection Determinism**
// For any build strategy and detection result, the BuildTypeSelector SHALL return
// a consistent build type recommendation.
// **Validates: Requirements 2.1, 7.3, 8.3**

// genBuildStrategy generates all valid build strategies.
func genBuildStrategy() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStrategyFlake,
		models.BuildStrategyAutoGo,
		models.BuildStrategyAutoRust,
		models.BuildStrategyAutoNode,
		models.BuildStrategyAutoPython,
		models.BuildStrategyDockerfile,
		models.BuildStrategyNixpacks,
		models.BuildStrategyAuto,
	)
}

func TestBuildTypeSelectionDeterminism(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	selector := NewSelector()

	properties.Property("same inputs always produce same build type", prop.ForAll(
		func(strategy models.BuildStrategy, detection *models.DetectionResult, userPref *models.BuildType) bool {
			// Call SelectBuildType multiple times with the same inputs
			result1 := selector.SelectBuildType(strategy, detection, userPref)
			result2 := selector.SelectBuildType(strategy, detection, userPref)
			result3 := selector.SelectBuildType(strategy, detection, userPref)

			// All results should be identical
			return result1 == result2 && result2 == result3
		},
		genBuildStrategy(),
		genDetectionResult(),
		genOptionalBuildType(),
	))

	properties.Property("same inputs always produce same recommendation", prop.ForAll(
		func(strategy models.BuildStrategy, detection *models.DetectionResult) bool {
			// Call GetRecommendation multiple times with the same inputs
			rec1 := selector.GetRecommendation(strategy, detection)
			rec2 := selector.GetRecommendation(strategy, detection)
			rec3 := selector.GetRecommendation(strategy, detection)

			// All recommendations should be identical
			return rec1.Recommended == rec2.Recommended &&
				rec2.Recommended == rec3.Recommended &&
				rec1.Reason == rec2.Reason &&
				rec2.Reason == rec3.Reason
		},
		genBuildStrategy(),
		genDetectionResult(),
	))

	properties.Property("OCI-enforced strategies always return OCI", prop.ForAll(
		func(detection *models.DetectionResult, userPref *models.BuildType) bool {
			// Test both OCI-enforced strategies
			dockerfileResult := selector.SelectBuildType(models.BuildStrategyDockerfile, detection, userPref)
			nixpacksResult := selector.SelectBuildType(models.BuildStrategyNixpacks, detection, userPref)

			return dockerfileResult == models.BuildTypeOCI && nixpacksResult == models.BuildTypeOCI
		},
		genDetectionResult(),
		genOptionalBuildType(),
	))

	properties.Property("user preference is respected for non-OCI-enforced strategies", prop.ForAll(
		func(strategy models.BuildStrategy, detection *models.DetectionResult) bool {
			// Skip OCI-enforced strategies
			if IsOCIEnforced(strategy) {
				return true
			}

			// Test that user preference is respected
			ociPref := ptrBuildType(models.BuildTypeOCI)
			pureNixPref := ptrBuildType(models.BuildTypePureNix)

			ociResult := selector.SelectBuildType(strategy, detection, ociPref)
			pureNixResult := selector.SelectBuildType(strategy, detection, pureNixPref)

			return ociResult == models.BuildTypeOCI && pureNixResult == models.BuildTypePureNix
		},
		genBuildStrategy(),
		genDetectionResult(),
	))

	properties.TestingRun(t)
}

// TestIsOCIEnforced tests the IsOCIEnforced helper function.
func TestIsOCIEnforced(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("only dockerfile and nixpacks are OCI-enforced", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			isEnforced := IsOCIEnforced(strategy)

			// Should be true only for dockerfile and nixpacks
			expectedEnforced := strategy == models.BuildStrategyDockerfile || strategy == models.BuildStrategyNixpacks

			return isEnforced == expectedEnforced
		},
		genBuildStrategy(),
	))

	properties.TestingRun(t)
}
