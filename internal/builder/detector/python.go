package detector

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/narvanalabs/control-plane/internal/models"
)

// Python version regex patterns.
var (
	pythonVersionRegex     = regexp.MustCompile(`^(\d+\.\d+(?:\.\d+)?)`)
	pyprojectVersionRegex  = regexp.MustCompile(`(?m)^python\s*=\s*["'][\^~>=<]*(\d+\.\d+)`)
	pyprojectRequiresRegex = regexp.MustCompile(`(?m)^requires-python\s*=\s*["']>=?(\d+\.\d+)`)
)

// Python framework detection patterns.
var pythonFrameworkPatterns = map[models.Framework][]string{
	models.FrameworkDjango:  {"django"},
	models.FrameworkFastAPI: {"fastapi"},
	models.FrameworkFlask:   {"flask"},
}

// DetectPython checks for Python application markers.
func (d *DefaultDetector) DetectPython(ctx context.Context, repoPath string) (*models.DetectionResult, error) {
	// Check for Python project markers
	hasPyProject := fileExists(filepath.Join(repoPath, "pyproject.toml"))
	hasRequirements := fileExists(filepath.Join(repoPath, "requirements.txt"))
	hasSetupPy := fileExists(filepath.Join(repoPath, "setup.py"))

	if !hasPyProject && !hasRequirements && !hasSetupPy {
		return nil, nil // Not a Python project
	}

	result := &models.DetectionResult{
		Strategy:             models.BuildStrategyAutoPython,
		Framework:            models.FrameworkGeneric,
		Confidence:           0.9,
		RecommendedBuildType: models.BuildTypePureNix,
		SuggestedConfig:      make(map[string]interface{}),
	}

	// Detect Python version
	pythonVersion := detectPythonVersion(repoPath, hasPyProject)
	if pythonVersion != "" {
		result.Version = pythonVersion
		result.SuggestedConfig["python_version"] = pythonVersion
	} else {
		// Default to Python 3.11
		result.SuggestedConfig["python_version"] = "3.11"
	}

	// Detect framework
	framework := detectPythonFramework(repoPath, hasPyProject, hasRequirements)
	result.Framework = framework

	// Set framework-specific configuration
	switch framework {
	case models.FrameworkDjango:
		result.SuggestedConfig["framework"] = "django"
		// Try to detect settings module
		if settingsModule := detectDjangoSettings(repoPath); settingsModule != "" {
			result.SuggestedConfig["settings_module"] = settingsModule
		}
	case models.FrameworkFastAPI:
		result.SuggestedConfig["framework"] = "fastapi"
		// Try to detect app module
		if appModule := detectFastAPIApp(repoPath); appModule != "" {
			result.SuggestedConfig["app_module"] = appModule
		}
	case models.FrameworkFlask:
		result.SuggestedConfig["framework"] = "flask"
	}

	// Detect dependency manager
	if hasPyProject {
		if hasPoetryLock(repoPath) {
			result.SuggestedConfig["dependency_manager"] = "poetry"
		} else {
			result.SuggestedConfig["dependency_manager"] = "pip"
		}
	} else {
		result.SuggestedConfig["dependency_manager"] = "pip"
	}

	return result, nil
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// detectPythonVersion determines the Python version from various sources.
func detectPythonVersion(repoPath string, hasPyProject bool) string {
	// Priority 1: .python-version file
	pythonVersionPath := filepath.Join(repoPath, ".python-version")
	if data, err := os.ReadFile(pythonVersionPath); err == nil {
		version := strings.TrimSpace(string(data))
		if matches := pythonVersionRegex.FindStringSubmatch(version); len(matches) > 1 {
			return matches[1]
		}
	}

	// Priority 2: pyproject.toml
	if hasPyProject {
		pyprojectPath := filepath.Join(repoPath, "pyproject.toml")
		if data, err := os.ReadFile(pyprojectPath); err == nil {
			content := string(data)

			// Check for requires-python
			if matches := pyprojectRequiresRegex.FindStringSubmatch(content); len(matches) > 1 {
				return matches[1]
			}

			// Check for python = "^3.x" in dependencies
			if matches := pyprojectVersionRegex.FindStringSubmatch(content); len(matches) > 1 {
				return matches[1]
			}
		}
	}

	// Priority 3: runtime.txt (Heroku-style)
	runtimePath := filepath.Join(repoPath, "runtime.txt")
	if data, err := os.ReadFile(runtimePath); err == nil {
		content := strings.TrimSpace(string(data))
		if strings.HasPrefix(content, "python-") {
			version := strings.TrimPrefix(content, "python-")
			if matches := pythonVersionRegex.FindStringSubmatch(version); len(matches) > 1 {
				return matches[1]
			}
		}
	}

	return ""
}

// detectPythonFramework identifies the framework used in the project.
func detectPythonFramework(repoPath string, hasPyProject, hasRequirements bool) models.Framework {
	var dependencies []string

	// Collect dependencies from pyproject.toml
	if hasPyProject {
		pyprojectPath := filepath.Join(repoPath, "pyproject.toml")
		if data, err := os.ReadFile(pyprojectPath); err == nil {
			dependencies = append(dependencies, extractPyProjectDependencies(string(data))...)
		}
	}

	// Collect dependencies from requirements.txt
	if hasRequirements {
		requirementsPath := filepath.Join(repoPath, "requirements.txt")
		if deps, err := parseRequirementsTxt(requirementsPath); err == nil {
			dependencies = append(dependencies, deps...)
		}
	}

	// Check for frameworks in order of specificity
	for framework, patterns := range pythonFrameworkPatterns {
		for _, pattern := range patterns {
			for _, dep := range dependencies {
				if strings.EqualFold(dep, pattern) {
					return framework
				}
			}
		}
	}

	return models.FrameworkGeneric
}

// extractPyProjectDependencies extracts dependency names from pyproject.toml.
func extractPyProjectDependencies(content string) []string {
	var deps []string

	// Simple regex to find dependency names
	depRegex := regexp.MustCompile(`(?m)^([a-zA-Z][a-zA-Z0-9_-]*)\s*=`)
	matches := depRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			deps = append(deps, strings.ToLower(match[1]))
		}
	}

	// Also check for dependencies in array format
	arrayDepRegex := regexp.MustCompile(`["']([a-zA-Z][a-zA-Z0-9_-]*)`)
	arrayMatches := arrayDepRegex.FindAllStringSubmatch(content, -1)
	for _, match := range arrayMatches {
		if len(match) > 1 {
			deps = append(deps, strings.ToLower(match[1]))
		}
	}

	return deps
}

