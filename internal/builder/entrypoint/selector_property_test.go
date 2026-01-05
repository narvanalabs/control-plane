package entrypoint

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

// **Feature: flexible-build-strategies, Property 19: Entry Point Validation**
// For any user-specified entry_point, the EntryPointSelector SHALL validate it exists
// in the repository before build.
// **Validates: Requirements 3.10**

// genValidEntryPointName generates valid entry point names.
func genValidEntryPointName() gopter.Gen {
	return gen.OneConstOf(
		"api",
		"server",
		"worker",
		"cli",
		"main",
		"app",
		"service",
	)
}

// genInvalidEntryPointName generates entry point names that won't exist.
func genInvalidEntryPointName() gopter.Gen {
	return gen.OneConstOf(
		"nonexistent",
		"missing",
		"invalid",
		"notfound",
		"fake",
		"bogus",
	)
}

// TestEntryPointValidation tests Property 19: Entry Point Validation.
// For any user-specified entry_point, the EntryPointSelector SHALL validate it exists
// in the repository before build.
func TestEntryPointValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Valid entry points pass validation
	properties.Property("valid entry points pass validation", prop.ForAll(
		func(entryPointName string) bool {
			// Create a temporary directory with a valid Go entry point
			dir := t.TempDir()

			// Create cmd directory with the entry point
			cmdDir := filepath.Join(dir, "cmd", entryPointName)
			if err := os.MkdirAll(cmdDir, 0755); err != nil {
				return false
			}

			// Create a main.go file in the entry point directory
			mainContent := `package main

func main() {
	println("hello")
}
`
			if err := os.WriteFile(filepath.Join(cmdDir, "main.go"), []byte(mainContent), 0644); err != nil {
				return false
			}

			// Validate the entry point
			selector := NewSelector()
			err := selector.Validate(context.Background(), dir, filepath.Join("cmd", entryPointName))

			// Should pass validation
			return err == nil
		},
		genValidEntryPointName(),
	))

	// Property: Invalid entry points fail validation
	properties.Property("invalid entry points fail validation", prop.ForAll(
		func(entryPointName string) bool {
			// Create a temporary directory without the entry point
			dir := t.TempDir()

			// Create go.mod to make it look like a valid repo
			goModContent := "module github.com/example/test\n\ngo 1.21\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644); err != nil {
				return false
			}

			// Validate a non-existent entry point
			selector := NewSelector()
			err := selector.Validate(context.Background(), dir, filepath.Join("cmd", entryPointName))

			// Should fail validation with ErrEntryPointNotFound
			return err != nil
		},
		genInvalidEntryPointName(),
	))

	properties.TestingRun(t)
}


// TestEntryPointValidationEmptyInputs tests edge cases for validation.
func TestEntryPointValidationEmptyInputs(t *testing.T) {
	selector := NewSelector()
	ctx := context.Background()

	// Empty repo path should fail
	err := selector.Validate(ctx, "", "cmd/api")
	if err != ErrEmptyRepoPath {
		t.Errorf("expected ErrEmptyRepoPath, got %v", err)
	}

	// Empty entry point should fail
	dir := t.TempDir()
	err = selector.Validate(ctx, dir, "")
	if err != ErrEmptyEntryPoint {
		t.Errorf("expected ErrEmptyEntryPoint, got %v", err)
	}

	// Non-existent repo should fail
	err = selector.Validate(ctx, "/nonexistent/path/to/repo", "cmd/api")
	if err != ErrRepoNotFound {
		t.Errorf("expected ErrRepoNotFound, got %v", err)
	}
}

