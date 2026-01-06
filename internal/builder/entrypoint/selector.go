// Package entrypoint provides entry point selection for multi-binary projects.
package entrypoint

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/narvanalabs/control-plane/internal/models"
)

// EntryPoint represents a buildable binary/script.
type EntryPoint struct {
	Path        string `json:"path"`        // e.g., "cmd/api", "src/main.py"
	Name        string `json:"name"`        // e.g., "api", "main"
	IsDefault   bool   `json:"is_default"`  // Heuristically determined default
	Description string `json:"description"` // Auto-generated description
}

// EntryPointSelector handles multi-binary project selection.
type EntryPointSelector interface {
	// ListEntryPoints returns all detected entry points.
	ListEntryPoints(ctx context.Context, repoPath string, language string) ([]EntryPoint, error)

	// SelectDefault returns the default entry point based on heuristics.
	SelectDefault(entryPoints []EntryPoint) *EntryPoint

	// Validate checks if a specified entry point exists.
	Validate(ctx context.Context, repoPath string, entryPoint string) error
}

// Selector implements the EntryPointSelector interface.
type Selector struct{}

// NewSelector creates a new Selector.
func NewSelector() *Selector {
	return &Selector{}
}

// ListEntryPoints returns all detected entry points for the given language.
func (s *Selector) ListEntryPoints(ctx context.Context, repoPath string, language string) ([]EntryPoint, error) {
	if repoPath == "" {
		return nil, ErrEmptyRepoPath
	}

	// Verify the repository path exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return nil, ErrRepoNotFound
	}

	switch strings.ToLower(language) {
	case "go", "golang":
		return s.listGoEntryPoints(ctx, repoPath)
	case "node", "nodejs", "javascript", "typescript":
		return s.listNodeEntryPoints(ctx, repoPath)
	case "rust":
		return s.listRustEntryPoints(ctx, repoPath)
	case "python":
		return s.listPythonEntryPoints(ctx, repoPath)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedLanguage, language)
	}
}

// SelectDefault returns the default entry point based on heuristics.
// Priority order:
// 1. Entry point already marked as default
// 2. Entry point named "main", "app", "server", or "api"
// 3. Entry point in root directory
// 4. First entry point alphabetically
func (s *Selector) SelectDefault(entryPoints []EntryPoint) *EntryPoint {
	if len(entryPoints) == 0 {
		return nil
	}

	// Check for already marked default
	for i := range entryPoints {
		if entryPoints[i].IsDefault {
			return &entryPoints[i]
		}
	}

	// Priority names for default selection
	priorityNames := []string{"main", "app", "server", "api"}

	// Check for priority names
	for _, name := range priorityNames {
		for i := range entryPoints {
			if strings.ToLower(entryPoints[i].Name) == name {
				return &entryPoints[i]
			}
		}
	}

	// Check for root directory entry point
	for i := range entryPoints {
		if entryPoints[i].Path == "." || entryPoints[i].Path == "" {
			return &entryPoints[i]
		}
	}

	// Return first entry point
	return &entryPoints[0]
}

// Validate checks if a specified entry point exists in the repository.
func (s *Selector) Validate(ctx context.Context, repoPath string, entryPoint string) error {
	if repoPath == "" {
		return ErrEmptyRepoPath
	}

	if entryPoint == "" {
		return ErrEmptyEntryPoint
	}

	// Verify the repository path exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return ErrRepoNotFound
	}

	// Construct the full path to the entry point
	entryPointPath := filepath.Join(repoPath, entryPoint)

	// Check if the entry point path exists
	info, err := os.Stat(entryPointPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrEntryPointNotFound, entryPoint)
	}
	if err != nil {
		return fmt.Errorf("checking entry point: %w", err)
	}

	// Entry point should be a directory (for Go cmd/*, Python packages)
	// or a file (for single-file entry points)
	if !info.IsDir() && !isExecutableFile(entryPointPath) {
		return fmt.Errorf("%w: %s is not a valid entry point", ErrInvalidEntryPoint, entryPoint)
	}

	return nil
}

// GoEntryPointDirs lists directories to check for main packages in Go repositories.
// **Validates: Requirements 19.1, 19.2, 19.3, 19.4**
var GoEntryPointDirs = []string{
	"cmd",      // Standard Go layout (cmd/*)
	"apps",     // Alternative layout (apps/*)
	"services", // Microservices layout (services/*)
}

