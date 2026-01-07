package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: release-changelog-cicd, Property 5: CHANGELOG.md Entry Generation**
// For any release data, generating a changelog entry SHALL produce output containing
// the version number, formatted date, and release notes content, with the new entry
// appearing before any existing entries.
// **Validates: Requirements 5.1, 5.2**

// **Feature: release-changelog-cicd, Property 6: Markdown Formatting Preservation**
// For any release notes containing markdown formatting (headers, lists, code blocks, links),
// the changelog generator SHALL preserve all formatting in the output.
// **Validates: Requirements 5.5**

// genChangelogEntry generates valid changelog entry data.
func genChangelogEntry() gopter.Gen {
	return gen.IntRange(0, 999).FlatMap(func(major interface{}) gopter.Gen {
		return gen.IntRange(0, 999).FlatMap(func(minor interface{}) gopter.Gen {
			return gen.IntRange(0, 999).FlatMap(func(patch interface{}) gopter.Gen {
				return gen.AlphaString().Map(func(notes string) ChangelogEntry {
					version := fmt.Sprintf("%d.%d.%d", major.(int), minor.(int), patch.(int))
					if notes == "" {
						notes = "### Added\n- New feature"
					}
					return ChangelogEntry{
						Version:      version,
						Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
						ReleaseNotes: notes,
					}
				})
			}, nil)
		}, nil)
	}, nil)
}

// genMarkdownContent generates markdown content with various formatting elements.
func genMarkdownContent() gopter.Gen {
	return gen.OneGenOf(
		// Headers
		gen.Const("### Added\n- New feature\n\n### Fixed\n- Bug fix"),
		// Lists
		gen.Const("- Item 1\n- Item 2\n- Item 3"),
		// Alternative lists
		gen.Const("* Item 1\n* Item 2\n* Item 3"),
		// Code blocks
		gen.Const("```go\nfunc main() {}\n```"),
		// Inline code
		gen.Const("Use `go run main.go` to start"),
		// Links
		gen.Const("See [documentation](https://example.com) for details"),
		// Bold and italic
		gen.Const("This is **bold** and _italic_ text"),
		// Mixed content
		gen.Const("### Features\n\n- **New**: Added `feature`\n- See [docs](https://example.com)\n\n```bash\nmake build\n```"),
	)
}

// genExistingChangelog generates an existing changelog with entries.
func genExistingChangelog() gopter.Gen {
	return gen.IntRange(1, 5).FlatMap(func(count interface{}) gopter.Gen {
		return gen.SliceOfN(count.(int), gen.IntRange(0, 99)).Map(func(versions []int) string {
			var builder strings.Builder
			builder.WriteString("# Changelog\n\nAll notable changes to Narvana are documented here.\n\n")

			for i, v := range versions {
				version := fmt.Sprintf("0.%d.%d", v, i)
				builder.WriteString(fmt.Sprintf("## [%s] - 2025-12-%02d\n\n", version, i+1))
				builder.WriteString("### Added\n- Some feature\n\n")
			}

			// Add links
			for i, v := range versions {
				version := fmt.Sprintf("0.%d.%d", v, i)
				builder.WriteString(fmt.Sprintf("[%s]: https://github.com/narvanalabs/control-plane/releases/tag/v%s\n", version, version))
			}

			return builder.String()
		})
	}, nil)
}

