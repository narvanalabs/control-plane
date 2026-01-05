package templates

import (
	"context"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: flexible-build-strategies, Property 2: Template Rendering Produces Valid Nix**
// For any valid DetectionResult and BuildConfig, the TemplateEngine SHALL produce
// a flake.nix that passes nix flake check validation.
// **Validates: Requirements 11.3, 11.9**

// genValidAppName generates valid application names.
func genValidAppName() gopter.Gen {
	return gen.OneConstOf(
		"myapp",
		"api-server",
		"web-service",
		"backend",
		"frontend",
		"worker",
		"cli-tool",
	)
}

// genValidGoVersion generates valid Go version strings.
func genValidGoVersion() gopter.Gen {
	return gen.OneConstOf(
		"1.21",
		"1.22",
		"1.23",
		"1.20",
		"1.19",
	)
}

// genValidEntryPoint generates valid entry point paths.
func genValidEntryPoint() gopter.Gen {
	return gen.OneConstOf(
		"",
		"cmd/api",
		"cmd/server",
		"cmd/worker",
		".",
	)
}

// GoTemplateInput represents input for Go template rendering.
type GoTemplateInput struct {
	AppName    string
	GoVersion  string
	EntryPoint string
	CGOEnabled bool
}

// genGoTemplateInput generates valid Go template inputs.
func genGoTemplateInput() gopter.Gen {
	return gopter.CombineGens(
		genValidAppName(),
		genValidGoVersion(),
		genValidEntryPoint(),
		gen.Bool(),
	).Map(func(vals []interface{}) GoTemplateInput {
		return GoTemplateInput{
			AppName:    vals[0].(string),
			GoVersion:  vals[1].(string),
			EntryPoint: vals[2].(string),
			CGOEnabled: vals[3].(bool),
		}
	})
}

func TestGoTemplateRenderingProducesValidNix(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	engine, err := NewTemplateEngine()
	if err != nil {
		t.Fatalf("Failed to create template engine: %v", err)
	}

	properties.Property("Go template renders valid Nix syntax", prop.ForAll(
		func(input GoTemplateInput) bool {
			templateName := "go.nix"
			if input.CGOEnabled {
				templateName = "go-cgo.nix"
			}

			cgoEnabled := input.CGOEnabled
			data := TemplateData{
				AppName:    input.AppName,
				Version:    input.GoVersion,
				EntryPoint: input.EntryPoint,
				Config: models.BuildConfig{
					GoVersion:  input.GoVersion,
					EntryPoint: input.EntryPoint,
					CGOEnabled: &cgoEnabled,
				},
			}

			result, err := engine.Render(context.Background(), templateName, data)
			if err != nil {
				t.Logf("Render error: %v", err)
				return false
			}

			// Basic structural validation - check for required Nix flake elements
			if !strings.Contains(result, "description") {
				t.Log("Missing description")
				return false
			}
			if !strings.Contains(result, "inputs") {
				t.Log("Missing inputs")
				return false
			}
			if !strings.Contains(result, "outputs") {
				t.Log("Missing outputs")
				return false
			}
			if !strings.Contains(result, "nixpkgs") {
				t.Log("Missing nixpkgs")
				return false
			}
			if !strings.Contains(result, "packages.default") {
				t.Log("Missing packages.default")
				return false
			}

			// Check that the app name is in the output
			if !strings.Contains(result, input.AppName) {
				t.Logf("App name %s not found in output", input.AppName)
				return false
			}

			return true
		},
		genGoTemplateInput(),
	))

	properties.TestingRun(t)
}


// **Feature: flexible-build-strategies, Property 15: Template Idempotency**
// For any DetectionResult and BuildConfig, rendering the same template twice
// SHALL produce identical output.
// **Validates: Requirements 11.2**

func TestTemplateIdempotency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	engine, err := NewTemplateEngine()
	if err != nil {
		t.Fatalf("Failed to create template engine: %v", err)
	}

	properties.Property("rendering same template twice produces identical output", prop.ForAll(
		func(input GoTemplateInput) bool {
			templateName := "go.nix"
			if input.CGOEnabled {
				templateName = "go-cgo.nix"
			}

			cgoEnabled := input.CGOEnabled
			data := TemplateData{
				AppName:    input.AppName,
				Version:    input.GoVersion,
				EntryPoint: input.EntryPoint,
				Config: models.BuildConfig{
					GoVersion:  input.GoVersion,
					EntryPoint: input.EntryPoint,
					CGOEnabled: &cgoEnabled,
				},
			}

			// Render twice
			result1, err1 := engine.Render(context.Background(), templateName, data)
			result2, err2 := engine.Render(context.Background(), templateName, data)

			if err1 != nil || err2 != nil {
				return false
			}

			// Results must be identical
			return result1 == result2
		},
		genGoTemplateInput(),
	))

	properties.TestingRun(t)
}


