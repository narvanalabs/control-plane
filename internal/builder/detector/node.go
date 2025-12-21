package detector

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/narvanalabs/control-plane/internal/models"
)

// PackageJSON represents the structure of a package.json file.
type PackageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Main            string            `json:"main"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Engines         struct {
		Node string `json:"node"`
		NPM  string `json:"npm"`
	} `json:"engines"`
	PackageManager string `json:"packageManager"`
}

// nodeVersionRegex matches Node.js version strings.
var nodeVersionRegex = regexp.MustCompile(`^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?`)

// semverRegex extracts major version from semver-like strings.
var semverRegex = regexp.MustCompile(`^[\^~>=<]*(\d+)`)

// DetectNode checks for Node.js application markers.
func (d *DefaultDetector) DetectNode(ctx context.Context, repoPath string) (*models.DetectionResult, error) {
	packageJSONPath := filepath.Join(repoPath, "package.json")
	if _, err := os.Stat(packageJSONPath); os.IsNotExist(err) {
		return nil, nil // Not a Node.js project
	}

	// Parse package.json
	pkg, err := parsePackageJSON(packageJSONPath)
	if err != nil {
		return nil, ErrInvalidPackageJSON
	}

	result := &models.DetectionResult{
		Strategy:             models.BuildStrategyAutoNode,
		Framework:            models.FrameworkGeneric,
		Confidence:           0.9,
		RecommendedBuildType: models.BuildTypePureNix,
		SuggestedConfig:      make(map[string]interface{}),
	}

	// Detect Node version
	nodeVersion := detectNodeVersion(repoPath, pkg)
	if nodeVersion != "" {
		result.Version = nodeVersion
		result.SuggestedConfig["node_version"] = nodeVersion
	}

	// Detect package manager
	packageManager := detectPackageManager(repoPath, pkg)
	result.SuggestedConfig["package_manager"] = packageManager

	// Detect framework
	framework, frameworkVersion := detectNodeFramework(pkg)
	result.Framework = framework

	// Set framework-specific configuration
	if framework == models.FrameworkNextJS {
		result.SuggestedConfig["framework"] = "nextjs"
		if frameworkVersion != "" {
			result.SuggestedConfig["nextjs_version"] = frameworkVersion
			// Determine router type based on version
			majorVersion := extractMajorVersion(frameworkVersion)
			if majorVersion >= 13 {
				result.SuggestedConfig["nextjs_router"] = "app"
			} else {
				result.SuggestedConfig["nextjs_router"] = "pages"
			}
		}
	}

	// Detect build and start scripts
	if buildCmd, ok := pkg.Scripts["build"]; ok {
		result.SuggestedConfig["build_command"] = buildCmd
	}
	if startCmd, ok := pkg.Scripts["start"]; ok {
		result.SuggestedConfig["start_command"] = startCmd
	}

	return result, nil
}

// parsePackageJSON reads and parses a package.json file.
func parsePackageJSON(path string) (*PackageJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	return &pkg, nil
}

// detectNodeVersion determines the Node.js version from various sources.
func detectNodeVersion(repoPath string, pkg *PackageJSON) string {
	// Priority 1: .nvmrc file
	nvmrcPath := filepath.Join(repoPath, ".nvmrc")
	if data, err := os.ReadFile(nvmrcPath); err == nil {
		version := strings.TrimSpace(string(data))
		if matches := nodeVersionRegex.FindStringSubmatch(version); len(matches) > 1 {
			return normalizeNodeVersion(version)
		}
	}

	// Priority 2: .node-version file
	nodeVersionPath := filepath.Join(repoPath, ".node-version")
	if data, err := os.ReadFile(nodeVersionPath); err == nil {
		version := strings.TrimSpace(string(data))
		if matches := nodeVersionRegex.FindStringSubmatch(version); len(matches) > 1 {
			return normalizeNodeVersion(version)
		}
	}

	// Priority 3: package.json engines.node
	if pkg.Engines.Node != "" {
		return normalizeNodeVersion(pkg.Engines.Node)
	}

	// Default to LTS
	return "20"
}

// normalizeNodeVersion extracts a clean version number.
func normalizeNodeVersion(version string) string {
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "^")
	version = strings.TrimPrefix(version, "~")
	version = strings.TrimPrefix(version, ">=")
	version = strings.TrimPrefix(version, ">")
	version = strings.TrimPrefix(version, "<=")
	version = strings.TrimPrefix(version, "<")

	// Extract just the major.minor or major version
	if matches := nodeVersionRegex.FindStringSubmatch(version); len(matches) > 1 {
		if matches[2] != "" {
			return matches[1] + "." + matches[2]
		}
		return matches[1]
	}

	return version
}

// detectPackageManager determines which package manager to use.
func detectPackageManager(repoPath string, pkg *PackageJSON) string {
	// Priority 1: packageManager field in package.json
	if pkg.PackageManager != "" {
		if strings.HasPrefix(pkg.PackageManager, "yarn") {
			return "yarn"
		}
		if strings.HasPrefix(pkg.PackageManager, "pnpm") {
			return "pnpm"
		}
		if strings.HasPrefix(pkg.PackageManager, "npm") {
			return "npm"
		}
	}

	// Priority 2: Lock file presence
	if _, err := os.Stat(filepath.Join(repoPath, "pnpm-lock.yaml")); err == nil {
		return "pnpm"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "yarn.lock")); err == nil {
		return "yarn"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "package-lock.json")); err == nil {
		return "npm"
	}

	// Default to npm
	return "npm"
}

// detectNodeFramework identifies the framework used in the project.
func detectNodeFramework(pkg *PackageJSON) (models.Framework, string) {
	allDeps := make(map[string]string)
	for k, v := range pkg.Dependencies {
		allDeps[k] = v
	}
	for k, v := range pkg.DevDependencies {
		allDeps[k] = v
	}

	// Check for Next.js
	if version, ok := allDeps["next"]; ok {
		return models.FrameworkNextJS, version
	}

	// Check for Express
	if _, ok := allDeps["express"]; ok {
		return models.FrameworkExpress, ""
	}

	// Check for Fastify
	if _, ok := allDeps["fastify"]; ok {
		return models.FrameworkFastify, ""
	}

	// Check for React (without Next.js)
	if _, ok := allDeps["react"]; ok {
		return models.FrameworkReact, ""
	}

	return models.FrameworkGeneric, ""
}

// extractMajorVersion extracts the major version number from a version string.
func extractMajorVersion(version string) int {
	// Remove any leading characters like ^, ~, >=, etc.
	if matches := semverRegex.FindStringSubmatch(version); len(matches) > 1 {
		var major int
		if _, err := parseVersionNumber(matches[1], &major); err == nil {
			return major
		}
	}
	return 0
}

// parseVersionNumber parses a version number string into an integer.
func parseVersionNumber(s string, result *int) (string, error) {
	for i, c := range s {
		if c < '0' || c > '9' {
			return s[i:], nil
		}
		*result = *result*10 + int(c-'0')
	}
	return "", nil
}
