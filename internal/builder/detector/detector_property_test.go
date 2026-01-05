package detector

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: platform-enhancements, Property 1: Build Type Determination**
// For any build strategy, the build type SHALL be determined as follows:
// if strategy is "dockerfile" then build_type is "oci", otherwise build_type is "pure-nix".
// **Validates: Requirements 4.1, 4.2, 4.7, 4.8**

// genAllBuildStrategies generates all valid build strategies for testing.
func genAllBuildStrategies() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStrategyFlake,
		models.BuildStrategyAutoGo,
		models.BuildStrategyAutoRust,
		models.BuildStrategyAutoNode,
		models.BuildStrategyAutoPython,
		models.BuildStrategyAutoDatabase,
		models.BuildStrategyDockerfile,
		models.BuildStrategyNixpacks,
		models.BuildStrategyAuto,
	)
}

// genNonDockerfileStrategies generates all build strategies except dockerfile.
func genNonDockerfileStrategies() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStrategyFlake,
		models.BuildStrategyAutoGo,
		models.BuildStrategyAutoRust,
		models.BuildStrategyAutoNode,
		models.BuildStrategyAutoPython,
		models.BuildStrategyAutoDatabase,
		models.BuildStrategyNixpacks,
		models.BuildStrategyAuto,
	)
}

// genLanguageSelection generates valid language selections for service creation.
func genLanguageSelection() gopter.Gen {
	return gen.OneConstOf(
		"go", "Go",
		"rust", "Rust",
		"python", "Python",
		"node", "nodejs", "Node.js", "node.js",
		"dockerfile", "Dockerfile",
	)
}

func TestBuildTypeDetermination(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 1a: Dockerfile strategy always returns OCI
	properties.Property("dockerfile strategy always returns OCI build type", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			// Only test dockerfile strategy
			if strategy != models.BuildStrategyDockerfile {
				return true // Skip non-dockerfile strategies in this property
			}
			result := DetermineBuildType(strategy)
			return result == models.BuildTypeOCI
		},
		genAllBuildStrategies(),
	))

	// Property 1b: All non-dockerfile strategies return pure-nix
	properties.Property("non-dockerfile strategies always return pure-nix build type", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			result := DetermineBuildType(strategy)
			return result == models.BuildTypePureNix
		},
		genNonDockerfileStrategies(),
	))

	// Property 1c: Build type determination is deterministic
	properties.Property("build type determination is deterministic", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			result1 := DetermineBuildType(strategy)
			result2 := DetermineBuildType(strategy)
			result3 := DetermineBuildType(strategy)
			return result1 == result2 && result2 == result3
		},
		genAllBuildStrategies(),
	))

	// Property 1d: IsOCIOnlyStrategy is consistent with DetermineBuildType
	properties.Property("IsOCIOnlyStrategy is consistent with DetermineBuildType", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			buildType := DetermineBuildType(strategy)
			isOCIOnly := IsOCIOnlyStrategy(strategy)

			// If IsOCIOnlyStrategy returns true, build type must be OCI
			if isOCIOnly {
				return buildType == models.BuildTypeOCI
			}
			// If IsOCIOnlyStrategy returns false, build type must be pure-nix
			return buildType == models.BuildTypePureNix
		},
		genAllBuildStrategies(),
	))

	properties.TestingRun(t)
}

