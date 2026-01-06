package validation

import (
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: environment-variables, Property 4: Key Validation Rejects Invalid Keys**
// For any key that is empty, contains spaces, starts with a digit, or contains
// invalid characters, the API should reject the request with a validation error.
// **Validates: Requirements 5.2**

// genValidEnvKey generates a valid environment variable key.
// Valid keys: start with letter or underscore, contain only letters, numbers, underscores.
func genValidEnvKey() gopter.Gen {
	return gen.IntRange(1, MaxEnvKeyLength).FlatMap(func(v interface{}) gopter.Gen {
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

// genInvalidEnvKeyStartsWithDigit generates keys that start with a digit.
func genInvalidEnvKeyStartsWithDigit() gopter.Gen {
	return gen.IntRange(1, 50).FlatMap(func(v interface{}) gopter.Gen {
		suffixLen := v.(int)
		return gen.SliceOfN(suffixLen, gen.IntRange(0, 62)).Map(func(chars []int) string {
			// Start with a digit
			result := make([]byte, len(chars)+1)
			result[0] = byte('0' + (chars[0] % 10))
			for i, c := range chars {
				if c < 26 {
					result[i+1] = byte('A' + c)
				} else if c < 52 {
					result[i+1] = byte('a' + (c - 26))
				} else if c < 62 {
					result[i+1] = byte('0' + (c - 52))
				} else {
					result[i+1] = '_'
				}
			}
			return string(result)
		})
	}, nil)
}

// genInvalidEnvKeyWithSpaces generates keys that contain spaces.
func genInvalidEnvKeyWithSpaces() gopter.Gen {
	return gen.IntRange(1, 20).FlatMap(func(v interface{}) gopter.Gen {
		prefixLen := v.(int)
		return gen.IntRange(1, 20).FlatMap(func(v2 interface{}) gopter.Gen {
			suffixLen := v2.(int)
			return gen.SliceOfN(prefixLen+suffixLen, gen.IntRange(0, 51)).Map(func(chars []int) string {
				prefix := make([]byte, prefixLen)
				suffix := make([]byte, suffixLen)
				for i := 0; i < prefixLen; i++ {
					c := chars[i]
					if i == 0 {
						if c < 26 {
							prefix[i] = byte('A' + c)
						} else {
							prefix[i] = byte('a' + (c - 26))
						}
					} else {
						if c < 26 {
							prefix[i] = byte('A' + c)
						} else {
							prefix[i] = byte('a' + (c - 26))
						}
					}
				}
				for i := 0; i < suffixLen; i++ {
					c := chars[prefixLen+i]
					if c < 26 {
						suffix[i] = byte('A' + c)
					} else {
						suffix[i] = byte('a' + (c - 26))
					}
				}
				return string(prefix) + " " + string(suffix)
			})
		}, nil)
	}, nil)
}

// genInvalidEnvKeyWithInvalidChars generates keys with invalid characters.
func genInvalidEnvKeyWithInvalidChars() gopter.Gen {
	invalidChars := []byte{'-', '.', '!', '@', '#', '$', '%', '^', '&', '*', '(', ')', '+', '=', '[', ']', '{', '}', '|', '\\', '/', '?', '<', '>', ',', ';', ':', '"', '\'', '`', '~'}
	return gen.IntRange(0, len(invalidChars)-1).FlatMap(func(v interface{}) gopter.Gen {
		invalidChar := invalidChars[v.(int)]
		return gen.IntRange(1, 20).FlatMap(func(v2 interface{}) gopter.Gen {
			prefixLen := v2.(int)
			return gen.SliceOfN(prefixLen, gen.IntRange(0, 51)).Map(func(chars []int) string {
				prefix := make([]byte, prefixLen)
				for i, c := range chars {
					if i == 0 {
						if c < 26 {
							prefix[i] = byte('A' + c)
						} else {
							prefix[i] = byte('a' + (c - 26))
						}
					} else {
						if c < 26 {
							prefix[i] = byte('A' + c)
						} else {
							prefix[i] = byte('a' + (c - 26))
						}
					}
				}
				return string(prefix) + string(invalidChar) + "suffix"
			})
		}, nil)
	}, nil)
}

// genTooLongEnvKey generates keys that exceed the maximum length.
func genTooLongEnvKey() gopter.Gen {
	return gen.IntRange(MaxEnvKeyLength+1, MaxEnvKeyLength+100).FlatMap(func(v interface{}) gopter.Gen {
		length := v.(int)
		return gen.SliceOfN(length, gen.IntRange(0, 51)).Map(func(chars []int) string {
			result := make([]byte, len(chars))
			for i, c := range chars {
				if i == 0 {
					if c < 26 {
						result[i] = byte('A' + c)
					} else {
						result[i] = byte('a' + (c - 26))
					}
				} else {
					if c < 26 {
						result[i] = byte('A' + c)
					} else {
						result[i] = byte('a' + (c - 26))
					}
				}
			}
			return string(result)
		})
	}, nil)
}

// TestEnvKeyValidationRejectsInvalidKeys tests Property 4: Key Validation Rejects Invalid Keys.
func TestEnvKeyValidationRejectsInvalidKeys(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 4.1: Valid keys are accepted
	properties.Property("valid env keys are accepted", prop.ForAll(
		func(key string) bool {
			err := ValidateEnvKey(key)
			return err == nil
		},
		genValidEnvKey(),
	))

	// Property 4.2: Empty keys are rejected
	properties.Property("empty keys are rejected", prop.ForAll(
		func(_ int) bool {
			err := ValidateEnvKey("")
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "key" &&
				strings.Contains(validationErr.Message, "required")
		},
		gen.IntRange(0, 1), // Dummy generator
	))

	// Property 4.3: Keys starting with digit are rejected
	properties.Property("keys starting with digit are rejected", prop.ForAll(
		func(key string) bool {
			err := ValidateEnvKey(key)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "key" &&
				strings.Contains(validationErr.Message, "must start with a letter or underscore")
		},
		genInvalidEnvKeyStartsWithDigit(),
	))

	// Property 4.4: Keys with spaces are rejected
	properties.Property("keys with spaces are rejected", prop.ForAll(
		func(key string) bool {
			err := ValidateEnvKey(key)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "key"
		},
		genInvalidEnvKeyWithSpaces(),
	))

	// Property 4.5: Keys with invalid characters are rejected
	properties.Property("keys with invalid characters are rejected", prop.ForAll(
		func(key string) bool {
			err := ValidateEnvKey(key)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "key"
		},
		genInvalidEnvKeyWithInvalidChars(),
	))

	// Property 4.6: Keys exceeding max length are rejected
	properties.Property("keys exceeding max length are rejected", prop.ForAll(
		func(key string) bool {
			err := ValidateEnvKey(key)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "key" &&
				strings.Contains(validationErr.Message, "256 characters or less")
		},
		genTooLongEnvKey(),
	))

	// Property 4.7: Single letter keys are valid
	properties.Property("single letter keys are valid", prop.ForAll(
		func(letter byte) bool {
			var key string
			if letter < 26 {
				key = string(byte('A' + letter))
			} else {
				key = string(byte('a' + (letter % 26)))
			}
			err := ValidateEnvKey(key)
			return err == nil
		},
		gen.UInt8Range(0, 51),
	))

	// Property 4.8: Single underscore key is valid
	properties.Property("single underscore key is valid", prop.ForAll(
		func(_ int) bool {
			err := ValidateEnvKey("_")
			return err == nil
		},
		gen.IntRange(0, 1), // Dummy generator
	))

	// Property 4.9: Keys with underscores in middle are valid
	properties.Property("keys with underscores in middle are valid", prop.ForAll(
		func(prefixChars []int, suffixChars []int) bool {
			prefix := make([]byte, len(prefixChars)+1)
			prefix[0] = byte('A' + (prefixChars[0] % 26))
			for i, c := range prefixChars {
				if c < 26 {
					prefix[i+1] = byte('A' + c)
				} else {
					prefix[i+1] = byte('a' + (c - 26))
				}
			}
			suffix := make([]byte, len(suffixChars))
			for i, c := range suffixChars {
				if c < 26 {
					suffix[i] = byte('A' + c)
				} else {
					suffix[i] = byte('a' + (c - 26))
				}
			}
			key := string(prefix) + "_" + string(suffix)
			if len(key) > MaxEnvKeyLength {
				return true // Skip keys that would be too long
			}
			err := ValidateEnvKey(key)
			return err == nil
		},
		gen.SliceOfN(5, gen.IntRange(0, 51)),
		gen.SliceOfN(5, gen.IntRange(0, 51)),
	))

	properties.TestingRun(t)
}


// **Feature: environment-variables, Property 5: Value Length Validation**
// For any value that exceeds the maximum length (32KB), the API should reject
// the request with a validation error.
// **Validates: Requirements 5.3**

// genValidEnvValue generates a valid environment variable value (within 32KB).
func genValidEnvValue() gopter.Gen {
	return gen.IntRange(0, MaxEnvValueLength).FlatMap(func(v interface{}) gopter.Gen {
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

// genTooLongEnvValue generates values that exceed the maximum length (32KB).
func genTooLongEnvValue() gopter.Gen {
	return gen.IntRange(MaxEnvValueLength+1, MaxEnvValueLength+1000).FlatMap(func(v interface{}) gopter.Gen {
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

// TestEnvValueLengthValidation tests Property 5: Value Length Validation.
func TestEnvValueLengthValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 5.1: Valid values (within 32KB) are accepted
	properties.Property("valid env values are accepted", prop.ForAll(
		func(value string) bool {
			err := ValidateEnvValue(value)
			return err == nil
		},
		genValidEnvValue(),
	))

	// Property 5.2: Empty values are accepted
	properties.Property("empty values are accepted", prop.ForAll(
		func(_ int) bool {
			err := ValidateEnvValue("")
			return err == nil
		},
		gen.IntRange(0, 1), // Dummy generator
	))

	// Property 5.3: Values exceeding 32KB are rejected
	properties.Property("values exceeding 32KB are rejected", prop.ForAll(
		func(value string) bool {
			err := ValidateEnvValue(value)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "value" &&
				strings.Contains(validationErr.Message, "32KB or less")
		},
		genTooLongEnvValue(),
	))

	// Property 5.4: Values with special characters are accepted
	properties.Property("values with special characters are accepted", prop.ForAll(
		func(chars []int) bool {
			// Generate a value with various special characters
			specialChars := []byte{' ', '\t', '\n', '!', '@', '#', '$', '%', '^', '&', '*', '(', ')', '-', '+', '=', '[', ']', '{', '}', '|', '\\', '/', '?', '<', '>', ',', '.', ';', ':', '"', '\'', '`', '~'}
			result := make([]byte, len(chars))
			for i, c := range chars {
				result[i] = specialChars[c%len(specialChars)]
			}
			value := string(result)
			err := ValidateEnvValue(value)
			return err == nil
		},
		gen.SliceOfN(100, gen.IntRange(0, 100)),
	))

	// Property 5.5: Values at exactly 32KB boundary are accepted
	properties.Property("values at exactly 32KB are accepted", prop.ForAll(
		func(_ int) bool {
			value := strings.Repeat("a", MaxEnvValueLength)
			err := ValidateEnvValue(value)
			return err == nil
		},
		gen.IntRange(0, 1), // Dummy generator
	))

	// Property 5.6: Values at 32KB + 1 byte are rejected
	properties.Property("values at 32KB + 1 byte are rejected", prop.ForAll(
		func(_ int) bool {
			value := strings.Repeat("a", MaxEnvValueLength+1)
			err := ValidateEnvValue(value)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "value" &&
				strings.Contains(validationErr.Message, "32KB or less")
		},
		gen.IntRange(0, 1), // Dummy generator
	))

	properties.TestingRun(t)
}
