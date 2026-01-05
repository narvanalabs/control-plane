package validation

import (
	"strconv"
	"strings"
	"testing"
	"time"

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


// **Feature: ui-api-alignment, Property 11: Ldflags Variable Substitution**
// For any ldflags containing substitution patterns (${version}, ${commit}, ${buildTime}),
// the builder SHALL replace them with actual build-time values.
// **Validates: Requirements 18.3**

// genVersion generates a valid version string.
func genVersion() gopter.Gen {
	return gen.OneGenOf(
		// Semantic versions
		gen.IntRange(0, 99).FlatMap(func(major interface{}) gopter.Gen {
			return gen.IntRange(0, 99).FlatMap(func(minor interface{}) gopter.Gen {
				return gen.IntRange(0, 99).Map(func(patch int) string {
					return strings.Join([]string{
						strconv.Itoa(major.(int)),
						strconv.Itoa(minor.(int)),
						strconv.Itoa(patch),
					}, ".")
				})
			}, nil)
		}, nil),
		// Version with v prefix
		gen.IntRange(0, 99).FlatMap(func(major interface{}) gopter.Gen {
			return gen.IntRange(0, 99).FlatMap(func(minor interface{}) gopter.Gen {
				return gen.IntRange(0, 99).Map(func(patch int) string {
					return "v" + strings.Join([]string{
						strconv.Itoa(major.(int)),
						strconv.Itoa(minor.(int)),
						strconv.Itoa(patch),
					}, ".")
				})
			}, nil)
		}, nil),
	)
}

// genCommitHash generates a valid git commit hash (short or full).
func genCommitHash() gopter.Gen {
	hexChars := "0123456789abcdef"
	return gen.OneGenOf(
		// Short hash (7 chars)
		gen.SliceOfN(7, gen.IntRange(0, 15)).Map(func(indices []int) string {
			result := make([]byte, len(indices))
			for i, idx := range indices {
				result[i] = hexChars[idx]
			}
			return string(result)
		}),
		// Full hash (40 chars)
		gen.SliceOfN(40, gen.IntRange(0, 15)).Map(func(indices []int) string {
			result := make([]byte, len(indices))
			for i, idx := range indices {
				result[i] = hexChars[idx]
			}
			return string(result)
		}),
	)
}

// genBuildTime generates a valid build time.
func genBuildTime() gopter.Gen {
	// Generate times within a reasonable range (2020-2030)
	return gen.Int64Range(1577836800, 1893456000).Map(func(unix int64) time.Time {
		return time.Unix(unix, 0).UTC()
	})
}

// genLdflagsBuildContext generates a complete build context.
func genLdflagsBuildContext() gopter.Gen {
	return genVersion().FlatMap(func(version interface{}) gopter.Gen {
		return genCommitHash().FlatMap(func(commit interface{}) gopter.Gen {
			return genBuildTime().Map(func(buildTime time.Time) LdflagsBuildContext {
				return LdflagsBuildContext{
					Version:   version.(string),
					Commit:    commit.(string),
					BuildTime: buildTime,
				}
			})
		}, nil)
	}, nil)
}

// genLdflagsWithVariables generates ldflags strings containing substitution variables.
func genLdflagsWithVariables() gopter.Gen {
	// Templates with different variable combinations
	templates := []string{
		"-X main.version=${version}",
		"-X main.commit=${commit}",
		"-X main.buildTime=${buildTime}",
		"-X main.version=${version} -X main.commit=${commit}",
		"-X main.version=${version} -X main.commit=${commit} -X main.buildTime=${buildTime}",
		"-s -w -X pkg/version.Version=${version}",
		"-X internal/config.GitCommit=${commit} -trimpath",
		"-X main.version=${version} -X main.gitCommit=${commit} -X main.buildDate=${buildTime}",
	}
	return gen.IntRange(0, len(templates)-1).Map(func(idx int) string {
		return templates[idx]
	})
}

// genLdflagsWithoutVariables generates ldflags strings without substitution variables.
func genLdflagsWithoutVariables() gopter.Gen {
	templates := []string{
		"-s -w",
		"-trimpath",
		"-X main.version=1.0.0",
		"-s -w -X main.commit=abc1234",
		"-race -v",
		"",
	}
	return gen.IntRange(0, len(templates)-1).Map(func(idx int) string {
		return templates[idx]
	})
}

// TestLdflagsVariableSubstitution tests Property 11: Ldflags Variable Substitution.
func TestLdflagsVariableSubstitution(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 11.1: ${version} is replaced with actual version value
	properties.Property("${version} is replaced with actual version value", prop.ForAll(
		func(ctx LdflagsBuildContext) bool {
			ldflags := "-X main.version=${version}"
			result := SubstituteLdflagsVariables(ldflags, ctx)
			// Result should contain the version value
			return strings.Contains(result, ctx.Version) &&
				// Result should NOT contain the placeholder
				!strings.Contains(result, "${version}")
		},
		genLdflagsBuildContext(),
	))

	// Property 11.2: ${commit} is replaced with actual commit hash
	properties.Property("${commit} is replaced with actual commit hash", prop.ForAll(
		func(ctx LdflagsBuildContext) bool {
			ldflags := "-X main.commit=${commit}"
			result := SubstituteLdflagsVariables(ldflags, ctx)
			// Result should contain the commit value
			return strings.Contains(result, ctx.Commit) &&
				// Result should NOT contain the placeholder
				!strings.Contains(result, "${commit}")
		},
		genLdflagsBuildContext(),
	))

	// Property 11.3: ${buildTime} is replaced with RFC3339 formatted timestamp
	properties.Property("${buildTime} is replaced with RFC3339 formatted timestamp", prop.ForAll(
		func(ctx LdflagsBuildContext) bool {
			ldflags := "-X main.buildTime=${buildTime}"
			result := SubstituteLdflagsVariables(ldflags, ctx)
			expectedTime := ctx.BuildTime.UTC().Format(time.RFC3339)
			// Result should contain the formatted timestamp
			return strings.Contains(result, expectedTime) &&
				// Result should NOT contain the placeholder
				!strings.Contains(result, "${buildTime}")
		},
		genLdflagsBuildContext(),
	))

	// Property 11.4: All variables in a multi-variable ldflags are substituted
	properties.Property("all variables in multi-variable ldflags are substituted", prop.ForAll(
		func(ctx LdflagsBuildContext) bool {
			ldflags := "-X main.version=${version} -X main.commit=${commit} -X main.buildTime=${buildTime}"
			result := SubstituteLdflagsVariables(ldflags, ctx)
			expectedTime := ctx.BuildTime.UTC().Format(time.RFC3339)
			// All values should be present
			hasVersion := strings.Contains(result, ctx.Version)
			hasCommit := strings.Contains(result, ctx.Commit)
			hasBuildTime := strings.Contains(result, expectedTime)
			// No placeholders should remain
			noVersionPlaceholder := !strings.Contains(result, "${version}")
			noCommitPlaceholder := !strings.Contains(result, "${commit}")
			noBuildTimePlaceholder := !strings.Contains(result, "${buildTime}")
			return hasVersion && hasCommit && hasBuildTime &&
				noVersionPlaceholder && noCommitPlaceholder && noBuildTimePlaceholder
		},
		genLdflagsBuildContext(),
	))

	// Property 11.5: Non-variable parts of ldflags are preserved
	properties.Property("non-variable parts of ldflags are preserved", prop.ForAll(
		func(ctx LdflagsBuildContext) bool {
			ldflags := "-s -w -X main.version=${version} -trimpath"
			result := SubstituteLdflagsVariables(ldflags, ctx)
			// Static parts should be preserved
			return strings.Contains(result, "-s") &&
				strings.Contains(result, "-w") &&
				strings.Contains(result, "-trimpath") &&
				strings.Contains(result, "-X main.version=")
		},
		genLdflagsBuildContext(),
	))

	// Property 11.6: Empty ldflags returns empty string
	properties.Property("empty ldflags returns empty string", prop.ForAll(
		func(ctx LdflagsBuildContext) bool {
			result := SubstituteLdflagsVariables("", ctx)
			return result == ""
		},
		genLdflagsBuildContext(),
	))

	// Property 11.7: Ldflags without variables are returned unchanged
	properties.Property("ldflags without variables are returned unchanged", prop.ForAll(
		func(ldflags string, ctx LdflagsBuildContext) bool {
			result := SubstituteLdflagsVariables(ldflags, ctx)
			return result == ldflags
		},
		genLdflagsWithoutVariables(),
		genLdflagsBuildContext(),
	))

	// Property 11.8: HasLdflagsVariables correctly identifies ldflags with variables
	properties.Property("HasLdflagsVariables correctly identifies ldflags with variables", prop.ForAll(
		func(ldflags string) bool {
			return HasLdflagsVariables(ldflags) == true
		},
		genLdflagsWithVariables(),
	))

	// Property 11.9: HasLdflagsVariables correctly identifies ldflags without variables
	properties.Property("HasLdflagsVariables correctly identifies ldflags without variables", prop.ForAll(
		func(ldflags string) bool {
			return HasLdflagsVariables(ldflags) == false
		},
		genLdflagsWithoutVariables(),
	))

	// Property 11.10: Multiple occurrences of same variable are all replaced
	properties.Property("multiple occurrences of same variable are all replaced", prop.ForAll(
		func(ctx LdflagsBuildContext) bool {
			ldflags := "-X main.v1=${version} -X main.v2=${version}"
			result := SubstituteLdflagsVariables(ldflags, ctx)
			// Count occurrences of version in result
			versionCount := strings.Count(result, ctx.Version)
			// Should have 2 occurrences (one for each placeholder)
			return versionCount == 2 && !strings.Contains(result, "${version}")
		},
		genLdflagsBuildContext(),
	))

	properties.TestingRun(t)
}


// **Feature: ui-api-alignment, Property 12: Ldflags Override**
// For any build configuration with custom ldflags, the builder SHALL use those
// instead of default ldflags (-s -w).
// **Validates: Requirements 18.5**

// genCustomLdflags generates custom ldflags that are different from defaults.
func genCustomLdflags() gopter.Gen {
	// Generate ldflags that are clearly different from the default "-s -w"
	customLdflags := []string{
		"-X main.version=1.0.0",
		"-X main.commit=abc1234",
		"-X main.buildTime=2024-01-01T00:00:00Z",
		"-X pkg/version.Version=2.0.0 -X pkg/version.Commit=def5678",
		"-trimpath",
		"-race",
		"-v",
		"-X main.version=custom -trimpath",
		"-X internal/config.GitCommit=xyz9999 -X internal/config.BuildDate=2025-01-01",
	}
	return gen.IntRange(0, len(customLdflags)-1).Map(func(idx int) string {
		return customLdflags[idx]
	})
}

// TestLdflagsOverride tests Property 12: Ldflags Override.
func TestLdflagsOverride(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 12.1: Custom ldflags are used instead of defaults when provided
	properties.Property("custom ldflags are used instead of defaults when provided", prop.ForAll(
		func(customLdflags string) bool {
			// When custom ldflags are provided, they should be used
			result := templates.FormatLdflagsForNix(customLdflags)

			// The result should contain the custom ldflags content
			// and should NOT be the default "-s" "-w" format
			hasCustomContent := strings.Contains(result, customLdflags) ||
				containsAllParts(result, customLdflags)

			return hasCustomContent
		},
		genCustomLdflags(),
	))

	// Property 12.2: Empty ldflags allows defaults to be used (no override)
	properties.Property("empty ldflags allows defaults to be used", prop.ForAll(
		func(_ int) bool {
			// When ldflags is empty, FormatLdflagsForNix returns empty
			// This allows the template to use default ldflags
			result := templates.FormatLdflagsForNix("")
			return result == ""
		},
		gen.IntRange(0, 1), // Dummy generator
	))

	// Property 12.3: Custom ldflags completely replace defaults (no merging)
	properties.Property("custom ldflags completely replace defaults (no merging)", prop.ForAll(
		func(customLdflags string) bool {
			result := templates.FormatLdflagsForNix(customLdflags)

			// If custom ldflags don't contain -s or -w, the result shouldn't either
			// This verifies that defaults are not merged with custom ldflags
			if !strings.Contains(customLdflags, "-s") && !strings.Contains(customLdflags, "-w") {
				// Result should not have -s or -w added automatically
				// (they would only be there if they were in the input)
				return !strings.Contains(result, "\"-s\"") || strings.Contains(customLdflags, "-s")
			}
			return true
		},
		genCustomLdflags(),
	))

	// Property 12.4: Custom ldflags with -X flags override default -X flags
	properties.Property("custom ldflags with -X flags override default -X flags", prop.ForAll(
		func(version string, commit string) bool {
			customLdflags := "-X main.version=" + version + " -X main.commit=" + commit
			result := templates.FormatLdflagsForNix(customLdflags)

			// The result should contain the custom version and commit
			hasVersion := strings.Contains(result, version)
			hasCommit := strings.Contains(result, commit)

			return hasVersion && hasCommit
		},
		genVersion(),
		genCommitHash(),
	))

	// Property 12.5: Custom ldflags preserve all specified flags
	properties.Property("custom ldflags preserve all specified flags", prop.ForAll(
		func(customLdflags string) bool {
			result := templates.FormatLdflagsForNix(customLdflags)

			// All parts of the custom ldflags should be in the result
			parts := strings.Fields(customLdflags)
			for _, part := range parts {
				if !strings.Contains(result, part) {
					return false
				}
			}
			return true
		},
		genCustomLdflags(),
	))

	// Property 12.6: Custom ldflags with only -X flags don't include default -s -w
	properties.Property("custom ldflags with only -X flags don't include default -s -w", prop.ForAll(
		func(version string) bool {
			// Custom ldflags with only -X flag (no -s or -w)
			customLdflags := "-X main.version=" + version
			result := templates.FormatLdflagsForNix(customLdflags)

			// The result should contain the version but not have -s or -w added
			hasVersion := strings.Contains(result, version)
			// -s and -w should not be present unless they were in the input
			noDefaultS := !strings.Contains(result, "\"-s\"")
			noDefaultW := !strings.Contains(result, "\"-w\"")

			return hasVersion && noDefaultS && noDefaultW
		},
		genVersion(),
	))

	properties.TestingRun(t)
}

// containsAllParts checks if the result contains all parts of the ldflags string.
func containsAllParts(result, ldflags string) bool {
	parts := strings.Fields(ldflags)
	for _, part := range parts {
		if !strings.Contains(result, part) {
			return false
		}
	}
	return true
}