func TestBuildTypeFromLanguageSelection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Language selection determines correct strategy and build type
	properties.Property("language selection determines correct strategy and build type", prop.ForAll(
		func(language string) bool {
			strategy, buildType := DetermineBuildTypeFromLanguage(language)

			// Dockerfile language should result in dockerfile strategy and OCI build type
			if language == "dockerfile" || language == "Dockerfile" {
				return strategy == models.BuildStrategyDockerfile && buildType == models.BuildTypeOCI
			}

			// All other languages should result in pure-nix build type
			return buildType == models.BuildTypePureNix
		},
		genLanguageSelection(),
	))

	// Property: Go language selection returns auto-go strategy
	properties.Property("Go language returns auto-go strategy with pure-nix", prop.ForAll(
		func(language string) bool {
			strategy, buildType := DetermineBuildTypeFromLanguage(language)
			return strategy == models.BuildStrategyAutoGo && buildType == models.BuildTypePureNix
		},
		gen.OneConstOf("go", "Go"),
	))

	// Property: Rust language selection returns auto-rust strategy
	properties.Property("Rust language returns auto-rust strategy with pure-nix", prop.ForAll(
		func(language string) bool {
			strategy, buildType := DetermineBuildTypeFromLanguage(language)
			return strategy == models.BuildStrategyAutoRust && buildType == models.BuildTypePureNix
		},
		gen.OneConstOf("rust", "Rust"),
	))

	// Property: Python language selection returns auto-python strategy
	properties.Property("Python language returns auto-python strategy with pure-nix", prop.ForAll(
		func(language string) bool {
			strategy, buildType := DetermineBuildTypeFromLanguage(language)
			return strategy == models.BuildStrategyAutoPython && buildType == models.BuildTypePureNix
		},
		gen.OneConstOf("python", "Python"),
	))

	// Property: Node.js language selection returns auto-node strategy
	properties.Property("Node.js language returns auto-node strategy with pure-nix", prop.ForAll(
		func(language string) bool {
			strategy, buildType := DetermineBuildTypeFromLanguage(language)
			return strategy == models.BuildStrategyAutoNode && buildType == models.BuildTypePureNix
		},
		gen.OneConstOf("node", "nodejs", "Node.js", "node.js"),
	))

	properties.TestingRun(t)
}

// **Feature: flexible-build-strategies, Property 1: Strategy Detection Consistency**
// For any repository with a go.mod file, the Build_Detector SHALL always identify it
// as a Go application with strategy auto-go when using auto-detection.
// **Validates: Requirements 1.2, 3.1**

// genValidGoVersion generates valid Go version strings.
func genValidGoVersion() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 1),   // Major version (always 1 for Go)
		gen.IntRange(18, 23), // Minor version (18-23 are common)
	).Map(func(vals []interface{}) string {
		major := vals[0].(int)
		minor := vals[1].(int)
		return intToStr(major) + "." + intToStr(minor)
	})
}

// genValidGoModuleName generates valid Go module names.
func genValidGoModuleName() gopter.Gen {
	return gen.OneConstOf(
		"github.com/example/myapp",
		"github.com/example/service",
		"github.com/example/api",
		"gitlab.com/org/project",
		"gitlab.com/org/backend",
		"example.com/pkg/lib",
	)
}

// GoModContent represents the content of a go.mod file.
type GoModContent struct {
	ModuleName string
	GoVersion  string
}

// genGoModContent generates valid go.mod content.
func genGoModContent() gopter.Gen {
	return gopter.CombineGens(
		genValidGoModuleName(),
		genValidGoVersion(),
	).Map(func(vals []interface{}) GoModContent {
		return GoModContent{
			ModuleName: vals[0].(string),
			GoVersion:  vals[1].(string),
		}
	})
}

func TestGoDetectionConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("repositories with go.mod are detected as Go projects", prop.ForAll(
		func(content GoModContent) bool {
			// Create a temporary directory with go.mod
			dir := t.TempDir()

			goModContent := "module " + content.ModuleName + "\n\ngo " + content.GoVersion + "\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644); err != nil {
				return false
			}

			// Create a simple main.go
			mainContent := `package main

func main() {
	println("hello")
}
`
			if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainContent), 0644); err != nil {
				return false
			}

			// Run detection
			detector := NewDetector()
			result, err := detector.Detect(context.Background(), dir)
			if err != nil {
				return false
			}

			// Verify the strategy is auto-go
			return result.Strategy == models.BuildStrategyAutoGo
		},
		genGoModContent(),
	))

	properties.TestingRun(t)
}

// **Feature: flexible-build-strategies, Property 6: Go Version Extraction Accuracy**
// For any Go repository with a go.mod file containing a go directive, the Build_Detector
// SHALL extract the exact Go version specified.
// **Validates: Requirements 3.2**

func TestGoVersionExtractionAccuracy(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Go version is extracted accurately from go.mod", prop.ForAll(
		func(content GoModContent) bool {
			// Create a temporary directory with go.mod
			dir := t.TempDir()

			goModContent := "module " + content.ModuleName + "\n\ngo " + content.GoVersion + "\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644); err != nil {
				return false
			}

			// Run detection
			detector := NewDetector()
			result, err := detector.DetectGo(context.Background(), dir)
			if err != nil || result == nil {
				return false
			}

			// Verify the version matches
			return result.Version == content.GoVersion
		},
		genGoModContent(),
	))

	properties.TestingRun(t)
}

