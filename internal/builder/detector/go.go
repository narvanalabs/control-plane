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

// goWorkUseRegex matches the use directive in go.work (e.g., "use ./mymodule" or "use ./path/to/module").
var goWorkUseRegex = regexp.MustCompile(`^\s*use\s+(\S+)`)

// GoWorkspaceResult contains the result of Go workspace detection.
// **Validates: Requirements 22.1, 22.4**
type GoWorkspaceResult struct {
	// IsWorkspace indicates whether a go.work file was found
	IsWorkspace bool `json:"is_workspace"`

	// GoVersion is the Go version specified in go.work (if any)
	GoVersion string `json:"go_version,omitempty"`

	// Modules lists the module paths specified in the use directives
	Modules []string `json:"modules,omitempty"`
}

// DetectGoWorkspace checks if a repository contains a go.work file and parses its contents.
// **Validates: Requirements 22.1, 22.4**
func DetectGoWorkspace(repoPath string) (*GoWorkspaceResult, error) {
	goWorkPath := filepath.Join(repoPath, "go.work")
	if _, err := os.Stat(goWorkPath); os.IsNotExist(err) {
		return &GoWorkspaceResult{IsWorkspace: false}, nil
	}

	result := &GoWorkspaceResult{
		IsWorkspace: true,
		Modules:     []string{},
	}

	// Parse go.work file
	file, err := os.Open(goWorkPath)
	if err != nil {
		return result, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inUseBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		// Check for go version directive
		if matches := goVersionRegex.FindStringSubmatch(line); len(matches) > 1 {
			result.GoVersion = matches[1]
			continue
		}

		// Check for use block start
		if line == "use (" {
			inUseBlock = true
			continue
		}

		// Check for use block end
		if line == ")" && inUseBlock {
			inUseBlock = false
			continue
		}

		// Parse use directives
		if inUseBlock {
			// Inside use block, each line is a module path
			modulePath := strings.TrimSpace(line)
			if modulePath != "" && !strings.HasPrefix(modulePath, "//") {
				result.Modules = append(result.Modules, cleanModulePath(modulePath))
			}
		} else if matches := goWorkUseRegex.FindStringSubmatch(line); len(matches) > 1 {
			// Single-line use directive
			result.Modules = append(result.Modules, cleanModulePath(matches[1]))
		}
	}

	if err := scanner.Err(); err != nil {
		return result, err
	}

	return result, nil
}

// cleanModulePath removes quotes and normalizes a module path from go.work.
func cleanModulePath(path string) string {
	// Remove surrounding quotes if present
	path = strings.Trim(path, `"'`)
	// Normalize path separators
	path = filepath.Clean(path)
	return path
}

// HasGoWorkspace checks if a repository contains a go.work file.
// This is a convenience function for quick workspace detection.
// **Validates: Requirements 22.1**
func HasGoWorkspace(repoPath string) bool {
	goWorkPath := filepath.Join(repoPath, "go.work")
	_, err := os.Stat(goWorkPath)
	return err == nil
}

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

	// Detect Go workspace (go.work file)
	// **Validates: Requirements 22.1, 22.4**
	workspaceResult, err := DetectGoWorkspace(repoPath)
	if err != nil {
		slog.Warn("Go workspace detection failed",
			"path", repoPath,
			"error", err,
		)
		result.Warnings = append(result.Warnings, "Could not parse go.work file")
	} else if workspaceResult.IsWorkspace {
		result.SuggestedConfig["is_workspace"] = true
		result.SuggestedConfig["workspace_modules"] = workspaceResult.Modules
		result.Warnings = append(result.Warnings, "Go workspace detected - multi-module project")

		// If go.work has a Go version, prefer it over go.mod version
		if workspaceResult.GoVersion != "" {
			result.Version = workspaceResult.GoVersion
			result.SuggestedConfig["go_version"] = workspaceResult.GoVersion
		}
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

// GoEntryPointDirs lists directories to check for main packages in Go repositories.
// **Validates: Requirements 19.1, 19.2, 19.3, 19.4**
var GoEntryPointDirs = []string{
	"cmd",      // Standard Go layout (cmd/*)
	"apps",     // Alternative layout (apps/*)
	"services", // Microservices layout (services/*)
}

// detectGoEntryPoints finds all main packages in the repository.
// **Validates: Requirements 19.1, 19.2, 19.3, 19.4**
func detectGoEntryPoints(repoPath string) ([]string, error) {
	var entryPoints []string

	// Check for main.go in root
	// **Validates: Requirements 19.1**
	if hasMainPackage(repoPath) {
		entryPoints = append(entryPoints, ".")
	}

	// Check all standard entry point directories
	// **Validates: Requirements 19.2, 19.3, 19.4**
	for _, dirName := range GoEntryPointDirs {
		dir := filepath.Join(repoPath, dirName)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue // Skip this directory but continue with others
			}

			for _, entry := range entries {
				if entry.IsDir() {
					subDir := filepath.Join(dir, entry.Name())
					if hasMainPackage(subDir) {
						entryPoints = append(entryPoints, filepath.Join(dirName, entry.Name()))
					}
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