// listGoEntryPoints finds all main packages in a Go repository.
// **Validates: Requirements 19.1, 19.2, 19.3, 19.4**
func (s *Selector) listGoEntryPoints(ctx context.Context, repoPath string) ([]EntryPoint, error) {
	var entryPoints []EntryPoint

	// Check for main.go in root
	// **Validates: Requirements 19.1**
	if hasGoMainPackage(repoPath) {
		entryPoints = append(entryPoints, EntryPoint{
			Path:        ".",
			Name:        "main",
			IsDefault:   true,
			Description: "Main package in root directory",
		})
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
					if hasGoMainPackage(subDir) {
						ep := EntryPoint{
							Path:        filepath.Join(dirName, entry.Name()),
							Name:        entry.Name(),
							IsDefault:   false,
							Description: fmt.Sprintf("Binary from %s: %s", dirName, entry.Name()),
						}
						entryPoints = append(entryPoints, ep)
					}
				}
			}
		}
	}

	// If we have entries but no root main, mark the best one as default
	if len(entryPoints) > 0 && entryPoints[0].Path != "." {
		// Apply default selection heuristics
		defaultEP := s.SelectDefault(entryPoints)
		if defaultEP != nil {
			for i := range entryPoints {
				if entryPoints[i].Path == defaultEP.Path {
					entryPoints[i].IsDefault = true
					break
				}
			}
		}
	}

	if len(entryPoints) == 0 {
		return nil, ErrNoEntryPointsFound
	}

	return entryPoints, nil
}

// listNodeEntryPoints finds entry points in a Node.js repository.
func (s *Selector) listNodeEntryPoints(ctx context.Context, repoPath string) ([]EntryPoint, error) {
	var entryPoints []EntryPoint

	// Read package.json
	packageJSONPath := filepath.Join(repoPath, "package.json")
	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return nil, fmt.Errorf("reading package.json: %w", err)
	}

	var pkg struct {
		Main    string            `json:"main"`
		Scripts map[string]string `json:"scripts"`
		Bin     interface{}       `json:"bin"` // Can be string or map
	}

	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parsing package.json: %w", err)
	}

	// Check for main field
	if pkg.Main != "" {
		entryPoints = append(entryPoints, EntryPoint{
			Path:        pkg.Main,
			Name:        filepath.Base(pkg.Main),
			IsDefault:   true,
			Description: "Main entry point from package.json",
		})
	}

	// Check for bin field
	switch bin := pkg.Bin.(type) {
	case string:
		if bin != "" && bin != pkg.Main {
			entryPoints = append(entryPoints, EntryPoint{
				Path:        bin,
				Name:        filepath.Base(bin),
				IsDefault:   len(entryPoints) == 0,
				Description: "Binary entry point",
			})
		}
	case map[string]interface{}:
		for name, path := range bin {
			pathStr, ok := path.(string)
			if !ok {
				continue
			}
			entryPoints = append(entryPoints, EntryPoint{
				Path:        pathStr,
				Name:        name,
				IsDefault:   false,
				Description: fmt.Sprintf("Binary: %s", name),
			})
		}
	}

	// Check for common entry point files if no main/bin specified
	if len(entryPoints) == 0 {
		commonEntryPoints := []string{
			"index.js", "index.ts", "src/index.js", "src/index.ts",
			"server.js", "server.ts", "src/server.js", "src/server.ts",
			"app.js", "app.ts", "src/app.js", "src/app.ts",
			"main.js", "main.ts", "src/main.js", "src/main.ts",
		}

		for _, ep := range commonEntryPoints {
			fullPath := filepath.Join(repoPath, ep)
			if _, err := os.Stat(fullPath); err == nil {
				entryPoints = append(entryPoints, EntryPoint{
					Path:        ep,
					Name:        strings.TrimSuffix(filepath.Base(ep), filepath.Ext(ep)),
					IsDefault:   len(entryPoints) == 0,
					Description: "Detected entry point file",
				})
			}
		}
	}

	// Apply default selection if needed
	if len(entryPoints) > 0 {
		hasDefault := false
		for _, ep := range entryPoints {
			if ep.IsDefault {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			defaultEP := s.SelectDefault(entryPoints)
			if defaultEP != nil {
				for i := range entryPoints {
					if entryPoints[i].Path == defaultEP.Path {
						entryPoints[i].IsDefault = true
						break
					}
				}
			}
		}
	}

	if len(entryPoints) == 0 {
		return nil, ErrNoEntryPointsFound
	}

	return entryPoints, nil
}

