package scripts

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: release-changelog-cicd, Property 2: Release Entry File Creation**
// For any release data containing version, date, and notes, creating a release entry
// SHALL produce a valid markdown file with frontmatter containing all required fields
// (title, date, versionNumber, description, image).
// **Validates: Requirements 3.1**

// genValidReleaseEntry generates valid release entry data.
func genValidReleaseEntry() gopter.Gen {
	return gen.IntRange(0, 999).FlatMap(func(major interface{}) gopter.Gen {
		return gen.IntRange(0, 999).FlatMap(func(minor interface{}) gopter.Gen {
			return gen.IntRange(0, 999).FlatMap(func(patch interface{}) gopter.Gen {
				return gen.AlphaString().Map(func(content string) ReleaseEntry {
					version := fmt.Sprintf("%d.%d.%d", major.(int), minor.(int), patch.(int))
					if content == "" {
						content = "Some release notes"
					}
					return ReleaseEntry{
						VersionNumber: version,
						Date:          time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
						Content:       content,
					}
				})
			}, nil)
		}, nil)
	}, nil)
}

// genValidDate generates valid dates.
func genValidDate() gopter.Gen {
	return gen.IntRange(2020, 2030).FlatMap(func(year interface{}) gopter.Gen {
		return gen.IntRange(1, 12).FlatMap(func(month interface{}) gopter.Gen {
			return gen.IntRange(1, 28).Map(func(day int) time.Time {
				return time.Date(year.(int), time.Month(month.(int)), day, 0, 0, 0, 0, time.UTC)
			})
		}, nil)
	}, nil)
}

// genReleaseNotes generates release notes content.
func genReleaseNotes() gopter.Gen {
	return gen.OneGenOf(
		gen.Const("### Added\n- New feature"),
		gen.Const("### Fixed\n- Bug fix"),
		gen.Const("### Changed\n- Updated behavior"),
		gen.AlphaString().Map(func(s string) string {
			if s == "" {
				return "Release notes content"
			}
			return s
		}),
	)
}

