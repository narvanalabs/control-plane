//go:build ignore

// Command release_entry_cmd generates a release entry markdown file for the changelog site.
// It supports both the legacy mode (raw release notes passthrough) and the enhanced mode
// (intelligent parsing, filtering, grouping, and formatting of commits).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/narvanalabs/control-plane/scripts"
)

func main() {
	var (
		version      string
		dateStr      string
		notesFile    string
		outputFile   string
		overrideFile string
		enhanced     bool
		projectName  string
	)

	flag.StringVar(&version, "version", "", "Version number (e.g., 1.0.0)")
	flag.StringVar(&dateStr, "date", "", "Release date (YYYY-MM-DD)")
	flag.StringVar(&notesFile, "notes", "", "Path to file containing release notes (commit messages, one per line)")
	flag.StringVar(&outputFile, "output", "", "Output file path")
	flag.StringVar(&overrideFile, "override", "", "Path to override file with custom title/intro/closing (optional)")
	flag.BoolVar(&enhanced, "enhanced", true, "Use enhanced release notes generation (default: true)")
	flag.StringVar(&projectName, "project", "Narvana", "Project name for release notes (default: Narvana)")
	flag.Parse()

	if version == "" {
		fmt.Fprintln(os.Stderr, "Error: -version is required")
		os.Exit(1)
	}

	if outputFile == "" {
		fmt.Fprintln(os.Stderr, "Error: -output is required")
		os.Exit(1)
	}

	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid date format: %v\n", err)
		os.Exit(1)
	}

	var releaseNotes string
	if notesFile != "" {
		content, err := os.ReadFile(notesFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to read notes file: %v\n", err)
			os.Exit(1)
		}
		releaseNotes = string(content)
	}

	var markdown string

	if enhanced {
		// Enhanced mode: parse, filter, group, and format commits
		markdown, err = generateEnhancedReleaseNotes(version, dateStr, releaseNotes, overrideFile, projectName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to generate enhanced release notes: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Legacy mode: passthrough raw release notes
		entry := scripts.ReleaseEntry{
			VersionNumber: version,
			Date:          date,
			Content:       releaseNotes,
		}

		markdown, err = scripts.GenerateReleaseEntry(entry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to generate release entry: %v\n", err)
			os.Exit(1)
		}
	}

	// Ensure directory exists
	dir := filepath.Dir(outputFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputFile, []byte(markdown), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to write release entry: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully created release entry at %s\n", outputFile)
}

// generateEnhancedReleaseNotes generates release notes using the intelligent processing pipeline.
// It parses commits, filters noise, groups related changes, and formats them into sections.
func generateEnhancedReleaseNotes(version, dateStr, releaseNotes, overrideFile, projectName string) (string, error) {
	// Parse override file if provided
	var override *scripts.OverrideContent
	if overrideFile != "" {
		var err error
		override, err = scripts.ParseOverrideFile(overrideFile)
		if err != nil {
			// Log warning but continue with defaults
			fmt.Fprintf(os.Stderr, "Warning: failed to parse override file: %v\n", err)
		}
	}

	// Parse raw release notes into individual commit messages
	// GitHub release notes typically have commits separated by newlines
	rawCommits := parseRawCommits(releaseNotes)

	// Process commits through the pipeline with fallback handling
	filterConfig := scripts.DefaultNoiseFilterConfig()
	categorizedContent := scripts.ProcessCommitsWithFallback(rawCommits, filterConfig)

	// Build the release notes config
	config := scripts.ReleaseNotesConfig{
		Version:     version,
		Date:        dateStr,
		ProjectName: projectName,
	}

	// Apply overrides if available
	if override != nil {
		config.Title = override.Title
		config.Introduction = override.Introduction
		config.Closing = override.Closing
	}

	// Generate the final markdown
	return scripts.GenerateReleaseNotes(categorizedContent, config)
}

// parseRawCommits splits raw release notes into individual commit messages.
// It handles various formats:
// - One commit per line
// - Commits prefixed with "* " or "- " (GitHub release notes format)
// - Commits with hash prefixes like "abc1234 feat: description"
func parseRawCommits(releaseNotes string) []string {
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
		// "* " or "- " bullet points
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "- ")

		// Remove leading hash if present (e.g., "abc1234 feat: description")
		// This pattern matches a short hash followed by space
		if len(line) > 8 && isHexString(line[:7]) && line[7] == ' ' {
			line = line[8:]
		}

		line = strings.TrimSpace(line)
		if line != "" {
			commits = append(commits, line)
		}
	}

	return commits
}

// isHexString checks if a string contains only hexadecimal characters.
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
