package deploy

import (
	"reflect"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: environment-variables, Property 6: Service-Level Override Precedence**
// For any app-level secret and service-level variable with the same key,
// the merged environment for deployment should contain the service-level value.
// **Validates: Requirements 3.2, 6.1, 6.3**

// genValidEnvKey generates a valid environment variable key.
// Valid keys: start with letter or underscore, contain only letters, numbers, underscores.
func genValidEnvKey() gopter.Gen {
	return gen.IntRange(1, 50).FlatMap(func(v interface{}) gopter.Gen {
		length := v.(int)
		return gen.SliceOfN(length, gen.IntRange(0, 62)).Map(func(chars []int) string {
			result := make([]byte, len(chars))
			for i, c := range chars {
				if i == 0 {
					// First char must be letter or underscore
					if c < 26 {
						result[i] = byte('A' + c)
					} else if c < 52 {
						result[i] = byte('a' + (c - 26))
					} else {
						result[i] = '_'
					}
				} else {
					// Subsequent chars can be letter, digit, or underscore
					if c < 26 {
						result[i] = byte('A' + c)
					} else if c < 52 {
						result[i] = byte('a' + (c - 26))
					} else if c < 62 {
						result[i] = byte('0' + (c - 52))
					} else {
						result[i] = '_'
					}
				}
			}
			return string(result)
		})
	}, nil)
}

// genEnvValue generates a random environment variable value.
func genEnvValue() gopter.Gen {
	return gen.IntRange(1, 100).FlatMap(func(v interface{}) gopter.Gen {
		length := v.(int)
		return gen.SliceOfN(length, gen.UInt8()).Map(func(chars []uint8) string {
			result := make([]byte, len(chars))
			for i, c := range chars {
				// Generate printable ASCII characters
				result[i] = byte(32 + (c % 95))
			}
			return string(result)
		})
	}, nil)
}

// genEnvMap generates a map of environment variables.
func genEnvMap() gopter.Gen {
	return gen.IntRange(0, 10).FlatMap(func(v interface{}) gopter.Gen {
		size := v.(int)
		return gen.SliceOfN(size, gen.Struct(reflect.TypeOf(struct {
			Key   string
			Value string
		}{}), map[string]gopter.Gen{
			"Key":   genValidEnvKey(),
			"Value": genEnvValue(),
		})).Map(func(entries []struct {
			Key   string
			Value string
		}) map[string]string {
			result := make(map[string]string, len(entries))
			for _, e := range entries {
				result[e.Key] = e.Value
			}
			return result
		})
	}, nil)
}

// TestServiceLevelOverridePrecedence tests Property 6: Service-Level Override Precedence.
func TestServiceLevelOverridePrecedence(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 6.1: Service-level values override app-level secrets with same key
	properties.Property("service-level values override app-level secrets", prop.ForAll(
		func(key, appValue, serviceValue string) bool {
			appSecrets := map[string]string{key: appValue}
			serviceEnvVars := map[string]string{key: serviceValue}

			merged := MergeEnvVars(appSecrets, serviceEnvVars)

			// The merged result should have the service-level value
			return merged[key] == serviceValue
		},
		genValidEnvKey(),
		genEnvValue(),
		genEnvValue(),
	))

	// Property 6.2: App-level secrets are preserved when no service-level override exists
	properties.Property("app-level secrets preserved when no override", prop.ForAll(
		func(appKey, appValue, serviceKey, serviceValue string) bool {
			// Ensure keys are different
			if appKey == serviceKey {
				return true // Skip this case
			}

			appSecrets := map[string]string{appKey: appValue}
			serviceEnvVars := map[string]string{serviceKey: serviceValue}

			merged := MergeEnvVars(appSecrets, serviceEnvVars)

			// Both values should be present
			return merged[appKey] == appValue && merged[serviceKey] == serviceValue
		},
		genValidEnvKey(),
		genEnvValue(),
		genValidEnvKey(),
		genEnvValue(),
	))

	// Property 6.3: Empty service env vars don't affect app secrets
	properties.Property("empty service env vars preserve app secrets", prop.ForAll(
		func(appKey, appValue string) bool {
			appSecrets := map[string]string{appKey: appValue}
			serviceEnvVars := map[string]string{}

			merged := MergeEnvVars(appSecrets, serviceEnvVars)

			return merged[appKey] == appValue && len(merged) == 1
		},
		genValidEnvKey(),
		genEnvValue(),
	))

	// Property 6.4: Empty app secrets don't affect service env vars
	properties.Property("empty app secrets preserve service env vars", prop.ForAll(
		func(serviceKey, serviceValue string) bool {
			appSecrets := map[string]string{}
			serviceEnvVars := map[string]string{serviceKey: serviceValue}

			merged := MergeEnvVars(appSecrets, serviceEnvVars)

			return merged[serviceKey] == serviceValue && len(merged) == 1
		},
		genValidEnvKey(),
		genEnvValue(),
	))

	// Property 6.5: Both empty maps result in empty merged map
	properties.Property("both empty maps result in empty merged map", prop.ForAll(
		func(_ int) bool {
			appSecrets := map[string]string{}
			serviceEnvVars := map[string]string{}

			merged := MergeEnvVars(appSecrets, serviceEnvVars)

			return len(merged) == 0
		},
		gen.IntRange(0, 1), // Dummy generator
	))

	// Property 6.6: Merged map size is at most sum of both maps
	properties.Property("merged map size is at most sum of both maps", prop.ForAll(
		func(appSecrets, serviceEnvVars map[string]string) bool {
			merged := MergeEnvVars(appSecrets, serviceEnvVars)

			return len(merged) <= len(appSecrets)+len(serviceEnvVars)
		},
		genEnvMap(),
		genEnvMap(),
	))

	// Property 6.7: All service env vars are present in merged result
	properties.Property("all service env vars present in merged result", prop.ForAll(
		func(appSecrets, serviceEnvVars map[string]string) bool {
			merged := MergeEnvVars(appSecrets, serviceEnvVars)

			for k, v := range serviceEnvVars {
				if merged[k] != v {
					return false
				}
			}
			return true
		},
		genEnvMap(),
		genEnvMap(),
	))

	// Property 6.8: App secrets without service override are present in merged result
	properties.Property("app secrets without override present in merged result", prop.ForAll(
		func(appSecrets, serviceEnvVars map[string]string) bool {
			merged := MergeEnvVars(appSecrets, serviceEnvVars)

			for k, v := range appSecrets {
				// If service doesn't have this key, app value should be present
				if _, hasServiceOverride := serviceEnvVars[k]; !hasServiceOverride {
					if merged[k] != v {
						return false
					}
				}
			}
			return true
		},
		genEnvMap(),
		genEnvMap(),
	))

	properties.TestingRun(t)
}
