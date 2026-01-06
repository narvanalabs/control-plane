package executor

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: build-detection-integration, Property 7: User config overrides detection**
// For any build where BuildConfig.CGOEnabled is explicitly set, that value must be used
// regardless of what detection returns.
// **Validates: Requirements 3.1**

// genBoolPtr generates a *bool (nil, true, or false).
func genBoolPtr() gopter.Gen {
	return gen.PtrOf(gen.Bool())
}

// genNonNilBoolPtr generates a non-nil *bool (true or false).
func genNonNilBoolPtr() gopter.Gen {
	return gen.Bool().Map(func(b bool) *bool {
		return &b
	})
}

// genBuildConfigWithCGO generates a BuildConfig with various CGO settings.
func genBuildConfigWithCGO() gopter.Gen {
	return gopter.CombineGens(
		genBoolPtr(),        // CGOEnabled
		gen.AlphaString(),   // GoVersion
		gen.AlphaString(),   // EntryPoint
	).Map(func(vals []interface{}) *models.BuildConfig {
		var cgoEnabled *bool
		if vals[0] != nil {
			cgoEnabled = vals[0].(*bool)
		}
		return &models.BuildConfig{
			CGOEnabled: cgoEnabled,
			GoVersion:  vals[1].(string),
			EntryPoint: vals[2].(string),
		}
	})
}

// genDetectedConfigWithCGO generates a detected BuildConfig with CGO setting.
func genDetectedConfigWithCGO() gopter.Gen {
	return gopter.CombineGens(
		genBoolPtr(),        // CGOEnabled
		gen.AlphaString(),   // GoVersion
	).Map(func(vals []interface{}) *models.BuildConfig {
		var cgoEnabled *bool
		if vals[0] != nil {
			cgoEnabled = vals[0].(*bool)
		}
		return &models.BuildConfig{
			CGOEnabled: cgoEnabled,
			GoVersion:  vals[1].(string),
		}
	})
}

// TestUserConfigOverridesDetection tests Property 7: User config overrides detection.
// **Feature: build-detection-integration, Property 7: User config overrides detection**
// **Validates: Requirements 3.1**
func TestUserConfigOverridesDetection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 7.1: When user explicitly sets CGOEnabled, that value is used regardless of detection
	properties.Property("explicit user CGOEnabled overrides detection", prop.ForAll(
		func(userCGO bool, detectedCGO bool) bool {
			userConfig := &models.BuildConfig{
				CGOEnabled: &userCGO,
			}
			detectedConfig := &models.BuildConfig{
				CGOEnabled: &detectedCGO,
			}

			merged := MergeConfigs(userConfig, detectedConfig, nil)

			// User's explicit CGO setting must be preserved
			if merged.CGOEnabled == nil {
				return false
			}
			return *merged.CGOEnabled == userCGO
		},
		gen.Bool(),
		gen.Bool(),
	))

	// Property 7.2: When user sets CGO=true, result is always true regardless of detection
	properties.Property("user CGO=true always results in CGO enabled", prop.ForAll(
		func(detectedCGO *bool) bool {
			userCGO := true
			userConfig := &models.BuildConfig{
				CGOEnabled: &userCGO,
			}
			detectedConfig := &models.BuildConfig{
				CGOEnabled: detectedCGO,
			}

			merged := MergeConfigs(userConfig, detectedConfig, nil)

			return merged.CGOEnabled != nil && *merged.CGOEnabled == true
		},
		genBoolPtr(),
	))

	// Property 7.3: When user sets CGO=false, result is always false regardless of detection
	properties.Property("user CGO=false always results in CGO disabled", prop.ForAll(
		func(detectedCGO *bool) bool {
			userCGO := false
			userConfig := &models.BuildConfig{
				CGOEnabled: &userCGO,
			}
			detectedConfig := &models.BuildConfig{
				CGOEnabled: detectedCGO,
			}

			merged := MergeConfigs(userConfig, detectedConfig, nil)

			return merged.CGOEnabled != nil && *merged.CGOEnabled == false
		},
		genBoolPtr(),
	))

	// Property 7.4: When user doesn't set CGO (nil), detection result is used
	properties.Property("nil user CGO uses detection result", prop.ForAll(
		func(detectedCGO bool) bool {
			userConfig := &models.BuildConfig{
				CGOEnabled: nil, // User didn't set
			}
			detectedConfig := &models.BuildConfig{
				CGOEnabled: &detectedCGO,
			}

			merged := MergeConfigs(userConfig, detectedConfig, nil)

			// Detection result should be used
			return merged.CGOEnabled != nil && *merged.CGOEnabled == detectedCGO
		},
		gen.Bool(),
	))

	// Property 7.5: User override is deterministic
	properties.Property("user override is deterministic", prop.ForAll(
		func(userCGO, detectedCGO bool) bool {
			userConfig := &models.BuildConfig{
				CGOEnabled: &userCGO,
			}
			detectedConfig := &models.BuildConfig{
				CGOEnabled: &detectedCGO,
			}

			merged1 := MergeConfigs(userConfig, detectedConfig, nil)
			merged2 := MergeConfigs(userConfig, detectedConfig, nil)
			merged3 := MergeConfigs(userConfig, detectedConfig, nil)

			if merged1.CGOEnabled == nil || merged2.CGOEnabled == nil || merged3.CGOEnabled == nil {
				return false
			}

			return *merged1.CGOEnabled == *merged2.CGOEnabled && *merged2.CGOEnabled == *merged3.CGOEnabled
		},
		gen.Bool(),
		gen.Bool(),
	))

	properties.TestingRun(t)
}


