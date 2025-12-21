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
