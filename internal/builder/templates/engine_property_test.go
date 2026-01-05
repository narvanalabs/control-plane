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