// **Feature: build-detection-integration, Property 8: Config merge preserves user values**
// For any merge of user config and detected config, all explicitly set user values
// must be preserved in the result.
// **Validates: Requirements 3.2**

// genNonEmptyString generates a non-empty string.
func genNonEmptyString() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0
	})
}

// genStringSlice generates a slice of strings.
func genStringSlice() gopter.Gen {
	return gen.SliceOf(gen.AlphaString())
}

// genStringMap generates a map of strings.
func genStringMap() gopter.Gen {
	return gen.MapOf(gen.AlphaString(), gen.AlphaString())
}

// TestConfigMergePreservesUserValues tests Property 8: Config merge preserves user values.
// **Feature: build-detection-integration, Property 8: Config merge preserves user values**
// **Validates: Requirements 3.2**
func TestConfigMergePreservesUserValues(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 8.1: User GoVersion is preserved in merge
	properties.Property("user GoVersion is preserved", prop.ForAll(
		func(userVersion, detectedVersion string) bool {
			if userVersion == "" {
				return true // Skip empty user values
			}

			userConfig := &models.BuildConfig{
				GoVersion: userVersion,
			}
			detectedConfig := &models.BuildConfig{
				GoVersion: detectedVersion,
			}

			merged := MergeConfigs(userConfig, detectedConfig, nil)

			return merged.GoVersion == userVersion
		},
		gen.AlphaString(),
		gen.AlphaString(),
	))

	// Property 8.2: User EntryPoint is preserved in merge
	properties.Property("user EntryPoint is preserved", prop.ForAll(
		func(userEntryPoint, detectedEntryPoint string) bool {
			if userEntryPoint == "" {
				return true // Skip empty user values
			}

			userConfig := &models.BuildConfig{
				EntryPoint: userEntryPoint,
			}
			detectedConfig := &models.BuildConfig{
				EntryPoint: detectedEntryPoint,
			}

			merged := MergeConfigs(userConfig, detectedConfig, nil)

			return merged.EntryPoint == userEntryPoint
		},
		gen.AlphaString(),
		gen.AlphaString(),
	))

	// Property 8.3: User BuildTags are preserved in merge
	properties.Property("user BuildTags are preserved", prop.ForAll(
		func(userTags, detectedTags []string) bool {
			if len(userTags) == 0 {
				return true // Skip empty user values
			}

			userConfig := &models.BuildConfig{
				BuildTags: userTags,
			}
			detectedConfig := &models.BuildConfig{
				BuildTags: detectedTags,
			}

			merged := MergeConfigs(userConfig, detectedConfig, nil)

			// User tags should be preserved
			if len(merged.BuildTags) != len(userTags) {
				return false
			}
			for i, tag := range userTags {
				if merged.BuildTags[i] != tag {
					return false
				}
			}
			return true
		},
		genStringSlice(),
		genStringSlice(),
	))

	// Property 8.4: User Ldflags is preserved in merge
	properties.Property("user Ldflags is preserved", prop.ForAll(
		func(userLdflags, detectedLdflags string) bool {
			if userLdflags == "" {
				return true // Skip empty user values
			}

			userConfig := &models.BuildConfig{
				Ldflags: userLdflags,
			}
			detectedConfig := &models.BuildConfig{
				Ldflags: detectedLdflags,
			}

			merged := MergeConfigs(userConfig, detectedConfig, nil)

			return merged.Ldflags == userLdflags
		},
		gen.AlphaString(),
		gen.AlphaString(),
	))

	// Property 8.5: User BuildTimeout is preserved in merge
	properties.Property("user BuildTimeout is preserved", prop.ForAll(
		func(userTimeout, detectedTimeout int) bool {
			if userTimeout == 0 {
				return true // Skip zero user values
			}

			userConfig := &models.BuildConfig{
				BuildTimeout: userTimeout,
			}
			detectedConfig := &models.BuildConfig{
				BuildTimeout: detectedTimeout,
			}

			merged := MergeConfigs(userConfig, detectedConfig, nil)

			return merged.BuildTimeout == userTimeout
		},
		gen.IntRange(1, 7200),
		gen.IntRange(1, 7200),
	))

	// Property 8.6: All user-set fields are preserved simultaneously
	properties.Property("all user fields preserved simultaneously", prop.ForAll(
		func(userVersion, userEntry, userLdflags string, userCGO bool, userTimeout int) bool {
			// Skip if all user values are empty/zero
			if userVersion == "" && userEntry == "" && userLdflags == "" && userTimeout == 0 {
				return true
			}

			userConfig := &models.BuildConfig{
				GoVersion:    userVersion,
				EntryPoint:   userEntry,
				Ldflags:      userLdflags,
				CGOEnabled:   &userCGO,
				BuildTimeout: userTimeout,
			}

			// Create detected config with different values
			detectedCGO := !userCGO
			detectedConfig := &models.BuildConfig{
				GoVersion:    "detected-version",
				EntryPoint:   "detected-entry",
				Ldflags:      "-detected",
				CGOEnabled:   &detectedCGO,
				BuildTimeout: 9999,
			}

			merged := MergeConfigs(userConfig, detectedConfig, nil)

			// All user values should be preserved
			if userVersion != "" && merged.GoVersion != userVersion {
				return false
			}
			if userEntry != "" && merged.EntryPoint != userEntry {
				return false
			}
			if userLdflags != "" && merged.Ldflags != userLdflags {
				return false
			}
			if merged.CGOEnabled == nil || *merged.CGOEnabled != userCGO {
				return false
			}
			if userTimeout != 0 && merged.BuildTimeout != userTimeout {
				return false
			}

			return true
		},
		gen.AlphaString(),
		gen.AlphaString(),
		gen.AlphaString(),
		gen.Bool(),
		gen.IntRange(0, 7200),
	))

	// Property 8.7: When user value is empty/nil, detected value is used
	properties.Property("empty user values use detected values", prop.ForAll(
		func(detectedVersion, detectedEntry string, detectedCGO bool) bool {
			userConfig := &models.BuildConfig{
				GoVersion:  "",
				EntryPoint: "",
				CGOEnabled: nil,
			}

			detectedConfig := &models.BuildConfig{
				GoVersion:  detectedVersion,
				EntryPoint: detectedEntry,
				CGOEnabled: &detectedCGO,
			}

			merged := MergeConfigs(userConfig, detectedConfig, nil)

			// Detected values should be used when user values are empty
			if merged.GoVersion != detectedVersion {
				return false
			}
			if merged.EntryPoint != detectedEntry {
				return false
			}
			if merged.CGOEnabled == nil || *merged.CGOEnabled != detectedCGO {
				return false
			}

			return true
		},
		gen.AlphaString(),
		gen.AlphaString(),
		gen.Bool(),
	))

	properties.TestingRun(t)
}

