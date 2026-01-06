package utils

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// UpsertEnvVars simulates the import upsert behavior.
// For any set of imported variables where some keys already exist,
// the import should update existing keys and create new ones.
// **Validates: Requirements 2.4, 2.5**
func UpsertEnvVars(existing map[string]string, imported map[string]string) map[string]string {
	result := make(map[string]string)
	// Copy existing vars
	for k, v := range existing {
		result[k] = v
	}
	// Upsert imported vars (update existing, create new)
	for k, v := range imported {
		result[k] = v
	}
	return result
}

// genEnvVarsMap generates a map of valid environment variable key-value pairs.
func genEnvVarsMap() gopter.Gen {
	return gen.IntRange(0, 5).FlatMap(func(v interface{}) gopter.Gen {
		count := v.(int)
		return gen.SliceOfN(count, gopter.CombineGens(
			genEnvKey(),
			genEnvValue(),
		)).Map(func(pairs [][]interface{}) map[string]string {
			result := make(map[string]string)
			for _, pair := range pairs {
				key := pair[0].(string)
				value := pair[1].(string)
				result[key] = value
			}
			return result
		})
	}, nil)
}

// **Feature: environment-variables, Property 7: Import Upsert Behavior**
// *For any* set of imported variables where some keys already exist,
// the import should update existing keys and create new ones,
// resulting in all imported values being present.
// **Validates: Requirements 2.4, 2.5**
func TestImportUpsertBehavior(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Property 7.1: All imported values are present in the result
	properties.Property("all imported values are present in result", prop.ForAll(
		func(existing, imported map[string]string) bool {
			result := UpsertEnvVars(existing, imported)

			// All imported values must be present with their imported values
			for key, importedValue := range imported {
				resultValue, exists := result[key]
				if !exists {
					return false
				}
				if resultValue != importedValue {
					return false
				}
			}
			return true
		},
		genEnvVarsMap(),
		genEnvVarsMap(),
	))

	// Property 7.2: Existing keys not in import are preserved
	properties.Property("existing keys not in import are preserved", prop.ForAll(
		func(existing, imported map[string]string) bool {
			result := UpsertEnvVars(existing, imported)

			// Existing keys not in import must be preserved
			for key, existingValue := range existing {
				if _, inImport := imported[key]; !inImport {
					resultValue, exists := result[key]
					if !exists {
						return false
					}
					if resultValue != existingValue {
						return false
					}
				}
			}
			return true
		},
		genEnvVarsMap(),
		genEnvVarsMap(),
	))

	// Property 7.3: Import overwrites existing keys with same name
	properties.Property("import overwrites existing keys with same name", prop.ForAll(
		func(key, existingValue, importedValue string) bool {
			existing := map[string]string{key: existingValue}
			imported := map[string]string{key: importedValue}

			result := UpsertEnvVars(existing, imported)

			// The result should have the imported value, not the existing one
			resultValue, exists := result[key]
			if !exists {
				return false
			}
			return resultValue == importedValue
		},
		genEnvKey(),
		genEnvValue(),
		genEnvValue(),
	))

	// Property 7.4: Result size is at most sum of unique keys
	properties.Property("result size is at most sum of unique keys", prop.ForAll(
		func(existing, imported map[string]string) bool {
			result := UpsertEnvVars(existing, imported)

			// Count unique keys
			uniqueKeys := make(map[string]bool)
			for k := range existing {
				uniqueKeys[k] = true
			}
			for k := range imported {
				uniqueKeys[k] = true
			}

			return len(result) == len(uniqueKeys)
		},
		genEnvVarsMap(),
		genEnvVarsMap(),
	))

	// Property 7.5: Empty import preserves all existing vars
	properties.Property("empty import preserves all existing vars", prop.ForAll(
		func(existing map[string]string) bool {
			imported := map[string]string{}
			result := UpsertEnvVars(existing, imported)

			if len(result) != len(existing) {
				return false
			}
			for k, v := range existing {
				if result[k] != v {
					return false
				}
			}
			return true
		},
		genEnvVarsMap(),
	))

	// Property 7.6: Import into empty existing creates all imported vars
	properties.Property("import into empty existing creates all imported vars", prop.ForAll(
		func(imported map[string]string) bool {
			existing := map[string]string{}
			result := UpsertEnvVars(existing, imported)

			if len(result) != len(imported) {
				return false
			}
			for k, v := range imported {
				if result[k] != v {
					return false
				}
			}
			return true
		},
		genEnvVarsMap(),
	))

	properties.TestingRun(t)
}
