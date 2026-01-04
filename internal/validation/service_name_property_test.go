package validation

import (
	"reflect"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: backend-source-of-truth, Property 9: Service Name DNS Label Validity**
// For any service name, it SHALL be accepted only if it is a valid DNS label:
// 1-63 characters, lowercase alphanumeric and hyphens, starting with a letter,
// not starting or ending with a hyphen.
// **Validates: Requirements 10.1, 10.2, 10.3**

// genValidDNSLabel generates a valid DNS label.
func genValidDNSLabel() gopter.Gen {
	// Generate a valid DNS label: starts with letter, contains lowercase alphanumeric and hyphens
	return gen.IntRange(1, 63).FlatMap(func(v interface{}) gopter.Gen {
		length := v.(int)
		return gen.SliceOfN(length, gen.IntRange(0, 37)).Map(func(chars []int) string {
			result := make([]byte, len(chars))
			for i, c := range chars {
				if i == 0 {
					// First char must be a letter
					result[i] = byte('a' + (c % 26))
				} else if i == len(chars)-1 && len(chars) > 1 {
					// Last char must be letter or digit (not hyphen)
					if c < 26 {
						result[i] = byte('a' + c)
					} else {
						result[i] = byte('0' + (c % 10))
					}
				} else {
					// Middle chars can be letter, digit, or hyphen
					if c < 26 {
						result[i] = byte('a' + c)
					} else if c < 36 {
						result[i] = byte('0' + (c - 26))
					} else {
						result[i] = '-'
					}
				}
			}
			return string(result)
		})
	}, reflect.TypeOf(""))
}

// genInvalidDNSLabel generates an invalid DNS label.
func genInvalidDNSLabel() gopter.Gen {
	return gen.OneGenOf(
		// Empty string
		gen.Const(""),
		// Too long (> 63 chars)
		gen.SliceOfN(64, gen.IntRange(0, 25)).Map(func(chars []int) string {
			result := make([]byte, len(chars))
			for i, c := range chars {
				result[i] = byte('a' + c)
			}
			return string(result)
		}),
		// Starts with hyphen
		gen.SliceOfN(5, gen.IntRange(0, 25)).Map(func(chars []int) string {
			result := make([]byte, len(chars))
			for i, c := range chars {
				result[i] = byte('a' + c)
			}
			return "-" + string(result)
		}),
		// Ends with hyphen
		gen.SliceOfN(5, gen.IntRange(0, 25)).Map(func(chars []int) string {
			result := make([]byte, len(chars)+1)
			result[0] = 'a'
			for i, c := range chars {
				result[i+1] = byte('a' + c)
			}
			return string(result) + "-"
		}),
		// Starts with digit
		gen.SliceOfN(5, gen.IntRange(0, 25)).Map(func(chars []int) string {
			result := make([]byte, len(chars))
			for i, c := range chars {
				result[i] = byte('a' + c)
			}
			return "1" + string(result)
		}),
		// Contains uppercase
		gen.SliceOfN(5, gen.IntRange(0, 25)).Map(func(chars []int) string {
			result := make([]byte, len(chars)+2)
			result[0] = 'a'
			result[1] = 'A' // uppercase
			for i, c := range chars {
				result[i+2] = byte('a' + c)
			}
			return string(result)
		}),
		// Contains invalid characters
		gen.SliceOfN(5, gen.IntRange(0, 25)).Map(func(chars []int) string {
			result := make([]byte, len(chars))
			for i, c := range chars {
				result[i] = byte('a' + c)
			}
			return "a_" + string(result)
		}),
	)
}

// TestServiceNameDNSLabelValidity tests Property 9: Service Name DNS Label Validity.
func TestServiceNameDNSLabelValidity(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 9.1: Valid DNS labels are accepted
	properties.Property("valid DNS labels are accepted", prop.ForAll(
		func(name string) bool {
			err := ValidateServiceName(name)
			return err == nil
		},
		genValidDNSLabel(),
	))

	// Property 9.2: Empty names are rejected
	properties.Property("empty names are rejected", prop.ForAll(
		func(_ int) bool {
			err := ValidateServiceName("")
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "name" &&
				validationErr.Message == "service name is required"
		},
		gen.IntRange(0, 1), // Dummy generator
	))

	// Property 9.3: Names longer than 63 characters are rejected
	properties.Property("names longer than 63 chars are rejected", prop.ForAll(
		func(extraLen int) bool {
			// Generate a name that's 64+ characters
			name := make([]byte, 64+extraLen)
			for i := range name {
				name[i] = 'a'
			}
			err := ValidateServiceName(string(name))
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "name" &&
				containsSubstring(validationErr.Message, "63 characters or less")
		},
		gen.IntRange(0, 100),
	))

	// Property 9.4: Names starting with hyphen are rejected
	properties.Property("names starting with hyphen are rejected", prop.ForAll(
		func(chars []int) bool {
			// Generate a lowercase suffix
			suffix := make([]byte, len(chars))
			for i, c := range chars {
				suffix[i] = byte('a' + (c % 26))
			}
			if len(suffix) == 0 {
				suffix = []byte("a")
			}
			name := "-" + string(suffix)
			err := ValidateServiceName(name)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "name" &&
				containsSubstring(validationErr.Message, "cannot start with a hyphen")
		},
		gen.SliceOfN(5, gen.IntRange(0, 25)),
	))

	// Property 9.5: Names ending with hyphen are rejected
	properties.Property("names ending with hyphen are rejected", prop.ForAll(
		func(chars []int) bool {
			// Generate a lowercase prefix starting with letter
			prefix := make([]byte, len(chars)+1)
			prefix[0] = byte('a' + (chars[0] % 26))
			for i, c := range chars {
				prefix[i+1] = byte('a' + (c % 26))
			}
			name := string(prefix) + "-"
			err := ValidateServiceName(name)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "name" &&
				containsSubstring(validationErr.Message, "cannot end with a hyphen")
		},
		gen.SliceOfN(5, gen.IntRange(0, 25)),
	))

	// Property 9.6: Names starting with digit are rejected
	properties.Property("names starting with digit are rejected", prop.ForAll(
		func(digit int, chars []int) bool {
			// Generate lowercase suffix
			suffix := make([]byte, len(chars))
			for i, c := range chars {
				suffix[i] = byte('a' + (c % 26))
			}
			name := string(byte('0'+digit%10)) + string(suffix)
			err := ValidateServiceName(name)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "name" &&
				containsSubstring(validationErr.Message, "valid DNS label")
		},
		gen.IntRange(0, 9),
		gen.SliceOfN(5, gen.IntRange(0, 25)),
	))

	// Property 9.7: Names with uppercase letters are rejected
	properties.Property("names with uppercase letters are rejected", prop.ForAll(
		func(prefixChars []int, upper byte, suffixChars []int) bool {
			// Generate lowercase prefix starting with letter
			prefix := make([]byte, len(prefixChars)+1)
			prefix[0] = byte('a' + (prefixChars[0] % 26))
			for i, c := range prefixChars {
				prefix[i+1] = byte('a' + (c % 26))
			}
			// Generate uppercase character
			upperChar := byte('A' + (upper % 26))
			// Generate lowercase suffix
			suffix := make([]byte, len(suffixChars))
			for i, c := range suffixChars {
				suffix[i] = byte('a' + (c % 26))
			}
			name := string(prefix) + string(upperChar) + string(suffix)
			err := ValidateServiceName(name)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "name" &&
				containsSubstring(validationErr.Message, "valid DNS label")
		},
		gen.SliceOfN(3, gen.IntRange(0, 25)),
		gen.UInt8(),
		gen.SliceOfN(3, gen.IntRange(0, 25)),
	))

	// Property 9.8: Single letter names are valid
	properties.Property("single letter names are valid", prop.ForAll(
		func(letter byte) bool {
			name := string(byte('a' + (letter % 26)))
			err := ValidateServiceName(name)
			return err == nil
		},
		gen.UInt8(),
	))

	// Property 9.9: Names with only hyphens in the middle are valid
	properties.Property("names with hyphens in middle are valid", prop.ForAll(
		func(prefixChars []int, suffixChars []int) bool {
			// Generate lowercase prefix starting with letter
			prefix := make([]byte, len(prefixChars)+1)
			prefix[0] = byte('a' + (prefixChars[0] % 26))
			for i, c := range prefixChars {
				prefix[i+1] = byte('a' + (c % 26))
			}
			// Generate lowercase suffix ending with letter/digit
			suffix := make([]byte, len(suffixChars)+1)
			for i, c := range suffixChars {
				suffix[i] = byte('a' + (c % 26))
			}
			suffix[len(suffix)-1] = byte('a' + (suffixChars[len(suffixChars)-1] % 26))

			name := string(prefix) + "-" + string(suffix)
			if len(name) > 63 {
				return true // Skip names that would be too long
			}
			err := ValidateServiceName(name)
			return err == nil
		},
		gen.SliceOfN(5, gen.IntRange(0, 25)),
		gen.SliceOfN(5, gen.IntRange(0, 25)),
	))

	properties.TestingRun(t)
}
