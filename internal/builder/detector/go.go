package detector

import (
	"bufio"
	"context"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/narvanalabs/control-plane/internal/models"
)

// goVersionRegex matches the go directive in go.mod (e.g., "go 1.21" or "go 1.21.0").
var goVersionRegex = regexp.MustCompile(`^go\s+(\d+\.\d+(?:\.\d+)?)`)

// DetectGo checks for Go application markers.
func (d *DefaultDetector) DetectGo(ctx context.Context, repoPath string) (*models.DetectionResult, error) {
	goModPath := filepath.Join(repoPath, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		return nil, nil // Not a Go project
	}

	result := &models.DetectionResult{
		Strategy:             models.BuildStrategyAutoGo,
		Framework:            models.FrameworkGeneric,
		Confidence:           0.9,
		RecommendedBuildType: models.BuildTypePureNix,
		SuggestedConfig:      make(map[string]interface{}),
	}

	// Parse go.mod for Go version
	version, err := parseGoVersion(goModPath)
	if err != nil {
		result.Warnings = append(result.Warnings, "Could not parse Go version from go.mod")
	} else {
		result.Version = version
		result.SuggestedConfig["go_version"] = version
	}

	// Detect main package locations
	entryPoints, err := detectGoEntryPoints(repoPath)
	if err != nil {
		result.Warnings = append(result.Warnings, "Could not detect entry points")
	} else {
		result.EntryPoints = entryPoints
		if len(entryPoints) == 1 {
			result.SuggestedConfig["entry_point"] = entryPoints[0]
		}
	}

	// Detect CGO usage
	usesCGO, err := detectCGOUsage(repoPath)
	if err != nil {
		result.Warnings = append(result.Warnings, "Could not detect CGO usage")
	} else if usesCGO {
		result.SuggestedConfig["cgo_enabled"] = true
		result.Warnings = append(result.Warnings, "CGO detected - build may require additional system dependencies")
	}

	return result, nil
}

// parseGoVersion extracts the Go version from go.mod.
func parseGoVersion(goModPath string) (string, error) {
	file, err := os.Open(goModPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if matches := goVersionRegex.FindStringSubmatch(line); len(matches) > 1 {
			return matches[1], nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", ErrInvalidGoMod
}

// detectGoEntryPoints finds all main packages in the repository.
func detectGoEntryPoints(repoPath string) ([]string, error) {
	var entryPoints []string

	// Check for main.go in root
	if hasMainPackage(repoPath) {
		entryPoints = append(entryPoints, ".")
	}

	// Check cmd/* directories
	cmdDir := filepath.Join(repoPath, "cmd")
	if info, err := os.Stat(cmdDir); err == nil && info.IsDir() {
		entries, err := os.ReadDir(cmdDir)
		if err != nil {
			return entryPoints, nil // Return what we have
		}

		for _, entry := range entries {
			if entry.IsDir() {
				subDir := filepath.Join(cmdDir, entry.Name())
				if hasMainPackage(subDir) {
					entryPoints = append(entryPoints, filepath.Join("cmd", entry.Name()))
				}
			}
		}
	}

	return entryPoints, nil
}

// hasMainPackage checks if a directory contains a main package.
func hasMainPackage(dir string) bool {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.PackageClauseOnly)
	if err != nil {
		return false
	}

	for _, pkg := range pkgs {
		if pkg.Name == "main" {
			return true
		}
	}
	return false
}

// detectCGOUsage scans Go files for CGO imports using the dedicated CGO detector.
// This function wraps DetectCGO for backward compatibility.
// **Validates: Requirements 16.1, 16.2, 16.4**
//
// On detection failure:
// - Logs a warning with the error details
// - Returns false (CGO_ENABLED=0) as the safe default
func detectCGOUsage(repoPath string) (bool, error) {
	result, err := DetectCGO(repoPath)
	if err != nil {
		// **Validates: Requirements 16.4** - Log warning and fall back to pure Go build
		slog.Warn("CGO detection failed, defaulting to CGO_ENABLED=0",
			"path", repoPath,
			"error", err,
		)
		return false, nil // Return false (no CGO) as safe default, don't propagate error
	}
	return result.RequiresCGO, nil
}