// listRustEntryPoints finds entry points in a Rust repository.
func (s *Selector) listRustEntryPoints(ctx context.Context, repoPath string) ([]EntryPoint, error) {
	var entryPoints []EntryPoint

	// Check for src/main.rs (default binary)
	mainRsPath := filepath.Join(repoPath, "src", "main.rs")
	if _, err := os.Stat(mainRsPath); err == nil {
		// Get binary name from Cargo.toml
		binaryName := getBinaryNameFromCargoToml(repoPath)
		if binaryName == "" {
			binaryName = "main"
		}
		entryPoints = append(entryPoints, EntryPoint{
			Path:        "src/main.rs",
			Name:        binaryName,
			IsDefault:   true,
			Description: "Default binary",
		})
	}

	// Check for src/bin/* directories
	binDir := filepath.Join(repoPath, "src", "bin")
	if info, err := os.Stat(binDir); err == nil && info.IsDir() {
		entries, err := os.ReadDir(binDir)
		if err == nil {
			for _, entry := range entries {
				name := entry.Name()
				if strings.HasSuffix(name, ".rs") {
					binName := strings.TrimSuffix(name, ".rs")
					entryPoints = append(entryPoints, EntryPoint{
						Path:        filepath.Join("src", "bin", name),
						Name:        binName,
						IsDefault:   false,
						Description: fmt.Sprintf("Binary: %s", binName),
					})
				} else if entry.IsDir() {
					// Check for main.rs in subdirectory
					subMainPath := filepath.Join(binDir, name, "main.rs")
					if _, err := os.Stat(subMainPath); err == nil {
						entryPoints = append(entryPoints, EntryPoint{
							Path:        filepath.Join("src", "bin", name),
							Name:        name,
							IsDefault:   false,
							Description: fmt.Sprintf("Binary: %s", name),
						})
					}
				}
			}
		}
	}

	if len(entryPoints) == 0 {
		return nil, ErrNoEntryPointsFound
	}

	return entryPoints, nil
}

// listPythonEntryPoints finds entry points in a Python repository.
func (s *Selector) listPythonEntryPoints(ctx context.Context, repoPath string) ([]EntryPoint, error) {
	var entryPoints []EntryPoint

	// Common Python entry point patterns
	commonEntryPoints := []struct {
		path        string
		description string
	}{
		{"main.py", "Main entry point"},
		{"app.py", "Application entry point"},
		{"run.py", "Run script"},
		{"server.py", "Server entry point"},
		{"manage.py", "Django management script"},
		{"wsgi.py", "WSGI entry point"},
		{"asgi.py", "ASGI entry point"},
		{"src/main.py", "Main entry point in src"},
		{"src/app.py", "Application entry point in src"},
	}

	for _, ep := range commonEntryPoints {
		fullPath := filepath.Join(repoPath, ep.path)
		if _, err := os.Stat(fullPath); err == nil {
			entryPoints = append(entryPoints, EntryPoint{
				Path:        ep.path,
				Name:        strings.TrimSuffix(filepath.Base(ep.path), ".py"),
				IsDefault:   len(entryPoints) == 0,
				Description: ep.description,
			})
		}
	}

	// Check pyproject.toml for entry points
	pyprojectPath := filepath.Join(repoPath, "pyproject.toml")
	if _, err := os.Stat(pyprojectPath); err == nil {
		pyEntryPoints := parsePyprojectEntryPoints(pyprojectPath)
		for _, ep := range pyEntryPoints {
			// Avoid duplicates
			isDuplicate := false
			for _, existing := range entryPoints {
				if existing.Path == ep.Path {
					isDuplicate = true
					break
				}
			}
			if !isDuplicate {
				entryPoints = append(entryPoints, ep)
			}
		}
	}

	// Apply default selection if needed
	if len(entryPoints) > 0 {
		hasDefault := false
		for _, ep := range entryPoints {
			if ep.IsDefault {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			defaultEP := s.SelectDefault(entryPoints)
			if defaultEP != nil {
				for i := range entryPoints {
					if entryPoints[i].Path == defaultEP.Path {
						entryPoints[i].IsDefault = true
						break
					}
				}
			}
		}
	}

	if len(entryPoints) == 0 {
		return nil, ErrNoEntryPointsFound
	}

	return entryPoints, nil
}

// hasGoMainPackage checks if a directory contains a main package.
func hasGoMainPackage(dir string) bool {
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

// isExecutableFile checks if a file is potentially executable.
func isExecutableFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	executableExtensions := map[string]bool{
		".go":   true,
		".py":   true,
		".js":   true,
		".ts":   true,
		".rs":   true,
		".sh":   true,
		".bash": true,
	}
	return executableExtensions[ext]
}

// getBinaryNameFromCargoToml extracts the binary name from Cargo.toml.
func getBinaryNameFromCargoToml(repoPath string) string {
	cargoPath := filepath.Join(repoPath, "Cargo.toml")
	file, err := os.Open(cargoPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inPackage := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "[package]" {
			inPackage = true
			continue
		}

		if strings.HasPrefix(line, "[") {
			inPackage = false
			continue
		}

		if inPackage && strings.HasPrefix(line, "name") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[1])
				name = strings.Trim(name, `"'`)
				return name
			}
		}
	}

	return ""
}