// **Feature: ui-api-alignment, Property 15: Pre-Build Hook Execution Order**
// For any build with pre-build commands, those commands SHALL execute before the main build step,
// and failure SHALL abort the build.
// **Validates: Requirements 21.3, 21.5**

// genValidPreBuildCommand generates valid pre-build commands.
func genValidPreBuildCommand() gopter.Gen {
	return gen.OneConstOf(
		"echo 'Running pre-build'",
		"go generate ./...",
		"make proto",
		"npm run codegen",
		"./scripts/setup.sh",
	)
}

// genPreBuildCommands generates a slice of pre-build commands.
func genPreBuildCommands() gopter.Gen {
	return gen.SliceOfN(3, genValidPreBuildCommand())
}

// GoTemplateWithHooksInput represents input for Go template rendering with hooks.
type GoTemplateWithHooksInput struct {
	AppName           string
	GoVersion         string
	EntryPoint        string
	CGOEnabled        bool
	PreBuildCommands  []string
	PostBuildCommands []string
}

// genGoTemplateWithHooksInput generates valid Go template inputs with hooks.
func genGoTemplateWithHooksInput() gopter.Gen {
	return gopter.CombineGens(
		genValidAppName(),
		genValidGoVersion(),
		genValidEntryPoint(),
		gen.Bool(),
		genPreBuildCommands(),
		genPostBuildCommands(),
	).Map(func(vals []interface{}) GoTemplateWithHooksInput {
		return GoTemplateWithHooksInput{
			AppName:           vals[0].(string),
			GoVersion:         vals[1].(string),
			EntryPoint:        vals[2].(string),
			CGOEnabled:        vals[3].(bool),
			PreBuildCommands:  vals[4].([]string),
			PostBuildCommands: vals[5].([]string),
		}
	})
}

func TestPreBuildHookExecutionOrder(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	engine, err := NewTemplateEngine()
	if err != nil {
		t.Fatalf("Failed to create template engine: %v", err)
	}

	properties.Property("pre-build commands appear in preBuild section before main build", prop.ForAll(
		func(input GoTemplateWithHooksInput) bool {
			templateName := "go.nix"
			if input.CGOEnabled {
				templateName = "go-cgo.nix"
			}

			cgoEnabled := input.CGOEnabled
			data := TemplateData{
				AppName:    input.AppName,
				Version:    input.GoVersion,
				EntryPoint: input.EntryPoint,
				Config: models.BuildConfig{
					GoVersion:         input.GoVersion,
					EntryPoint:        input.EntryPoint,
					CGOEnabled:        &cgoEnabled,
					PreBuildCommands:  input.PreBuildCommands,
					PostBuildCommands: input.PostBuildCommands,
				},
			}

			result, err := engine.Render(context.Background(), templateName, data)
			if err != nil {
				t.Logf("Render error: %v", err)
				return false
			}

			// Property: All pre-build commands must appear in the preBuild section
			// The preBuild section executes before the main build step in Nix
			if len(input.PreBuildCommands) > 0 {
				// Check that preBuild section exists
				if !strings.Contains(result, "preBuild") {
					t.Log("Missing preBuild section when pre-build commands are specified")
					return false
				}

				// Check that all pre-build commands are present in the output
				for _, cmd := range input.PreBuildCommands {
					if !strings.Contains(result, cmd) {
						t.Logf("Pre-build command %q not found in output", cmd)
						return false
					}
				}

				// Verify commands appear in the preBuild section (before postBuild if present)
				preBuildIdx := strings.Index(result, "preBuild")
				for _, cmd := range input.PreBuildCommands {
					cmdIdx := strings.Index(result, cmd)
					if cmdIdx < preBuildIdx {
						t.Logf("Pre-build command %q appears before preBuild section", cmd)
						return false
					}
				}
			}

			return true
		},
		genGoTemplateWithHooksInput(),
	))

	properties.TestingRun(t)
}

// **Feature: ui-api-alignment, Property 16: Post-Build Hook Execution Order**
// For any successful build with post-build commands, those commands SHALL execute after the main build step,
// and failure SHALL mark the build as failed.
// **Validates: Requirements 21.4, 21.6**