// **Feature: flexible-build-strategies, Property 8: Multi-Entry Point Detection**
// For any Go repository with multiple main packages in cmd/*, the Build_Detector
// SHALL list all entry points in the detection result.
// **Validates: Requirements 3.8**

// genEntryPointNames generates a list of valid entry point names.
func genEntryPointNames() gopter.Gen {
	return gen.OneConstOf(
		[]string{"api", "worker"},
		[]string{"api", "worker", "cli"},
		[]string{"server", "client"},
		[]string{"web", "api", "worker"},
		[]string{"frontend", "backend"},
	)
}

func TestMultiEntryPointDetection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("multiple entry points in cmd/* are detected", prop.ForAll(
		func(entryPoints []string) bool {
			if len(entryPoints) < 2 {
				return true // Skip if not enough entry points
			}

			// Create a temporary directory
			dir := t.TempDir()

			// Create go.mod
			goModContent := "module github.com/example/multibin\n\ngo 1.21\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644); err != nil {
				return false
			}

			// Create cmd directory
			cmdDir := filepath.Join(dir, "cmd")
			if err := os.MkdirAll(cmdDir, 0755); err != nil {
				return false
			}

			// Create entry points
			for _, ep := range entryPoints {
				epDir := filepath.Join(cmdDir, ep)
				if err := os.MkdirAll(epDir, 0755); err != nil {
					return false
				}

				mainContent := `package main

func main() {
	println("` + ep + `")
}
`
				if err := os.WriteFile(filepath.Join(epDir, "main.go"), []byte(mainContent), 0644); err != nil {
					return false
				}
			}

			// Run detection
			detector := NewDetector()
			result, err := detector.DetectGo(context.Background(), dir)
			if err != nil || result == nil {
				return false
			}

			// Verify all entry points are detected
			if len(result.EntryPoints) != len(entryPoints) {
				return false
			}

			// Verify each entry point is in the result
			detectedSet := make(map[string]bool)
			for _, ep := range result.EntryPoints {
				detectedSet[ep] = true
			}

			for _, ep := range entryPoints {
				expectedPath := filepath.Join("cmd", ep)
				if !detectedSet[expectedPath] {
					return false
				}
			}

			return true
		},
		genEntryPointNames(),
	))

	properties.TestingRun(t)
}

// **Feature: flexible-build-strategies, Property 7: Node.js Framework Detection**
// For any Node.js repository with next in package.json dependencies, the Build_Detector
// SHALL identify the framework as nextjs.
// **Validates: Requirements 4.2, 4.6**

// genNextJSVersion generates valid Next.js version strings.
func genNextJSVersion() gopter.Gen {
	return gen.Weighted([]gen.WeightedGen{
		// Next.js 13+ (app router)
		{Weight: 5, Gen: gen.IntRange(13, 15).Map(func(v int) string {
			return intToStr(v) + ".0.0"
		})},
		// Next.js 12 and below (pages router)
		{Weight: 3, Gen: gen.IntRange(10, 12).Map(func(v int) string {
			return intToStr(v) + ".0.0"
		})},
	})
}

// NodePackageJSON represents a package.json structure for testing.
type NodePackageJSON struct {
	Name        string
	NextVersion string
}

// genNodePackageJSON generates valid package.json content with Next.js.
func genNodePackageJSON() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("myapp", "webapp", "frontend", "nextapp", "project"),
		genNextJSVersion(),
	).Map(func(vals []interface{}) NodePackageJSON {
		return NodePackageJSON{
			Name:        vals[0].(string),
			NextVersion: vals[1].(string),
		}
	})
}

func TestNodeJSFrameworkDetection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Next.js is detected when next is in dependencies", prop.ForAll(
		func(pkg NodePackageJSON) bool {
			// Create a temporary directory
			dir := t.TempDir()

			// Create package.json with Next.js dependency
			packageJSON := `{
  "name": "` + pkg.Name + `",
  "version": "1.0.0",
  "dependencies": {
    "next": "` + pkg.NextVersion + `",
    "react": "18.0.0",
    "react-dom": "18.0.0"
  },
  "scripts": {
    "dev": "next dev",
    "build": "next build",
    "start": "next start"
  }
}
`
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(packageJSON), 0644); err != nil {
				return false
			}

			// Run detection
			detector := NewDetector()
			result, err := detector.DetectNode(context.Background(), dir)
			if err != nil || result == nil {
				return false
			}

			// Verify the framework is Next.js
			return result.Framework == models.FrameworkNextJS
		},
		genNodePackageJSON(),
	))

	properties.TestingRun(t)
}

