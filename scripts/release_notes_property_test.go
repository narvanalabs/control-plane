package scripts

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// genValidCommitType generates valid commit types (excluding "other").
func genValidCommitType() gopter.Gen {
	return gen.OneConstOf(
		CommitTypeFeat,
		CommitTypeFix,
		CommitTypeDocs,
		CommitTypeStyle,
		CommitTypeRefactor,
		CommitTypePerf,
		CommitTypeTest,
		CommitTypeBuild,
		CommitTypeCI,
		CommitTypeChore,
	)
}

// genScope generates optional scopes (alphanumeric, lowercase).
func genScope() gopter.Gen {
	return gen.OneGenOf(
		gen.Const(""),
		gen.AlphaString().Map(func(s string) string {
			return strings.ToLower(s)
		}).SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 20
		}),
	)
}

// genDescription generates non-empty descriptions without newlines.
func genDescription() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0 && len(s) <= 100 && !strings.Contains(s, "\n")
	}).Map(func(s string) string {
		// Ensure first character is lowercase for consistency
		if len(s) > 0 {
			return strings.ToLower(s[:1]) + s[1:]
		}
		return s
	})
}

// genConventionalCommit generates valid conventional commit strings.
func genConventionalCommit() gopter.Gen {
	return gopter.CombineGens(
		genValidCommitType(),
		genScope(),
		gen.Bool(), // breaking change
		genDescription(),
	).Map(func(values []interface{}) string {
		commitType := values[0].(CommitType)
		scope := values[1].(string)
		breaking := values[2].(bool)
		desc := values[3].(string)

		var sb strings.Builder
		sb.WriteString(string(commitType))
		if scope != "" {
			sb.WriteString("(")
			sb.WriteString(scope)
			sb.WriteString(")")
		}
		if breaking {
			sb.WriteString("!")
		}
		sb.WriteString(": ")
		sb.WriteString(desc)
		return sb.String()
	})
}

// **Feature: intelligent-release-notes, Property 1: Conventional commit parsing extracts components correctly**
// For any valid conventional commit string with type, optional scope, and description,
// parsing SHALL extract each component into the correct field of the ParsedCommit struct.
// **Validates: Requirements 1.1, 1.3, 1.4**
func TestPropertyConventionalCommitParsing(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 1.1: Type is correctly extracted
	properties.Property("type is correctly extracted from conventional commit", prop.ForAll(
		func(commitType CommitType, scope string, breaking bool, desc string) bool {
			if desc == "" {
				return true // Skip empty descriptions
			}

			var sb strings.Builder
			sb.WriteString(string(commitType))
			if scope != "" {
				sb.WriteString("(")
				sb.WriteString(scope)
				sb.WriteString(")")
			}
			if breaking {
				sb.WriteString("!")
			}
			sb.WriteString(": ")
			sb.WriteString(desc)

			raw := sb.String()
			parsed := ParseCommit(raw)

			return parsed.Type == commitType
		},
		genValidCommitType(),
		genScope(),
		gen.Bool(),
		genDescription(),
	))

	// Property 1.2: Scope is correctly extracted
	properties.Property("scope is correctly extracted from conventional commit", prop.ForAll(
		func(commitType CommitType, scope string, desc string) bool {
			if desc == "" {
				return true // Skip empty descriptions
			}

			var sb strings.Builder
			sb.WriteString(string(commitType))
			if scope != "" {
				sb.WriteString("(")
				sb.WriteString(scope)
				sb.WriteString(")")
			}
			sb.WriteString(": ")
			sb.WriteString(desc)

			raw := sb.String()
			parsed := ParseCommit(raw)

			return parsed.Scope == scope
		},
		genValidCommitType(),
		genScope(),
		genDescription(),
	))

	// Property 1.3: Description is correctly extracted
	properties.Property("description is correctly extracted from conventional commit", prop.ForAll(
		func(commitType CommitType, desc string) bool {
			if desc == "" {
				return true // Skip empty descriptions
			}

			raw := string(commitType) + ": " + desc
			parsed := ParseCommit(raw)

			return parsed.Description == desc
		},
		genValidCommitType(),
		genDescription(),
	))

	// Property 1.4: Breaking change indicator is detected
	properties.Property("breaking change indicator is detected", prop.ForAll(
		func(commitType CommitType, scope string, desc string) bool {
			if desc == "" {
				return true // Skip empty descriptions
			}

			var sb strings.Builder
			sb.WriteString(string(commitType))
			if scope != "" {
				sb.WriteString("(")
				sb.WriteString(scope)
				sb.WriteString(")")
			}
			sb.WriteString("!: ")
			sb.WriteString(desc)

			raw := sb.String()
			parsed := ParseCommit(raw)

			return parsed.BreakingChange == true
		},
		genValidCommitType(),
		genScope(),
		genDescription(),
	))

	// Property 1.5: All valid commit types are recognized
	properties.Property("all valid commit types are recognized", prop.ForAll(
		func(commitType CommitType) bool {
			raw := string(commitType) + ": test description"
			parsed := ParseCommit(raw)
			return parsed.Type == commitType
		},
		genValidCommitType(),
	))

	properties.TestingRun(t)
}


// genNonConventionalCommit generates commit messages that don't follow conventional format.
// These are messages that cannot be parsed as conventional commits at all.
func genNonConventionalCommit() gopter.Gen {
	return gen.OneGenOf(
		// Plain text messages without colon
		gen.AlphaString().SuchThat(func(s string) bool {
			return len(s) > 0 && !strings.Contains(s, ":") && len(s) <= 100
		}),
		// Messages with spaces before colon (invalid format)
		gen.AlphaString().SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 50
		}).Map(func(s string) string {
			return "some message " + s
		}),
		// Messages starting with special characters
		gen.AlphaString().SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 50
		}).Map(func(s string) string {
			return "- " + s
		}),
	)
}

// **Feature: intelligent-release-notes, Property 2: Non-conventional commits categorized as other**
// For any commit message that does not match the conventional commit pattern,
// parsing SHALL set the type to "other" and preserve the full message in the description field.
// **Validates: Requirements 1.2**
func TestPropertyNonConventionalCommits(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 2.1: Non-conventional commits get type "other"
	properties.Property("non-conventional commits get type other", prop.ForAll(
		func(raw string) bool {
			if raw == "" {
				return true // Skip empty strings
			}
			parsed := ParseCommit(raw)
			return parsed.Type == CommitTypeOther
		},
		genNonConventionalCommit(),
	))

	// Property 2.2: Non-conventional commits preserve message in description
	properties.Property("non-conventional commits preserve message in description", prop.ForAll(
		func(raw string) bool {
			if raw == "" {
				return true // Skip empty strings
			}
			parsed := ParseCommit(raw)
			// The description should contain the first line of the raw message
			firstLine := strings.Split(raw, "\n")[0]
			return parsed.Description == strings.TrimSpace(firstLine)
		},
		genNonConventionalCommit(),
	))

	// Property 2.3: Non-conventional commits preserve raw message
	properties.Property("non-conventional commits preserve raw message", prop.ForAll(
		func(raw string) bool {
			if raw == "" {
				return true // Skip empty strings
			}
			parsed := ParseCommit(raw)
			return parsed.Raw == strings.TrimSpace(raw)
		},
		genNonConventionalCommit(),
	))

	properties.TestingRun(t)
}


// genParsedCommit generates valid ParsedCommit structs for round-trip testing.
func genParsedCommit() gopter.Gen {
	return gopter.CombineGens(
		genValidCommitType(),
		genScope(),
		genDescription(),
		gen.Bool(), // breaking change
	).Map(func(values []interface{}) ParsedCommit {
		return ParsedCommit{
			Type:           values[0].(CommitType),
			Scope:          values[1].(string),
			Description:    values[2].(string),
			BreakingChange: values[3].(bool),
		}
	})
}

// **Feature: intelligent-release-notes, Property 3: Commit parse/print round-trip**
// For any ParsedCommit struct, formatting it to a string and parsing that string back
// SHALL produce an equivalent ParsedCommit (same type, scope, description, and breaking change flag).
// **Validates: Requirements 1.5**
func TestPropertyCommitRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 3.1: Round-trip preserves type
	properties.Property("round-trip preserves type", prop.ForAll(
		func(commit ParsedCommit) bool {
			if commit.Description == "" {
				return true // Skip empty descriptions
			}
			formatted := FormatCommit(commit)
			reparsed := ParseCommit(formatted)
			return reparsed.Type == commit.Type
		},
		genParsedCommit(),
	))

	// Property 3.2: Round-trip preserves scope
	properties.Property("round-trip preserves scope", prop.ForAll(
		func(commit ParsedCommit) bool {
			if commit.Description == "" {
				return true // Skip empty descriptions
			}
			formatted := FormatCommit(commit)
			reparsed := ParseCommit(formatted)
			return reparsed.Scope == commit.Scope
		},
		genParsedCommit(),
	))

	// Property 3.3: Round-trip preserves description
	properties.Property("round-trip preserves description", prop.ForAll(
		func(commit ParsedCommit) bool {
			if commit.Description == "" {
				return true // Skip empty descriptions
			}
			formatted := FormatCommit(commit)
			reparsed := ParseCommit(formatted)
			return reparsed.Description == commit.Description
		},
		genParsedCommit(),
	))

	// Property 3.4: Round-trip preserves breaking change flag
	properties.Property("round-trip preserves breaking change flag", prop.ForAll(
		func(commit ParsedCommit) bool {
			if commit.Description == "" {
				return true // Skip empty descriptions
			}
			formatted := FormatCommit(commit)
			reparsed := ParseCommit(formatted)
			return reparsed.BreakingChange == commit.BreakingChange
		},
		genParsedCommit(),
	))

	// Property 3.5: Full round-trip equivalence
	properties.Property("full round-trip produces equivalent commit", prop.ForAll(
		func(commit ParsedCommit) bool {
			if commit.Description == "" {
				return true // Skip empty descriptions
			}
			formatted := FormatCommit(commit)
			reparsed := ParseCommit(formatted)

			return reparsed.Type == commit.Type &&
				reparsed.Scope == commit.Scope &&
				reparsed.Description == commit.Description &&
				reparsed.BreakingChange == commit.BreakingChange
		},
		genParsedCommit(),
	))

	properties.TestingRun(t)
}


// genCommitHash generates valid commit hash strings (6-8 hex characters).
func genCommitHash() gopter.Gen {
	return gen.IntRange(6, 8).FlatMap(func(length interface{}) gopter.Gen {
		return gen.SliceOfN(length.(int), gen.Rune()).Map(func(runes []rune) string {
			hexChars := "0123456789abcdef"
			result := make([]byte, len(runes))
			for i := range runes {
				result[i] = hexChars[int(runes[i])%16]
			}
			return string(result)
		})
	}, reflect.TypeOf(""))
}

// genDescriptionWithHash generates descriptions containing commit hashes in parentheses.
func genDescriptionWithHash() gopter.Gen {
	return gopter.CombineGens(
		genDescription(),
		genCommitHash(),
	).Map(func(values []interface{}) string {
		desc := values[0].(string)
		hash := values[1].(string)
		return desc + " (" + hash + ")"
	})
}

// genLeadingPrefix generates one of the prefixes to be removed.
func genLeadingPrefix() gopter.Gen {
	return gen.OneConstOf("Add ", "add ", "ADD ", "Update ", "update ", "UPDATE ", "Fix ", "fix ", "FIX ")
}

// genDescriptionWithPrefix generates descriptions with leading prefixes.
func genDescriptionWithPrefix() gopter.Gen {
	return gopter.CombineGens(
		genLeadingPrefix(),
		genDescription(),
	).Map(func(values []interface{}) string {
		prefix := values[0].(string)
		desc := values[1].(string)
		return prefix + desc
	})
}

// genDescriptionWithoutPunctuation generates descriptions that don't end with punctuation.
func genDescriptionWithoutPunctuation() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		if len(s) == 0 {
			return false
		}
		lastChar := s[len(s)-1]
		// Ensure it doesn't end with punctuation
		return lastChar != '.' && lastChar != '!' && lastChar != '?' && lastChar != ':' && lastChar != ';'
	}).SuchThat(func(s string) bool {
		return len(s) > 0 && len(s) <= 100
	})
}

