package utils

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// genEnvKey generates a valid environment variable key.
func genEnvKey() gopter.Gen {
	return gen.IntRange(1, 30).FlatMap(func(v interface{}) gopter.Gen {
		length := v.(int)
		return gen.SliceOfN(length, gen.IntRange(0, 62)).Map(func(chars []int) string {
			result := make([]byte, len(chars))
			for i, c := range chars {
				if i == 0 {
					if c < 26 {
						result[i] = byte('A' + c)
					} else if c < 52 {
						result[i] = byte('a' + (c - 26))
					} else {
						result[i] = '_'
					}
				} else {
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

// genEnvValue generates a valid environment variable value.
func genEnvValue() gopter.Gen {
	return gen.IntRange(0, 50).FlatMap(func(v interface{}) gopter.Gen {
		length := v.(int)
		return gen.SliceOfN(length, gen.IntRange(32, 126)).Map(func(chars []int) string {
			result := make([]byte, len(chars))
			for i, c := range chars {
				result[i] = byte(c)
			}
			return string(result)
		})
	}, nil)
}


// **Feature: environment-variables, Property 3: .env File Parsing Round-Trip**
// **Validates: Requirements 2.2, 2.3**
func TestEnvFileParsingRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("round-trip preserves all key-value pairs", prop.ForAll(
		func(key string, value string) bool {
			// Create a single-entry map
			vars := map[string]string{key: value}

			// Serialize to .env format
			envContent := SerializeEnvFile(vars)

			// Parse the serialized content
			parsed := ParseEnvFile(envContent)

			// Verify round-trip
			if len(parsed) != 1 {
				return false
			}

			actualValue, exists := parsed[key]
			if !exists {
				return false
			}
			return actualValue == value
		},
		genEnvKey(),
		genEnvValue(),
	))

	properties.TestingRun(t)
}


// TestParseEnvFileHandlesComments verifies that comments are properly ignored.
func TestParseEnvFileHandlesComments(t *testing.T) {
	content := `# This is a comment
KEY1=value1
# Another comment
KEY2=value2
  # Indented comment
KEY3=value3`

	vars := ParseEnvFile(content)

	if len(vars) != 3 {
		t.Fatalf("Expected 3 vars, got %d", len(vars))
	}
	if vars["KEY1"] != "value1" {
		t.Errorf("KEY1: expected 'value1', got %q", vars["KEY1"])
	}
	if vars["KEY2"] != "value2" {
		t.Errorf("KEY2: expected 'value2', got %q", vars["KEY2"])
	}
	if vars["KEY3"] != "value3" {
		t.Errorf("KEY3: expected 'value3', got %q", vars["KEY3"])
	}
}

// TestParseEnvFileHandlesQuotedValues verifies that quoted values are properly parsed.
func TestParseEnvFileHandlesQuotedValues(t *testing.T) {
	content := `DOUBLE_QUOTED="hello world"
SINGLE_QUOTED='hello world'
WITH_EQUALS="key=value"
WITH_NEWLINE="line1\nline2"
WITH_TAB="col1\tcol2"`

	vars := ParseEnvFile(content)

	if vars["DOUBLE_QUOTED"] != "hello world" {
		t.Errorf("DOUBLE_QUOTED: expected 'hello world', got %q", vars["DOUBLE_QUOTED"])
	}
	if vars["SINGLE_QUOTED"] != "hello world" {
		t.Errorf("SINGLE_QUOTED: expected 'hello world', got %q", vars["SINGLE_QUOTED"])
	}
	if vars["WITH_EQUALS"] != "key=value" {
		t.Errorf("WITH_EQUALS: expected 'key=value', got %q", vars["WITH_EQUALS"])
	}
	if vars["WITH_NEWLINE"] != "line1\nline2" {
		t.Errorf("WITH_NEWLINE: expected 'line1\\nline2', got %q", vars["WITH_NEWLINE"])
	}
	if vars["WITH_TAB"] != "col1\tcol2" {
		t.Errorf("WITH_TAB: expected 'col1\\tcol2', got %q", vars["WITH_TAB"])
	}
}


// TestParseEnvFileHandlesExportPrefix verifies that export prefix is properly handled.
func TestParseEnvFileHandlesExportPrefix(t *testing.T) {
	content := `export KEY1=value1
export KEY2="value2"
KEY3=value3`

	vars := ParseEnvFile(content)

	if len(vars) != 3 {
		t.Fatalf("Expected 3 vars, got %d", len(vars))
	}
	if vars["KEY1"] != "value1" {
		t.Errorf("KEY1: expected 'value1', got %q", vars["KEY1"])
	}
	if vars["KEY2"] != "value2" {
		t.Errorf("KEY2: expected 'value2', got %q", vars["KEY2"])
	}
	if vars["KEY3"] != "value3" {
		t.Errorf("KEY3: expected 'value3', got %q", vars["KEY3"])
	}
}

// TestParseEnvFileHandlesEmptyLines verifies that empty lines are properly ignored.
func TestParseEnvFileHandlesEmptyLines(t *testing.T) {
	content := `KEY1=value1

KEY2=value2

   
KEY3=value3`

	vars := ParseEnvFile(content)

	if len(vars) != 3 {
		t.Fatalf("Expected 3 vars, got %d", len(vars))
	}
}
