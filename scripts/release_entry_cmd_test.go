package scripts

import (
	"fmt"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: intelligent-release-notes, Property CLI-1: End-to-end generation with sample commits**
// For any valid set of conventional commits, the enhanced release notes generation pipeline
// SHALL produce valid markdown with frontmatter containing all required fields.
// **Validates: Requirements 15.1, 15.2, 15.3, 15.4**

// TestIntegrationEnhancedReleaseNotes tests the end-to-end generation pipeline.
func TestIntegrationEnhancedReleaseNotes(t *testing.T) {
	// Test with sample commits that represent a typical release
	sampleCommits := []string{
		"feat(api): add user authentication endpoint",
		"feat(web): implement dashboard redesign",
		"fix(db): resolve connection pool leak",
		"fix(auth): correct token validation logic",
		"perf(api): optimize query performance",
		"docs: update API documentation",
		"chore: update dependencies",
		"style: fix formatting issues",
		"feat!: redesign configuration format",
	}

	// Process commits through the pipeline
	filterConfig := DefaultNoiseFilterConfig()
	categorizedContent := ProcessCommitsWithFallback(sampleCommits, filterConfig)

	// Build config
	config := ReleaseNotesConfig{
		Version:     "1.2.3",
		Date:        "2026-01-08",
		ProjectName: "Narvana",
	}

	// Generate release notes
	markdown, err := GenerateReleaseNotes(categorizedContent, config)
	if err != nil {
		t.Fatalf("Failed to generate release notes: %v", err)
	}

	// Verify frontmatter
	fm, _, err := ParseReleaseNotesFrontmatter(markdown)
	if err != nil {
		t.Fatalf("Failed to parse frontmatter: %v", err)
	}

	// Check required fields
	if fm.Title == "" {
		t.Error("Title should not be empty")
	}
	if fm.Date != "2026-01-08" {
		t.Errorf("Date mismatch: expected 2026-01-08, got %s", fm.Date)
	}
	if fm.VersionNumber != "1.2.3" {
		t.Errorf("VersionNumber mismatch: expected 1.2.3, got %s", fm.VersionNumber)
	}
	if fm.Description == "" {
		t.Error("Description should not be empty")
	}
	if fm.Image.Src == "" {
		t.Error("Image.Src should not be empty")
	}

	// Verify sections are present (using emoji format)
	if !strings.Contains(markdown, "🚀 New Features") {
		t.Error("Features section should be present")
	}
	if !strings.Contains(markdown, "🐛 Bug Fixes") {
		t.Error("Bug Fixes section should be present")
	}
	if !strings.Contains(markdown, "⚠️ Breaking Changes") {
		t.Error("Breaking Changes section should be present")
	}

	// Verify noise commits are filtered
	if strings.Contains(markdown, "update dependencies") {
		t.Error("Chore commits should be filtered out")
	}
	if strings.Contains(markdown, "fix formatting") {
		t.Error("Style commits should be filtered out")
	}

	// Introduction and closing are no longer included by default
	// They are only included when explicitly provided via override
}

// TestIntegrationEmptyCommits tests generation with no commits.
func TestIntegrationEmptyCommits(t *testing.T) {
	// Process empty commits
	filterConfig := DefaultNoiseFilterConfig()
	categorizedContent := ProcessCommitsWithFallback([]string{}, filterConfig)

	config := ReleaseNotesConfig{
		Version:     "1.0.0",
		Date:        "2026-01-08",
		ProjectName: "Narvana",
	}

	markdown, err := GenerateReleaseNotes(categorizedContent, config)
	if err != nil {
		t.Fatalf("Failed to generate release notes: %v", err)
	}

	// Should still produce valid markdown
	fm, _, err := ParseReleaseNotesFrontmatter(markdown)
	if err != nil {
		t.Fatalf("Failed to parse frontmatter: %v", err)
	}

	if fm.Title == "" {
		t.Error("Title should not be empty even with no commits")
	}
	if fm.VersionNumber != "1.0.0" {
		t.Errorf("VersionNumber mismatch: expected 1.0.0, got %s", fm.VersionNumber)
	}

	// Should have placeholder message
	if !strings.Contains(markdown, "No significant changes") {
		t.Error("Should contain placeholder message for empty releases")
	}
}

// TestIntegrationAllNoiseCommits tests generation when all commits are noise.
func TestIntegrationAllNoiseCommits(t *testing.T) {
	noiseCommits := []string{
		"chore: update dependencies",
		"style: fix whitespace",
		"ci: update workflow",
		"test: add unit tests",
		"fix typo in readme",
		"fix lint errors",
	}

	filterConfig := DefaultNoiseFilterConfig()
	categorizedContent := ProcessCommitsWithFallback(noiseCommits, filterConfig)

	config := ReleaseNotesConfig{
		Version:     "1.0.1",
		Date:        "2026-01-08",
		ProjectName: "Narvana",
	}

	markdown, err := GenerateReleaseNotes(categorizedContent, config)
	if err != nil {
		t.Fatalf("Failed to generate release notes: %v", err)
	}

	// Should still produce valid markdown
	fm, _, err := ParseReleaseNotesFrontmatter(markdown)
	if err != nil {
		t.Fatalf("Failed to parse frontmatter: %v", err)
	}

	if fm.Title == "" {
		t.Error("Title should not be empty")
	}

	// Should have placeholder message since all commits were filtered
	if !strings.Contains(markdown, "No significant changes") {
		t.Error("Should contain placeholder message when all commits are noise")
	}
}

// TestIntegrationWithOverride tests generation with override content.
func TestIntegrationWithOverride(t *testing.T) {
	sampleCommits := []string{
		"feat(api): add new endpoint",
		"fix(web): resolve display issue",
	}

	filterConfig := DefaultNoiseFilterConfig()
	categorizedContent := ProcessCommitsWithFallback(sampleCommits, filterConfig)

	config := ReleaseNotesConfig{
		Version:      "2.0.0",
		Date:         "2026-01-08",
		ProjectName:  "Narvana",
		Title:        "A New Era with 2.0",
		Introduction: "Welcome to version 2.0! This is a major release.",
		Closing:      "Thanks for using Narvana!",
	}

	markdown, err := GenerateReleaseNotes(categorizedContent, config)
	if err != nil {
		t.Fatalf("Failed to generate release notes: %v", err)
	}

	// Verify custom title
	if !strings.Contains(markdown, "A New Era with 2.0") {
		t.Error("Custom title should be used")
	}

	// Verify custom introduction
	if !strings.Contains(markdown, "Welcome to version 2.0") {
		t.Error("Custom introduction should be used")
	}

	// Verify custom closing
	if !strings.Contains(markdown, "Thanks for using Narvana") {
		t.Error("Custom closing should be used")
	}
}

// TestIntegrationMajorVersionTitle tests creative title generation for major versions.
func TestIntegrationMajorVersionTitle(t *testing.T) {
	sampleCommits := []string{
		"feat: initial release",
	}

	filterConfig := DefaultNoiseFilterConfig()
	categorizedContent := ProcessCommitsWithFallback(sampleCommits, filterConfig)

	config := ReleaseNotesConfig{
		Version:     "1.0.0",
		Date:        "2026-01-08",
		ProjectName: "Narvana",
	}

	markdown, err := GenerateReleaseNotes(categorizedContent, config)
	if err != nil {
		t.Fatalf("Failed to generate release notes: %v", err)
	}

	// Major version should get creative title
	if !strings.Contains(markdown, "The Beginning") {
		t.Error("Major version 1.0.0 should get creative title 'The Beginning'")
	}
}

// TestIntegrationMinorVersionTitle tests standard title for minor versions.
func TestIntegrationMinorVersionTitle(t *testing.T) {
	sampleCommits := []string{
		"feat: new feature",
	}

	filterConfig := DefaultNoiseFilterConfig()
	categorizedContent := ProcessCommitsWithFallback(sampleCommits, filterConfig)

	config := ReleaseNotesConfig{
		Version:     "1.2.0",
		Date:        "2026-01-08",
		ProjectName: "Narvana",
	}

	markdown, err := GenerateReleaseNotes(categorizedContent, config)
	if err != nil {
		t.Fatalf("Failed to generate release notes: %v", err)
	}

	// Minor version should get standard title
	if !strings.Contains(markdown, "Narvana v1.2.0") {
		t.Error("Minor version should get standard 'Narvana vX.Y.Z' title")
	}
}

// TestIntegrationBannerPath tests banner path generation.
func TestIntegrationBannerPath(t *testing.T) {
	sampleCommits := []string{
		"feat: new feature",
	}

	filterConfig := DefaultNoiseFilterConfig()
	categorizedContent := ProcessCommitsWithFallback(sampleCommits, filterConfig)

	config := ReleaseNotesConfig{
		Version:     "1.2.3",
		Date:        "2026-01-08",
		ProjectName: "Narvana",
	}

	markdown, err := GenerateReleaseNotes(categorizedContent, config)
	if err != nil {
		t.Fatalf("Failed to generate release notes: %v", err)
	}

	// Banner path should use correct format
	if !strings.Contains(markdown, "../../assets/release-1_2_3.svg") {
		t.Error("Banner path should use correct relative format with underscores")
	}
}

// TestPropertyEndToEndGeneration tests the full pipeline with property-based testing.
// **Feature: intelligent-release-notes, Property CLI-2: Output format verification**
// For any valid version and date, the generated release notes SHALL match the expected format.
// **Validates: Requirements 15.1, 15.2, 15.3, 15.4**
func TestPropertyEndToEndGeneration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Generated markdown always has valid frontmatter
	properties.Property("generated markdown always has valid frontmatter", prop.ForAll(
		func(major, minor, patch int) bool {
			version := genVersionString(major, minor, patch)
			
			commits := []string{
				"feat: add feature",
				"fix: fix bug",
			}

			filterConfig := DefaultNoiseFilterConfig()
			categorizedContent := ProcessCommitsWithFallback(commits, filterConfig)

			config := ReleaseNotesConfig{
				Version:     version,
				Date:        "2026-01-08",
				ProjectName: "Narvana",
			}

			markdown, err := GenerateReleaseNotes(categorizedContent, config)
			if err != nil {
				return false
			}

			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}

			return fm.Title != "" &&
				fm.Date == "2026-01-08" &&
				fm.VersionNumber == version &&
				fm.Description != "" &&
				fm.Image.Src != ""
		},
		gen.IntRange(0, 99),
		gen.IntRange(0, 99),
		gen.IntRange(0, 99),
	))

	// Property: Version number in frontmatter matches input
	properties.Property("version number in frontmatter matches input", prop.ForAll(
		func(major, minor, patch int) bool {
			version := genVersionString(major, minor, patch)
			
			commits := []string{"feat: test"}
			filterConfig := DefaultNoiseFilterConfig()
			categorizedContent := ProcessCommitsWithFallback(commits, filterConfig)

			config := ReleaseNotesConfig{
				Version:     version,
				Date:        "2026-01-08",
				ProjectName: "Narvana",
			}

			markdown, err := GenerateReleaseNotes(categorizedContent, config)
			if err != nil {
				return false
			}

			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}

			return fm.VersionNumber == version
		},
		gen.IntRange(0, 99),
		gen.IntRange(0, 99),
		gen.IntRange(0, 99),
	))

	// Property: Banner path uses correct format
	properties.Property("banner path uses correct format", prop.ForAll(
		func(major, minor, patch int) bool {
			version := genVersionString(major, minor, patch)
			
			commits := []string{"feat: test"}
			filterConfig := DefaultNoiseFilterConfig()
			categorizedContent := ProcessCommitsWithFallback(commits, filterConfig)

			config := ReleaseNotesConfig{
				Version:     version,
				Date:        "2026-01-08",
				ProjectName: "Narvana",
			}

			markdown, err := GenerateReleaseNotes(categorizedContent, config)
			if err != nil {
				return false
			}

			fm, _, err := ParseReleaseNotesFrontmatter(markdown)
			if err != nil {
				return false
			}

			// Banner path should start with ../../assets/ and end with .svg
			return strings.HasPrefix(fm.Image.Src, "../../assets/") &&
				strings.HasSuffix(fm.Image.Src, ".svg")
		},
		gen.IntRange(0, 99),
		gen.IntRange(0, 99),
		gen.IntRange(0, 99),
	))

	// Property: Introduction is present when provided
	properties.Property("introduction is present when provided", prop.ForAll(
		func(major, minor, patch int) bool {
			version := genVersionString(major, minor, patch)
			
			commits := []string{"feat: test"}
			filterConfig := DefaultNoiseFilterConfig()
			categorizedContent := ProcessCommitsWithFallback(commits, filterConfig)

			config := ReleaseNotesConfig{
				Version:      version,
				Date:         "2026-01-08",
				ProjectName:  "Narvana",
				Introduction: "Custom intro text",
			}

			markdown, err := GenerateReleaseNotes(categorizedContent, config)
			if err != nil {
				return false
			}

			// Should contain the custom introduction
			return strings.Contains(markdown, "Custom intro text")
		},
		gen.IntRange(0, 99),
		gen.IntRange(0, 99),
		gen.IntRange(0, 99),
	))

	// Property: Closing is present when provided
	properties.Property("closing is present when provided", prop.ForAll(
		func(major, minor, patch int) bool {
			version := genVersionString(major, minor, patch)
			
			commits := []string{"feat: test"}
			filterConfig := DefaultNoiseFilterConfig()
			categorizedContent := ProcessCommitsWithFallback(commits, filterConfig)

			config := ReleaseNotesConfig{
				Version:     version,
				Date:        "2026-01-08",
				ProjectName: "Narvana",
				Closing:     "Custom closing text",
			}

			markdown, err := GenerateReleaseNotes(categorizedContent, config)
			if err != nil {
				return false
			}

			// Should contain the custom closing
			return strings.Contains(markdown, "Custom closing text")
		},
		gen.IntRange(0, 99),
		gen.IntRange(0, 99),
		gen.IntRange(0, 99),
	))

	properties.TestingRun(t)
}

