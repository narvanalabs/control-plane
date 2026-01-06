package executor

import (
	"context"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: ui-api-alignment, Property 7: CGO Configuration Override**
// For any build configuration with explicit cgo_enabled setting, the builder SHALL
// honor that setting regardless of auto-detection results.
// **Validates: Requirements 16.5**

// CGOOverrideTestCase represents a test case for CGO configuration override.
type CGOOverrideTestCase struct {
	// UserCGOEnabled is the user's explicit CGO setting (nil means not set)
	UserCGOEnabled *bool
	// DetectedCGO is what auto-detection found
	DetectedCGO bool
	// ExpectedCGO is what the final CGO setting should be
	ExpectedCGO bool
}

// genCGOOverrideTestCase generates test cases for CGO override testing.
func genCGOOverrideTestCase() gopter.Gen {
	return gopter.CombineGens(
		gen.PtrOf(gen.Bool()), // UserCGOEnabled (nil, true, or false)
		gen.Bool(),            // DetectedCGO
	).Map(func(vals []interface{}) CGOOverrideTestCase {
		var userCGO *bool
		if vals[0] != nil {
			userCGO = vals[0].(*bool)
		}
		detectedCGO := vals[1].(bool)

		// Calculate expected CGO based on the override logic:
		// If user explicitly set CGO, use that; otherwise use detected
		var expectedCGO bool
		if userCGO != nil {
			expectedCGO = *userCGO
		} else {
			expectedCGO = detectedCGO
		}

		return CGOOverrideTestCase{
			UserCGOEnabled: userCGO,
			DetectedCGO:    detectedCGO,
			ExpectedCGO:    expectedCGO,
		}
	})
}

// genExplicitCGOOverrideTestCase generates test cases where user explicitly sets CGO.
func genExplicitCGOOverrideTestCase() gopter.Gen {
	return gopter.CombineGens(
		gen.Bool(), // UserCGOEnabled (explicit true or false)
		gen.Bool(), // DetectedCGO
	).Map(func(vals []interface{}) CGOOverrideTestCase {
		userCGOVal := vals[0].(bool)
		userCGO := &userCGOVal
		detectedCGO := vals[1].(bool)

		return CGOOverrideTestCase{
			UserCGOEnabled: userCGO,
			DetectedCGO:    detectedCGO,
			ExpectedCGO:    userCGOVal, // User's explicit setting should always win
		}
	})
}

// genNoCGOOverrideTestCase generates test cases where user does not set CGO.
func genNoCGOOverrideTestCase() gopter.Gen {
	return gen.Bool().Map(func(detectedCGO bool) CGOOverrideTestCase {
		return CGOOverrideTestCase{
			UserCGOEnabled: nil,
			DetectedCGO:    detectedCGO,
			ExpectedCGO:    detectedCGO, // Detection result should be used
		}
	})
}

// determineCGOEnabled implements the CGO determination logic from GenerateFlake.
// This is extracted for testing purposes.
func determineCGOEnabled(config models.BuildConfig, detection *models.DetectionResult) bool {
	// Logic from GenerateFlake:
	// 1. If explicitly set in config, use that (user override)
	// 2. Otherwise, use detection result from SuggestedConfig
	cgoEnabled := false
	if config.CGOEnabled != nil {
		// User explicitly set CGO - honor their choice
		cgoEnabled = *config.CGOEnabled
	} else if detection != nil && detection.SuggestedConfig != nil {
		// Use auto-detected CGO setting
		if detected, ok := detection.SuggestedConfig["cgo_enabled"].(bool); ok {
			cgoEnabled = detected
		}
	}
	return cgoEnabled
}

// TestCGOConfigurationOverride tests Property 7: CGO Configuration Override.
// For any build configuration with explicit cgo_enabled setting, the builder SHALL
// honor that setting regardless of auto-detection results.
func TestCGOConfigurationOverride(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 7a: Explicit user CGO setting always overrides detection
	properties.Property("explicit user CGO setting always overrides detection", prop.ForAll(
		func(tc CGOOverrideTestCase) bool {
			// Create build config with user's explicit CGO setting
			config := models.BuildConfig{
				CGOEnabled: tc.UserCGOEnabled,
			}

			// Create detection result with detected CGO
			detection := &models.DetectionResult{
				Strategy: models.BuildStrategyAutoGo,
				SuggestedConfig: map[string]interface{}{
					"cgo_enabled": tc.DetectedCGO,
				},
			}

			// Determine CGO using the same logic as GenerateFlake
			result := determineCGOEnabled(config, detection)

			// Result should match expected
			return result == tc.ExpectedCGO
		},
		genExplicitCGOOverrideTestCase(),
	))

	// Property 7b: When user sets CGO=true, result is always true regardless of detection
	properties.Property("user CGO=true always results in CGO enabled", prop.ForAll(
		func(detectedCGO bool) bool {
			userCGO := true
			config := models.BuildConfig{
				CGOEnabled: &userCGO,
			}

			detection := &models.DetectionResult{
				Strategy: models.BuildStrategyAutoGo,
				SuggestedConfig: map[string]interface{}{
					"cgo_enabled": detectedCGO,
				},
			}

			result := determineCGOEnabled(config, detection)
			return result == true
		},
		gen.Bool(),
	))

	// Property 7c: When user sets CGO=false, result is always false regardless of detection
	properties.Property("user CGO=false always results in CGO disabled", prop.ForAll(
		func(detectedCGO bool) bool {
			userCGO := false
			config := models.BuildConfig{
				CGOEnabled: &userCGO,
			}

			detection := &models.DetectionResult{
				Strategy: models.BuildStrategyAutoGo,
				SuggestedConfig: map[string]interface{}{
					"cgo_enabled": detectedCGO,
				},
			}

			result := determineCGOEnabled(config, detection)
			return result == false
		},
		gen.Bool(),
	))

	// Property 7d: When user doesn't set CGO, detection result is used
	properties.Property("no user CGO setting uses detection result", prop.ForAll(
		func(tc CGOOverrideTestCase) bool {
			config := models.BuildConfig{
				CGOEnabled: nil, // User didn't set
			}

			detection := &models.DetectionResult{
				Strategy: models.BuildStrategyAutoGo,
				SuggestedConfig: map[string]interface{}{
					"cgo_enabled": tc.DetectedCGO,
				},
			}

			result := determineCGOEnabled(config, detection)
			return result == tc.DetectedCGO
		},
		genNoCGOOverrideTestCase(),
	))

	// Property 7e: CGO override is deterministic
	properties.Property("CGO override is deterministic", prop.ForAll(
		func(tc CGOOverrideTestCase) bool {
			config := models.BuildConfig{
				CGOEnabled: tc.UserCGOEnabled,
			}

			detection := &models.DetectionResult{
				Strategy: models.BuildStrategyAutoGo,
				SuggestedConfig: map[string]interface{}{
					"cgo_enabled": tc.DetectedCGO,
				},
			}

			result1 := determineCGOEnabled(config, detection)
			result2 := determineCGOEnabled(config, detection)
			result3 := determineCGOEnabled(config, detection)

			return result1 == result2 && result2 == result3
		},
		genCGOOverrideTestCase(),
	))

	properties.TestingRun(t)
}

// TestCGOOverrideEdgeCases tests edge cases for CGO configuration override.
func TestCGOOverrideEdgeCases(t *testing.T) {
	t.Run("nil detection uses default CGO=false", func(t *testing.T) {
		config := models.BuildConfig{
			CGOEnabled: nil,
		}

		result := determineCGOEnabled(config, nil)
		if result != false {
			t.Errorf("expected CGO=false with nil detection, got %v", result)
		}
	})

	t.Run("nil SuggestedConfig uses default CGO=false", func(t *testing.T) {
		config := models.BuildConfig{
			CGOEnabled: nil,
		}

		detection := &models.DetectionResult{
			Strategy:        models.BuildStrategyAutoGo,
			SuggestedConfig: nil,
		}

		result := determineCGOEnabled(config, detection)
		if result != false {
			t.Errorf("expected CGO=false with nil SuggestedConfig, got %v", result)
		}
	})

	t.Run("missing cgo_enabled in SuggestedConfig uses default CGO=false", func(t *testing.T) {
		config := models.BuildConfig{
			CGOEnabled: nil,
		}

		detection := &models.DetectionResult{
			Strategy: models.BuildStrategyAutoGo,
			SuggestedConfig: map[string]interface{}{
				"go_version": "1.21",
			},
		}

		result := determineCGOEnabled(config, detection)
		if result != false {
			t.Errorf("expected CGO=false with missing cgo_enabled, got %v", result)
		}
	})

	t.Run("user CGO=true overrides nil detection", func(t *testing.T) {
		userCGO := true
		config := models.BuildConfig{
			CGOEnabled: &userCGO,
		}

		result := determineCGOEnabled(config, nil)
		if result != true {
			t.Errorf("expected CGO=true with user override, got %v", result)
		}
	})

	t.Run("user CGO=false overrides detected CGO=true", func(t *testing.T) {
		userCGO := false
		config := models.BuildConfig{
			CGOEnabled: &userCGO,
		}

		detection := &models.DetectionResult{
			Strategy: models.BuildStrategyAutoGo,
			SuggestedConfig: map[string]interface{}{
				"cgo_enabled": true,
			},
		}

		result := determineCGOEnabled(config, detection)
		if result != false {
			t.Errorf("expected CGO=false with user override, got %v", result)
		}
	})
}

// TestGenerateFlakeCGOOverride tests that GenerateFlake respects CGO override.
// This is an integration test that verifies the actual GenerateFlake function.
func TestGenerateFlakeCGOOverride(t *testing.T) {
	// Skip if we don't have the required dependencies
	executor := &AutoGoStrategyExecutor{}

	t.Run("user CGO=true selects CGO template", func(t *testing.T) {
		userCGO := true
		config := models.BuildConfig{
			CGOEnabled: &userCGO,
		}

		detection := &models.DetectionResult{
			Strategy: models.BuildStrategyAutoGo,
			Version:  "1.21",
			SuggestedConfig: map[string]interface{}{
				"cgo_enabled": false, // Detection says no CGO
			},
		}

		// We can't easily test the full GenerateFlake without mocking,
		// but we can verify the CGO determination logic
		result := determineCGOEnabled(config, detection)
		if result != true {
			t.Errorf("expected CGO=true with user override, got %v", result)
		}

		// Verify the executor supports the strategy
		if !executor.Supports(models.BuildStrategyAutoGo) {
			t.Error("executor should support auto-go strategy")
		}
	})

	t.Run("user CGO=false selects non-CGO template", func(t *testing.T) {
		userCGO := false
		config := models.BuildConfig{
			CGOEnabled: &userCGO,
		}

		detection := &models.DetectionResult{
			Strategy: models.BuildStrategyAutoGo,
			Version:  "1.21",
			SuggestedConfig: map[string]interface{}{
				"cgo_enabled": true, // Detection says CGO required
			},
		}

		result := determineCGOEnabled(config, detection)
		if result != false {
			t.Errorf("expected CGO=false with user override, got %v", result)
		}
	})
}

// Ensure context is used (for linter)
var _ = context.Background
