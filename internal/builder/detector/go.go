package detector

import (
	"bufio"
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/narvanalabs/control-plane/internal/models"
)

// goVersionRegex matches the go directive in go.mod (e.g., "go 1.21" or "go 1.21.0").
var goVersionRegex = regexp.MustCompile(`^go\s+(\d+\.\d+(?:\.\d+)?)`)

// cgoImports is a list of packages that indicate CGO usage.
var cgoImports = map[string]bool{
	"C":                     true,
	"unsafe":                true, // Often used with CGO
	"runtime/cgo":           true,
	"database/sql":          true, // Often uses CGO drivers
	"github.com/mattn/go-sqlite3": true,
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

// detectCGOUsage scans Go files for CGO imports.
func detectCGOUsage(repoPath string) (bool, error) {
	usesCGO := false

	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip vendor and hidden directories
		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process .go files
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Parse the file for imports
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil // Skip files that can't be parsed
		}

		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if cgoImports[importPath] {
				usesCGO = true
				return filepath.SkipAll // Found CGO, stop walking
			}
		}

		// Also check for //go:cgo_import_* directives
		if hasCGODirectives(node) {
			usesCGO = true
			return filepath.SkipAll
		}

		return nil
	})

	return usesCGO, err
}

// hasCGODirectives checks if a file has CGO-related directives.
func hasCGODirectives(node *ast.File) bool {
	for _, cg := range node.Comments {
		for _, c := range cg.List {
			text := c.Text
			if strings.Contains(text, "cgo") || strings.Contains(text, "CGO") {
				return true
			}
		}
	}
	return false
}