// Helper function to convert int to string.
func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	result := ""
	negative := i < 0
	if negative {
		i = -i
	}
	for i > 0 {
		result = string(rune('0'+i%10)) + result
		i /= 10
	}
	if negative {
		result = "-" + result
	}
	return result
}

// **Feature: ui-api-alignment, Property 6: CGO Detection Accuracy**
// For any Go project that imports packages requiring CGO (e.g., import "C", go-sqlite3),
// the builder SHALL detect this and select the CGO-enabled template.
// **Validates: Requirements 16.1, 16.2**

// CGOTestCase represents a test case for CGO detection.
type CGOTestCase struct {
	// HasCImport indicates if the project has `import "C"`
	HasCImport bool
	// CGOPackage is a known CGO-requiring package (empty if none)
	CGOPackage string
	// HasCGODirective indicates if the project has CGO directives
	HasCGODirective bool
	// ModuleName is the Go module name
	ModuleName string
	// GoVersion is the Go version
	GoVersion string
}

// genCGOPackage generates a known CGO-requiring package or empty string.
func genCGOPackage() gopter.Gen {
	return gen.OneConstOf(
		"",                              // No CGO package
		"github.com/mattn/go-sqlite3",   // SQLite driver
		"github.com/shirou/gopsutil",    // System info
		"github.com/boltdb/bolt",        // BoltDB
	)
}

// genCGOTestCase generates test cases for CGO detection.
func genCGOTestCase() gopter.Gen {
	return gopter.CombineGens(
		gen.Bool(),                                                    // HasCImport
		genCGOPackage(),                                               // CGOPackage
		gen.Bool(),                                                    // HasCGODirective
		gen.OneConstOf("github.com/example/app", "example.com/myapp"), // ModuleName
		gen.OneConstOf("1.21", "1.22", "1.23"),                        // GoVersion
	).Map(func(vals []interface{}) CGOTestCase {
		return CGOTestCase{
			HasCImport:      vals[0].(bool),
			CGOPackage:      vals[1].(string),
			HasCGODirective: vals[2].(bool),
			ModuleName:      vals[3].(string),
			GoVersion:       vals[4].(string),
		}
	})
}

// genCGORequiringTestCase generates test cases that definitely require CGO.
func genCGORequiringTestCase() gopter.Gen {
	return gen.Weighted([]gen.WeightedGen{
		// Case 1: Has import "C"
		{Weight: 3, Gen: gopter.CombineGens(
			gen.Const(true),                                               // HasCImport
			genCGOPackage(),                                               // CGOPackage (any)
			gen.Bool(),                                                    // HasCGODirective (any)
			gen.OneConstOf("github.com/example/app", "example.com/myapp"), // ModuleName
			gen.OneConstOf("1.21", "1.22"),                                // GoVersion
		).Map(func(vals []interface{}) CGOTestCase {
			return CGOTestCase{
				HasCImport:      vals[0].(bool),
				CGOPackage:      vals[1].(string),
				HasCGODirective: vals[2].(bool),
				ModuleName:      vals[3].(string),
				GoVersion:       vals[4].(string),
			}
		})},
		// Case 2: Has known CGO package
		{Weight: 3, Gen: gopter.CombineGens(
			gen.Bool(),                                                    // HasCImport (any)
			gen.OneConstOf("github.com/mattn/go-sqlite3", "github.com/shirou/gopsutil", "github.com/boltdb/bolt"), // CGOPackage (must be non-empty)
			gen.Bool(),                                                    // HasCGODirective (any)
			gen.OneConstOf("github.com/example/app", "example.com/myapp"), // ModuleName
			gen.OneConstOf("1.21", "1.22"),                                // GoVersion
		).Map(func(vals []interface{}) CGOTestCase {
			return CGOTestCase{
				HasCImport:      vals[0].(bool),
				CGOPackage:      vals[1].(string),
				HasCGODirective: vals[2].(bool),
				ModuleName:      vals[3].(string),
				GoVersion:       vals[4].(string),
			}
		})},
		// Case 3: Has CGO directive
		{Weight: 2, Gen: gopter.CombineGens(
			gen.Bool(),                                                    // HasCImport (any)
			genCGOPackage(),                                               // CGOPackage (any)
			gen.Const(true),                                               // HasCGODirective
			gen.OneConstOf("github.com/example/app", "example.com/myapp"), // ModuleName
			gen.OneConstOf("1.21", "1.22"),                                // GoVersion
		).Map(func(vals []interface{}) CGOTestCase {
			return CGOTestCase{
				HasCImport:      vals[0].(bool),
				CGOPackage:      vals[1].(string),
				HasCGODirective: vals[2].(bool),
				ModuleName:      vals[3].(string),
				GoVersion:       vals[4].(string),
			}
		})},
	})
}