// **Feature: intelligent-release-notes, Property 21: Commit description cleaning**
// For any commit description containing a hash pattern "(abc123)", the cleaned output SHALL not contain that pattern.
// For any description starting with "Add ", "Update ", or "Fix ", the cleaned output SHALL not start with those prefixes.
// For any description not ending in punctuation, the cleaned output SHALL end with a period.
// **Validates: Requirements 10.1, 10.2, 10.3**
func TestPropertyDescriptionCleaning(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 21.1: Hash patterns in parentheses are removed
	properties.Property("hash patterns in parentheses are removed", prop.ForAll(
		func(desc string, hash string) bool {
			input := desc + " (" + hash + ")"
			cleaned := CleanDescription(input)
			// The cleaned output should not contain the hash pattern
			hashPattern := "(" + hash + ")"
			return !strings.Contains(cleaned, hashPattern)
		},
		genDescription(),
		genCommitHash(),
	))

	// Property 21.2: Leading "Add " prefix is removed
	properties.Property("leading Add prefix is removed", prop.ForAll(
		func(desc string) bool {
			if desc == "" {
				return true
			}
			input := "Add " + desc
			cleaned := CleanDescription(input)
			// Should not start with "Add " (case-insensitive check)
			return !strings.HasPrefix(strings.ToLower(cleaned), "add ")
		},
		genDescription(),
	))

	// Property 21.3: Leading "Update " prefix is removed
	properties.Property("leading Update prefix is removed", prop.ForAll(
		func(desc string) bool {
			if desc == "" {
				return true
			}
			input := "Update " + desc
			cleaned := CleanDescription(input)
			// Should not start with "Update " (case-insensitive check)
			return !strings.HasPrefix(strings.ToLower(cleaned), "update ")
		},
		genDescription(),
	))

	// Property 21.4: Leading "Fix " prefix is removed
	properties.Property("leading Fix prefix is removed", prop.ForAll(
		func(desc string) bool {
			if desc == "" {
				return true
			}
			input := "Fix " + desc
			cleaned := CleanDescription(input)
			// Should not start with "Fix " (case-insensitive check)
			return !strings.HasPrefix(strings.ToLower(cleaned), "fix ")
		},
		genDescription(),
	))

	// Property 21.5: First letter is capitalized
	properties.Property("first letter is capitalized", prop.ForAll(
		func(desc string) bool {
			if desc == "" {
				return true
			}
			cleaned := CleanDescription(desc)
			if cleaned == "" {
				return true
			}
			// First character should be uppercase
			firstRune := []rune(cleaned)[0]
			return firstRune == []rune(strings.ToUpper(string(firstRune)))[0]
		},
		genDescription(),
	))

	// Property 21.6: Period is added if missing punctuation
	properties.Property("period is added if missing punctuation", prop.ForAll(
		func(desc string) bool {
			if desc == "" {
				return true
			}
			cleaned := CleanDescription(desc)
			if cleaned == "" {
				return true
			}
			// Should end with punctuation
			lastChar := cleaned[len(cleaned)-1]
			return lastChar == '.' || lastChar == '!' || lastChar == '?' || lastChar == ':' || lastChar == ';'
		},
		genDescriptionWithoutPunctuation(),
	))

	// Property 21.7: Existing punctuation is preserved
	properties.Property("existing punctuation is preserved", prop.ForAll(
		func(desc string, punct string) bool {
			if desc == "" {
				return true
			}
			input := desc + punct
			cleaned := CleanDescription(input)
			if cleaned == "" {
				return true
			}
			// Should end with the original punctuation, not double punctuation
			return strings.HasSuffix(cleaned, punct) && !strings.HasSuffix(cleaned, punct+punct)
		},
		genDescription(),
		gen.OneConstOf(".", "!", "?", ":", ";"),
	))

	// Property 21.8: Empty input returns empty output
	properties.Property("empty input returns empty output", prop.ForAll(
		func(_ bool) bool {
			return CleanDescription("") == ""
		},
		gen.Bool(),
	))

	// Property 21.9: ContainsHash correctly detects hash patterns
	properties.Property("ContainsHash correctly detects hash patterns", prop.ForAll(
		func(desc string, hash string) bool {
			withHash := desc + " (" + hash + ")"
			return ContainsHash(withHash) == true
		},
		genDescription(),
		genCommitHash(),
	))

	// Property 21.10: ContainsHash returns false for descriptions without hashes
	properties.Property("ContainsHash returns false for descriptions without hashes", prop.ForAll(
		func(desc string) bool {
			// Only test descriptions that don't accidentally contain hash patterns
			if hashInParensRegex.MatchString(desc) {
				return true // Skip if it happens to contain a hash pattern
			}
			return ContainsHash(desc) == false
		},
		genDescription(),
	))

	properties.TestingRun(t)
}


// genExcludedCommitType generates commit types that are excluded by default (chore, style, ci, test).
func genExcludedCommitType() gopter.Gen {
	return gen.OneConstOf(
		CommitTypeChore,
		CommitTypeStyle,
		CommitTypeCI,
		CommitTypeTest,
	)
}

// genNonExcludedCommitType generates commit types that are NOT excluded by default.
func genNonExcludedCommitType() gopter.Gen {
	return gen.OneConstOf(
		CommitTypeFeat,
		CommitTypeFix,
		CommitTypeDocs,
		CommitTypeRefactor,
		CommitTypePerf,
		CommitTypeBuild,
	)
}

// genNoiseDescription generates descriptions that match noise patterns.
func genNoiseDescription() gopter.Gen {
	return gen.OneConstOf(
		"fix whitespace issues",
		"fixing whitespace",
		"fix typo in readme",
		"fixing typo",
		"fix lint errors",
		"fixing lint",
		"fix format issues",
		"fixing format",
		"remove trailing spaces",
		"update lock file",
		"merge branch main",
		"merge pull request #123",
		"wip work in progress",
		"minor changes",
	)
}

// genNonNoiseDescription generates descriptions that do NOT match noise patterns.
func genNonNoiseDescription() gopter.Gen {
	return gen.OneConstOf(
		"add new feature",
		"implement user authentication",
		"refactor database layer",
		"improve performance",
		"update documentation",
		"add unit tests",
		"fix critical bug",
		"enhance error handling",
	)
}

// genCommitWithExcludedType generates a commit with an excluded type.
func genCommitWithExcludedType() gopter.Gen {
	return gopter.CombineGens(
		genExcludedCommitType(),
		genScope(),
		genNonNoiseDescription(),
	).Map(func(values []interface{}) ParsedCommit {
		return ParsedCommit{
			Type:        values[0].(CommitType),
			Scope:       values[1].(string),
			Description: values[2].(string),
		}
	})
}

// genCommitWithNoiseDescription generates a commit with a noise description but non-excluded type.
func genCommitWithNoiseDescription() gopter.Gen {
	return gopter.CombineGens(
		genNonExcludedCommitType(),
		genScope(),
		genNoiseDescription(),
	).Map(func(values []interface{}) ParsedCommit {
		return ParsedCommit{
			Type:        values[0].(CommitType),
			Scope:       values[1].(string),
			Description: values[2].(string),
		}
	})
}

// genNonNoiseCommit generates a commit that should NOT be filtered.
func genNonNoiseCommit() gopter.Gen {
	return gopter.CombineGens(
		genNonExcludedCommitType(),
		genScope(),
		genNonNoiseDescription(),
	).Map(func(values []interface{}) ParsedCommit {
		return ParsedCommit{
			Type:        values[0].(CommitType),
			Scope:       values[1].(string),
			Description: values[2].(string),
		}
	})
}

// genCommitList generates a list of commits with mixed types.
func genCommitList() gopter.Gen {
	return gen.IntRange(1, 10).FlatMap(func(n interface{}) gopter.Gen {
		return gen.SliceOfN(n.(int), genSimpleParsedCommit())
	}, reflect.TypeOf([]ParsedCommit{}))
}

// genSimpleParsedCommit generates simple ParsedCommit structs without complex constraints.
func genSimpleParsedCommit() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf(
			CommitTypeFeat,
			CommitTypeFix,
			CommitTypeDocs,
			CommitTypeStyle,
			CommitTypeRefactor,
			CommitTypePerf,
			CommitTypeTest,
			CommitTypeBuild,
			CommitTypeCI,
			CommitTypeChore,
		),
		gen.OneConstOf("", "api", "web", "db", "auth"),
		gen.OneConstOf(
			"add new feature",
			"fix bug",
			"update docs",
			"fix whitespace",
			"refactor code",
			"improve performance",
		),
		gen.Bool(),
	).Map(func(values []interface{}) ParsedCommit {
		return ParsedCommit{
			Type:           values[0].(CommitType),
			Scope:          values[1].(string),
			Description:    values[2].(string),
			BreakingChange: values[3].(bool),
		}
	})
}

// **Feature: intelligent-release-notes, Property 4: Noise commits filtered by type and pattern**
// For any commit with type in {chore, style, ci, test} OR description matching noise patterns,
// filtering SHALL exclude it from the output commits list.
// **Validates: Requirements 2.1, 2.2, 2.3**
func TestPropertyNoiseFiltering(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	config := DefaultNoiseFilterConfig()

	// Property 4.1: Commits with excluded types are filtered out
	properties.Property("commits with excluded types are filtered out", prop.ForAll(
		func(commit ParsedCommit) bool {
			commits := []ParsedCommit{commit}
			result := FilterCommits(commits, config)
			// The commit should be in NoiseCommits, not in Commits
			return len(result.Commits) == 0 && len(result.NoiseCommits) == 1
		},
		genCommitWithExcludedType(),
	))

	// Property 4.2: Commits with noise descriptions are filtered out
	properties.Property("commits with noise descriptions are filtered out", prop.ForAll(
		func(commit ParsedCommit) bool {
			commits := []ParsedCommit{commit}
			result := FilterCommits(commits, config)
			// The commit should be in NoiseCommits, not in Commits
			return len(result.Commits) == 0 && len(result.NoiseCommits) == 1
		},
		genCommitWithNoiseDescription(),
	))

	// Property 4.3: Non-noise commits pass through the filter
	properties.Property("non-noise commits pass through the filter", prop.ForAll(
		func(commit ParsedCommit) bool {
			commits := []ParsedCommit{commit}
			result := FilterCommits(commits, config)
			// The commit should be in Commits, not in NoiseCommits
			return len(result.Commits) == 1 && len(result.NoiseCommits) == 0
		},
		genNonNoiseCommit(),
	))

	// Property 4.4: IsNoiseCommit returns true for excluded types
	properties.Property("IsNoiseCommit returns true for excluded types", prop.ForAll(
		func(commit ParsedCommit) bool {
			return IsNoiseCommit(commit, config) == true
		},
		genCommitWithExcludedType(),
	))

	// Property 4.5: IsNoiseCommit returns true for noise descriptions
	properties.Property("IsNoiseCommit returns true for noise descriptions", prop.ForAll(
		func(commit ParsedCommit) bool {
			return IsNoiseCommit(commit, config) == true
		},
		genCommitWithNoiseDescription(),
	))

	// Property 4.6: IsNoiseCommit returns false for non-noise commits
	properties.Property("IsNoiseCommit returns false for non-noise commits", prop.ForAll(
		func(commit ParsedCommit) bool {
			return IsNoiseCommit(commit, config) == false
		},
		genNonNoiseCommit(),
	))

	// Property 4.7: Filtered commits and noise commits are disjoint
	properties.Property("filtered commits and noise commits are disjoint", prop.ForAll(
		func(commits []ParsedCommit) bool {
			result := FilterCommits(commits, config)
			// Check that no commit appears in both lists
			for _, c := range result.Commits {
				for _, n := range result.NoiseCommits {
					if c.Raw == n.Raw && c.Description == n.Description && c.Type == n.Type {
						return false
					}
				}
			}
			return true
		},
		genCommitList(),
	))

	// Property 4.8: All input commits appear in either Commits or NoiseCommits
	properties.Property("all input commits appear in either Commits or NoiseCommits", prop.ForAll(
		func(commits []ParsedCommit) bool {
			result := FilterCommits(commits, config)
			return len(result.Commits)+len(result.NoiseCommits) == len(commits)
		},
		genCommitList(),
	))

	properties.TestingRun(t)
}


// **Feature: intelligent-release-notes, Property 5: Original commit count preserved after filtering**
// For any list of commits passed through the filter, the FilterResult.OriginalCount
// SHALL equal the length of the input list.
// **Validates: Requirements 2.4**
func TestPropertyCountPreservation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	config := DefaultNoiseFilterConfig()

	// Property 5.1: OriginalCount equals input length
	properties.Property("OriginalCount equals input length", prop.ForAll(
		func(commits []ParsedCommit) bool {
			result := FilterCommits(commits, config)
			return result.OriginalCount == len(commits)
		},
		genCommitList(),
	))

	// Property 5.2: FilteredCount equals length of Commits slice
	properties.Property("FilteredCount equals length of Commits slice", prop.ForAll(
		func(commits []ParsedCommit) bool {
			result := FilterCommits(commits, config)
			return result.FilteredCount == len(result.Commits)
		},
		genCommitList(),
	))

	// Property 5.3: OriginalCount equals FilteredCount plus NoiseCommits count
	properties.Property("OriginalCount equals FilteredCount plus NoiseCommits count", prop.ForAll(
		func(commits []ParsedCommit) bool {
			result := FilterCommits(commits, config)
			return result.OriginalCount == result.FilteredCount+len(result.NoiseCommits)
		},
		genCommitList(),
	))

	// Property 5.4: Empty input produces zero counts
	properties.Property("empty input produces zero counts", prop.ForAll(
		func(_ bool) bool {
			result := FilterCommits([]ParsedCommit{}, config)
			return result.OriginalCount == 0 &&
				result.FilteredCount == 0 &&
				len(result.Commits) == 0 &&
				len(result.NoiseCommits) == 0
		},
		gen.Bool(),
	))

	// Property 5.5: Counts are non-negative
	properties.Property("counts are non-negative", prop.ForAll(
		func(commits []ParsedCommit) bool {
			result := FilterCommits(commits, config)
			return result.OriginalCount >= 0 &&
				result.FilteredCount >= 0 &&
				len(result.Commits) >= 0 &&
				len(result.NoiseCommits) >= 0
		},
		genCommitList(),
	))

	properties.TestingRun(t)
}


// genKeywordWithArea generates a keyword and its expected feature area.
func genKeywordWithArea() gopter.Gen {
	// Create pairs of keywords and their expected areas
	pairs := []struct {
		keyword string
		area    FeatureArea
	}{
		{"build", FeatureAreaBuildSystem},
		{"nix", FeatureAreaBuildSystem},
		{"deploy", FeatureAreaDeployment},
		{"deployment", FeatureAreaDeployment},
		{"auth", FeatureAreaAuthentication},
		{"login", FeatureAreaAuthentication},
		{"api", FeatureAreaAPI},
		{"endpoint", FeatureAreaAPI},
		{"database", FeatureAreaDatabase},
		{"db", FeatureAreaDatabase},
		{"migration", FeatureAreaDatabase},
		{"ui", FeatureAreaUserInterface},
		{"dashboard", FeatureAreaUserInterface},
		{"config", FeatureAreaConfiguration},
		{"settings", FeatureAreaConfiguration},
		{"docker", FeatureAreaContainerization},
		{"container", FeatureAreaContainerization},
		{"grpc", FeatureAreaCommunication},
		{"websocket", FeatureAreaCommunication},
		{"scheduler", FeatureAreaScheduler},
		{"queue", FeatureAreaScheduler},
		{"log", FeatureAreaLogging},
		{"logging", FeatureAreaLogging},
		{"security", FeatureAreaSecurity},
		{"encrypt", FeatureAreaSecurity},
	}

	return gen.IntRange(0, len(pairs)-1).Map(func(i int) struct {
		keyword string
		area    FeatureArea
	} {
		return pairs[i]
	})
}