// parseRequirementsTxt parses a requirements.txt file.
func parseRequirementsTxt(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var deps []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip -r includes
		if strings.HasPrefix(line, "-r") || strings.HasPrefix(line, "-e") {
			continue
		}

		// Extract package name (before any version specifier)
		depRegex := regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_-]*)`)
		if matches := depRegex.FindStringSubmatch(line); len(matches) > 1 {
			deps = append(deps, strings.ToLower(matches[1]))
		}
	}

	return deps, scanner.Err()
}

// hasPoetryLock checks if poetry.lock exists.
func hasPoetryLock(repoPath string) bool {
	return fileExists(filepath.Join(repoPath, "poetry.lock"))
}

// detectDjangoSettings tries to find the Django settings module.
func detectDjangoSettings(repoPath string) string {
	// Look for manage.py and extract settings module
	managePyPath := filepath.Join(repoPath, "manage.py")
	if data, err := os.ReadFile(managePyPath); err == nil {
		content := string(data)
		settingsRegex := regexp.MustCompile(`DJANGO_SETTINGS_MODULE.*?["']([^"']+)["']`)
		if matches := settingsRegex.FindStringSubmatch(content); len(matches) > 1 {
			return matches[1]
		}
	}

	// Look for common patterns
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() {
			settingsPath := filepath.Join(repoPath, entry.Name(), "settings.py")
			if fileExists(settingsPath) {
				return entry.Name() + ".settings"
			}
		}
	}

	return ""
}

// detectFastAPIApp tries to find the FastAPI app module.
func detectFastAPIApp(repoPath string) string {
	// Common patterns for FastAPI apps
	patterns := []struct {
		file   string
		module string
	}{
		{"main.py", "main:app"},
		{"app/main.py", "app.main:app"},
		{"src/main.py", "src.main:app"},
		{"app.py", "app:app"},
	}

	for _, p := range patterns {
		filePath := filepath.Join(repoPath, p.file)
		if data, err := os.ReadFile(filePath); err == nil {
			content := string(data)
			// Check if file contains FastAPI app definition
			if strings.Contains(content, "FastAPI(") {
				return p.module
			}
		}
	}

	return ""
}