// TestSelectDefaultHeuristics tests the default selection heuristics.
func TestSelectDefaultHeuristics(t *testing.T) {
	selector := NewSelector()

	tests := []struct {
		name        string
		entryPoints []EntryPoint
		expected    string
	}{
		{
			name:        "empty list returns nil",
			entryPoints: []EntryPoint{},
			expected:    "",
		},
		{
			name: "already marked default is selected",
			entryPoints: []EntryPoint{
				{Path: "cmd/worker", Name: "worker", IsDefault: false},
				{Path: "cmd/api", Name: "api", IsDefault: true},
			},
			expected: "cmd/api",
		},
		{
			name: "main is preferred",
			entryPoints: []EntryPoint{
				{Path: "cmd/worker", Name: "worker", IsDefault: false},
				{Path: "cmd/main", Name: "main", IsDefault: false},
			},
			expected: "cmd/main",
		},
		{
			name: "app is preferred over worker",
			entryPoints: []EntryPoint{
				{Path: "cmd/worker", Name: "worker", IsDefault: false},
				{Path: "cmd/app", Name: "app", IsDefault: false},
			},
			expected: "cmd/app",
		},
		{
			name: "server is preferred over worker",
			entryPoints: []EntryPoint{
				{Path: "cmd/worker", Name: "worker", IsDefault: false},
				{Path: "cmd/server", Name: "server", IsDefault: false},
			},
			expected: "cmd/server",
		},
		{
			name: "api is preferred over worker",
			entryPoints: []EntryPoint{
				{Path: "cmd/worker", Name: "worker", IsDefault: false},
				{Path: "cmd/api", Name: "api", IsDefault: false},
			},
			expected: "cmd/api",
		},
		{
			name: "root entry point is preferred",
			entryPoints: []EntryPoint{
				{Path: "cmd/worker", Name: "worker", IsDefault: false},
				{Path: ".", Name: "main", IsDefault: false},
			},
			expected: ".",
		},
		{
			name: "first entry point is fallback",
			entryPoints: []EntryPoint{
				{Path: "cmd/alpha", Name: "alpha", IsDefault: false},
				{Path: "cmd/beta", Name: "beta", IsDefault: false},
			},
			expected: "cmd/alpha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := selector.SelectDefault(tt.entryPoints)
			if tt.expected == "" {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				if result == nil {
					t.Errorf("expected %s, got nil", tt.expected)
				} else if result.Path != tt.expected {
					t.Errorf("expected %s, got %s", tt.expected, result.Path)
				}
			}
		})
	}
}

// TestListGoEntryPoints tests Go entry point detection.
func TestListGoEntryPoints(t *testing.T) {
	selector := NewSelector()
	ctx := context.Background()

	t.Run("detects root main package", func(t *testing.T) {
		dir := t.TempDir()

		// Create go.mod
		goModContent := "module github.com/example/test\n\ngo 1.21\n"
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644); err != nil {
			t.Fatal(err)
		}

		// Create main.go in root
		mainContent := `package main

func main() {
	println("hello")
}
`
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainContent), 0644); err != nil {
			t.Fatal(err)
		}

		entryPoints, err := selector.ListEntryPoints(ctx, dir, "go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(entryPoints) != 1 {
			t.Fatalf("expected 1 entry point, got %d", len(entryPoints))
		}

		if entryPoints[0].Path != "." {
			t.Errorf("expected path '.', got %s", entryPoints[0].Path)
		}

		if !entryPoints[0].IsDefault {
			t.Error("expected root entry point to be default")
		}
	})

	t.Run("detects cmd/* entry points", func(t *testing.T) {
		dir := t.TempDir()

		// Create go.mod
		goModContent := "module github.com/example/test\n\ngo 1.21\n"
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644); err != nil {
			t.Fatal(err)
		}

		// Create cmd/api and cmd/worker
		for _, name := range []string{"api", "worker"} {
			cmdDir := filepath.Join(dir, "cmd", name)
			if err := os.MkdirAll(cmdDir, 0755); err != nil {
				t.Fatal(err)
			}

			mainContent := `package main

func main() {
	println("` + name + `")
}
`
			if err := os.WriteFile(filepath.Join(cmdDir, "main.go"), []byte(mainContent), 0644); err != nil {
				t.Fatal(err)
			}
		}

		entryPoints, err := selector.ListEntryPoints(ctx, dir, "go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(entryPoints) != 2 {
			t.Fatalf("expected 2 entry points, got %d", len(entryPoints))
		}

		// Check that api is marked as default (priority name)
		foundAPI := false
		for _, ep := range entryPoints {
			if ep.Name == "api" {
				foundAPI = true
				if !ep.IsDefault {
					t.Error("expected api to be default")
				}
			}
		}
		if !foundAPI {
			t.Error("expected to find api entry point")
		}
	})
}