// parsePyprojectEntryPoints extracts entry points from pyproject.toml.
func parsePyprojectEntryPoints(pyprojectPath string) []EntryPoint {
	var entryPoints []EntryPoint

	file, err := os.Open(pyprojectPath)
	if err != nil {
		return entryPoints
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inScripts := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "[project.scripts]" || line == "[tool.poetry.scripts]" {
			inScripts = true
			continue
		}

		if strings.HasPrefix(line, "[") {
			inScripts = false
			continue
		}

		if inScripts && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[0])
				path := strings.TrimSpace(parts[1])
				path = strings.Trim(path, `"'`)

				entryPoints = append(entryPoints, EntryPoint{
					Path:        path,
					Name:        name,
					IsDefault:   false,
					Description: fmt.Sprintf("Script: %s", name),
				})
			}
		}
	}

	return entryPoints
}

// MergeConfig merges a user-provided BuildConfig with detected values from DetectionResult.
// User-provided values always override detected values.
func MergeConfig(detected map[string]interface{}, userConfig *models.BuildConfig) *models.BuildConfig {
	if userConfig == nil {
		userConfig = &models.BuildConfig{}
	}

	// If no detected config, return user config as-is
	if detected == nil {
		return userConfig
	}

	// Create a result config starting with user values
	result := *userConfig

	// Apply detected values only if user hasn't specified them
	if result.GoVersion == "" {
		if v, ok := detected["go_version"].(string); ok {
			result.GoVersion = v
		}
	}

	if result.NodeVersion == "" {
		if v, ok := detected["node_version"].(string); ok {
			result.NodeVersion = v
		}
	}

	if result.PythonVersion == "" {
		if v, ok := detected["python_version"].(string); ok {
			result.PythonVersion = v
		}
	}

	if result.RustEdition == "" {
		if v, ok := detected["rust_edition"].(string); ok {
			result.RustEdition = v
		}
	}

	if result.EntryPoint == "" {
		if v, ok := detected["entry_point"].(string); ok {
			result.EntryPoint = v
		}
	}

	if result.BuildCommand == "" {
		if v, ok := detected["build_command"].(string); ok {
			result.BuildCommand = v
		}
	}

	if result.StartCommand == "" {
		if v, ok := detected["start_command"].(string); ok {
			result.StartCommand = v
		}
	}

	if result.PackageManager == "" {
		if v, ok := detected["package_manager"].(string); ok {
			result.PackageManager = v
		}
	}

	// CGOEnabled is a *bool, so we need special handling
	// Only use detected value if user hasn't explicitly set it
	if result.CGOEnabled == nil {
		if v, ok := detected["cgo_enabled"].(bool); ok {
			result.CGOEnabled = &v
		}
	}

	// BuildTimeout: use detected if user hasn't set (0 means not set)
	if result.BuildTimeout == 0 {
		if v, ok := detected["build_timeout"].(int); ok {
			result.BuildTimeout = v
		}
	}

	return &result
}