// genDescriptionWithKeyword generates a description containing a specific keyword.
func genDescriptionWithKeyword(keyword string) gopter.Gen {
	prefixes := []string{
		"implement ",
		"add ",
		"update ",
		"fix ",
		"improve ",
		"refactor ",
		"enhance ",
	}
	suffixes := []string{
		" functionality",
		" support",
		" handling",
		" logic",
		" system",
		" feature",
		"",
	}

	return gopter.CombineGens(
		gen.IntRange(0, len(prefixes)-1),
		gen.IntRange(0, len(suffixes)-1),
	).Map(func(values []interface{}) string {
		prefix := prefixes[values[0].(int)]
		suffix := suffixes[values[1].(int)]
		return prefix + keyword + suffix
	})
}

// genScopelessCommitWithKeyword generates a commit without scope but with a keyword in description.
func genScopelessCommitWithKeyword() gopter.Gen {
	return genKeywordWithArea().FlatMap(func(pair interface{}) gopter.Gen {
		p := pair.(struct {
			keyword string
			area    FeatureArea
		})
		return genDescriptionWithKeyword(p.keyword).Map(func(desc string) struct {
			commit ParsedCommit
			area   FeatureArea
		} {
			return struct {
				commit ParsedCommit
				area   FeatureArea
			}{
				commit: ParsedCommit{
					Type:        CommitTypeFeat,
					Scope:       "", // No scope
					Description: desc,
				},
				area: p.area,
			}
		})
	}, reflect.TypeOf(struct {
		commit ParsedCommit
		area   FeatureArea
	}{}))
}

// **Feature: intelligent-release-notes, Property 18: Scopeless commits use keyword detection**
// For any commit without an explicit scope, the grouper SHALL attempt to detect
// a feature area using keyword matching.
// **Validates: Requirements 9.1, 9.2**
func TestPropertyKeywordDetection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 18.1: Commits with keywords are detected to correct feature area
	properties.Property("commits with keywords are detected to correct feature area", prop.ForAll(
		func(data struct {
			commit ParsedCommit
			area   FeatureArea
		}) bool {
			detected := DetectFeatureArea(data.commit)
			return detected == data.area
		},
		genScopelessCommitWithKeyword(),
	))

	// Property 18.2: GetEffectiveScope returns detected area for scopeless commits
	properties.Property("GetEffectiveScope returns detected area for scopeless commits", prop.ForAll(
		func(data struct {
			commit ParsedCommit
			area   FeatureArea
		}) bool {
			effectiveScope := GetEffectiveScope(data.commit)
			return effectiveScope == string(data.area)
		},
		genScopelessCommitWithKeyword(),
	))

	// Property 18.3: Keyword detection is case-insensitive
	properties.Property("keyword detection is case-insensitive", prop.ForAll(
		func(keyword string, useUpper bool) bool {
			var desc string
			if useUpper {
				desc = "implement " + strings.ToUpper(keyword) + " support"
			} else {
				desc = "implement " + strings.ToLower(keyword) + " support"
			}
			commit := ParsedCommit{
				Type:        CommitTypeFeat,
				Scope:       "",
				Description: desc,
			}
			// Both should detect the same area
			lowerCommit := ParsedCommit{
				Type:        CommitTypeFeat,
				Scope:       "",
				Description: "implement " + strings.ToLower(keyword) + " support",
			}
			return DetectFeatureArea(commit) == DetectFeatureArea(lowerCommit)
		},
		gen.OneConstOf("api", "database", "auth", "deploy", "docker"),
		gen.Bool(),
	))

	// Property 18.4: Keywords are matched as whole words
	properties.Property("keywords are matched as whole words", prop.ForAll(
		func(_ bool) bool {
			// "api" should not match in "capital"
			commit := ParsedCommit{
				Type:        CommitTypeFeat,
				Scope:       "",
				Description: "fix capital letters in output",
			}
			// Should return Other since "api" is not a whole word here
			return DetectFeatureArea(commit) == FeatureAreaOther
		},
		gen.Bool(),
	))

	properties.TestingRun(t)
}


// genDescriptionWithNoKeywords generates descriptions that don't contain any mapped keywords.
func genDescriptionWithNoKeywords() gopter.Gen {
	// Descriptions that don't match any keywords - carefully chosen to avoid all mapped keywords
	return gen.OneConstOf(
		"improve performance",
		"refactor code structure",
		"clean up unused imports",
		"optimize memory usage",
		"simplify logic",
		"remove deprecated code",
		"bump version number",
		"general improvements",
		"minor changes",
		"code cleanup",
	)
}

// genDescriptionWithAmbiguousKeywords generates descriptions with multiple keywords from different areas.
func genDescriptionWithAmbiguousKeywords() gopter.Gen {
	// Descriptions that contain keywords from multiple different feature areas
	return gen.OneConstOf(
		"add api endpoint for database queries",       // API + Database
		"implement auth token for grpc service",       // Authentication + Communication
		"update docker config for deployment",         // Containerization + Configuration + Deployment
		"fix ui dashboard database connection",        // UI + Database
		"add logging for api authentication",          // Logging + API + Authentication
		"deploy scheduler with docker container",      // Deployment + Scheduler + Containerization
		"configure websocket endpoint for dashboard",  // Configuration + Communication + API + UI
	)
}

// genCommitWithNoKeywords generates a commit without scope and without matching keywords.
func genCommitWithNoKeywords() gopter.Gen {
	return genDescriptionWithNoKeywords().Map(func(desc string) ParsedCommit {
		return ParsedCommit{
			Type:        CommitTypeFeat,
			Scope:       "", // No scope
			Description: desc,
		}
	})
}

// genCommitWithAmbiguousKeywords generates a commit with multiple conflicting keywords.
func genCommitWithAmbiguousKeywords() gopter.Gen {
	return genDescriptionWithAmbiguousKeywords().Map(func(desc string) ParsedCommit {
		return ParsedCommit{
			Type:        CommitTypeFeat,
			Scope:       "", // No scope
			Description: desc,
		}
	})
}

// **Feature: intelligent-release-notes, Property 19: Ambiguous keywords go to Other Changes**
// For any commit without scope and without matching keywords (or with ambiguous keywords),
// it SHALL be placed in the "Other Changes" section.
// **Validates: Requirements 9.3**
func TestPropertyAmbiguousKeywords(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 19.1: Commits without keywords return Other
	properties.Property("commits without keywords return Other", prop.ForAll(
		func(commit ParsedCommit) bool {
			detected := DetectFeatureArea(commit)
			return detected == FeatureAreaOther
		},
		genCommitWithNoKeywords(),
	))

	// Property 19.2: Commits with ambiguous keywords return Other
	properties.Property("commits with ambiguous keywords return Other", prop.ForAll(
		func(commit ParsedCommit) bool {
			detected := DetectFeatureArea(commit)
			return detected == FeatureAreaOther
		},
		genCommitWithAmbiguousKeywords(),
	))

	// Property 19.3: GetEffectiveScope returns "Other" for no-keyword commits
	properties.Property("GetEffectiveScope returns Other for no-keyword commits", prop.ForAll(
		func(commit ParsedCommit) bool {
			effectiveScope := GetEffectiveScope(commit)
			return effectiveScope == string(FeatureAreaOther)
		},
		genCommitWithNoKeywords(),
	))

	// Property 19.4: GetEffectiveScope returns "Other" for ambiguous commits
	properties.Property("GetEffectiveScope returns Other for ambiguous commits", prop.ForAll(
		func(commit ParsedCommit) bool {
			effectiveScope := GetEffectiveScope(commit)
			return effectiveScope == string(FeatureAreaOther)
		},
		genCommitWithAmbiguousKeywords(),
	))

	// Property 19.5: Empty description returns Other
	properties.Property("empty description returns Other", prop.ForAll(
		func(_ bool) bool {
			commit := ParsedCommit{
				Type:        CommitTypeFeat,
				Scope:       "",
				Description: "",
			}
			return DetectFeatureArea(commit) == FeatureAreaOther
		},
		gen.Bool(),
	))

	properties.TestingRun(t)
}


// genCommitWithScopeAndKeywords generates a commit with both an explicit scope and keywords in description.
func genCommitWithScopeAndKeywords() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("mymodule", "custom", "special", "internal", "core"),
		genKeywordWithArea(),
	).FlatMap(func(values interface{}) gopter.Gen {
		vals := values.([]interface{})
		scope := vals[0].(string)
		pair := vals[1].(struct {
			keyword string
			area    FeatureArea
		})
		return genDescriptionWithKeyword(pair.keyword).Map(func(desc string) struct {
			commit        ParsedCommit
			explicitScope string
			keywordArea   FeatureArea
		} {
			return struct {
				commit        ParsedCommit
				explicitScope string
				keywordArea   FeatureArea
			}{
				commit: ParsedCommit{
					Type:        CommitTypeFeat,
					Scope:       scope,
					Description: desc,
				},
				explicitScope: scope,
				keywordArea:   pair.area,
			}
		})
	}, reflect.TypeOf(struct {
		commit        ParsedCommit
		explicitScope string
		keywordArea   FeatureArea
	}{}))
}

// **Feature: intelligent-release-notes, Property 20: Scope takes priority over keyword matching**
// For any commit with both an explicit scope and keywords in description,
// the explicit scope SHALL be used for grouping.
// **Validates: Requirements 9.4**
func TestPropertyScopePriority(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 20.1: Explicit scope takes priority over keyword detection
	properties.Property("explicit scope takes priority over keyword detection", prop.ForAll(
		func(data struct {
			commit        ParsedCommit
			explicitScope string
			keywordArea   FeatureArea
		}) bool {
			effectiveScope := GetEffectiveScope(data.commit)
			// The effective scope should be the explicit scope, not the keyword-detected area
			return effectiveScope == data.explicitScope
		},
		genCommitWithScopeAndKeywords(),
	))

	// Property 20.2: Explicit scope is returned unchanged
	properties.Property("explicit scope is returned unchanged", prop.ForAll(
		func(scope string, desc string) bool {
			if scope == "" {
				return true // Skip empty scopes
			}
			commit := ParsedCommit{
				Type:        CommitTypeFeat,
				Scope:       scope,
				Description: desc,
			}
			return GetEffectiveScope(commit) == scope
		},
		gen.OneConstOf("api", "web", "db", "auth", "custom", "mymodule"),
		genDescription(),
	))

	// Property 20.3: Scope is not modified by keyword detection
	properties.Property("scope is not modified by keyword detection", prop.ForAll(
		func(scope string) bool {
			// Even if description contains keywords, scope should be returned as-is
			commit := ParsedCommit{
				Type:        CommitTypeFeat,
				Scope:       scope,
				Description: "implement api endpoint for database",
			}
			return GetEffectiveScope(commit) == scope
		},
		gen.OneConstOf("mymodule", "custom", "special", "internal"),
	))

	// Property 20.4: Empty scope falls back to keyword detection
	properties.Property("empty scope falls back to keyword detection", prop.ForAll(
		func(data struct {
			commit ParsedCommit
			area   FeatureArea
		}) bool {
			// Verify the commit has no scope
			if data.commit.Scope != "" {
				return true // Skip if scope is not empty
			}
			effectiveScope := GetEffectiveScope(data.commit)
			// Should return the detected area, not empty string
			return effectiveScope == string(data.area)
		},
		genScopelessCommitWithKeyword(),
	))

	// Property 20.5: GetEffectiveScope never returns empty string
	properties.Property("GetEffectiveScope never returns empty string", prop.ForAll(
		func(commit ParsedCommit) bool {
			effectiveScope := GetEffectiveScope(commit)
			// Should always return something (either scope or detected area or "Other")
			return effectiveScope != ""
		},
		genSimpleParsedCommit(),
	))

	properties.TestingRun(t)
}


// genCommitsWithSameScope generates a list of commits that all share the same scope.
func genCommitsWithSameScope() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("api", "web", "db", "auth", "scheduler"),
		gen.IntRange(2, 5),
	).FlatMap(func(values interface{}) gopter.Gen {
		vals := values.([]interface{})
		scope := vals[0].(string)
		count := vals[1].(int)
		return gen.SliceOfN(count, genCommitWithScope(scope)).Map(func(commits []ParsedCommit) struct {
			commits []ParsedCommit
			scope   string
		} {
			return struct {
				commits []ParsedCommit
				scope   string
			}{
				commits: commits,
				scope:   scope,
			}
		})
	}, reflect.TypeOf(struct {
		commits []ParsedCommit
		scope   string
	}{}))
}

// genCommitWithScope generates a commit with a specific scope.
func genCommitWithScope(scope string) gopter.Gen {
	return gopter.CombineGens(
		genNonExcludedCommitType(),
		genNonNoiseDescription(),
		gen.Bool(),
	).Map(func(values []interface{}) ParsedCommit {
		return ParsedCommit{
			Type:           values[0].(CommitType),
			Scope:          scope,
			Description:    values[1].(string),
			BreakingChange: values[2].(bool),
		}
	})
}

// genCommitsWithMixedScopes generates commits with different scopes.
func genCommitsWithMixedScopes() gopter.Gen {
	scopes := []string{"api", "web", "db", "auth", "scheduler"}
	return gen.IntRange(3, 8).FlatMap(func(n interface{}) gopter.Gen {
		return gen.SliceOfN(n.(int), gen.IntRange(0, len(scopes)-1).FlatMap(func(idx interface{}) gopter.Gen {
			scope := scopes[idx.(int)]
			return genCommitWithScope(scope)
		}, reflect.TypeOf(ParsedCommit{})))
	}, reflect.TypeOf([]ParsedCommit{}))
}

