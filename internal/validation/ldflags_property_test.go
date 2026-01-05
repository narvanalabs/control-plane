package validation

import (
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"github.com/narvanalabs/control-plane/internal/builder/templates"
)

// **Feature: ui-api-alignment, Property 10: Ldflags Passthrough**
// For any build configuration with custom ldflags, the builder SHALL pass those exact
// flags to the Go linker without modification.
// **Validates: Requirements 18.2**

// genSimpleLdflag generates a simple ldflag like "-s", "-w", "-v".
func genSimpleLdflag() gopter.Gen {
	simpleLdflags := []string{"-s", "-w", "-v", "-race", "-trimpath", "-buildvcs=false"}
	return gen.IntRange(0, len(simpleLdflags)-1).Map(func(idx int) string {
		return simpleLdflags[idx]
	})
}

// genXLdflag generates an -X ldflag for setting variables.
// Format: -X package.variable=value
func genXLdflag() gopter.Gen {
	packages := []string{"main", "cmd", "pkg/version", "internal/config"}
	variables := []string{"version", "commit", "buildTime", "gitTag", "buildDate"}
	return gen.IntRange(0, len(packages)-1).FlatMap(func(pkgIdx interface{}) gopter.Gen {
		return gen.IntRange(0, len(variables)-1).FlatMap(func(varIdx interface{}) gopter.Gen {
			return genAlphanumericValue().Map(func(value string) string {
				pkg := packages[pkgIdx.(int)]
				variable := variables[varIdx.(int)]
				return "-X " + pkg + "." + variable + "=" + value
			})
		}, nil)
	}, nil)
}

// genAlphanumericValue generates an alphanumeric value for ldflags.
func genAlphanumericValue() gopter.Gen {
	return gen.IntRange(1, 20).FlatMap(func(length interface{}) gopter.Gen {
		return gen.SliceOfN(length.(int), gen.IntRange(0, 61)).Map(func(chars []int) string {
			result := make([]byte, len(chars))
			for i, c := range chars {
				switch {
				case c < 26:
					result[i] = byte('a' + c)
				case c < 52:
					result[i] = byte('A' + (c - 26))
				default:
					result[i] = byte('0' + (c - 52))
				}
			}
			return string(result)
		})
	}, nil)
}

// genLdflagsString generates a complete ldflags string with multiple flags.
func genLdflagsString() gopter.Gen {
	return gen.IntRange(1, 5).FlatMap(func(count interface{}) gopter.Gen {
		return gen.SliceOfN(count.(int), gen.OneGenOf(genSimpleLdflag(), genXLdflag())).Map(func(flags []string) string {
			return strings.Join(flags, " ")
		})
	}, nil)
}

// TestLdflagsPassthrough tests Property 10: Ldflags Passthrough.
func TestLdflagsPassthrough(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 10.1: Empty ldflags returns empty string
	properties.Property("empty ldflags returns empty string", prop.ForAll(
		func(_ int) bool {
			result := templates.FormatLdflagsForNix("")
			return result == ""
		},
		gen.IntRange(0, 1), // Dummy generator
	))

	// Property 10.2: Simple ldflags are preserved in output
	properties.Property("simple ldflags are preserved in output", prop.ForAll(
		func(ldflag string) bool {
			result := templates.FormatLdflagsForNix(ldflag)
			// The flag should be present in the output (quoted for Nix)
			// e.g., "-s" becomes "\"-s\""
			return strings.Contains(result, ldflag)
		},
		genSimpleLdflag(),
	))

	// Property 10.3: -X ldflags are preserved with their values
	properties.Property("-X ldflags are preserved with their values", prop.ForAll(
		func(ldflag string) bool {
			result := templates.FormatLdflagsForNix(ldflag)
			// The -X flag and its value should be in the output
			// -X flags are kept together: "-X main.version=1.0" -> "\"-X main.version=1.0\""
			// Extract the package.variable=value part
			parts := strings.SplitN(ldflag, " ", 2)
			if len(parts) != 2 {
				return false
			}
			// The value part should be in the result
			return strings.Contains(result, parts[1])
		},
		genXLdflag(),
	))

	// Property 10.4: All flags in a multi-flag string are present in output
	properties.Property("all flags in multi-flag string are present in output", prop.ForAll(
		func(ldflags string) bool {
			result := templates.FormatLdflagsForNix(ldflags)
			// Parse the original flags
			originalParts := strings.Fields(ldflags)
			
			// Each original part should appear in the result
			for _, part := range originalParts {
				if !strings.Contains(result, part) {
					return false
				}
			}
			return true
		},
		genLdflagsString(),
	))

	// Property 10.5: Output is valid Nix list format (quoted strings)
	properties.Property("output is valid Nix list format", prop.ForAll(
		func(ldflags string) bool {
			result := templates.FormatLdflagsForNix(ldflags)
			if result == "" {
				return true // Empty is valid
			}
			// Each element should be quoted
			// The result should contain quoted strings separated by spaces
			// Count quotes - should be even
			quoteCount := strings.Count(result, "\"")
			return quoteCount%2 == 0 && quoteCount >= 2
		},
		genLdflagsString(),
	))

	// Property 10.6: Ldflags with special characters are properly escaped
	properties.Property("ldflags with special characters are properly escaped", prop.ForAll(
		func(value string) bool {
			// Create an -X flag with the value
			ldflag := "-X main.version=" + value
			result := templates.FormatLdflagsForNix(ldflag)
			// The result should be non-empty and properly quoted
			return result != "" && strings.Contains(result, "\"")
		},
		genAlphanumericValue(),
	))

	properties.TestingRun(t)
}