// **Feature: flexible-build-strategies, Property 9: Build Config Override**
// For any detection result and user-provided build_config, the user-provided values
// SHALL override detected values in the final configuration.
// **Validates: Requirements 13.3**

// genGoVersionString generates valid Go version strings.
func genGoVersionString() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 1),   // Major version (always 1 for Go)
		gen.IntRange(18, 23), // Minor version (18-23 are common)
	).Map(func(vals []interface{}) string {
		major := vals[0].(int)
		minor := vals[1].(int)
		return intToStr(major) + "." + intToStr(minor)
	})
}

// genNodeVersionString generates valid Node.js version strings.
func genNodeVersionString() gopter.Gen {
	return gen.IntRange(16, 22).Map(func(v int) string {
		return intToStr(v) + ".0.0"
	})
}

// genPythonVersionString generates valid Python version strings.
func genPythonVersionString() gopter.Gen {
	return gen.IntRange(8, 12).Map(func(v int) string {
		return "3." + intToStr(v)
	})
}

// genEntryPointString generates valid entry point strings.
func genEntryPointString() gopter.Gen {
	return gen.OneConstOf(
		"cmd/api",
		"cmd/server",
		"cmd/worker",
		"src/main.py",
		"main.go",
		".",
	)
}

// genBuildCommand generates valid build commands.
func genBuildCommand() gopter.Gen {
	return gen.OneConstOf(
		"go build -o app .",
		"npm run build",
		"cargo build --release",
		"python setup.py build",
		"make build",
	)
}

// genStartCommand generates valid start commands.
func genStartCommand() gopter.Gen {
	return gen.OneConstOf(
		"./app",
		"npm start",
		"./target/release/app",
		"python -m app",
		"./bin/server",
	)
}

// DetectedConfig represents a detected configuration for testing.
type DetectedConfig struct {
	GoVersion      string
	NodeVersion    string
	PythonVersion  string
	EntryPoint     string
	BuildCommand   string
	StartCommand   string
	PackageManager string
	CGOEnabled     bool
	BuildTimeout   int
}

// genDetectedConfig generates a detected configuration.
func genDetectedConfig() gopter.Gen {
	return gopter.CombineGens(
		genGoVersionString(),
		genNodeVersionString(),
		genPythonVersionString(),
		genEntryPointString(),
		genBuildCommand(),
		genStartCommand(),
		gen.OneConstOf("npm", "yarn", "pnpm"),
		gen.Bool(),
		gen.IntRange(300, 3600),
	).Map(func(vals []interface{}) DetectedConfig {
		return DetectedConfig{
			GoVersion:      vals[0].(string),
			NodeVersion:    vals[1].(string),
			PythonVersion:  vals[2].(string),
			EntryPoint:     vals[3].(string),
			BuildCommand:   vals[4].(string),
			StartCommand:   vals[5].(string),
			PackageManager: vals[6].(string),
			CGOEnabled:     vals[7].(bool),
			BuildTimeout:   vals[8].(int),
		}
	})
}

// UserConfig represents a user-provided configuration for testing.
type UserConfig struct {
	GoVersion      string
	NodeVersion    string
	PythonVersion  string
	EntryPoint     string
	BuildCommand   string
	StartCommand   string
	PackageManager string
	CGOEnabled     *bool
	BuildTimeout   int
}