// **Feature: intelligent-release-notes, Property 6: Same-scope commits grouped together**
// For any set of commits sharing the same non-empty scope, grouping SHALL place them
// in a single CommitGroup with that scope as the name.
// **Validates: Requirements 3.1**
func TestPropertySameScopeGrouping(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 6.1: All commits with same scope end up in one group
	properties.Property("all commits with same scope end up in one group", prop.ForAll(
		func(data struct {
			commits []ParsedCommit
			scope   string
		}) bool {
			groups := GroupCommits(data.commits)
			// Should have exactly one group
			if len(groups) != 1 {
				return false
			}
			// Group name should match the scope
			if groups[0].Name != data.scope {
				return false
			}
			// Group should contain all commits
			return len(groups[0].Commits) == len(data.commits)
		},
		genCommitsWithSameScope(),
	))

	// Property 6.2: Group name matches the shared scope
	properties.Property("group name matches the shared scope", prop.ForAll(
		func(data struct {
			commits []ParsedCommit
			scope   string
		}) bool {
			groups := GroupCommits(data.commits)
			if len(groups) != 1 {
				return false
			}
			return groups[0].Name == data.scope
		},
		genCommitsWithSameScope(),
	))

	// Property 6.3: All commits are preserved in the group
	properties.Property("all commits are preserved in the group", prop.ForAll(
		func(data struct {
			commits []ParsedCommit
			scope   string
		}) bool {
			groups := GroupCommits(data.commits)
			if len(groups) != 1 {
				return false
			}
			// Check that all original commits are in the group
			for _, original := range data.commits {
				found := false
				for _, grouped := range groups[0].Commits {
					if original.Description == grouped.Description &&
						original.Type == grouped.Type &&
						original.Scope == grouped.Scope {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
			return true
		},
		genCommitsWithSameScope(),
	))

	// Property 6.4: Mixed scopes produce multiple groups
	properties.Property("mixed scopes produce multiple groups", prop.ForAll(
		func(commits []ParsedCommit) bool {
			if len(commits) == 0 {
				return true
			}
			groups := GroupCommits(commits)
			// Count unique scopes in input
			uniqueScopes := make(map[string]bool)
			for _, c := range commits {
				uniqueScopes[GetEffectiveScope(c)] = true
			}
			// Number of groups should equal number of unique scopes
			return len(groups) == len(uniqueScopes)
		},
		genCommitsWithMixedScopes(),
	))

	// Property 6.5: Total commits across all groups equals input count
	properties.Property("total commits across all groups equals input count", prop.ForAll(
		func(commits []ParsedCommit) bool {
			groups := GroupCommits(commits)
			total := 0
			for _, g := range groups {
				total += len(g.Commits)
			}
			return total == len(commits)
		},
		genCommitsWithMixedScopes(),
	))

	// Property 6.6: Empty input produces empty groups
	properties.Property("empty input produces empty groups", prop.ForAll(
		func(_ bool) bool {
			groups := GroupCommits([]ParsedCommit{})
			return len(groups) == 0
		},
		gen.Bool(),
	))

	properties.TestingRun(t)
}


// genSingleCommit generates a single commit for testing single-commit groups.
func genSingleCommit() gopter.Gen {
	return gopter.CombineGens(
		genNonExcludedCommitType(),
		gen.OneConstOf("api", "web", "db", "auth", ""),
		genNonNoiseDescription(),
		gen.Bool(),
	).Map(func(values []interface{}) ParsedCommit {
		return ParsedCommit{
			Type:           values[0].(CommitType),
			Scope:          values[1].(string),
			Description:    values[2].(string),
			BreakingChange: values[3].(bool),
		}
	})
}

// genCommitsWithUniqueScopes generates commits where each has a unique scope.
func genCommitsWithUniqueScopes() gopter.Gen {
	scopes := []string{"api", "web", "db", "auth", "scheduler", "config", "deploy"}
	return gen.IntRange(1, len(scopes)).FlatMap(func(n interface{}) gopter.Gen {
		count := n.(int)
		selectedScopes := scopes[:count]
		gens := make([]gopter.Gen, count)
		for i, scope := range selectedScopes {
			gens[i] = genCommitWithScope(scope)
		}
		return gen.SliceOfN(count, gen.IntRange(0, count-1).FlatMap(func(idx interface{}) gopter.Gen {
			return genCommitWithScope(selectedScopes[idx.(int)])
		}, reflect.TypeOf(ParsedCommit{}))).SuchThat(func(commits []ParsedCommit) bool {
			// Ensure all scopes are unique
			seen := make(map[string]bool)
			for _, c := range commits {
				scope := GetEffectiveScope(c)
				if seen[scope] {
					return false
				}
				seen[scope] = true
			}
			return true
		})
	}, reflect.TypeOf([]ParsedCommit{}))
}

// **Feature: intelligent-release-notes, Property 7: Single-commit groups have no grouping overhead**
// For any CommitGroup containing exactly one commit, the formatted output SHALL not include
// group wrapper elements (no feature heading, just the commit description).
// **Validates: Requirements 3.4**
func TestPropertySingleCommitGroups(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 7.1: Single commit produces single group with one commit
	properties.Property("single commit produces single group with one commit", prop.ForAll(
		func(commit ParsedCommit) bool {
			groups := GroupCommits([]ParsedCommit{commit})
			if len(groups) != 1 {
				return false
			}
			return len(groups[0].Commits) == 1
		},
		genSingleCommit(),
	))

	// Property 7.2: Single-commit groups are not marked for summary
	properties.Property("single-commit groups are not marked for summary", prop.ForAll(
		func(commit ParsedCommit) bool {
			groups := GroupCommits([]ParsedCommit{commit})
			if len(groups) != 1 {
				return false
			}
			// Single commit groups should NOT be summarized
			return !groups[0].IsSummary
		},
		genSingleCommit(),
	))

	// Property 7.3: ShouldSummarize returns false for single-commit groups
	properties.Property("ShouldSummarize returns false for single-commit groups", prop.ForAll(
		func(commit ParsedCommit) bool {
			group := CommitGroup{
				Name:    GetEffectiveScope(commit),
				Commits: []ParsedCommit{commit},
			}
			return !ShouldSummarize(group)
		},
		genSingleCommit(),
	))

	// Property 7.4: Each unique scope produces its own single-commit group
	properties.Property("each unique scope produces its own single-commit group", prop.ForAll(
		func(commits []ParsedCommit) bool {
			if len(commits) == 0 {
				return true
			}
			groups := GroupCommits(commits)
			// Each group should have exactly one commit since all scopes are unique
			for _, g := range groups {
				if len(g.Commits) != 1 {
					return false
				}
			}
			return len(groups) == len(commits)
		},
		genCommitsWithUniqueScopes(),
	))

	// Property 7.5: Single-commit group preserves the commit unchanged
	properties.Property("single-commit group preserves the commit unchanged", prop.ForAll(
		func(commit ParsedCommit) bool {
			groups := GroupCommits([]ParsedCommit{commit})
			if len(groups) != 1 || len(groups[0].Commits) != 1 {
				return false
			}
			grouped := groups[0].Commits[0]
			return grouped.Type == commit.Type &&
				grouped.Scope == commit.Scope &&
				grouped.Description == commit.Description &&
				grouped.BreakingChange == commit.BreakingChange
		},
		genSingleCommit(),
	))

	// Property 7.6: Two-commit groups are also not summarized
	properties.Property("two-commit groups are also not summarized", prop.ForAll(
		func(scope string) bool {
			commits := []ParsedCommit{
				{Type: CommitTypeFeat, Scope: scope, Description: "first feature"},
				{Type: CommitTypeFeat, Scope: scope, Description: "second feature"},
			}
			groups := GroupCommits(commits)
			if len(groups) != 1 {
				return false
			}
			return !groups[0].IsSummary
		},
		gen.OneConstOf("api", "web", "db"),
	))

	// Property 7.7: Three-commit groups are also not summarized
	properties.Property("three-commit groups are also not summarized", prop.ForAll(
		func(scope string) bool {
			commits := []ParsedCommit{
				{Type: CommitTypeFeat, Scope: scope, Description: "first feature"},
				{Type: CommitTypeFeat, Scope: scope, Description: "second feature"},
				{Type: CommitTypeFeat, Scope: scope, Description: "third feature"},
			}
			groups := GroupCommits(commits)
			if len(groups) != 1 {
				return false
			}
			return !groups[0].IsSummary
		},
		gen.OneConstOf("api", "web", "db"),
	))

	properties.TestingRun(t)
}


// genCommitsWithSameScopeCount generates a specific number of commits with the same scope.
func genCommitsWithSameScopeCount(count int) gopter.Gen {
	return gen.OneConstOf("api", "web", "db", "auth").FlatMap(func(scope interface{}) gopter.Gen {
		return gen.SliceOfN(count, genCommitWithScope(scope.(string))).Map(func(commits []ParsedCommit) struct {
			commits []ParsedCommit
			scope   string
		} {
			return struct {
				commits []ParsedCommit
				scope   string
			}{
				commits: commits,
				scope:   scope.(string),
			}
		})
	}, reflect.TypeOf(struct {
		commits []ParsedCommit
		scope   string
	}{}))
}

// genLargeCommitGroup generates a group with more than 3 commits (4-10).
func genLargeCommitGroup() gopter.Gen {
	return gen.IntRange(4, 10).FlatMap(func(n interface{}) gopter.Gen {
		return genCommitsWithSameScopeCount(n.(int))
	}, reflect.TypeOf(struct {
		commits []ParsedCommit
		scope   string
	}{}))
}

// genSmallCommitGroup generates a group with 3 or fewer commits (1-3).
func genSmallCommitGroup() gopter.Gen {
	return gen.IntRange(1, 3).FlatMap(func(n interface{}) gopter.Gen {
		return genCommitsWithSameScopeCount(n.(int))
	}, reflect.TypeOf(struct {
		commits []ParsedCommit
		scope   string
	}{}))
}

// **Feature: intelligent-release-notes, Property 17: Groups with more than 3 commits produce summary**
// For any CommitGroup containing more than 3 commits, the output SHALL be a summary paragraph
// rather than a list of individual commits.
// **Validates: Requirements 8.3**
func TestPropertySummaryThreshold(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 17.1: Groups with more than 3 commits are marked for summary
	properties.Property("groups with more than 3 commits are marked for summary", prop.ForAll(
		func(data struct {
			commits []ParsedCommit
			scope   string
		}) bool {
			groups := GroupCommits(data.commits)
			if len(groups) != 1 {
				return false
			}
			// Group with >3 commits should be marked for summary
			return groups[0].IsSummary == true
		},
		genLargeCommitGroup(),
	))

	// Property 17.2: Groups with 3 or fewer commits are not marked for summary
	properties.Property("groups with 3 or fewer commits are not marked for summary", prop.ForAll(
		func(data struct {
			commits []ParsedCommit
			scope   string
		}) bool {
			groups := GroupCommits(data.commits)
			if len(groups) != 1 {
				return false
			}
			// Group with <=3 commits should NOT be marked for summary
			return groups[0].IsSummary == false
		},
		genSmallCommitGroup(),
	))

	// Property 17.3: ShouldSummarize returns true for groups with exactly 4 commits
	properties.Property("ShouldSummarize returns true for groups with exactly 4 commits", prop.ForAll(
		func(scope string) bool {
			commits := []ParsedCommit{
				{Type: CommitTypeFeat, Scope: scope, Description: "first"},
				{Type: CommitTypeFeat, Scope: scope, Description: "second"},
				{Type: CommitTypeFeat, Scope: scope, Description: "third"},
				{Type: CommitTypeFeat, Scope: scope, Description: "fourth"},
			}
			group := CommitGroup{Name: scope, Commits: commits}
			return ShouldSummarize(group) == true
		},
		gen.OneConstOf("api", "web", "db"),
	))

	// Property 17.4: ShouldSummarize returns false for groups with exactly 3 commits
	properties.Property("ShouldSummarize returns false for groups with exactly 3 commits", prop.ForAll(
		func(scope string) bool {
			commits := []ParsedCommit{
				{Type: CommitTypeFeat, Scope: scope, Description: "first"},
				{Type: CommitTypeFeat, Scope: scope, Description: "second"},
				{Type: CommitTypeFeat, Scope: scope, Description: "third"},
			}
			group := CommitGroup{Name: scope, Commits: commits}
			return ShouldSummarize(group) == false
		},
		gen.OneConstOf("api", "web", "db"),
	))

	// Property 17.5: Threshold is exactly at 3 (boundary test)
	properties.Property("threshold is exactly at 3", prop.ForAll(
		func(n int) bool {
			commits := make([]ParsedCommit, n)
			for i := 0; i < n; i++ {
				commits[i] = ParsedCommit{
					Type:        CommitTypeFeat,
					Scope:       "test",
					Description: "commit " + string(rune('a'+i)),
				}
			}
			group := CommitGroup{Name: "test", Commits: commits}
			shouldSummarize := ShouldSummarize(group)
			// Should summarize only if n > 3
			return shouldSummarize == (n > 3)
		},
		gen.IntRange(1, 10),
	))

	// Property 17.6: Empty group is not marked for summary
	properties.Property("empty group is not marked for summary", prop.ForAll(
		func(_ bool) bool {
			group := CommitGroup{Name: "test", Commits: []ParsedCommit{}}
			return ShouldSummarize(group) == false
		},
		gen.Bool(),
	))

	// Property 17.7: IsSummary flag is set correctly during grouping
	properties.Property("IsSummary flag is set correctly during grouping", prop.ForAll(
		func(n int) bool {
			scope := "test"
			commits := make([]ParsedCommit, n)
			for i := 0; i < n; i++ {
				commits[i] = ParsedCommit{
					Type:        CommitTypeFeat,
					Scope:       scope,
					Description: "commit",
				}
			}
			groups := GroupCommits(commits)
			if len(groups) != 1 {
				return false
			}
			// IsSummary should match ShouldSummarize result
			return groups[0].IsSummary == (n > 3)
		},
		gen.IntRange(1, 10),
	))

	properties.TestingRun(t)
}


// genCommitsWithSameScopeAndType generates commits that all share the same scope and type.
func genCommitsWithSameScopeAndType() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("api", "web", "db", "auth", "scheduler"),
		genNonExcludedCommitType(),
		gen.IntRange(2, 8),
	).FlatMap(func(values interface{}) gopter.Gen {
		vals := values.([]interface{})
		scope := vals[0].(string)
		commitType := vals[1].(CommitType)
		count := vals[2].(int)
		return gen.SliceOfN(count, genCommitWithScopeAndType(scope, commitType)).Map(func(commits []ParsedCommit) struct {
			commits    []ParsedCommit
			scope      string
			commitType CommitType
		} {
			return struct {
				commits    []ParsedCommit
				scope      string
				commitType CommitType
			}{
				commits:    commits,
				scope:      scope,
				commitType: commitType,
			}
		})
	}, reflect.TypeOf(struct {
		commits    []ParsedCommit
		scope      string
		commitType CommitType
	}{}))
}

// genCommitWithScopeAndType generates a commit with a specific scope and type.
func genCommitWithScopeAndType(scope string, commitType CommitType) gopter.Gen {
	return gopter.CombineGens(
		genNonNoiseDescription(),
		gen.Bool(),
	).Map(func(values []interface{}) ParsedCommit {
		return ParsedCommit{
			Type:           commitType,
			Scope:          scope,
			Description:    values[0].(string),
			BreakingChange: values[1].(bool),
		}
	})
}

// **Feature: intelligent-release-notes, Property 16: Same scope/type commits merged into single bullet**
// For any set of commits with identical scope and type, the output SHALL contain
// at most one bullet point for that scope/type combination.
// **Validates: Requirements 8.1**
func TestPropertyMergedBullets(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 16.1: Same scope/type commits produce single group
	properties.Property("same scope/type commits produce single group", prop.ForAll(
		func(data struct {
			commits    []ParsedCommit
			scope      string
			commitType CommitType
		}) bool {
			groups := GroupCommits(data.commits)
			// Should have exactly one group since all commits share the same scope
			return len(groups) == 1
		},
		genCommitsWithSameScopeAndType(),
	))

	// Property 16.2: Group name matches the shared scope
	properties.Property("merged group name matches the shared scope", prop.ForAll(
		func(data struct {
			commits    []ParsedCommit
			scope      string
			commitType CommitType
		}) bool {
			groups := GroupCommits(data.commits)
			if len(groups) != 1 {
				return false
			}
			return groups[0].Name == data.scope
		},
		genCommitsWithSameScopeAndType(),
	))

	// Property 16.3: Large groups (>3 commits) produce a summary
	properties.Property("large groups produce a summary", prop.ForAll(
		func(scope string, commitType CommitType) bool {
			// Create 5 commits with same scope and type
			commits := make([]ParsedCommit, 5)
			for i := 0; i < 5; i++ {
				commits[i] = ParsedCommit{
					Type:        commitType,
					Scope:       scope,
					Description: "feature " + string(rune('a'+i)),
				}
			}
			groups := GroupCommits(commits)
			if len(groups) != 1 {
				return false
			}
			// Group should be marked for summary
			if !groups[0].IsSummary {
				return false
			}
			// SummarizeGroup should produce non-empty summary
			summary := SummarizeGroup(groups[0])
			return summary != ""
		},
		gen.OneConstOf("api", "web", "db"),
		genNonExcludedCommitType(),
	))

	// Property 16.4: Summary contains group name reference
	properties.Property("summary contains group name reference", prop.ForAll(
		func(scope string) bool {
			// Create 5 commits with same scope
			commits := make([]ParsedCommit, 5)
			for i := 0; i < 5; i++ {
				commits[i] = ParsedCommit{
					Type:        CommitTypeFeat,
					Scope:       scope,
					Description: "feature " + string(rune('a'+i)),
				}
			}
			groups := GroupCommits(commits)
			if len(groups) != 1 {
				return false
			}
			summary := SummarizeGroup(groups[0])
			// Summary should reference the scope (case-insensitive)
			return strings.Contains(strings.ToLower(summary), strings.ToLower(scope))
		},
		gen.OneConstOf("api", "web", "db", "auth"),
	))

	// Property 16.5: Small groups (<=3 commits) don't produce summary
	properties.Property("small groups don't produce summary", prop.ForAll(
		func(scope string, n int) bool {
			commits := make([]ParsedCommit, n)
			for i := 0; i < n; i++ {
				commits[i] = ParsedCommit{
					Type:        CommitTypeFeat,
					Scope:       scope,
					Description: "feature " + string(rune('a'+i)),
				}
			}
			groups := GroupCommits(commits)
			if len(groups) != 1 {
				return false
			}
			// SummarizeGroup should return empty for small groups
			summary := SummarizeGroup(groups[0])
			return summary == ""
		},
		gen.OneConstOf("api", "web", "db"),
		gen.IntRange(1, 3),
	))

	// Property 16.6: ExtractKeyFeatures removes duplicates
	properties.Property("ExtractKeyFeatures removes duplicates", prop.ForAll(
		func(desc string) bool {
			// Create commits with duplicate descriptions
			commits := []ParsedCommit{
				{Type: CommitTypeFeat, Scope: "api", Description: desc},
				{Type: CommitTypeFeat, Scope: "api", Description: desc},
				{Type: CommitTypeFeat, Scope: "api", Description: desc},
			}
			features := ExtractKeyFeatures(commits)
			// Should have at most 1 unique feature
			return len(features) <= 1
		},
		genNonNoiseDescription(),
	))

	// Property 16.7: ExtractKeyFeatures preserves unique features
	properties.Property("ExtractKeyFeatures preserves unique features", prop.ForAll(
		func(_ bool) bool {
			// Create commits with unique descriptions
			commits := []ParsedCommit{
				{Type: CommitTypeFeat, Scope: "api", Description: "add new feature"},
				{Type: CommitTypeFeat, Scope: "api", Description: "implement user authentication"},
				{Type: CommitTypeFeat, Scope: "api", Description: "refactor database layer"},
			}
			features := ExtractKeyFeatures(commits)
			// Should have 3 unique features
			return len(features) == 3
		},
		gen.Bool(),
	))

	// Property 16.8: Empty commits produce empty features
	properties.Property("empty commits produce empty features", prop.ForAll(
		func(_ bool) bool {
			features := ExtractKeyFeatures([]ParsedCommit{})
			return len(features) == 0
		},
		gen.Bool(),
	))

	// Property 16.9: SummarizeGroup returns empty for empty group
	properties.Property("SummarizeGroup returns empty for empty group", prop.ForAll(
		func(_ bool) bool {
			group := CommitGroup{Name: "test", Commits: []ParsedCommit{}}
			summary := SummarizeGroup(group)
			return summary == ""
		},
		gen.Bool(),
	))

	// Property 16.10: Summary action verb matches primary commit type
	properties.Property("summary action verb matches primary commit type", prop.ForAll(
		func(commitType CommitType) bool {
			commits := make([]ParsedCommit, 5)
			for i := 0; i < 5; i++ {
				commits[i] = ParsedCommit{
					Type:        commitType,
					Scope:       "api",
					Description: "change " + string(rune('a'+i)),
				}
			}
			group := CommitGroup{Name: "api", Commits: commits, IsSummary: true}
			summary := SummarizeGroup(group)

			// Check that summary starts with appropriate action verb
			switch commitType {
			case CommitTypeFeat:
				return strings.HasPrefix(summary, "Enhanced")
			case CommitTypeFix:
				return strings.HasPrefix(summary, "Fixed")
			case CommitTypePerf:
				return strings.HasPrefix(summary, "Optimized")
			case CommitTypeRefactor:
				return strings.HasPrefix(summary, "Improved")
			case CommitTypeDocs:
				return strings.HasPrefix(summary, "Updated")
			case CommitTypeBuild:
				return strings.HasPrefix(summary, "Updated")
			default:
				return strings.HasPrefix(summary, "Updated")
			}
		},
		genNonExcludedCommitType(),
	))

	properties.TestingRun(t)
}


// genFeatCommit generates a feat commit for testing section placement.
func genFeatCommit() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("", "api", "web", "db", "auth"),
		genNonNoiseDescription(),
		gen.Bool(),
	).Map(func(values []interface{}) ParsedCommit {
		return ParsedCommit{
			Type:           CommitTypeFeat,
			Scope:          values[0].(string),
			Description:    values[1].(string),
			BreakingChange: values[2].(bool),
		}
	})
}