func TestPropertyReleaseEntryFileCreation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 2.1: Generated markdown contains frontmatter delimiters
	properties.Property("generated markdown contains frontmatter delimiters", prop.ForAll(
		func(major, minor, patch int, content string) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			if content == "" {
				content = "Some content"
			}
			entry := ReleaseEntry{
				VersionNumber: version,
				Date:          time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				Content:       content,
			}

			markdown, err := GenerateReleaseEntry(entry)
			if err != nil {
				return false
			}

			// Check for frontmatter delimiters
			lines := strings.Split(markdown, "\n")
			if len(lines) < 2 {
				return false
			}
			return lines[0] == "---" && strings.Contains(markdown, "\n---\n")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.AlphaString(),
	))

	// Property 2.2: Generated markdown contains title field
	properties.Property("generated markdown contains title field", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			entry := ReleaseEntry{
				VersionNumber: version,
				Date:          time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				Content:       "Some content",
			}

			markdown, err := GenerateReleaseEntry(entry)
			if err != nil {
				return false
			}

			return strings.Contains(markdown, "title:")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 2.3: Generated markdown contains date field
	properties.Property("generated markdown contains date field", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			entry := ReleaseEntry{
				VersionNumber: version,
				Date:          time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				Content:       "Some content",
			}

			markdown, err := GenerateReleaseEntry(entry)
			if err != nil {
				return false
			}

			return strings.Contains(markdown, "date:")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 2.4: Generated markdown contains versionNumber field
	properties.Property("generated markdown contains versionNumber field", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			entry := ReleaseEntry{
				VersionNumber: version,
				Date:          time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				Content:       "Some content",
			}

			markdown, err := GenerateReleaseEntry(entry)
			if err != nil {
				return false
			}

			return strings.Contains(markdown, "versionNumber:")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 2.5: Generated markdown contains description field
	properties.Property("generated markdown contains description field", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			entry := ReleaseEntry{
				VersionNumber: version,
				Date:          time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				Content:       "Some content",
			}

			markdown, err := GenerateReleaseEntry(entry)
			if err != nil {
				return false
			}

			return strings.Contains(markdown, "description:")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 2.6: Generated markdown contains image field
	properties.Property("generated markdown contains image field", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			entry := ReleaseEntry{
				VersionNumber: version,
				Date:          time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				Content:       "Some content",
			}

			markdown, err := GenerateReleaseEntry(entry)
			if err != nil {
				return false
			}

			return strings.Contains(markdown, "image:")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 2.7: Version number in frontmatter matches input
	properties.Property("version number in frontmatter matches input", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			entry := ReleaseEntry{
				VersionNumber: version,
				Date:          time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				Content:       "Some content",
			}

			markdown, err := GenerateReleaseEntry(entry)
			if err != nil {
				return false
			}

			frontmatter, err := ParseReleaseEntryFrontmatter(markdown)
			if err != nil {
				return false
			}

			return frontmatter["versionNumber"] == version
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 2.8: Date in frontmatter is in YYYY-MM-DD format
	properties.Property("date in frontmatter is in YYYY-MM-DD format", prop.ForAll(
		func(year, month, day int) bool {
			// Ensure valid date
			if month < 1 || month > 12 || day < 1 || day > 28 {
				return true // Skip invalid dates
			}
			date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
			entry := ReleaseEntry{
				VersionNumber: "1.0.0",
				Date:          date,
				Content:       "Some content",
			}

			markdown, err := GenerateReleaseEntry(entry)
			if err != nil {
				return false
			}

			frontmatter, err := ParseReleaseEntryFrontmatter(markdown)
			if err != nil {
				return false
			}

			expectedDate := date.Format("2006-01-02")
			return frontmatter["date"] == expectedDate
		},
		gen.IntRange(2020, 2030),
		gen.IntRange(1, 12),
		gen.IntRange(1, 28),
	))

	// Property 2.9: Title contains version number
	properties.Property("title contains version number", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			entry := ReleaseEntry{
				VersionNumber: version,
				Date:          time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				Content:       "Some content",
			}

			markdown, err := GenerateReleaseEntry(entry)
			if err != nil {
				return false
			}

			frontmatter, err := ParseReleaseEntryFrontmatter(markdown)
			if err != nil {
				return false
			}

			return strings.Contains(frontmatter["title"], version)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 2.10: Generated filename uses underscore format
	properties.Property("generated filename uses underscore format", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			filename := GenerateReleaseFilename(version)

			expected := fmt.Sprintf("%d_%d_%d.md", major, minor, patch)
			return filename == expected
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 2.11: NewReleaseEntryFromTag creates valid entry
	properties.Property("NewReleaseEntryFromTag creates valid entry", prop.ForAll(
		func(major, minor, patch int, content string) bool {
			tag := fmt.Sprintf("v%d.%d.%d", major, minor, patch)
			if content == "" {
				content = "Some content"
			}
			date := time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC)

			entry, err := NewReleaseEntryFromTag(tag, content, date)
			if err != nil {
				return false
			}

			expectedVersion := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			return entry.VersionNumber == expectedVersion &&
				entry.Date.Equal(date) &&
				entry.Content == content
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.AlphaString(),
	))

	// Property 2.12: HasRequiredFrontmatterFields returns true for valid entries
	properties.Property("HasRequiredFrontmatterFields returns true for valid entries", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			entry := ReleaseEntry{
				VersionNumber: version,
				Date:          time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				Content:       "Some content",
			}

			markdown, err := GenerateReleaseEntry(entry)
			if err != nil {
				return false
			}

			frontmatter, err := ParseReleaseEntryFrontmatter(markdown)
			if err != nil {
				return false
			}

			return HasRequiredFrontmatterFields(frontmatter)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	properties.TestingRun(t)
}