func TestPropertyChangelogEntryGeneration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 5.1: Generated entry contains version number
	properties.Property("generated entry contains version number", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			entry := ChangelogEntry{
				Version:      version,
				Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				ReleaseNotes: "### Added\n- Feature",
			}

			result, err := GenerateChangelogEntry(entry)
			if err != nil {
				return false
			}

			return strings.Contains(result, version)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 5.2: Generated entry contains formatted date
	properties.Property("generated entry contains formatted date", prop.ForAll(
		func(year, month, day int) bool {
			if month < 1 || month > 12 || day < 1 || day > 28 {
				return true // Skip invalid dates
			}
			date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
			entry := ChangelogEntry{
				Version:      "1.0.0",
				Date:         date,
				ReleaseNotes: "### Added\n- Feature",
			}

			result, err := GenerateChangelogEntry(entry)
			if err != nil {
				return false
			}

			expectedDate := date.Format("2006-01-02")
			return strings.Contains(result, expectedDate)
		},
		gen.IntRange(2020, 2030),
		gen.IntRange(1, 12),
		gen.IntRange(1, 28),
	))

	// Property 5.3: Generated entry contains release notes content
	properties.Property("generated entry contains release notes content", prop.ForAll(
		func(notes string) bool {
			if notes == "" {
				notes = "Some notes"
			}
			entry := ChangelogEntry{
				Version:      "1.0.0",
				Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				ReleaseNotes: notes,
			}

			result, err := GenerateChangelogEntry(entry)
			if err != nil {
				return false
			}

			return strings.Contains(result, strings.TrimSpace(notes))
		},
		gen.AlphaString(),
	))

	// Property 5.4: New entry appears before existing entries when prepended
	properties.Property("new entry appears before existing entries when prepended", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			existingChangelog := `# Changelog

All notable changes to Narvana are documented here.

## [0.1.0] - 2025-01-01

### Added
- Initial release

[0.1.0]: https://github.com/narvanalabs/control-plane/releases/tag/v0.1.0
`
			entry := ChangelogEntry{
				Version:      version,
				Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				ReleaseNotes: "### Added\n- New feature",
			}

			result, err := PrependChangelogEntry(existingChangelog, entry)
			if err != nil {
				return false
			}

			// Find positions of both version entries
			newVersionPos := strings.Index(result, fmt.Sprintf("## [%s]", version))
			oldVersionPos := strings.Index(result, "## [0.1.0]")

			// New entry should appear before old entry
			return newVersionPos != -1 && oldVersionPos != -1 && newVersionPos < oldVersionPos
		},
		gen.IntRange(1, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 5.5: Entry uses conventional changelog format with ## [version] - date
	properties.Property("entry uses conventional changelog format", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			entry := ChangelogEntry{
				Version:      version,
				Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				ReleaseNotes: "### Added\n- Feature",
			}

			result, err := GenerateChangelogEntry(entry)
			if err != nil {
				return false
			}

			expectedHeader := fmt.Sprintf("## [%s] - 2026-01-07", version)
			return strings.Contains(result, expectedHeader)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 5.6: Version link is generated correctly
	properties.Property("version link is generated correctly", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			entry := ChangelogEntry{
				Version:      version,
				Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				ReleaseNotes: "### Added\n- Feature",
			}

			link, err := GenerateChangelogLink(entry)
			if err != nil {
				return false
			}

			expectedLink := fmt.Sprintf("[%s]: https://github.com/narvanalabs/control-plane/releases/tag/v%s", version, version)
			return strings.Contains(link, expectedLink)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 5.7: ExtractVersionsFromChangelog finds all versions
	properties.Property("ExtractVersionsFromChangelog finds all versions", prop.ForAll(
		func(versions []int) bool {
			if len(versions) == 0 {
				return true
			}

			var builder strings.Builder
			builder.WriteString("# Changelog\n\n")

			expectedVersions := make([]string, len(versions))
			for i, v := range versions {
				version := fmt.Sprintf("1.%d.%d", v%100, i)
				expectedVersions[i] = version
				builder.WriteString(fmt.Sprintf("## [%s] - 2026-01-%02d\n\n", version, (i%28)+1))
				builder.WriteString("### Added\n- Feature\n\n")
			}

			changelog := builder.String()
			foundVersions := ExtractVersionsFromChangelog(changelog)

			if len(foundVersions) != len(expectedVersions) {
				return false
			}

			for i, expected := range expectedVersions {
				if foundVersions[i] != expected {
					return false
				}
			}
			return true
		},
		gen.SliceOfN(5, gen.IntRange(0, 99)),
	))

	// Property 5.8: ChangelogContainsVersion correctly identifies existing versions
	properties.Property("ChangelogContainsVersion correctly identifies existing versions", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			changelog := fmt.Sprintf("# Changelog\n\n## [%s] - 2026-01-07\n\n### Added\n- Feature\n", version)

			return ChangelogContainsVersion(changelog, version)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 5.9: ChangelogContainsVersion returns false for non-existing versions
	properties.Property("ChangelogContainsVersion returns false for non-existing versions", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			otherVersion := fmt.Sprintf("%d.%d.%d", major+1, minor+1, patch+1)
			changelog := fmt.Sprintf("# Changelog\n\n## [%s] - 2026-01-07\n\n### Added\n- Feature\n", version)

			return !ChangelogContainsVersion(changelog, otherVersion)
		},
		gen.IntRange(0, 998),
		gen.IntRange(0, 998),
		gen.IntRange(0, 998),
	))

	properties.TestingRun(t)
}

func TestPropertyMarkdownFormattingPreservation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 6.1: Headers are preserved
	properties.Property("headers are preserved", prop.ForAll(
		func(_ int) bool {
			notes := "### Added\n- Feature\n\n### Fixed\n- Bug"
			entry := ChangelogEntry{
				Version:      "1.0.0",
				Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				ReleaseNotes: notes,
			}

			result, err := GenerateChangelogEntry(entry)
			if err != nil {
				return false
			}

			return strings.Count(result, "###") == strings.Count(notes, "###")
		},
		gen.IntRange(0, 1),
	))

	// Property 6.2: List items are preserved
	properties.Property("list items are preserved", prop.ForAll(
		func(_ int) bool {
			notes := "- Item 1\n- Item 2\n- Item 3"
			entry := ChangelogEntry{
				Version:      "1.0.0",
				Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				ReleaseNotes: notes,
			}

			result, err := GenerateChangelogEntry(entry)
			if err != nil {
				return false
			}

			return strings.Count(result, "- ") >= strings.Count(notes, "- ")
		},
		gen.IntRange(0, 1),
	))

	// Property 6.3: Code blocks are preserved
	properties.Property("code blocks are preserved", prop.ForAll(
		func(_ int) bool {
			notes := "```go\nfunc main() {}\n```"
			entry := ChangelogEntry{
				Version:      "1.0.0",
				Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				ReleaseNotes: notes,
			}

			result, err := GenerateChangelogEntry(entry)
			if err != nil {
				return false
			}

			return strings.Count(result, "```") == strings.Count(notes, "```")
		},
		gen.IntRange(0, 1),
	))

	// Property 6.4: Links are preserved
	properties.Property("links are preserved", prop.ForAll(
		func(_ int) bool {
			notes := "See [documentation](https://example.com) for details"
			entry := ChangelogEntry{
				Version:      "1.0.0",
				Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				ReleaseNotes: notes,
			}

			result, err := GenerateChangelogEntry(entry)
			if err != nil {
				return false
			}

			return strings.Contains(result, "[documentation](https://example.com)")
		},
		gen.IntRange(0, 1),
	))

	// Property 6.5: Bold text is preserved
	properties.Property("bold text is preserved", prop.ForAll(
		func(_ int) bool {
			notes := "This is **bold** text"
			entry := ChangelogEntry{
				Version:      "1.0.0",
				Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				ReleaseNotes: notes,
			}

			result, err := GenerateChangelogEntry(entry)
			if err != nil {
				return false
			}

			return strings.Contains(result, "**bold**")
		},
		gen.IntRange(0, 1),
	))

	// Property 6.6: Inline code is preserved
	properties.Property("inline code is preserved", prop.ForAll(
		func(_ int) bool {
			notes := "Use `go run main.go` to start"
			entry := ChangelogEntry{
				Version:      "1.0.0",
				Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				ReleaseNotes: notes,
			}

			result, err := GenerateChangelogEntry(entry)
			if err != nil {
				return false
			}

			return strings.Contains(result, "`go run main.go`")
		},
		gen.IntRange(0, 1),
	))

	// Property 6.7: ValidateMarkdownPreservation correctly validates preservation
	properties.Property("ValidateMarkdownPreservation correctly validates preservation", prop.ForAll(
		func(content string) bool {
			if content == "" {
				return true
			}
			processed := PreserveMarkdownFormatting(content)
			return ValidateMarkdownPreservation(strings.TrimSpace(content), processed)
		},
		genMarkdownContent(),
	))

	// Property 6.8: Mixed markdown content is preserved
	properties.Property("mixed markdown content is preserved", prop.ForAll(
		func(_ int) bool {
			notes := "### Features\n\n- **New**: Added `feature`\n- See [docs](https://example.com)\n\n```bash\nmake build\n```"
			entry := ChangelogEntry{
				Version:      "1.0.0",
				Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				ReleaseNotes: notes,
			}

			result, err := GenerateChangelogEntry(entry)
			if err != nil {
				return false
			}

			// Check all markdown elements are present
			return strings.Contains(result, "###") &&
				strings.Contains(result, "- **New**") &&
				strings.Contains(result, "`feature`") &&
				strings.Contains(result, "[docs](https://example.com)") &&
				strings.Contains(result, "```bash")
		},
		gen.IntRange(0, 1),
	))

	// Property 6.9: Prepending preserves existing markdown formatting
	properties.Property("prepending preserves existing markdown formatting", prop.ForAll(
		func(_ int) bool {
			existingChangelog := `# Changelog

All notable changes to Narvana are documented here.

## [0.1.0] - 2025-01-01

### Added
- **Feature**: Initial release with ` + "`core`" + ` functionality
- See [docs](https://example.com)

` + "```go\nfunc main() {}\n```" + `

[0.1.0]: https://github.com/narvanalabs/control-plane/releases/tag/v0.1.0
`
			entry := ChangelogEntry{
				Version:      "1.0.0",
				Date:         time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				ReleaseNotes: "### Added\n- New feature",
			}

			result, err := PrependChangelogEntry(existingChangelog, entry)
			if err != nil {
				return false
			}

			// Check existing markdown is preserved
			return strings.Contains(result, "**Feature**") &&
				strings.Contains(result, "`core`") &&
				strings.Contains(result, "[docs](https://example.com)") &&
				strings.Contains(result, "```go")
		},
		gen.IntRange(0, 1),
	))

	properties.TestingRun(t)
}