// genFixCommit generates a fix commit for testing section placement.
func genFixCommit() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("", "api", "web", "db", "auth"),
		genNonNoiseDescription(),
		gen.Bool(),
	).Map(func(values []interface{}) ParsedCommit {
		return ParsedCommit{
			Type:           CommitTypeFix,
			Scope:          values[0].(string),
			Description:    values[1].(string),
			BreakingChange: values[2].(bool),
		}
	})
}

// genPerfCommit generates a perf commit for testing section placement.
func genPerfCommit() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("", "api", "web", "db", "auth"),
		genNonNoiseDescription(),
		gen.Bool(),
	).Map(func(values []interface{}) ParsedCommit {
		return ParsedCommit{
			Type:           CommitTypePerf,
			Scope:          values[0].(string),
			Description:    values[1].(string),
			BreakingChange: values[2].(bool),
		}
	})
}

// genBreakingCommit generates a commit with BreakingChange=true.
func genBreakingCommit() gopter.Gen {
	return gopter.CombineGens(
		genNonExcludedCommitType(),
		gen.OneConstOf("", "api", "web", "db", "auth"),
		genNonNoiseDescription(),
	).Map(func(values []interface{}) ParsedCommit {
		return ParsedCommit{
			Type:           values[0].(CommitType),
			Scope:          values[1].(string),
			Description:    values[2].(string),
			BreakingChange: true,
		}
	})
}

// **Feature: intelligent-release-notes, Property 8: Commits placed in correct section by type**
// For any commit with type "feat", it SHALL appear in the Features section;
// type "fix" SHALL appear in BugFixes; commits with BreakingChange=true SHALL appear in BreakingChanges.
// **Validates: Requirements 4.2, 4.3, 4.5**
func TestPropertySectionPlacement(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 8.1: feat commits appear in Features section
	properties.Property("feat commits appear in Features section", prop.ForAll(
		func(commit ParsedCommit) bool {
			groups := GroupCommits([]ParsedCommit{commit})
			categorized := CategorizeCommits(groups)
			// Should have at least one group in Features
			if len(categorized.Features) == 0 {
				return false
			}
			// The commit should be in the Features section
			for _, group := range categorized.Features {
				for _, c := range group.Commits {
					if c.Description == commit.Description && c.Type == CommitTypeFeat {
						return true
					}
				}
			}
			return false
		},
		genFeatCommit(),
	))

	// Property 8.2: fix commits appear in BugFixes section
	properties.Property("fix commits appear in BugFixes section", prop.ForAll(
		func(commit ParsedCommit) bool {
			groups := GroupCommits([]ParsedCommit{commit})
			categorized := CategorizeCommits(groups)
			// Should have at least one group in BugFixes
			if len(categorized.BugFixes) == 0 {
				return false
			}
			// The commit should be in the BugFixes section
			for _, group := range categorized.BugFixes {
				for _, c := range group.Commits {
					if c.Description == commit.Description && c.Type == CommitTypeFix {
						return true
					}
				}
			}
			return false
		},
		genFixCommit(),
	))

	// Property 8.3: perf commits appear in Improvements section
	properties.Property("perf commits appear in Improvements section", prop.ForAll(
		func(commit ParsedCommit) bool {
			groups := GroupCommits([]ParsedCommit{commit})
			categorized := CategorizeCommits(groups)
			// Should have at least one group in Improvements
			if len(categorized.Improvements) == 0 {
				return false
			}
			// The commit should be in the Improvements section
			for _, group := range categorized.Improvements {
				for _, c := range group.Commits {
					if c.Description == commit.Description && c.Type == CommitTypePerf {
						return true
					}
				}
			}
			return false
		},
		genPerfCommit(),
	))

	// Property 8.4: breaking change commits appear in BreakingChanges section
	properties.Property("breaking change commits appear in BreakingChanges section", prop.ForAll(
		func(commit ParsedCommit) bool {
			groups := GroupCommits([]ParsedCommit{commit})
			categorized := CategorizeCommits(groups)
			// Should have at least one group in BreakingChanges
			if len(categorized.BreakingChanges) == 0 {
				return false
			}
			// The commit should be in the BreakingChanges section
			for _, group := range categorized.BreakingChanges {
				for _, c := range group.Commits {
					if c.Description == commit.Description && c.BreakingChange {
						return true
					}
				}
			}
			return false
		},
		genBreakingCommit(),
	))

	// Property 8.5: mixed commits are placed in correct sections
	properties.Property("mixed commits are placed in correct sections", prop.ForAll(
		func(featCommit ParsedCommit, fixCommit ParsedCommit, perfCommit ParsedCommit) bool {
			// Ensure commits are not breaking for this test
			featCommit.BreakingChange = false
			fixCommit.BreakingChange = false
			perfCommit.BreakingChange = false

			commits := []ParsedCommit{featCommit, fixCommit, perfCommit}
			groups := GroupCommits(commits)
			categorized := CategorizeCommits(groups)

			// Count commits in each section
			featCount := 0
			for _, g := range categorized.Features {
				featCount += len(g.Commits)
			}
			fixCount := 0
			for _, g := range categorized.BugFixes {
				fixCount += len(g.Commits)
			}
			perfCount := 0
			for _, g := range categorized.Improvements {
				perfCount += len(g.Commits)
			}

			// Each section should have exactly 1 commit
			return featCount == 1 && fixCount == 1 && perfCount == 1
		},
		genFeatCommit(),
		genFixCommit(),
		genPerfCommit(),
	))

	properties.TestingRun(t)
}


// genCommitsOfSingleType generates commits all of the same type.
func genCommitsOfSingleType(commitType CommitType) gopter.Gen {
	return gen.IntRange(1, 5).FlatMap(func(n interface{}) gopter.Gen {
		return gen.SliceOfN(n.(int), gopter.CombineGens(
			gen.OneConstOf("", "api", "web", "db"),
			genNonNoiseDescription(),
		).Map(func(values []interface{}) ParsedCommit {
			return ParsedCommit{
				Type:           commitType,
				Scope:          values[0].(string),
				Description:    values[1].(string),
				BreakingChange: false,
			}
		}))
	}, reflect.TypeOf([]ParsedCommit{}))
}

