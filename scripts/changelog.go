// Package scripts provides CHANGELOG.md generation utilities.
package scripts

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"
)

// ChangelogEntry represents a single release entry in the changelog.
type ChangelogEntry struct {
	Version     string    // e.g., "1.0.0"
	Date        time.Time // Release date
	ReleaseURL  string    // URL to the GitHub release
	ReleaseNotes string   // Release notes content (markdown)
}

// changelogEntryTemplate is the template for a single changelog entry.
const changelogEntryTemplate = `## [{{.Version}}] - {{.FormattedDate}}

{{.ReleaseNotes}}

`

// changelogHeaderTemplate is the template for the changelog header.
const changelogHeaderTemplate = `# Changelog

All notable changes to Narvana are documented here.

`

// changelogFooterLinkTemplate is the template for version links at the bottom.
const changelogFooterLinkTemplate = `[{{.Version}}]: {{.ReleaseURL}}
`

// entryTemplateData is used to pass data to the entry template.
type entryTemplateData struct {
	Version       string
	FormattedDate string
	ReleaseNotes  string
}

// linkTemplateData is used to pass data to the link template.
type linkTemplateData struct {
	Version    string
	ReleaseURL string
}

// GenerateChangelogEntry generates a single changelog entry in conventional format.
func GenerateChangelogEntry(entry ChangelogEntry) (string, error) {
	if entry.Version == "" {
		return "", fmt.Errorf("version is required")
	}
	if entry.Date.IsZero() {
		return "", fmt.Errorf("date is required")
	}

	tmpl, err := template.New("entry").Parse(changelogEntryTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	data := entryTemplateData{
		Version:       entry.Version,
		FormattedDate: entry.Date.Format("2006-01-02"),
		ReleaseNotes:  strings.TrimSpace(entry.ReleaseNotes),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// GenerateChangelogLink generates a version link for the changelog footer.
func GenerateChangelogLink(entry ChangelogEntry) (string, error) {
	if entry.Version == "" {
		return "", fmt.Errorf("version is required")
	}
	if entry.ReleaseURL == "" {
		entry.ReleaseURL = fmt.Sprintf("https://github.com/narvanalabs/control-plane/releases/tag/v%s", entry.Version)
	}

	tmpl, err := template.New("link").Parse(changelogFooterLinkTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	data := linkTemplateData{
		Version:    entry.Version,
		ReleaseURL: entry.ReleaseURL,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// PrependChangelogEntry prepends a new entry to an existing changelog.
// It preserves the header and inserts the new entry after it.
func PrependChangelogEntry(existingChangelog string, entry ChangelogEntry) (string, error) {
	newEntry, err := GenerateChangelogEntry(entry)
	if err != nil {
		return "", err
	}

	newLink, err := GenerateChangelogLink(entry)
	if err != nil {
		return "", err
	}

	// If changelog is empty or doesn't have header, create new one
	if existingChangelog == "" || !strings.HasPrefix(existingChangelog, "# Changelog") {
		return changelogHeaderTemplate + newEntry + newLink, nil
	}

	// Find the position after the header (after "documented here.\n\n")
	headerEndMarker := "documented here.\n\n"
	headerEndIdx := strings.Index(existingChangelog, headerEndMarker)
	if headerEndIdx == -1 {
		// Try alternative: just after first blank line after header
		headerEndMarker = "\n\n"
		headerEndIdx = strings.Index(existingChangelog, headerEndMarker)
		if headerEndIdx == -1 {
			// No proper header found, prepend after first line
			firstNewline := strings.Index(existingChangelog, "\n")
			if firstNewline == -1 {
				return existingChangelog + "\n\n" + newEntry + newLink, nil
			}
			headerEndIdx = firstNewline
			headerEndMarker = "\n"
		}
	}

	insertPos := headerEndIdx + len(headerEndMarker)

	// Find where version links start (lines starting with [version]:)
	linkSectionRegex := regexp.MustCompile(`(?m)^\[[\d.]+\]:`)
	linkMatch := linkSectionRegex.FindStringIndex(existingChangelog)

	var result strings.Builder
	result.WriteString(existingChangelog[:insertPos])
	result.WriteString(newEntry)

	if linkMatch != nil {
		// Insert content between header and links, then add new link before existing links
		result.WriteString(existingChangelog[insertPos:linkMatch[0]])
		result.WriteString(newLink)
		result.WriteString(existingChangelog[linkMatch[0]:])
	} else {
		// No existing links, just append the rest and add link at end
		result.WriteString(existingChangelog[insertPos:])
		result.WriteString("\n")
		result.WriteString(newLink)
	}

	return result.String(), nil
}

// CreateNewChangelog creates a new changelog with a single entry.
func CreateNewChangelog(entry ChangelogEntry) (string, error) {
	return PrependChangelogEntry("", entry)
}

// ExtractVersionsFromChangelog extracts all version numbers from a changelog.
func ExtractVersionsFromChangelog(changelog string) []string {
	versionRegex := regexp.MustCompile(`## \[(\d+\.\d+\.\d+)\]`)
	matches := versionRegex.FindAllStringSubmatch(changelog, -1)

	versions := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			versions = append(versions, match[1])
		}
	}
	return versions
}

// ChangelogContainsVersion checks if a changelog already contains a specific version.
func ChangelogContainsVersion(changelog, version string) bool {
	versions := ExtractVersionsFromChangelog(changelog)
	for _, v := range versions {
		if v == version {
			return true
		}
	}
	return false
}

// PreserveMarkdownFormatting ensures markdown formatting is preserved in release notes.
// This is a pass-through function that validates the content is not corrupted.
func PreserveMarkdownFormatting(content string) string {
	// The content is already markdown, we just ensure it's trimmed properly
	return strings.TrimSpace(content)
}

// ValidateMarkdownPreservation checks if markdown elements are preserved.
func ValidateMarkdownPreservation(original, processed string) bool {
	// Check that key markdown elements are preserved
	checks := []string{
		"###",  // Headers
		"- ",   // List items
		"* ",   // Alternative list items
		"```",  // Code blocks
		"[",    // Links (opening)
		"](",   // Links (middle)
		"**",   // Bold
		"_",    // Italic
		"`",    // Inline code
	}

	for _, check := range checks {
		originalCount := strings.Count(original, check)
		processedCount := strings.Count(processed, check)
		if originalCount != processedCount {
			return false
		}
	}
	return true
}
