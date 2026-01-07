//go:build ignore

// Command changelog_cmd updates CHANGELOG.md with a new release entry.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/narvanalabs/control-plane/scripts"
)

func main() {
	var (
		version    string
		dateStr    string
		notesFile  string
		outputFile string
	)

	flag.StringVar(&version, "version", "", "Version number (e.g., 1.0.0)")
	flag.StringVar(&dateStr, "date", "", "Release date (YYYY-MM-DD)")
	flag.StringVar(&notesFile, "notes", "", "Path to file containing release notes")
	flag.StringVar(&outputFile, "output", "CHANGELOG.md", "Output file path")
	flag.Parse()

	if version == "" {
		fmt.Fprintln(os.Stderr, "Error: -version is required")
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

	// Read existing changelog if it exists
	var existingChangelog string
	if content, err := os.ReadFile(outputFile); err == nil {
		existingChangelog = string(content)
	}

	entry := scripts.ChangelogEntry{
		Version:      version,
		Date:         date,
		ReleaseNotes: releaseNotes,
		ReleaseURL:   fmt.Sprintf("https://github.com/narvanalabs/control-plane/releases/tag/v%s", version),
	}

	// Check if version already exists
	if scripts.ChangelogContainsVersion(existingChangelog, version) {
		fmt.Printf("Version %s already exists in changelog, skipping\n", version)
		os.Exit(0)
	}

	newChangelog, err := scripts.PrependChangelogEntry(existingChangelog, entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to generate changelog: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputFile, []byte(newChangelog), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to write changelog: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully updated %s with version %s\n", outputFile, version)
}