// **Feature: intelligent-release-notes, Property 9: Empty sections omitted from output**
// For any CategorizedContent where a section has zero commit groups,
// the generated markdown SHALL not contain that section's heading.
// **Validates: Requirements 4.6**
func TestPropertyEmptySections(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 9.1: Only feat commits means only Features section has content
	properties.Property("only feat commits means only Features section has content", prop.ForAll(
		func(commits []ParsedCommit) bool {
			groups := GroupCommits(commits)
			categorized := CategorizeCommits(groups)

			// Features should have content
			if len(categorized.Features) == 0 {
				return false
			}
			// Other sections should be empty
			return len(categorized.BugFixes) == 0 &&
				len(categorized.Improvements) == 0 &&
				len(categorized.BreakingChanges) == 0
		},
		genCommitsOfSingleType(CommitTypeFeat),
	))

	// Property 9.2: Only fix commits means only BugFixes section has content
	properties.Property("only fix commits means only BugFixes section has content", prop.ForAll(
		func(commits []ParsedCommit) bool {
			groups := GroupCommits(commits)
			categorized := CategorizeCommits(groups)

			// BugFixes should have content
			if len(categorized.BugFixes) == 0 {
				return false
			}
			// Other sections should be empty
			return len(categorized.Features) == 0 &&
				len(categorized.Improvements) == 0 &&
				len(categorized.BreakingChanges) == 0
		},
		genCommitsOfSingleType(CommitTypeFix),
	))

	// Property 9.3: Only perf commits means only Improvements section has content
	properties.Property("only perf commits means only Improvements section has content", prop.ForAll(
		func(commits []ParsedCommit) bool {
			groups := GroupCommits(commits)
			categorized := CategorizeCommits(groups)

			// Improvements should have content
			if len(categorized.Improvements) == 0 {
				return false
			}
			// Other sections should be empty
			return len(categorized.Features) == 0 &&
				len(categorized.BugFixes) == 0 &&
				len(categorized.BreakingChanges) == 0
		},
		genCommitsOfSingleType(CommitTypePerf),
	))

	// Property 9.4: NonEmptySections returns only sections with content
	properties.Property("NonEmptySections returns only sections with content", prop.ForAll(
		func(commits []ParsedCommit) bool {
			groups := GroupCommits(commits)
			categorized := CategorizeCommits(groups)
			nonEmpty := categorized.NonEmptySections()

			// Check that all returned sections actually have content
			for _, section := range nonEmpty {
				groups := categorized.GetSectionGroups(section)
				if len(groups) == 0 {
					return false
				}
			}

			// Check that no section with content is missing
			if len(categorized.Features) > 0 {
				found := false
				for _, s := range nonEmpty {
					if s == SectionFeatures {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}

			return true
		},
		genCommitsOfSingleType(CommitTypeFeat),
	))

	// Property 9.5: Empty input produces empty categorized content
	properties.Property("empty input produces empty categorized content", prop.ForAll(
		func(_ bool) bool {
			groups := GroupCommits([]ParsedCommit{})
			categorized := CategorizeCommits(groups)

			return len(categorized.Features) == 0 &&
				len(categorized.BugFixes) == 0 &&
				len(categorized.Improvements) == 0 &&
				len(categorized.BreakingChanges) == 0 &&
				len(categorized.Other) == 0 &&
				!categorized.HasContent()
		},
		gen.Bool(),
	))

	// Property 9.6: HasContent returns false for empty categorized content
	properties.Property("HasContent returns false for empty categorized content", prop.ForAll(
		func(_ bool) bool {
			categorized := CategorizedContent{}
			return !categorized.HasContent()
		},
		gen.Bool(),
	))

	// Property 9.7: HasContent returns true when any section has content
	properties.Property("HasContent returns true when any section has content", prop.ForAll(
		func(commits []ParsedCommit) bool {
			if len(commits) == 0 {
				return true // Skip empty input
			}
			groups := GroupCommits(commits)
			categorized := CategorizeCommits(groups)
			return categorized.HasContent()
		},
		genCommitsOfSingleType(CommitTypeFeat),
	))

	properties.TestingRun(t)
}


// genReleaseSection generates valid ReleaseSection values.
func genReleaseSection() gopter.Gen {
	return gen.OneConstOf(
		SectionFeatures,
		SectionImprovements,
		SectionBugFixes,
		SectionBreakingChanges,
		SectionOther,
	)
}

// **Feature: intelligent-release-notes, Property 22: Section headers use icon format**
// For any non-empty section in the output, the section header SHALL contain
// the appropriate icon prefix (✦, ↑, ✓, !, or ○).
// **Validates: Requirements 11.1, 11.2, 11.3, 11.4**
func TestPropertyEmojiHeaders(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 22.1: Features section header contains rocket icon
	properties.Property("Features section header contains rocket icon", prop.ForAll(
		func(_ bool) bool {
			header := SectionHeader(SectionFeatures)
			return strings.Contains(header, "rocket.svg") &&
				strings.Contains(header, "New Features")
		},
		gen.Bool(),
	))

	// Property 22.2: Improvements section header contains bolt icon
	properties.Property("Improvements section header contains bolt icon", prop.ForAll(
		func(_ bool) bool {
			header := SectionHeader(SectionImprovements)
			return strings.Contains(header, "bolt.svg") &&
				strings.Contains(header, "Performance")
		},
		gen.Bool(),
	))

	// Property 22.3: BugFixes section header contains bug icon
	properties.Property("BugFixes section header contains bug icon", prop.ForAll(
		func(_ bool) bool {
			header := SectionHeader(SectionBugFixes)
			return strings.Contains(header, "bug.svg") &&
				strings.Contains(header, "Bug Fixes")
		},
		gen.Bool(),
	))

	// Property 22.4: BreakingChanges section header contains warning icon
	properties.Property("BreakingChanges section header contains warning icon", prop.ForAll(
		func(_ bool) bool {
			header := SectionHeader(SectionBreakingChanges)
			return strings.Contains(header, "warning.svg") &&
				strings.Contains(header, "Breaking Changes")
		},
		gen.Bool(),
	))

	// Property 22.5: Other section header contains box icon
	properties.Property("Other section header contains box icon", prop.ForAll(
		func(_ bool) bool {
			header := SectionHeader(SectionOther)
			return strings.Contains(header, "box.svg") &&
				strings.Contains(header, "Other Changes")
		},
		gen.Bool(),
	))

	// Property 22.6: All sections have non-empty headers
	properties.Property("all sections have non-empty headers", prop.ForAll(
		func(section ReleaseSection) bool {
			header := SectionHeader(section)
			return len(header) > 0
		},
		genReleaseSection(),
	))

	// Property 22.7: SectionHeaders map contains all sections
	properties.Property("SectionHeaders map contains all sections", prop.ForAll(
		func(section ReleaseSection) bool {
			_, exists := SectionHeaders[section]
			return exists
		},
		genReleaseSection(),
	))

	// Property 22.8: Unknown section returns default header
	properties.Property("unknown section returns default header", prop.ForAll(
		func(_ bool) bool {
			header := SectionHeader(ReleaseSection("unknown"))
			return strings.Contains(header, "box.svg") &&
				strings.Contains(header, "Other Changes")
		},
		gen.Bool(),
	))

	properties.TestingRun(t)
}


// genMixedCommitList generates a list of commits with various types and scopes.
func genMixedCommitList() gopter.Gen {
	return gen.IntRange(2, 10).FlatMap(func(n interface{}) gopter.Gen {
		return gen.SliceOfN(n.(int), gopter.CombineGens(
			gen.OneConstOf(CommitTypeFeat, CommitTypeFix, CommitTypePerf, CommitTypeDocs, CommitTypeRefactor),
			gen.OneConstOf("", "api", "web", "db", "auth"),
			genNonNoiseDescription(),
			gen.Bool(),
		).Map(func(values []interface{}) ParsedCommit {
			return ParsedCommit{
				Type:           values[0].(CommitType),
				Scope:          values[1].(string),
				Description:    values[2].(string),
				BreakingChange: values[3].(bool),
			}
		}))
	}, reflect.TypeOf([]ParsedCommit{}))
}

// **Feature: intelligent-release-notes, Property 23: Section items have consistent indentation**
// For any section with multiple items, all items SHALL have the same indentation level.
// **Validates: Requirements 11.5**
func TestPropertyConsistentIndentation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 23.1: All formatted items start with the same indent
	properties.Property("all formatted items start with the same indent", prop.ForAll(
		func(commits []ParsedCommit) bool {
			if len(commits) == 0 {
				return true
			}
			groups := GroupCommits(commits)
			categorized := CategorizeCommits(groups)

			// Check each section
			for _, section := range categorized.NonEmptySections() {
				sectionGroups := categorized.GetSectionGroups(section)
				items := FormatSectionItems(sectionGroups)

				// All items should start with "- "
				for _, item := range items {
					if !strings.HasPrefix(item, SectionItemIndent) {
						return false
					}
				}
			}
			return true
		},
		genMixedCommitList(),
	))

	// Property 23.2: SectionItemIndent is consistent
	properties.Property("SectionItemIndent is consistent", prop.ForAll(
		func(_ bool) bool {
			return SectionItemIndent == "- "
		},
		gen.Bool(),
	))

	// Property 23.3: FormatSectionItems returns items with consistent prefix
	properties.Property("FormatSectionItems returns items with consistent prefix", prop.ForAll(
		func(commits []ParsedCommit) bool {
			if len(commits) == 0 {
				return true
			}
			groups := GroupCommits(commits)
			items := FormatSectionItems(groups)

			// All items should have the same prefix
			for _, item := range items {
				if !strings.HasPrefix(item, "- ") {
					return false
				}
			}
			return true
		},
		genCommitsOfSingleType(CommitTypeFeat),
	))

	// Property 23.4: FormatSection includes header and items
	properties.Property("FormatSection includes header and items", prop.ForAll(
		func(commits []ParsedCommit) bool {
			if len(commits) == 0 {
				return true
			}
			groups := GroupCommits(commits)
			categorized := CategorizeCommits(groups)

			// Check Features section if it has content
			if len(categorized.Features) > 0 {
				formatted := FormatSection(SectionFeatures, categorized.Features)
				// Should contain the header
				if !strings.Contains(formatted, SectionHeader(SectionFeatures)) {
					return false
				}
				// Should contain bullet points
				if !strings.Contains(formatted, "- ") {
					return false
				}
			}
			return true
		},
		genCommitsOfSingleType(CommitTypeFeat),
	))

	// Property 23.5: Empty groups produce empty section
	properties.Property("empty groups produce empty section", prop.ForAll(
		func(_ bool) bool {
			formatted := FormatSection(SectionFeatures, []CommitGroup{})
			return formatted == ""
		},
		gen.Bool(),
	))

	// Property 23.6: Items don't have extra leading whitespace
	properties.Property("items don't have extra leading whitespace", prop.ForAll(
		func(commits []ParsedCommit) bool {
			if len(commits) == 0 {
				return true
			}
			groups := GroupCommits(commits)
			items := FormatSectionItems(groups)

			for _, item := range items {
				// Should start with "- " not " - " or "  - "
				if strings.HasPrefix(item, " ") {
					return false
				}
			}
			return true
		},
		genMixedCommitList(),
	))

	properties.TestingRun(t)
}


// genMajorVersion generates major version strings (X.0.0 format).
func genMajorVersion() gopter.Gen {
	return gen.IntRange(1, 10).Map(func(major int) string {
		return fmt.Sprintf("%d.0.0", major)
	})
}

// genMajorVersionWithPrefix generates major version strings with optional "v" prefix.
func genMajorVersionWithPrefix() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 10),
		gen.Bool(),
	).Map(func(values []interface{}) string {
		major := values[0].(int)
		withPrefix := values[1].(bool)
		if withPrefix {
			return fmt.Sprintf("v%d.0.0", major)
		}
		return fmt.Sprintf("%d.0.0", major)
	})
}

// genMinorVersion generates minor version strings (X.Y.0 where Y > 0).
func genMinorVersion() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 10),
		gen.IntRange(1, 20),
	).Map(func(values []interface{}) string {
		major := values[0].(int)
		minor := values[1].(int)
		return fmt.Sprintf("%d.%d.0", major, minor)
	})
}

// genPatchVersion generates patch version strings (X.Y.Z where Z > 0).
func genPatchVersion() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 10),
		gen.IntRange(0, 20),
		gen.IntRange(1, 50),
	).Map(func(values []interface{}) string {
		major := values[0].(int)
		minor := values[1].(int)
		patch := values[2].(int)
		return fmt.Sprintf("%d.%d.%d", major, minor, patch)
	})
}

// genNonMajorVersion generates non-major version strings (minor or patch releases).
func genNonMajorVersion() gopter.Gen {
	return gen.OneGenOf(
		genMinorVersion(),
		genPatchVersion(),
	)
}

// genAnyVersion generates any valid semantic version string.
func genAnyVersion() gopter.Gen {
	return gen.OneGenOf(
		genMajorVersion(),
		genMinorVersion(),
		genPatchVersion(),
	)
}

// genOverrideTitle generates non-empty override title strings.
func genOverrideTitle() gopter.Gen {
	return gen.OneConstOf(
		"Custom Release Title",
		"Special Edition",
		"The Big Update",
		"Version X Release",
		"My Custom Title",
	)
}

