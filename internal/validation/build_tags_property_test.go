package validation

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: ui-api-alignment, Property 9: Build Tags Validation**
// For any build tag string, the builder SHALL reject tags containing invalid characters
// (spaces, special characters except underscore and dots).
// **Validates: Requirements 17.5**

// genValidBuildTag generates a valid Go build tag.
// Valid tags contain only alphanumeric characters, underscores, and dots.
func genValidBuildTag() gopter.Gen {
	return gen.IntRange(1, 30).FlatMap(func(v interface{}) gopter.Gen {
		length := v.(int)
		return gen.SliceOfN(length, gen.IntRange(0, 63)).Map(func(chars []int) string {
			result := make([]byte, len(chars))
			for i, c := range chars {
				switch {
				case c < 26:
					result[i] = byte('a' + c)
				case c < 52:
					result[i] = byte('A' + (c - 26))
				case c < 62:
					result[i] = byte('0' + (c - 52))
				case c == 62:
					result[i] = '_'
				default:
					result[i] = '.'
				}
			}
			return string(result)
		})
	}, nil)
}

// genBuildTagWithSpace generates a build tag containing a space.
func genBuildTagWithSpace() gopter.Gen {
	return gen.IntRange(1, 10).FlatMap(func(v interface{}) gopter.Gen {
		prefixLen := v.(int)
		return gen.IntRange(1, 10).FlatMap(func(v2 interface{}) gopter.Gen {
			suffixLen := v2.(int)
			return gen.SliceOfN(prefixLen+suffixLen, gen.IntRange(0, 25)).Map(func(chars []int) string {
				prefix := make([]byte, prefixLen)
				suffix := make([]byte, suffixLen)
				for i := 0; i < prefixLen && i < len(chars); i++ {
					prefix[i] = byte('a' + (chars[i] % 26))
				}
				for i := 0; i < suffixLen && prefixLen+i < len(chars); i++ {
					suffix[i] = byte('a' + (chars[prefixLen+i] % 26))
				}
				return string(prefix) + " " + string(suffix)
			})
		}, nil)
	}, nil)
}

// genBuildTagWithInvalidChar generates a build tag containing invalid special characters.
func genBuildTagWithInvalidChar() gopter.Gen {
	invalidChars := []byte{'!', '@', '#', '$', '%', '^', '&', '*', '(', ')', '-', '+', '=', '[', ']', '{', '}', '|', '\\', '/', '?', '<', '>', ',', ';', ':', '"', '\'', '`', '~'}
	return gen.IntRange(1, 10).FlatMap(func(v interface{}) gopter.Gen {
		prefixLen := v.(int)
		return gen.IntRange(0, len(invalidChars)-1).FlatMap(func(v2 interface{}) gopter.Gen {
			invalidCharIdx := v2.(int)
			return gen.SliceOfN(prefixLen, gen.IntRange(0, 25)).Map(func(chars []int) string {
				prefix := make([]byte, prefixLen)
				for i, c := range chars {
					prefix[i] = byte('a' + (c % 26))
				}
				return string(prefix) + string(invalidChars[invalidCharIdx])
			})
		}, nil)
	}, nil)
}

// TestBuildTagsValidation tests Property 9: Build Tags Validation.
func TestBuildTagsValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 9.1: Valid build tags are accepted
	properties.Property("valid build tags are accepted", prop.ForAll(
		func(tag string) bool {
			err := ValidateBuildTag(tag)
			return err == nil
		},
		genValidBuildTag(),
	))

	// Property 9.2: Empty build tags are rejected
	properties.Property("empty build tags are rejected", prop.ForAll(
		func(_ int) bool {
			err := ValidateBuildTag("")
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "build_tag" &&
				containsSubstring(validationErr.Message, "cannot be empty")
		},
		gen.IntRange(0, 1), // Dummy generator
	))

	// Property 9.3: Build tags with spaces are rejected
	properties.Property("build tags with spaces are rejected", prop.ForAll(
		func(tag string) bool {
			err := ValidateBuildTag(tag)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "build_tag" &&
				containsSubstring(validationErr.Message, "cannot contain spaces")
		},
		genBuildTagWithSpace(),
	))

	// Property 9.4: Build tags with invalid special characters are rejected
	properties.Property("build tags with invalid special characters are rejected", prop.ForAll(
		func(tag string) bool {
			err := ValidateBuildTag(tag)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "build_tag"
		},
		genBuildTagWithInvalidChar(),
	))

	// Property 9.5: Empty build tags slice is valid
	properties.Property("empty build tags slice is valid", prop.ForAll(
		func(_ int) bool {
			err := ValidateBuildTags([]string{})
			return err == nil
		},
		gen.IntRange(0, 1), // Dummy generator
	))

	// Property 9.6: Slice with all valid tags is accepted
	properties.Property("slice with all valid tags is accepted", prop.ForAll(
		func(tags []string) bool {
			err := ValidateBuildTags(tags)
			return err == nil
		},
		gen.SliceOfN(5, genValidBuildTag()),
	))

	// Property 9.7: Slice with any invalid tag is rejected
	properties.Property("slice with any invalid tag is rejected", prop.ForAll(
		func(validTags []string, invalidTag string) bool {
			// Insert invalid tag at a random position
			tags := append(validTags, invalidTag)
			err := ValidateBuildTags(tags)
			return err != nil
		},
		gen.SliceOfN(3, genValidBuildTag()),
		genBuildTagWithSpace(),
	))

	properties.TestingRun(t)
}
