//go:build ignore

// Command release_entry_cmd generates a release entry markdown file for the changelog site.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
	flag.StringVar(&outputFile, "output", "", "Output file path")
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

	entry := scripts.ReleaseEntry{
		VersionNumber: version,
		Date:          date,
		Content:       releaseNotes,
	}

	markdown, err := scripts.GenerateReleaseEntry(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to generate release entry: %v\n", err)
		os.Exit(1)
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