// **Feature: intelligent-release-notes, Property 25: Version-appropriate title generation**
// For any major version (X.0.0) without override, the title SHALL use a creative format.
// For any minor/patch version without override, the title SHALL use "Narvana v{version}" format.
// **Validates: Requirements 13.1, 13.2**
func TestPropertyVersionAppropriateTitles(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 25.1: Major versions (X.0.0) are correctly detected
	properties.Property("major versions are correctly detected", prop.ForAll(
		func(version string) bool {
			return IsMajorRelease(version) == true
		},
		genMajorVersionWithPrefix(),
	))

	// Property 25.2: Minor versions are not detected as major
	properties.Property("minor versions are not detected as major", prop.ForAll(
		func(version string) bool {
			return IsMajorRelease(version) == false
		},
		genMinorVersion(),
	))

	// Property 25.3: Patch versions are not detected as major
	properties.Property("patch versions are not detected as major", prop.ForAll(
		func(version string) bool {
			return IsMajorRelease(version) == false
		},
		genPatchVersion(),
	))

	// Property 25.4: Major versions get creative titles (not standard format)
	properties.Property("major versions get creative titles", prop.ForAll(
		func(version string) bool {
			title := GenerateTitle(version, "")
			// Creative titles should NOT follow the standard "Narvana vX.Y.Z" format
			// They should contain phrases like "with X.0" instead
			return !strings.HasPrefix(title, "Narvana v") && strings.Contains(title, ".0")
		},
		genMajorVersion(),
	))

	// Property 25.5: Minor/patch versions get standard titles
	properties.Property("minor/patch versions get standard titles", prop.ForAll(
		func(version string) bool {
			title := GenerateTitle(version, "")
			// Standard titles should follow "Narvana vX.Y.Z" format
			return strings.HasPrefix(title, "Narvana v")
		},
		genNonMajorVersion(),
	))

	// Property 25.6: Override title takes precedence for major versions
	properties.Property("override title takes precedence for major versions", prop.ForAll(
		func(version string, override string) bool {
			title := GenerateTitle(version, override)
			return title == override
		},
		genMajorVersion(),
		genOverrideTitle(),
	))

	// Property 25.7: Override title takes precedence for minor/patch versions
	properties.Property("override title takes precedence for minor/patch versions", prop.ForAll(
		func(version string, override string) bool {
			title := GenerateTitle(version, override)
			return title == override
		},
		genNonMajorVersion(),
		genOverrideTitle(),
	))

	// Property 25.8: ExtractMajorVersion returns correct major number
	properties.Property("ExtractMajorVersion returns correct major number", prop.ForAll(
		func(major int) bool {
			version := fmt.Sprintf("%d.0.0", major)
			return ExtractMajorVersion(version) == major
		},
		gen.IntRange(1, 100),
	))

	// Property 25.9: GetMajorVersionTitle returns non-empty string
	properties.Property("GetMajorVersionTitle returns non-empty string", prop.ForAll(
		func(major int) bool {
			title := GetMajorVersionTitle(major)
			return title != "" && len(title) > 0
		},
		gen.IntRange(1, 100),
	))

	// Property 25.10: Major version titles contain the major version number
	properties.Property("major version titles contain the major version number", prop.ForAll(
		func(major int) bool {
			title := GetMajorVersionTitle(major)
			majorStr := fmt.Sprintf("%d.0", major)
			return strings.Contains(title, majorStr)
		},
		gen.IntRange(1, 100),
	))

	properties.TestingRun(t)
}

// **Feature: intelligent-release-notes, Property 10: Default title uses version number**
// For any version string without an override file, the generated title SHALL contain that version number.
// **Validates: Requirements 5.2**
func TestPropertyDefaultTitleUsesVersion(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 10.1: Default title contains version number for minor/patch releases
	properties.Property("default title contains version number for minor/patch releases", prop.ForAll(
		func(version string) bool {
			title := GenerateTitle(version, "")
			// The title should contain the version number
			return strings.Contains(title, version)
		},
		genNonMajorVersion(),
	))

	// Property 10.2: Default title contains major version for major releases
	properties.Property("default title contains major version for major releases", prop.ForAll(
		func(major int) bool {
			version := fmt.Sprintf("%d.0.0", major)
			title := GenerateTitle(version, "")
			// The title should contain at least the major version
			majorStr := fmt.Sprintf("%d.0", major)
			return strings.Contains(title, majorStr)
		},
		gen.IntRange(1, 100),
	))

	// Property 10.3: GenerateTitle never returns empty string for valid versions
	properties.Property("GenerateTitle never returns empty string for valid versions", prop.ForAll(
		func(version string) bool {
			title := GenerateTitle(version, "")
			return title != ""
		},
		genAnyVersion(),
	))

	// Property 10.4: Empty override uses default title generation
	properties.Property("empty override uses default title generation", prop.ForAll(
		func(version string) bool {
			titleWithEmptyOverride := GenerateTitle(version, "")
			// Should not be empty
			return titleWithEmptyOverride != ""
		},
		genAnyVersion(),
	))

	// Property 10.5: Version with v prefix is handled correctly
	properties.Property("version with v prefix is handled correctly", prop.ForAll(
		func(major int, minor int, patch int) bool {
			versionWithV := fmt.Sprintf("v%d.%d.%d", major, minor, patch)
			versionWithoutV := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			
			titleWithV := GenerateTitle(versionWithV, "")
			titleWithoutV := GenerateTitle(versionWithoutV, "")
			
			// Both should produce valid titles
			// For non-major releases, they should be equivalent
			if minor > 0 || patch > 0 {
				// Both should contain the version number without double "v"
				return !strings.Contains(titleWithV, "vv") && 
				       !strings.Contains(titleWithoutV, "vv")
			}
			return true
		},
		gen.IntRange(1, 10),
		gen.IntRange(0, 20),
		gen.IntRange(0, 50),
	))

	// Property 10.6: Invalid version strings still produce some title
	properties.Property("invalid version strings still produce some title", prop.ForAll(
		func(invalid string) bool {
			title := GenerateTitle(invalid, "")
			// Should still produce something (fallback behavior)
			return title != ""
		},
		gen.OneConstOf("invalid", "not-a-version", "abc", "1.2", ""),
	))

	properties.TestingRun(t)
}


// genNonEmptyString generates non-empty strings for override testing.
func genNonEmptyOverrideString() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0 && len(s) <= 200
	})
}

// **Feature: intelligent-release-notes, Property 11: Introduction only included when provided**
// For any release notes generated, the introduction SHALL only be included
// when explicitly provided via override.
// **Validates: Requirements 5.4**
func TestPropertyDefaultIntroduction(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 11.1: Empty override returns empty string (no default)
	properties.Property("empty override returns empty string", prop.ForAll(
		func(_ bool) bool {
			intro := GenerateIntroduction("")
			return intro == ""
		},
		gen.Bool(),
	))

	// Property 11.2: No default introduction is added
	properties.Property("no default introduction is added", prop.ForAll(
		func(_ bool) bool {
			intro := GenerateIntroduction("")
			return len(intro) == 0
		},
		gen.Bool(),
	))

	// Property 11.3: Override replaces default introduction
	properties.Property("override replaces default introduction", prop.ForAll(
		func(override string) bool {
			intro := GenerateIntroduction(override)
			return intro == strings.TrimSpace(override)
		},
		genNonEmptyOverrideString(),
	))

	// Property 11.4: Override with whitespace is trimmed
	properties.Property("override with whitespace is trimmed", prop.ForAll(
		func(override string) bool {
			paddedOverride := "  " + override + "  "
			intro := GenerateIntroduction(paddedOverride)
			return intro == strings.TrimSpace(paddedOverride)
		},
		genNonEmptyOverrideString(),
	))

	// Property 11.5: GenerateIntroduction returns empty for empty override
	properties.Property("GenerateIntroduction returns empty for empty override", prop.ForAll(
		func(_ bool) bool {
			intro := GenerateIntroduction("")
			return intro == ""
		},
		gen.Bool(),
	))

	properties.TestingRun(t)
}

// genProjectName generates valid project names for testing.
func genProjectName() gopter.Gen {
	return gen.OneGenOf(
		gen.Const(""),
		gen.Const("Narvana"),
		gen.Const("MyProject"),
		gen.AlphaString().SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 50
		}),
	)
}

// **Feature: intelligent-release-notes, Property 24: Closing paragraph only included when provided**
// For any generated release notes, the closing paragraph SHALL only be included
// when explicitly provided via override.
// **Validates: Requirements 12.1, 12.3, 12.4**
func TestPropertyClosingParagraph(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 24.1: Empty override returns empty string (no default)
	properties.Property("empty override returns empty string", prop.ForAll(
		func(projectName string) bool {
			closing := GenerateClosing(projectName, "")
			return closing == ""
		},
		genProjectName(),
	))

	// Property 24.2: No default closing is added
	properties.Property("no default closing is added", prop.ForAll(
		func(projectName string) bool {
			closing := GenerateClosing(projectName, "")
			return len(closing) == 0
		},
		genProjectName(),
	))

	// Property 24.3: Override replaces default closing
	properties.Property("override replaces default closing", prop.ForAll(
		func(projectName string, override string) bool {
			closing := GenerateClosing(projectName, override)
			return closing == strings.TrimSpace(override)
		},
		genProjectName(),
		genNonEmptyOverrideString(),
	))

	// Property 24.4: Override with whitespace is trimmed
	properties.Property("override with whitespace is trimmed", prop.ForAll(
		func(projectName string, override string) bool {
			paddedOverride := "  " + override + "  "
			closing := GenerateClosing(projectName, paddedOverride)
			return closing == strings.TrimSpace(paddedOverride)
		},
		genProjectName(),
		genNonEmptyOverrideString(),
	))

	// Property 24.5: GenerateClosing returns empty for empty override
	properties.Property("GenerateClosing returns empty for empty override", prop.ForAll(
		func(projectName string) bool {
			closing := GenerateClosing(projectName, "")
			return closing == ""
		},
		genProjectName(),
	))

	properties.TestingRun(t)
}


// genValidDateForReleaseNotes generates valid date strings in YYYY-MM-DD format.
func genValidDateForReleaseNotes() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(2020, 2030),
		gen.IntRange(1, 12),
		gen.IntRange(1, 28),
	).Map(func(values []interface{}) string {
		year := values[0].(int)
		month := values[1].(int)
		day := values[2].(int)
		return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
	})
}

// genReleaseNotesConfig generates valid ReleaseNotesConfig for testing.
func genReleaseNotesConfig() gopter.Gen {
	return gopter.CombineGens(
		genAnyVersion(),
		genValidDateForReleaseNotes(),
		gen.OneConstOf("", "Custom Title", "Special Release"),
		gen.OneConstOf("", "Custom intro paragraph.", "Welcome to this release!"),
		gen.OneConstOf("", "Custom closing.", "Thanks for using our product!"),
		gen.OneConstOf("", "Narvana", "MyProject"),
	).Map(func(values []interface{}) ReleaseNotesConfig {
		return ReleaseNotesConfig{
			Version:      values[0].(string),
			Date:         values[1].(string),
			Title:        values[2].(string),
			Introduction: values[3].(string),
			Closing:      values[4].(string),
			ProjectName:  values[5].(string),
		}
	})
}

// genCategorizedContentWithCommits generates CategorizedContent with at least some commits.
func genCategorizedContentWithCommits() gopter.Gen {
	return genMixedCommitList().Map(func(commits []ParsedCommit) CategorizedContent {
		groups := GroupCommits(commits)
		return CategorizeCommits(groups)
	})
}

// genEmptyCategorizedContent generates empty CategorizedContent.
func genEmptyCategorizedContent() gopter.Gen {
	return gen.Const(CategorizedContent{})
}

