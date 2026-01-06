// Package detector provides build strategy detection for repositories.
package detector

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// CGOPackages lists common packages that require CGO.
// These are well-known packages that use C bindings.
// **Validates: Requirements 16.1, 1.3**
var CGOPackages = []string{
	// SQLite drivers (CGO-based)
	"github.com/mattn/go-sqlite3",

	// Crypto and security
	"github.com/miekg/pkcs11",
	"github.com/spacemonkeygo/openssl",

	// System interaction (some features require CGO)
	"github.com/shirou/gopsutil",

	// GUI libraries
	"github.com/therecipe/qt",
	"fyne.io/fyne",

	// RocksDB bindings
	"github.com/tecbot/gorocksdb",
	"github.com/linxGnu/grocksdb",

	// LevelDB CGO bindings (not the pure Go version)
	"github.com/jmhodges/levigo",

	// Image processing with C bindings
	"github.com/gographics/imagick",
	"gopkg.in/gographics/imagick.v2",
	"gopkg.in/gographics/imagick.v3",

	// Audio/Video processing
	"github.com/giorgisio/goav",
	"github.com/3d0c/gmf",

	// Compression with C bindings
	"github.com/DataDog/zstd",
	"github.com/valyala/gozstd",

	// Network/System
	"github.com/google/gopacket",

	// Machine learning
	"gorgonia.org/tensor",
	"github.com/tensorflow/tensorflow/tensorflow/go",
}

// CGOResult contains the result of CGO detection.
type CGOResult struct {
	// RequiresCGO indicates whether the project requires CGO
	RequiresCGO bool `json:"requires_cgo"`

	// Reason explains why CGO is required (or not)
	Reason string `json:"reason,omitempty"`

	// DetectedPackages lists the packages that triggered CGO detection
	DetectedPackages []string `json:"detected_packages,omitempty"`

	// HasCImport indicates if any file has `import "C"`
	HasCImport bool `json:"has_c_import"`

	// HasCGODirectives indicates if any file has CGO directives
	HasCGODirectives bool `json:"has_cgo_directives"`
}

// DetectCGO checks if a Go project requires CGO.
// It scans the repository for:
// 1. Known CGO-requiring packages in go.mod
// 2. `import "C"` statements in Go files
// 3. CGO-related build directives
//
// **Validates: Requirements 16.1**
func DetectCGO(repoPath string) (*CGOResult, error) {
	result := &CGOResult{
		RequiresCGO:      false,
		DetectedPackages: []string{},
	}

	// Check go.mod for known CGO packages
	goModPath := filepath.Join(repoPath, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		packages, err := scanGoModForCGOPackages(goModPath)
		if err != nil {
			slog.Warn("failed to scan go.mod for CGO packages",
				"path", goModPath,
				"error", err,
			)
			// Continue with other detection methods
		} else if len(packages) > 0 {
			result.RequiresCGO = true
			result.DetectedPackages = append(result.DetectedPackages, packages...)
			result.Reason = "detected CGO-requiring packages in go.mod"
		}
	}

	// Scan Go files for CGO imports and directives
	hasCImport, hasCGODirectives, err := scanGoFilesForCGO(repoPath)
	if err != nil {
		slog.Warn("failed to scan Go files for CGO",
			"path", repoPath,
			"error", err,
		)
		// Continue with what we have
	}

	result.HasCImport = hasCImport
	result.HasCGODirectives = hasCGODirectives

	if hasCImport {
		result.RequiresCGO = true
		if result.Reason == "" {
			result.Reason = "detected import \"C\" in source files"
		} else {
			result.Reason += "; detected import \"C\" in source files"
		}
	}

	if hasCGODirectives {
		result.RequiresCGO = true
		if result.Reason == "" {
			result.Reason = "detected CGO directives in source files"
		} else {
			result.Reason += "; detected CGO directives in source files"
		}
	}

	if !result.RequiresCGO {
		result.Reason = "no CGO requirements detected"
	}

	return result, nil
}

// scanGoModForCGOPackages scans go.mod for known CGO-requiring packages.
func scanGoModForCGOPackages(goModPath string) ([]string, error) {
	file, err := os.Open(goModPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var detectedPackages []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check each known CGO package
		for _, pkg := range CGOPackages {
			if strings.Contains(line, pkg) {
				detectedPackages = append(detectedPackages, pkg)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return detectedPackages, err
	}

	return detectedPackages, nil
}

// scanGoFilesForCGO scans Go files for CGO imports and directives.
func scanGoFilesForCGO(repoPath string) (hasCImport bool, hasCGODirectives bool, err error) {
	err = filepath.Walk(repoPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // Skip errors, continue walking
		}

		// Skip vendor and hidden directories
		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == "node_modules" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process .go files (skip test files for performance)
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Parse the file
		fset := token.NewFileSet()
		node, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly|parser.ParseComments)
		if parseErr != nil {
			return nil // Skip files that can't be parsed
		}

		// Check for import "C"
		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if importPath == "C" {
				hasCImport = true
			}
		}

		// Check for CGO directives in comments
		if checkCGODirectives(node) {
			hasCGODirectives = true
		}

		// Early exit if both found
		if hasCImport && hasCGODirectives {
			return filepath.SkipAll
		}

		return nil
	})

	return hasCImport, hasCGODirectives, err
}

// checkCGODirectives checks if a file has CGO-related directives.
func checkCGODirectives(node *ast.File) bool {
	for _, cg := range node.Comments {
		for _, c := range cg.List {
			text := c.Text
			// Check for common CGO directives
			if strings.Contains(text, "#cgo") ||
				strings.Contains(text, "//go:cgo_") ||
				strings.Contains(text, "// #cgo") {
				return true
			}
		}
	}
	return false
}

// IsCGOPackage checks if a package path is a known CGO-requiring package.
func IsCGOPackage(pkgPath string) bool {
	for _, pkg := range CGOPackages {
		if strings.HasPrefix(pkgPath, pkg) {
			return true
		}
	}
	return false
}