// TestConfigMergeEdgeCases tests edge cases for config merging.
func TestConfigMergeEdgeCases(t *testing.T) {
	t.Run("nil user config returns detected config", func(t *testing.T) {
		detectedCGO := true
		detectedConfig := &models.BuildConfig{
			GoVersion:  "1.21",
			CGOEnabled: &detectedCGO,
		}

		merged := MergeConfigs(nil, detectedConfig, nil)

		if merged.GoVersion != "1.21" {
			t.Errorf("expected GoVersion=1.21, got %s", merged.GoVersion)
		}
		if merged.CGOEnabled == nil || *merged.CGOEnabled != true {
			t.Errorf("expected CGOEnabled=true, got %v", merged.CGOEnabled)
		}
	})

	t.Run("nil detected config returns user config", func(t *testing.T) {
		userCGO := false
		userConfig := &models.BuildConfig{
			GoVersion:  "1.22",
			CGOEnabled: &userCGO,
		}

		merged := MergeConfigs(userConfig, nil, nil)

		if merged.GoVersion != "1.22" {
			t.Errorf("expected GoVersion=1.22, got %s", merged.GoVersion)
		}
		if merged.CGOEnabled == nil || *merged.CGOEnabled != false {
			t.Errorf("expected CGOEnabled=false, got %v", merged.CGOEnabled)
		}
	})

	t.Run("both nil returns empty config", func(t *testing.T) {
		merged := MergeConfigs(nil, nil, nil)

		if merged == nil {
			t.Error("expected non-nil config")
		}
		if merged.GoVersion != "" {
			t.Errorf("expected empty GoVersion, got %s", merged.GoVersion)
		}
		if merged.CGOEnabled != nil {
			t.Errorf("expected nil CGOEnabled, got %v", merged.CGOEnabled)
		}
	})

	t.Run("environment vars are merged with user precedence", func(t *testing.T) {
		userConfig := &models.BuildConfig{
			EnvironmentVars: map[string]string{
				"KEY1": "user-value1",
				"KEY2": "user-value2",
			},
		}
		detectedConfig := &models.BuildConfig{
			EnvironmentVars: map[string]string{
				"KEY1": "detected-value1",
				"KEY3": "detected-value3",
			},
		}

		merged := MergeConfigs(userConfig, detectedConfig, nil)

		// User values should override detected values
		if merged.EnvironmentVars["KEY1"] != "user-value1" {
			t.Errorf("expected KEY1=user-value1, got %s", merged.EnvironmentVars["KEY1"])
		}
		// User values should be preserved
		if merged.EnvironmentVars["KEY2"] != "user-value2" {
			t.Errorf("expected KEY2=user-value2, got %s", merged.EnvironmentVars["KEY2"])
		}
		// Detected values should be included when not overridden
		if merged.EnvironmentVars["KEY3"] != "detected-value3" {
			t.Errorf("expected KEY3=detected-value3, got %s", merged.EnvironmentVars["KEY3"])
		}
	})
}