// genUserConfig generates a user configuration (some fields may be empty).
func genUserConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("", "1.21", "1.22", "1.23"),
		gen.OneConstOf("", "18.0.0", "20.0.0", "22.0.0"),
		gen.OneConstOf("", "3.10", "3.11", "3.12"),
		gen.OneConstOf("", "cmd/custom", "src/custom.py"),
		gen.OneConstOf("", "custom build", "make custom"),
		gen.OneConstOf("", "custom start", "./custom"),
		gen.OneConstOf("", "npm", "yarn", "pnpm"),
		gen.PtrOf(gen.Bool()),
		gen.OneConstOf(0, 600, 1200, 1800),
	).Map(func(vals []interface{}) UserConfig {
		var cgoEnabled *bool
		if vals[7] != nil {
			cgoEnabled = vals[7].(*bool)
		}
		return UserConfig{
			GoVersion:      vals[0].(string),
			NodeVersion:    vals[1].(string),
			PythonVersion:  vals[2].(string),
			EntryPoint:     vals[3].(string),
			BuildCommand:   vals[4].(string),
			StartCommand:   vals[5].(string),
			PackageManager: vals[6].(string),
			CGOEnabled:     cgoEnabled,
			BuildTimeout:   vals[8].(int),
		}
	})
}

// toDetectedMap converts DetectedConfig to a map for MergeConfig.
func (d DetectedConfig) toMap() map[string]interface{} {
	return map[string]interface{}{
		"go_version":      d.GoVersion,
		"node_version":    d.NodeVersion,
		"python_version":  d.PythonVersion,
		"entry_point":     d.EntryPoint,
		"build_command":   d.BuildCommand,
		"start_command":   d.StartCommand,
		"package_manager": d.PackageManager,
		"cgo_enabled":     d.CGOEnabled,
		"build_timeout":   d.BuildTimeout,
	}
}

// toBuildConfig converts UserConfig to a models.BuildConfig.
func (u UserConfig) toBuildConfig() *models.BuildConfig {
	return &models.BuildConfig{
		GoVersion:      u.GoVersion,
		NodeVersion:    u.NodeVersion,
		PythonVersion:  u.PythonVersion,
		EntryPoint:     u.EntryPoint,
		BuildCommand:   u.BuildCommand,
		StartCommand:   u.StartCommand,
		PackageManager: u.PackageManager,
		CGOEnabled:     u.CGOEnabled,
		BuildTimeout:   u.BuildTimeout,
	}
}

// TestBuildConfigOverride tests Property 9: Build Config Override.
// For any detection result and user-provided build_config, the user-provided values
// SHALL override detected values in the final configuration.
func TestBuildConfigOverride(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("user-provided values override detected values", prop.ForAll(
		func(detected DetectedConfig, user UserConfig) bool {
			detectedMap := detected.toMap()
			userConfig := user.toBuildConfig()

			result := MergeConfig(detectedMap, userConfig)

			// Check each field: if user provided a value, it should be in result
			// If user didn't provide a value, detected value should be used

			// GoVersion
			if user.GoVersion != "" {
				if result.GoVersion != user.GoVersion {
					return false
				}
			} else {
				if result.GoVersion != detected.GoVersion {
					return false
				}
			}

			// NodeVersion
			if user.NodeVersion != "" {
				if result.NodeVersion != user.NodeVersion {
					return false
				}
			} else {
				if result.NodeVersion != detected.NodeVersion {
					return false
				}
			}

			// PythonVersion
			if user.PythonVersion != "" {
				if result.PythonVersion != user.PythonVersion {
					return false
				}
			} else {
				if result.PythonVersion != detected.PythonVersion {
					return false
				}
			}

			// EntryPoint
			if user.EntryPoint != "" {
				if result.EntryPoint != user.EntryPoint {
					return false
				}
			} else {
				if result.EntryPoint != detected.EntryPoint {
					return false
				}
			}

			// BuildCommand
			if user.BuildCommand != "" {
				if result.BuildCommand != user.BuildCommand {
					return false
				}
			} else {
				if result.BuildCommand != detected.BuildCommand {
					return false
				}
			}

			// StartCommand
			if user.StartCommand != "" {
				if result.StartCommand != user.StartCommand {
					return false
				}
			} else {
				if result.StartCommand != detected.StartCommand {
					return false
				}
			}

			// PackageManager
			if user.PackageManager != "" {
				if result.PackageManager != user.PackageManager {
					return false
				}
			} else {
				if result.PackageManager != detected.PackageManager {
					return false
				}
			}

			// CGOEnabled - special case: if user sets a value, it should be used
			// If user doesn't set (nil), detected value is used
			if user.CGOEnabled != nil {
				if result.CGOEnabled == nil || *result.CGOEnabled != *user.CGOEnabled {
					return false
				}
			} else {
				// User didn't set, so detected value should be used
				if result.CGOEnabled != nil && *result.CGOEnabled != detected.CGOEnabled {
					return false
				}
			}

			// BuildTimeout
			if user.BuildTimeout != 0 {
				if result.BuildTimeout != user.BuildTimeout {
					return false
				}
			} else {
				if result.BuildTimeout != detected.BuildTimeout {
					return false
				}
			}

			return true
		},
		genDetectedConfig(),
		genUserConfig(),
	))

	properties.TestingRun(t)
}