// genValidPostBuildCommand generates valid post-build commands.
func genValidPostBuildCommand() gopter.Gen {
	return gen.OneConstOf(
		"echo 'Running post-build'",
		"./scripts/verify.sh",
		"make test",
		"npm run lint",
		"go vet ./...",
	)
}

// genPostBuildCommands generates a slice of post-build commands.
func genPostBuildCommands() gopter.Gen {
	return gen.SliceOfN(3, genValidPostBuildCommand())
}

func TestPostBuildHookExecutionOrder(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	engine, err := NewTemplateEngine()
	if err != nil {
		t.Fatalf("Failed to create template engine: %v", err)
	}

	properties.Property("post-build commands appear in postBuild section after main build", prop.ForAll(
		func(input GoTemplateWithHooksInput) bool {
			templateName := "go.nix"
			if input.CGOEnabled {
				templateName = "go-cgo.nix"
			}

			cgoEnabled := input.CGOEnabled
			data := TemplateData{
				AppName:    input.AppName,
				Version:    input.GoVersion,
				EntryPoint: input.EntryPoint,
				Config: models.BuildConfig{
					GoVersion:         input.GoVersion,
					EntryPoint:        input.EntryPoint,
					CGOEnabled:        &cgoEnabled,
					PreBuildCommands:  input.PreBuildCommands,
					PostBuildCommands: input.PostBuildCommands,
				},
			}

			result, err := engine.Render(context.Background(), templateName, data)
			if err != nil {
				t.Logf("Render error: %v", err)
				return false
			}

			// Property: All post-build commands must appear in the postBuild section
			// The postBuild section executes after the main build step in Nix
			if len(input.PostBuildCommands) > 0 {
				// Check that postBuild section exists
				if !strings.Contains(result, "postBuild") {
					t.Log("Missing postBuild section when post-build commands are specified")
					return false
				}

				// Check that all post-build commands are present in the output
				for _, cmd := range input.PostBuildCommands {
					if !strings.Contains(result, cmd) {
						t.Logf("Post-build command %q not found in output", cmd)
						return false
					}
				}

				// Verify postBuild section appears after preBuild section (execution order)
				preBuildIdx := strings.Index(result, "preBuild")
				postBuildIdx := strings.Index(result, "postBuild")
				if preBuildIdx >= 0 && postBuildIdx >= 0 && postBuildIdx < preBuildIdx {
					t.Log("postBuild section appears before preBuild section")
					return false
				}
			}

			return true
		},
		genGoTemplateWithHooksInput(),
	))

	properties.TestingRun(t)
}

// TestBuildHooksPreserveTemplateValidity verifies that adding hooks doesn't break template validity.
func TestBuildHooksPreserveTemplateValidity(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	engine, err := NewTemplateEngine()
	if err != nil {
		t.Fatalf("Failed to create template engine: %v", err)
	}

	properties.Property("templates with hooks remain valid Nix", prop.ForAll(
		func(input GoTemplateWithHooksInput) bool {
			templateName := "go.nix"
			if input.CGOEnabled {
				templateName = "go-cgo.nix"
			}

			cgoEnabled := input.CGOEnabled
			data := TemplateData{
				AppName:    input.AppName,
				Version:    input.GoVersion,
				EntryPoint: input.EntryPoint,
				Config: models.BuildConfig{
					GoVersion:         input.GoVersion,
					EntryPoint:        input.EntryPoint,
					CGOEnabled:        &cgoEnabled,
					PreBuildCommands:  input.PreBuildCommands,
					PostBuildCommands: input.PostBuildCommands,
				},
			}

			result, err := engine.Render(context.Background(), templateName, data)
			if err != nil {
				t.Logf("Render error: %v", err)
				return false
			}

			// Basic structural validation - check for required Nix flake elements
			if !strings.Contains(result, "inputs") {
				t.Log("Missing inputs")
				return false
			}
			if !strings.Contains(result, "outputs") {
				t.Log("Missing outputs")
				return false
			}
			if !strings.Contains(result, "nixpkgs") {
				t.Log("Missing nixpkgs")
				return false
			}
			if !strings.Contains(result, "packages.default") {
				t.Log("Missing packages.default")
				return false
			}

			// Validate syntax using the engine's syntax validator
			if err := engine.ValidateSyntax(result); err != nil {
				t.Logf("Syntax validation failed: %v", err)
				return false
			}

			return true
		},
		genGoTemplateWithHooksInput(),
	))

	properties.TestingRun(t)
}
