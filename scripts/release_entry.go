// Package scripts provides release entry generation utilities for the changelog site.
package scripts

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"
)

// ReleaseEntry represents the data needed to generate a release entry markdown file.
type ReleaseEntry struct {
	Title         string    // e.g., "Narvana v1.0.0"
	Date          time.Time // Release date
	VersionNumber string    // e.g., "1.0.0"
	Description   string    // Brief summary of the release
	ImagePath     string    // Path to banner image (relative to markdown file)
	ImageAlt      string    // Alt text for banner image
	Content       string    // Release notes content (markdown)
}

// releaseEntryTemplate is the template for generating release entry markdown files.
const releaseEntryTemplate = `---
title: "{{.Title}}"
date: "{{.FormattedDate}}"
versionNumber: "{{.VersionNumber}}"
description: "{{.Description}}"
image:
  src: "{{.ImagePath}}"
  alt: "{{.ImageAlt}}"
---

## What's New in v{{.VersionNumber}}

{{.Content}}
`

// templateData is used to pass data to the template.
type templateData struct {
	Title         string
	FormattedDate string
	VersionNumber string
	Description   string
	ImagePath     string
	ImageAlt      string
	Content       string
}

// GenerateReleaseEntry generates a release entry markdown file content.
func GenerateReleaseEntry(entry ReleaseEntry) (string, error) {
	if entry.VersionNumber == "" {
		return "", fmt.Errorf("version number is required")
	}
	if entry.Date.IsZero() {
		return "", fmt.Errorf("date is required")
	}

	// Set defaults
	if entry.Title == "" {
		entry.Title = fmt.Sprintf("Narvana v%s", entry.VersionNumber)
	}
	if entry.Description == "" {
		entry.Description = fmt.Sprintf("Release notes for Narvana v%s", entry.VersionNumber)
	}
	if entry.ImagePath == "" {
		entry.ImagePath = fmt.Sprintf("../../assets/release-%s.svg", VersionToUnderscore(entry.VersionNumber))
	}
	if entry.ImageAlt == "" {
		entry.ImageAlt = fmt.Sprintf("Narvana v%s Release", entry.VersionNumber)
	}
	if entry.Content == "" {
		entry.Content = "No release notes provided."
	}

	tmpl, err := template.New("release").Parse(releaseEntryTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	data := templateData{
		Title:         entry.Title,
		FormattedDate: entry.Date.Format("2006-01-02"),
		VersionNumber: entry.VersionNumber,
		Description:   entry.Description,
		ImagePath:     entry.ImagePath,
		ImageAlt:      entry.ImageAlt,
		Content:       strings.TrimSpace(entry.Content),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// GenerateReleaseFilename generates the filename for a release entry.
// Example: "1.0.0" -> "1_0_0.md"
func GenerateReleaseFilename(version string) string {
	return VersionToUnderscore(version) + ".md"
}

// NewReleaseEntryFromTag creates a ReleaseEntry from a git tag and release notes.
func NewReleaseEntryFromTag(tag string, releaseNotes string, releaseDate time.Time) (ReleaseEntry, error) {
	version, err := ExtractVersionString(tag)
	if err != nil {
		return ReleaseEntry{}, fmt.Errorf("invalid tag: %w", err)
	}

	return ReleaseEntry{
		Title:         fmt.Sprintf("Narvana v%s", version),
		Date:          releaseDate,
		VersionNumber: version,
		Description:   fmt.Sprintf("Release notes for Narvana v%s", version),
		Content:       releaseNotes,
	}, nil
}

// ValidateReleaseEntry validates that a release entry has all required fields.
func ValidateReleaseEntry(entry ReleaseEntry) error {
	if entry.VersionNumber == "" {
		return fmt.Errorf("version number is required")
	}
	if entry.Date.IsZero() {
		return fmt.Errorf("date is required")
	}
	return nil
}

// ParseReleaseEntryFrontmatter extracts frontmatter fields from generated markdown.
// Returns a map of field names to values.
func ParseReleaseEntryFrontmatter(markdown string) (map[string]string, error) {
	lines := strings.Split(markdown, "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return nil, fmt.Errorf("invalid frontmatter: missing opening delimiter")
	}

	result := make(map[string]string)
	inFrontmatter := true
	for i := 1; i < len(lines) && inFrontmatter; i++ {
		line := lines[i]
		if line == "---" {
			inFrontmatter = false
			continue
		}

		// Parse key: value pairs
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			// Remove quotes from values
			value = strings.Trim(value, "\"")
			result[key] = value
		}
	}

	return result, nil
}

// HasRequiredFrontmatterFields checks if the frontmatter contains all required fields.
func HasRequiredFrontmatterFields(frontmatter map[string]string) bool {
	requiredFields := []string{"title", "date", "versionNumber", "description", "image"}
	for _, field := range requiredFields {
		if _, ok := frontmatter[field]; !ok {
			return false
		}
	}
	return true
}