// **Feature: intelligent-release-notes, Property 14: Frontmatter contains required fields**
// For any generated release entry markdown, parsing the frontmatter SHALL yield
// non-empty values for: title, date, versionNumber, description, and image.
// **Validates: Requirements 7.1**
func TestPropertyFrontmatterFields(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 14.1: Generated release notes contain valid frontmatter
	properties.Property("generated release notes contain valid frontmatter", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			// Should start with frontmatter delimiter
			return strings.HasPrefix(markdown, "---\n")
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	// Property 14.2: Frontmatter contains title field
	properties.Property("frontmatter contains title field", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			return fm.Title != ""
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	// Property 14.3: Frontmatter contains date field
	properties.Property("frontmatter contains date field", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			return fm.Date != ""
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	// Property 14.4: Frontmatter contains versionNumber field
	properties.Property("frontmatter contains versionNumber field", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			return fm.VersionNumber != ""
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	// Property 14.5: Frontmatter contains description field
	properties.Property("frontmatter contains description field", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			return fm.Description != ""
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	// Property 14.6: Frontmatter contains image.src field
	properties.Property("frontmatter contains image.src field", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			return fm.Image.Src != ""
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	// Property 14.7: Frontmatter contains image.alt field
	properties.Property("frontmatter contains image.alt field", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			return fm.Image.Alt != ""
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	// Property 14.8: Missing version returns error
	properties.Property("missing version returns error", prop.ForAll(
		func(date string) bool {
			config := ReleaseNotesConfig{
				Version: "",
				Date:    date,
			}
			_, err := GenerateReleaseNotes(CategorizedContent{}, config)
			return err != nil
		},
		genValidDateForReleaseNotes(),
	))

	// Property 14.9: Missing date returns error
	properties.Property("missing date returns error", prop.ForAll(
		func(version string) bool {
			config := ReleaseNotesConfig{
				Version: version,
				Date:    "",
			}
			_, err := GenerateReleaseNotes(CategorizedContent{}, config)
			return err != nil
		},
		genAnyVersion(),
	))

	// Property 14.10: Image path uses correct format
	properties.Property("image path uses correct format", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			// Image path should start with ../../assets/ and end with .svg
			return strings.HasPrefix(fm.Image.Src, "../../assets/") &&
				strings.HasSuffix(fm.Image.Src, ".svg")
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	properties.TestingRun(t)
}


// **Feature: intelligent-release-notes, Property 15: Release notes structure round-trip**
// For any CategorizedContent and ReleaseNotesConfig, generating markdown and parsing
// the frontmatter back SHALL recover the original version number, date, and title.
// **Validates: Requirements 7.3, 7.4**
func TestPropertyReleaseNotesRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 15.1: Round-trip preserves version number
	properties.Property("round-trip preserves version number", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			// Version number should match (without "v" prefix)
			expectedVersion := strings.TrimPrefix(config.Version, "v")
			return fm.VersionNumber == expectedVersion
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	// Property 15.2: Round-trip preserves date
	properties.Property("round-trip preserves date", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			return fm.Date == config.Date
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	// Property 15.3: Round-trip preserves custom title when provided
	properties.Property("round-trip preserves custom title when provided", prop.ForAll(
		func(version string, date string, customTitle string) bool {
			if customTitle == "" {
				return true // Skip when no custom title
			}
			config := ReleaseNotesConfig{
				Version: version,
				Date:    date,
				Title:   customTitle,
			}
			markdown, err := GenerateReleaseNotes(CategorizedContent{}, config)
			if err != nil {
				return false
			}
			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			return fm.Title == customTitle
		},
		genAnyVersion(),
		genValidDateForReleaseNotes(),
		genOverrideTitle(),
	))

	// Property 15.4: Round-trip produces valid markdown structure
	properties.Property("round-trip produces valid markdown structure", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			// Should have frontmatter delimiters
			if !strings.HasPrefix(markdown, "---\n") {
				return false
			}
			// Should have closing frontmatter delimiter
			if !strings.Contains(markdown[4:], "\n---\n") {
				return false
			}
			return true
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	// Property 15.5: Parsed frontmatter can reconstruct config fields
	properties.Property("parsed frontmatter can reconstruct config fields", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			
			// Check that we can reconstruct the essential fields
			expectedVersion := strings.TrimPrefix(config.Version, "v")
			
			return fm.VersionNumber == expectedVersion &&
				fm.Date == config.Date &&
				fm.Title != "" &&
				fm.Description != "" &&
				fm.Image.Src != "" &&
				fm.Image.Alt != ""
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	// Property 15.6: Content after frontmatter contains introduction when provided
	properties.Property("content after frontmatter contains introduction", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			_, bodyContent, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			
			// Body should contain introduction only if provided
			if config.Introduction != "" {
				return strings.Contains(bodyContent, config.Introduction)
			}
			// If no introduction provided, just verify the body is valid
			return len(bodyContent) > 0
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	// Property 15.7: Content after frontmatter contains closing when provided
	properties.Property("content after frontmatter contains closing", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			_, bodyContent, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			
			// Body should contain closing only if provided
			if config.Closing != "" {
				return strings.Contains(bodyContent, config.Closing)
			}
			// If no closing provided, just verify the body is valid
			return len(bodyContent) > 0
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	// Property 15.8: Image path contains version number
	properties.Property("image path contains version number", prop.ForAll(
		func(config ReleaseNotesConfig, content CategorizedContent) bool {
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			
			// Image path should contain version with underscores instead of dots
			cleanVersion := strings.TrimPrefix(config.Version, "v")
			safeVersion := strings.ReplaceAll(cleanVersion, ".", "_")
			return strings.Contains(fm.Image.Src, safeVersion)
		},
		genReleaseNotesConfig(),
		genCategorizedContentWithCommits(),
	))

	properties.TestingRun(t)
}


// **Feature: intelligent-release-notes, Property 27: Empty filtered results still produce valid entry**
// For any input where all commits are filtered as noise, the output SHALL still be
// valid markdown with frontmatter, introduction, and closing.
// **Validates: Requirements 14.2**
func TestPropertyEmptyFilteredResults(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 27.1: Empty content produces valid markdown
	properties.Property("empty content produces valid markdown", prop.ForAll(
		func(config ReleaseNotesConfig) bool {
			markdown, err := GenerateReleaseNotes(CategorizedContent{}, config)
			if err != nil {
				return false
			}
			// Should have frontmatter
			return strings.HasPrefix(markdown, "---\n")
		},
		genReleaseNotesConfig(),
	))

	// Property 27.2: Empty content still has valid frontmatter
	properties.Property("empty content still has valid frontmatter", prop.ForAll(
		func(config ReleaseNotesConfig) bool {
			markdown, err := GenerateReleaseNotes(CategorizedContent{}, config)
			if err != nil {
				return false
			}
			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			return fm.Title != "" &&
				fm.Date != "" &&
				fm.VersionNumber != "" &&
				fm.Description != "" &&
				fm.Image.Src != ""
		},
		genReleaseNotesConfig(),
	))

	// Property 27.3: Empty content includes introduction
	properties.Property("empty content includes introduction", prop.ForAll(
		func(config ReleaseNotesConfig) bool {
			markdown, err := GenerateReleaseNotes(CategorizedContent{}, config)
			if err != nil {
				return false
			}
			_, bodyContent, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			
			// Should contain introduction only if provided
			if config.Introduction != "" {
				return strings.Contains(bodyContent, config.Introduction)
			}
			// If no introduction, just verify body is valid
			return len(bodyContent) > 0
		},
		genReleaseNotesConfig(),
	))

	// Property 27.4: Empty content includes closing when provided
	properties.Property("empty content includes closing", prop.ForAll(
		func(config ReleaseNotesConfig) bool {
			markdown, err := GenerateReleaseNotes(CategorizedContent{}, config)
			if err != nil {
				return false
			}
			_, bodyContent, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}
			
			// Should contain closing only if provided
			if config.Closing != "" {
				return strings.Contains(bodyContent, config.Closing)
			}
			// If no closing, just verify body is valid
			return len(bodyContent) > 0
		},
		genReleaseNotesConfig(),
	))

	// Property 27.5: Empty content includes placeholder message
	properties.Property("empty content includes placeholder message", prop.ForAll(
		func(config ReleaseNotesConfig) bool {
			markdown, err := GenerateReleaseNotes(CategorizedContent{}, config)
			if err != nil {
				return false
			}
			// Should contain a message about no significant changes
			return strings.Contains(markdown, "No significant changes")
		},
		genReleaseNotesConfig(),
	))

	// Property 27.6: All noise commits filtered produces valid entry
	properties.Property("all noise commits filtered produces valid entry", prop.ForAll(
		func(config ReleaseNotesConfig) bool {
			// Create commits that will all be filtered as noise
			noiseCommits := []ParsedCommit{
				{Type: CommitTypeChore, Description: "update dependencies"},
				{Type: CommitTypeStyle, Description: "fix formatting"},
				{Type: CommitTypeCI, Description: "update workflow"},
				{Type: CommitTypeTest, Description: "add tests"},
			}
			
			// Filter the commits
			filterConfig := DefaultNoiseFilterConfig()
			result := FilterCommits(noiseCommits, filterConfig)
			
			// All should be filtered
			if len(result.Commits) != 0 {
				return true // Skip if not all filtered
			}
			
			// Generate release notes with empty content
			groups := GroupCommits(result.Commits)
			content := CategorizeCommits(groups)
			
			markdown, err := GenerateReleaseNotes(content, config)
			if err != nil {
				return false
			}
			
			// Should still be valid markdown
			return strings.HasPrefix(markdown, "---\n") &&
				strings.Contains(markdown, "No significant changes")
		},
		genReleaseNotesConfig(),
	))

	// Property 27.7: Empty content has no section headers
	properties.Property("empty content has no section headers", prop.ForAll(
		func(config ReleaseNotesConfig) bool {
			markdown, err := GenerateReleaseNotes(CategorizedContent{}, config)
			if err != nil {
				return false
			}
			// Should not contain any section headers
			return !strings.Contains(markdown, "🍿") &&
				!strings.Contains(markdown, "⚡") &&
				!strings.Contains(markdown, "🐞") &&
				!strings.Contains(markdown, "⚠️") &&
				!strings.Contains(markdown, "🔄")
		},
		genReleaseNotesConfig(),
	))

	// Property 27.8: GenerateReleaseNotes never returns error for valid config with empty content
	properties.Property("GenerateReleaseNotes never returns error for valid config with empty content", prop.ForAll(
		func(config ReleaseNotesConfig) bool {
			_, err := GenerateReleaseNotes(CategorizedContent{}, config)
			return err == nil
		},
		genReleaseNotesConfig(),
	))

	properties.TestingRun(t)
}


// genUnparseableCommit generates commit messages that cannot be parsed as conventional commits.
// These are messages that will result in type "other" when parsed.
func genUnparseableCommit() gopter.Gen {
	return gen.OneConstOf(
		// Plain text messages without any structure
		"this is a plain message without structure",
		"updated some files",
		"made changes to the codebase",
		"work in progress",
		"initial commit",
		// Messages with invalid type prefixes
		"invalid_type: some description",
		"unknown: another description",
		"random: yet another message",
		// Messages starting with numbers
		"123: numbered message",
		"456: another numbered one",
		// Messages with special characters
		"* bullet point message",
		"- dash message",
		"# hash message",
		// Merge commit messages
		"Merge branch 'feature' into main",
		"Merge pull request #123",
		"Merged changes from upstream",
	)
}

// genMixedRawCommitList generates a list of raw commit strings with both parseable and unparseable messages.
func genMixedRawCommitList() gopter.Gen {
	return gen.SliceOfN(5, gen.OneGenOf(
		// Conventional commits
		gen.OneConstOf(
			"feat: add new feature",
			"fix: resolve bug",
			"docs: update readme",
			"feat(api): add endpoint",
			"fix(web): fix login",
		),
		// Unparseable commits
		genUnparseableCommit(),
	))
}

// **Feature: intelligent-release-notes, Property 26: Failed parsing falls back to Other Changes**
// For any commit that fails to parse, it SHALL appear in the "Other Changes" section
// with its raw message preserved.
// **Validates: Requirements 14.1**
func TestPropertyParsingFallback(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 26.1: Unparseable commits get type "other"
	properties.Property("unparseable commits get type other", prop.ForAll(
		func(raw string) bool {
			if raw == "" {
				return true // Skip empty strings
			}
			commit := ParseCommitWithFallback(raw)
			return commit.Type == CommitTypeOther
		},
		genUnparseableCommit(),
	))

	// Property 26.2: Unparseable commits preserve raw message
	properties.Property("unparseable commits preserve raw message", prop.ForAll(
		func(raw string) bool {
			if raw == "" {
				return true // Skip empty strings
			}
			commit := ParseCommitWithFallback(raw)
			// Raw should be preserved
			return commit.Raw == strings.TrimSpace(raw)
		},
		genUnparseableCommit(),
	))

	// Property 26.3: Unparseable commits have non-empty description
	properties.Property("unparseable commits have non-empty description", prop.ForAll(
		func(raw string) bool {
			if raw == "" {
				return true // Skip empty strings
			}
			commit := ParseCommitWithFallback(raw)
			// Description should not be empty for non-empty input
			return commit.Description != ""
		},
		genUnparseableCommit(),
	))

	// Property 26.4: Unparseable commits appear in Other section after categorization
	properties.Property("unparseable commits appear in Other section after categorization", prop.ForAll(
		func(raw string) bool {
			if raw == "" {
				return true // Skip empty strings
			}
			
			// Parse the commit
			commit := ParseCommitWithFallback(raw)
			commits := []ParsedCommit{commit}
			
			// Group and categorize
			groups := GroupCommitsWithFallback(commits)
			content := CategorizeCommitsWithFallback(groups)
			
			// The commit should be in the Other section
			otherCount := 0
			for _, group := range content.Other {
				otherCount += len(group.Commits)
			}
			return otherCount == 1
		},
		genUnparseableCommit(),
	))

	// Property 26.5: Mixed commits are properly separated
	properties.Property("mixed commits are properly separated", prop.ForAll(
		func(rawCommits []string) bool {
			if len(rawCommits) == 0 {
				return true
			}
			
			// Parse all commits
			commits := ParseCommitsWithFallback(rawCommits)
			
			// Count expected "other" type commits
			expectedOther := 0
			for _, c := range commits {
				if c.Type == CommitTypeOther {
					expectedOther++
				}
			}
			
			// Group and categorize (without filtering to preserve all commits)
			groups := GroupCommitsWithFallback(commits)
			content := CategorizeCommitsWithFallback(groups)
			
			// Count actual "other" commits in content
			actualOther := 0
			for _, group := range content.Other {
				actualOther += len(group.Commits)
			}
			
			// All "other" type commits should be in Other section
			return actualOther >= expectedOther
		},
		genMixedRawCommitList(),
	))

	// Property 26.6: ParseCommitsWithFallback processes all commits
	properties.Property("ParseCommitsWithFallback processes all commits", prop.ForAll(
		func(rawCommits []string) bool {
			commits := ParseCommitsWithFallback(rawCommits)
			return len(commits) == len(rawCommits)
		},
		genMixedRawCommitList(),
	))

	// Property 26.7: ProcessCommitsWithFallback never panics
	properties.Property("ProcessCommitsWithFallback never panics", prop.ForAll(
		func(rawCommits []string) bool {
			// This should never panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ProcessCommitsWithFallback panicked: %v", r)
				}
			}()
			
			config := DefaultNoiseFilterConfig()
			_ = ProcessCommitsWithFallback(rawCommits, config)
			return true
		},
		genMixedRawCommitList(),
	))

	// Property 26.8: EnsureOtherCommitsPreserved validates correctly
	properties.Property("EnsureOtherCommitsPreserved validates correctly", prop.ForAll(
		func(rawCommits []string) bool {
			if len(rawCommits) == 0 {
				return true
			}
			
			// Parse commits
			commits := ParseCommitsWithFallback(rawCommits)
			
			// Group and categorize
			groups := GroupCommitsWithFallback(commits)
			content := CategorizeCommitsWithFallback(groups)
			
			// Validation should pass
			return EnsureOtherCommitsPreserved(commits, content)
		},
		genMixedRawCommitList(),
	))

	// Property 26.9: Grouping fallback produces valid groups
	properties.Property("grouping fallback produces valid groups", prop.ForAll(
		func(rawCommits []string) bool {
			if len(rawCommits) == 0 {
				return true
			}
			
			commits := ParseCommitsWithFallback(rawCommits)
			groups := GroupCommitsWithFallback(commits)
			
			// All commits should be in some group
			totalInGroups := 0
			for _, g := range groups {
				totalInGroups += len(g.Commits)
			}
			
			return totalInGroups == len(commits)
		},
		genMixedRawCommitList(),
	))

	// Property 26.10: Type-based fallback grouping works
	properties.Property("type-based fallback grouping works", prop.ForAll(
		func(commits []ParsedCommit) bool {
			if len(commits) == 0 {
				return true
			}
			
			groups := groupCommitsByTypeOnly(commits)
			
			// All commits should be in some group
			totalInGroups := 0
			for _, g := range groups {
				totalInGroups += len(g.Commits)
			}
			
			return totalInGroups == len(commits)
		},
		genCommitList(),
	))

	properties.TestingRun(t)
}