// genNonCGOTestCase generates test cases that do not require CGO.
func genNonCGOTestCase() gopter.Gen {
	return gopter.CombineGens(
		gen.Const(false),                                              // HasCImport = false
		gen.Const(""),                                                 // CGOPackage = empty
		gen.Const(false),                                              // HasCGODirective = false
		gen.OneConstOf("github.com/example/app", "example.com/myapp"), // ModuleName
		gen.OneConstOf("1.21", "1.22"),                                // GoVersion
	).Map(func(vals []interface{}) CGOTestCase {
		return CGOTestCase{
			HasCImport:      vals[0].(bool),
			CGOPackage:      vals[1].(string),
			HasCGODirective: vals[2].(bool),
			ModuleName:      vals[3].(string),
			GoVersion:       vals[4].(string),
		}
	})
}

// createCGOTestRepo creates a temporary Go repository for CGO detection testing.
func createCGOTestRepo(t *testing.T, tc CGOTestCase) string {
	dir := t.TempDir()

	// Create go.mod
	goModContent := "module " + tc.ModuleName + "\n\ngo " + tc.GoVersion + "\n"
	if tc.CGOPackage != "" {
		goModContent += "\nrequire (\n\t" + tc.CGOPackage + " v1.0.0\n)\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	// Create main.go
	var mainContent string
	if tc.HasCImport {
		mainContent = `package main

/*
#include <stdlib.h>
*/
import "C"

func main() {
	println("hello with CGO")
}
`
	} else if tc.HasCGODirective {
		mainContent = `package main

// #cgo LDFLAGS: -lm
// #include <math.h>
import "C"

func main() {
	println("hello with CGO directive")
}
`
	} else {
		mainContent = `package main

func main() {
	println("hello")
}
`
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	return dir
}

// TestCGODetectionAccuracy tests that CGO is detected accurately.
func TestCGODetectionAccuracy(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 6a: Projects with import "C" are detected as requiring CGO
	properties.Property("projects with import C are detected as requiring CGO", prop.ForAll(
		func(tc CGOTestCase) bool {
			dir := createCGOTestRepo(t, tc)

			result, err := DetectCGO(dir)
			if err != nil {
				return false
			}

			// If HasCImport is true, RequiresCGO must be true
			return result.RequiresCGO && result.HasCImport
		},
		genCGORequiringTestCase().SuchThat(func(tc CGOTestCase) bool {
			return tc.HasCImport
		}),
	))

	// Property 6b: Projects with known CGO packages are detected as requiring CGO
	properties.Property("projects with known CGO packages are detected as requiring CGO", prop.ForAll(
		func(tc CGOTestCase) bool {
			dir := createCGOTestRepo(t, tc)

			result, err := DetectCGO(dir)
			if err != nil {
				return false
			}

			// If CGOPackage is non-empty, RequiresCGO must be true
			return result.RequiresCGO && len(result.DetectedPackages) > 0
		},
		genCGORequiringTestCase().SuchThat(func(tc CGOTestCase) bool {
			return tc.CGOPackage != ""
		}),
	))

	// Property 6c: Projects without CGO indicators are not detected as requiring CGO
	properties.Property("projects without CGO indicators are not detected as requiring CGO", prop.ForAll(
		func(tc CGOTestCase) bool {
			dir := createCGOTestRepo(t, tc)

			result, err := DetectCGO(dir)
			if err != nil {
				return false
			}

			// If no CGO indicators, RequiresCGO must be false
			return !result.RequiresCGO &&
				!result.HasCImport &&
				!result.HasCGODirectives &&
				len(result.DetectedPackages) == 0
		},
		genNonCGOTestCase(),
	))

	// Property 6d: CGO detection is deterministic
	properties.Property("CGO detection is deterministic", prop.ForAll(
		func(tc CGOTestCase) bool {
			dir := createCGOTestRepo(t, tc)

			result1, err1 := DetectCGO(dir)
			result2, err2 := DetectCGO(dir)
			result3, err3 := DetectCGO(dir)

			if err1 != nil || err2 != nil || err3 != nil {
				return false
			}

			// All results should be identical
			return result1.RequiresCGO == result2.RequiresCGO &&
				result2.RequiresCGO == result3.RequiresCGO &&
				result1.HasCImport == result2.HasCImport &&
				result2.HasCImport == result3.HasCImport
		},
		genCGOTestCase(),
	))

	properties.TestingRun(t)
}

// TestIsCGOPackageAccuracy tests that IsCGOPackage correctly identifies CGO packages.
func TestIsCGOPackageAccuracy(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Known CGO packages are identified correctly
	properties.Property("known CGO packages are identified correctly", prop.ForAll(
		func(pkg string) bool {
			return IsCGOPackage(pkg)
		},
		gen.OneConstOf(
			"github.com/mattn/go-sqlite3",
			"github.com/mattn/go-sqlite3/v2",
			"github.com/shirou/gopsutil",
			"github.com/shirou/gopsutil/v3",
			"github.com/boltdb/bolt",
		),
	))

	// Property: Non-CGO packages are not identified as CGO packages
	properties.Property("non-CGO packages are not identified as CGO packages", prop.ForAll(
		func(pkg string) bool {
			return !IsCGOPackage(pkg)
		},
		gen.OneConstOf(
			"github.com/gin-gonic/gin",
			"github.com/gorilla/mux",
			"github.com/stretchr/testify",
			"golang.org/x/sync",
			"github.com/spf13/cobra",
		),
	))

	properties.TestingRun(t)
}

// **Feature: ui-api-alignment, Property 17: Go Workspace Detection**
// For any repository containing a go.work file, the builder SHALL detect it as a workspace
// and configure the build environment accordingly.
// **Validates: Requirements 22.1, 22.4**

// GoWorkspaceTestCase represents a test case for Go workspace detection.
type GoWorkspaceTestCase struct {
	// HasGoWork indicates if the project has a go.work file
	HasGoWork bool
	// GoVersion is the Go version in go.work (if any)
	GoVersion string
	// Modules lists the module paths in the workspace
	Modules []string
	// ModuleName is the main module name (for go.mod)
	ModuleName string
}

// genWorkspaceModules generates a list of workspace module paths.
func genWorkspaceModules() gopter.Gen {
	return gen.OneConstOf(
		[]string{"./api", "./worker"},
		[]string{"./cmd/server", "./cmd/client"},
		[]string{"./services/auth", "./services/api", "./services/worker"},
		[]string{"./pkg/core", "./apps/web"},
		[]string{"."},
	)
}

// genGoWorkspaceTestCase generates test cases for Go workspace detection.
func genGoWorkspaceTestCase() gopter.Gen {
	return gopter.CombineGens(
		gen.Bool(),                                                    // HasGoWork
		gen.OneConstOf("1.21", "1.22", "1.23"),                        // GoVersion
		genWorkspaceModules(),                                         // Modules
		gen.OneConstOf("github.com/example/app", "example.com/myapp"), // ModuleName
	).Map(func(vals []interface{}) GoWorkspaceTestCase {
		return GoWorkspaceTestCase{
			HasGoWork:  vals[0].(bool),
			GoVersion:  vals[1].(string),
			Modules:    vals[2].([]string),
			ModuleName: vals[3].(string),
		}
	})
}

// genWorkspaceWithGoWork generates test cases that have a go.work file.
func genWorkspaceWithGoWork() gopter.Gen {
	return gopter.CombineGens(
		gen.Const(true),                                               // HasGoWork = true
		gen.OneConstOf("1.21", "1.22", "1.23"),                        // GoVersion
		genWorkspaceModules(),                                         // Modules
		gen.OneConstOf("github.com/example/app", "example.com/myapp"), // ModuleName
	).Map(func(vals []interface{}) GoWorkspaceTestCase {
		return GoWorkspaceTestCase{
			HasGoWork:  vals[0].(bool),
			GoVersion:  vals[1].(string),
			Modules:    vals[2].([]string),
			ModuleName: vals[3].(string),
		}
	})
}

// genWorkspaceWithoutGoWork generates test cases that do not have a go.work file.
func genWorkspaceWithoutGoWork() gopter.Gen {
	return gopter.CombineGens(
		gen.Const(false),                                              // HasGoWork = false
		gen.OneConstOf("1.21", "1.22", "1.23"),                        // GoVersion
		genWorkspaceModules(),                                         // Modules (not used)
		gen.OneConstOf("github.com/example/app", "example.com/myapp"), // ModuleName
	).Map(func(vals []interface{}) GoWorkspaceTestCase {
		return GoWorkspaceTestCase{
			HasGoWork:  vals[0].(bool),
			GoVersion:  vals[1].(string),
			Modules:    vals[2].([]string),
			ModuleName: vals[3].(string),
		}
	})
}

// createGoWorkspaceTestRepo creates a temporary Go repository for workspace detection testing.
func createGoWorkspaceTestRepo(t *testing.T, tc GoWorkspaceTestCase) string {
	dir := t.TempDir()

	// Create go.mod in root
	goModContent := "module " + tc.ModuleName + "\n\ngo " + tc.GoVersion + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	// Create main.go in root
	mainContent := `package main

func main() {
	println("hello")
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	// Create go.work if needed
	if tc.HasGoWork {
		var goWorkContent string
		goWorkContent = "go " + tc.GoVersion + "\n\nuse (\n"
		for _, mod := range tc.Modules {
			goWorkContent += "\t" + mod + "\n"
		}
		goWorkContent += ")\n"

		if err := os.WriteFile(filepath.Join(dir, "go.work"), []byte(goWorkContent), 0644); err != nil {
			t.Fatalf("failed to create go.work: %v", err)
		}

		// Create module directories with go.mod files
		for _, mod := range tc.Modules {
			if mod == "." {
				continue // Root module already created
			}
			modDir := filepath.Join(dir, mod)
			if err := os.MkdirAll(modDir, 0755); err != nil {
				t.Fatalf("failed to create module directory %s: %v", mod, err)
			}

			// Create go.mod for sub-module
			subModContent := "module " + tc.ModuleName + "/" + filepath.Base(mod) + "\n\ngo " + tc.GoVersion + "\n"
			if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte(subModContent), 0644); err != nil {
				t.Fatalf("failed to create go.mod in %s: %v", mod, err)
			}

			// Create a simple main.go
			subMainContent := `package main

func main() {
	println("` + filepath.Base(mod) + `")
}
`
			if err := os.WriteFile(filepath.Join(modDir, "main.go"), []byte(subMainContent), 0644); err != nil {
				t.Fatalf("failed to create main.go in %s: %v", mod, err)
			}
		}
	}

	return dir
}

// TestGoWorkspaceDetection tests that Go workspaces are detected correctly.
// **Feature: ui-api-alignment, Property 17: Go Workspace Detection**
// **Validates: Requirements 22.1, 22.4**
func TestGoWorkspaceDetection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 17a: Repositories with go.work are detected as workspaces
	properties.Property("repositories with go.work are detected as workspaces", prop.ForAll(
		func(tc GoWorkspaceTestCase) bool {
			dir := createGoWorkspaceTestRepo(t, tc)

			result, err := DetectGoWorkspace(dir)
			if err != nil {
				return false
			}

			// If HasGoWork is true, IsWorkspace must be true
			return result.IsWorkspace
		},
		genWorkspaceWithGoWork(),
	))

	// Property 17b: Repositories without go.work are not detected as workspaces
	properties.Property("repositories without go.work are not detected as workspaces", prop.ForAll(
		func(tc GoWorkspaceTestCase) bool {
			dir := createGoWorkspaceTestRepo(t, tc)

			result, err := DetectGoWorkspace(dir)
			if err != nil {
				return false
			}

			// If HasGoWork is false, IsWorkspace must be false
			return !result.IsWorkspace
		},
		genWorkspaceWithoutGoWork(),
	))

	// Property 17c: Go version is extracted from go.work
	properties.Property("Go version is extracted from go.work", prop.ForAll(
		func(tc GoWorkspaceTestCase) bool {
			dir := createGoWorkspaceTestRepo(t, tc)

			result, err := DetectGoWorkspace(dir)
			if err != nil {
				return false
			}

			// Go version should match what was written to go.work
			return result.GoVersion == tc.GoVersion
		},
		genWorkspaceWithGoWork(),
	))

	// Property 17d: Modules are extracted from go.work use directives
	properties.Property("modules are extracted from go.work use directives", prop.ForAll(
		func(tc GoWorkspaceTestCase) bool {
			dir := createGoWorkspaceTestRepo(t, tc)

			result, err := DetectGoWorkspace(dir)
			if err != nil {
				return false
			}

			// Number of modules should match
			if len(result.Modules) != len(tc.Modules) {
				return false
			}

			// Create a set of expected modules (normalized)
			expectedSet := make(map[string]bool)
			for _, mod := range tc.Modules {
				expectedSet[filepath.Clean(mod)] = true
			}

			// Check all detected modules are expected
			for _, mod := range result.Modules {
				if !expectedSet[mod] {
					return false
				}
			}

			return true
		},
		genWorkspaceWithGoWork(),
	))

	// Property 17e: Workspace detection is deterministic
	properties.Property("workspace detection is deterministic", prop.ForAll(
		func(tc GoWorkspaceTestCase) bool {
			dir := createGoWorkspaceTestRepo(t, tc)

			result1, err1 := DetectGoWorkspace(dir)
			result2, err2 := DetectGoWorkspace(dir)
			result3, err3 := DetectGoWorkspace(dir)

			if err1 != nil || err2 != nil || err3 != nil {
				return false
			}

			// All results should be identical
			return result1.IsWorkspace == result2.IsWorkspace &&
				result2.IsWorkspace == result3.IsWorkspace &&
				result1.GoVersion == result2.GoVersion &&
				result2.GoVersion == result3.GoVersion &&
				len(result1.Modules) == len(result2.Modules) &&
				len(result2.Modules) == len(result3.Modules)
		},
		genGoWorkspaceTestCase(),
	))

	// Property 17f: HasGoWorkspace convenience function is consistent with DetectGoWorkspace
	properties.Property("HasGoWorkspace is consistent with DetectGoWorkspace", prop.ForAll(
		func(tc GoWorkspaceTestCase) bool {
			dir := createGoWorkspaceTestRepo(t, tc)

			hasWorkspace := HasGoWorkspace(dir)
			result, err := DetectGoWorkspace(dir)
			if err != nil {
				return false
			}

			// HasGoWorkspace should return the same as result.IsWorkspace
			return hasWorkspace == result.IsWorkspace
		},
		genGoWorkspaceTestCase(),
	))

	properties.TestingRun(t)
}

// TestGoWorkspaceInDetectGo tests that workspace detection is integrated into DetectGo.
// **Feature: ui-api-alignment, Property 17: Go Workspace Detection**
// **Validates: Requirements 22.1, 22.4**
func TestGoWorkspaceInDetectGo(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: DetectGo includes workspace information in SuggestedConfig
	properties.Property("DetectGo includes workspace information in SuggestedConfig", prop.ForAll(
		func(tc GoWorkspaceTestCase) bool {
			dir := createGoWorkspaceTestRepo(t, tc)

			detector := NewDetector()
			result, err := detector.DetectGo(context.Background(), dir)
			if err != nil || result == nil {
				return false
			}

			// Check if is_workspace is set correctly in SuggestedConfig
			isWorkspace, ok := result.SuggestedConfig["is_workspace"]
			if tc.HasGoWork {
				// Should have is_workspace = true
				if !ok {
					return false
				}
				if isWorkspaceBool, isBool := isWorkspace.(bool); !isBool || !isWorkspaceBool {
					return false
				}
			} else {
				// Should not have is_workspace or it should be false
				if ok {
					if isWorkspaceBool, isBool := isWorkspace.(bool); isBool && isWorkspaceBool {
						return false
					}
				}
			}

			return true
		},
		genGoWorkspaceTestCase(),
	))

	properties.TestingRun(t)
}
