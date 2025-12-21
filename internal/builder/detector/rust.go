package detector

import (
	"context"
	"os"
	"path/filepath"
	"regexp"

	"github.com/narvanalabs/control-plane/internal/models"
)

// Rust TOML parsing regexes.
var (
	rustEditionRegex    = regexp.MustCompile(`(?m)^edition\s*=\s*["'](\d{4})["']`)
	rustBinaryNameRegex = regexp.MustCompile(`(?m)^\[\[bin\]\][\s\S]*?name\s*=\s*["']([^"']+)["']`)
	rustPackageNameRegex = regexp.MustCompile(`(?m)^\[package\][\s\S]*?name\s*=\s*["']([^"']+)["']`)
)

// DetectRust checks for Rust application markers.
func (d *DefaultDetector) DetectRust(ctx context.Context, repoPath string) (*models.DetectionResult, error) {
	cargoTomlPath := filepath.Join(repoPath, "Cargo.toml")
	if _, err := os.Stat(cargoTomlPath); os.IsNotExist(err) {
		return nil, nil // Not a Rust project
	}

	result := &models.DetectionResult{
		Strategy:             models.BuildStrategyAutoRust,
		Framework:            models.FrameworkGeneric,
		Confidence:           0.9,
		RecommendedBuildType: models.BuildTypePureNix,
		SuggestedConfig:      make(map[string]interface{}),
	}

	// Parse Cargo.toml
	data, err := os.ReadFile(cargoTomlPath)
	if err != nil {
		result.Warnings = append(result.Warnings, "Could not read Cargo.toml")
		return result, nil
	}

	content := string(data)

	// Extract Rust edition
	if matches := rustEditionRegex.FindStringSubmatch(content); len(matches) > 1 {
		result.Version = matches[1]
		result.SuggestedConfig["rust_edition"] = matches[1]
	} else {
		// Default to 2021 edition
		result.SuggestedConfig["rust_edition"] = "2021"
	}

	// Extract binary name
	binaryName := extractRustBinaryName(content)
	if binaryName != "" {
		result.SuggestedConfig["binary_name"] = binaryName
		result.EntryPoints = []string{binaryName}
	}

	// Check for workspace
	if isRustWorkspace(content) {
		result.Warnings = append(result.Warnings, "Rust workspace detected - may need to specify package")
	}

	return result, nil
}

// extractRustBinaryName extracts the binary name from Cargo.toml.
func extractRustBinaryName(content string) string {
	// First check for explicit [[bin]] section
	if matches := rustBinaryNameRegex.FindStringSubmatch(content); len(matches) > 1 {
		return matches[1]
	}

	// Fall back to package name
	if matches := rustPackageNameRegex.FindStringSubmatch(content); len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// isRustWorkspace checks if the Cargo.toml defines a workspace.
func isRustWorkspace(content string) bool {
	workspaceRegex := regexp.MustCompile(`(?m)^\[workspace\]`)
	return workspaceRegex.MatchString(content)
}