// genVersionString generates a version string from major, minor, patch components.
func genVersionString(major, minor, patch int) string {
	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}

// TestParseRawCommits tests the commit parsing helper function.
func TestParseRawCommits(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single commit",
			input:    "feat: add feature",
			expected: []string{"feat: add feature"},
		},
		{
			name:     "multiple commits",
			input:    "feat: add feature\nfix: fix bug\ndocs: update docs",
			expected: []string{"feat: add feature", "fix: fix bug", "docs: update docs"},
		},
		{
			name:     "commits with bullet points",
			input:    "* feat: add feature\n* fix: fix bug",
			expected: []string{"feat: add feature", "fix: fix bug"},
		},
		{
			name:     "commits with dash bullets",
			input:    "- feat: add feature\n- fix: fix bug",
			expected: []string{"feat: add feature", "fix: fix bug"},
		},
		{
			name:     "commits with hash prefix",
			input:    "abc1234 feat: add feature\ndef5678 fix: fix bug",
			expected: []string{"feat: add feature", "fix: fix bug"},
		},
		{
			name:     "mixed format",
			input:    "* abc1234 feat: add feature\n- def5678 fix: fix bug\nperf: improve speed",
			expected: []string{"feat: add feature", "fix: fix bug", "perf: improve speed"},
		},
		{
			name:     "empty lines ignored",
			input:    "feat: add feature\n\nfix: fix bug\n\n",
			expected: []string{"feat: add feature", "fix: fix bug"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRawCommitsForTest(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d commits, got %d", len(tt.expected), len(result))
				return
			}
			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Commit %d: expected %q, got %q", i, expected, result[i])
				}
			}
		})
	}
}

// parseRawCommitsForTest is a copy of the parseRawCommits function from release_entry_cmd.go
// for testing purposes (since the cmd file uses //go:build ignore).
func parseRawCommitsForTest(releaseNotes string) []string {
	if strings.TrimSpace(releaseNotes) == "" {
		return []string{}
	}

	lines := strings.Split(releaseNotes, "\n")
	commits := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Remove common prefixes from GitHub release notes
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "- ")

		// Remove leading hash if present
		if len(line) > 8 && isHexStringForTest(line[:7]) && line[7] == ' ' {
			line = line[8:]
		}

		line = strings.TrimSpace(line)
		if line != "" {
			commits = append(commits, line)
		}
	}

	return commits
}

// isHexStringForTest checks if a string contains only hexadecimal characters.
func isHexStringForTest(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