// TestMergeConfigNilInputs tests edge cases for MergeConfig.
func TestMergeConfigNilInputs(t *testing.T) {
	t.Run("nil user config returns detected values", func(t *testing.T) {
		detected := map[string]interface{}{
			"go_version":    "1.21",
			"entry_point":   "cmd/api",
			"cgo_enabled":   true,
			"build_timeout": 1800,
		}

		result := MergeConfig(detected, nil)

		if result.GoVersion != "1.21" {
			t.Errorf("expected GoVersion 1.21, got %s", result.GoVersion)
		}
		if result.EntryPoint != "cmd/api" {
			t.Errorf("expected EntryPoint cmd/api, got %s", result.EntryPoint)
		}
		if result.CGOEnabled == nil || !*result.CGOEnabled {
			t.Error("expected CGOEnabled true")
		}
		if result.BuildTimeout != 1800 {
			t.Errorf("expected BuildTimeout 1800, got %d", result.BuildTimeout)
		}
	})

	t.Run("nil detected config returns user values", func(t *testing.T) {
		cgoEnabled := true
		userConfig := &models.BuildConfig{
			GoVersion:    "1.22",
			EntryPoint:   "cmd/custom",
			CGOEnabled:   &cgoEnabled,
			BuildTimeout: 600,
		}

		result := MergeConfig(nil, userConfig)

		if result.GoVersion != "1.22" {
			t.Errorf("expected GoVersion 1.22, got %s", result.GoVersion)
		}
		if result.EntryPoint != "cmd/custom" {
			t.Errorf("expected EntryPoint cmd/custom, got %s", result.EntryPoint)
		}
		if result.CGOEnabled == nil || !*result.CGOEnabled {
			t.Error("expected CGOEnabled true")
		}
		if result.BuildTimeout != 600 {
			t.Errorf("expected BuildTimeout 600, got %d", result.BuildTimeout)
		}
	})

	t.Run("both nil returns empty config", func(t *testing.T) {
		result := MergeConfig(nil, nil)

		if result.GoVersion != "" {
			t.Errorf("expected empty GoVersion, got %s", result.GoVersion)
		}
		if result.EntryPoint != "" {
			t.Errorf("expected empty EntryPoint, got %s", result.EntryPoint)
		}
		if result.CGOEnabled != nil {
			t.Error("expected CGOEnabled nil")
		}
		if result.BuildTimeout != 0 {
			t.Errorf("expected BuildTimeout 0, got %d", result.BuildTimeout)
		}
	})
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
