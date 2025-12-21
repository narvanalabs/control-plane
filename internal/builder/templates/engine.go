// Package templates provides Nix flake template rendering for build strategies.
package templates

import (
	"context"
	"embed"
	"fmt"
	"os/exec"
	"strings"
	"text/template"

	builderrors "github.com/narvanalabs/control-plane/internal/builder/errors"
	"github.com/narvanalabs/control-plane/internal/models"
)

//go:embed *.tmpl
var templateFS embed.FS

// TemplateEngine renders Nix flake templates.
type TemplateEngine interface {
	// Render generates a flake.nix from a template.
	Render(ctx context.Context, templateName string, data TemplateData) (string, error)

	// Validate checks if generated flake.nix is syntactically valid.
	Validate(ctx context.Context, flakeContent string) error

	// ListTemplates returns available template names.
	ListTemplates() []string
}

// TemplateData contains data passed to templates.
type TemplateData struct {
	AppName         string
	Version         string
	Framework       models.Framework
	EntryPoint      string
	BuildCommand    string
	StartCommand    string
	Config          models.BuildConfig
	DetectionResult *models.DetectionResult
}

// DefaultTemplateEngine is the default implementation of TemplateEngine.
type DefaultTemplateEngine struct {
	templates map[string]*template.Template
}

// NewTemplateEngine creates a new DefaultTemplateEngine with embedded templates.
func NewTemplateEngine() (*DefaultTemplateEngine, error) {
	engine := &DefaultTemplateEngine{
		templates: make(map[string]*template.Template),
	}

	// Load all embedded templates
	entries, err := templateFS.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("failed to read template directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}

		content, err := templateFS.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read template %s: %w", entry.Name(), err)
		}

		tmpl, err := template.New(entry.Name()).Funcs(templateFuncs()).Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", entry.Name(), err)
		}

		// Store without .tmpl extension for easier lookup
		name := strings.TrimSuffix(entry.Name(), ".tmpl")
		engine.templates[name] = tmpl
	}

	return engine, nil
}

// templateFuncs returns custom template functions.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"default": func(defaultVal, val string) string {
			if val == "" {
				return defaultVal
			}
			return val
		},
		"quote": func(s string) string {
			return fmt.Sprintf("%q", s)
		},
		"nixString": func(s string) string {
			// Escape special characters for Nix strings
			s = strings.ReplaceAll(s, "\\", "\\\\")
			s = strings.ReplaceAll(s, "\"", "\\\"")
			s = strings.ReplaceAll(s, "${", "\\${")
			return fmt.Sprintf("\"%s\"", s)
		},
		"join": strings.Join,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,
	}
}


// Render generates a flake.nix from a template.
func (e *DefaultTemplateEngine) Render(ctx context.Context, templateName string, data TemplateData) (string, error) {
	tmpl, ok := e.templates[templateName]
	if !ok {
		return "", builderrors.NewTemplateNotFoundError(templateName)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", builderrors.NewTemplateRenderError(err, templateName)
	}

	return buf.String(), nil
}

// Validate checks if generated flake.nix is syntactically valid using nix flake check.
func (e *DefaultTemplateEngine) Validate(ctx context.Context, flakeContent string) error {
	// Create a temporary directory for validation
	tmpDir, err := createTempFlakeDir(flakeContent)
	if err != nil {
		return fmt.Errorf("failed to create temp dir for validation: %w", err)
	}
	defer cleanupTempDir(tmpDir)

	// Run nix flake check
	cmd := exec.CommandContext(ctx, "nix", "flake", "check", "--no-build", tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Include the generated flake in the error for debugging
		return builderrors.NewInvalidFlakeSyntaxError(
			fmt.Errorf("%s", string(output)),
			flakeContent,
		)
	}

	return nil
}

// ListTemplates returns available template names.
func (e *DefaultTemplateEngine) ListTemplates() []string {
	names := make([]string, 0, len(e.templates))
	for name := range e.templates {
		names = append(names, name)
	}
	return names
}

// GetTemplateForStrategy returns the appropriate template name for a build strategy.
func GetTemplateForStrategy(strategy models.BuildStrategy, config models.BuildConfig) string {
	switch strategy {
	case models.BuildStrategyAutoGo:
		if config.CGOEnabled {
			return "go-cgo.nix"
		}
		return "go.nix"
	case models.BuildStrategyAutoNode:
		if config.NextJSOptions != nil {
			return "nextjs.nix"
		}
		return "nodejs.nix"
	case models.BuildStrategyAutoRust:
		return "rust.nix"
	case models.BuildStrategyAutoPython:
		return "python.nix"
	case models.BuildStrategyDockerfile:
		return "dockerfile.nix"
	default:
		return ""
	}
}

// RenderAndValidate renders a template and validates the output.
// This is a convenience method that combines Render and Validate.
func (e *DefaultTemplateEngine) RenderAndValidate(ctx context.Context, templateName string, data TemplateData) (string, error) {
	result, err := e.Render(ctx, templateName, data)
	if err != nil {
		return "", err
	}

	if err := e.Validate(ctx, result); err != nil {
		return result, err // Return the result even on validation failure for debugging
	}

	return result, nil
}

// ValidateSyntax performs a basic syntax check on the flake content without running nix.
// This is faster than full validation and catches common template errors.
func (e *DefaultTemplateEngine) ValidateSyntax(flakeContent string) error {
	// Check for balanced braces
	braceCount := 0
	bracketCount := 0
	parenCount := 0

	for _, ch := range flakeContent {
		switch ch {
		case '{':
			braceCount++
		case '}':
			braceCount--
		case '[':
			bracketCount++
		case ']':
			bracketCount--
		case '(':
			parenCount++
		case ')':
			parenCount--
		}

		if braceCount < 0 || bracketCount < 0 || parenCount < 0 {
			return builderrors.NewInvalidFlakeSyntaxError(
				fmt.Errorf("unbalanced brackets"),
				flakeContent,
			)
		}
	}

	if braceCount != 0 {
		return builderrors.NewInvalidFlakeSyntaxError(
			fmt.Errorf("unbalanced braces (count: %d)", braceCount),
			flakeContent,
		)
	}
	if bracketCount != 0 {
		return builderrors.NewInvalidFlakeSyntaxError(
			fmt.Errorf("unbalanced square brackets (count: %d)", bracketCount),
			flakeContent,
		)
	}
	if parenCount != 0 {
		return builderrors.NewInvalidFlakeSyntaxError(
			fmt.Errorf("unbalanced parentheses (count: %d)", parenCount),
			flakeContent,
		)
	}

	// Check for required flake structure
	if !strings.Contains(flakeContent, "description") {
		return builderrors.NewInvalidFlakeSyntaxError(
			fmt.Errorf("missing description"),
			flakeContent,
		)
	}
	if !strings.Contains(flakeContent, "inputs") {
		return builderrors.NewInvalidFlakeSyntaxError(
			fmt.Errorf("missing inputs"),
			flakeContent,
		)
	}
	if !strings.Contains(flakeContent, "outputs") {
		return builderrors.NewInvalidFlakeSyntaxError(
			fmt.Errorf("missing outputs"),
			flakeContent,
		)
	}

	return nil
}

// Errors for template operations - kept for backward compatibility.
var (
	ErrTemplateNotFound     = fmt.Errorf("template not found")
	ErrTemplateRenderFailed = fmt.Errorf("failed to render template")
	ErrInvalidFlakeSyntax   = fmt.Errorf("generated flake has invalid syntax")
)


// createTempFlakeDir creates a temporary directory with a flake.nix for validation.
func createTempFlakeDir(flakeContent string) (string, error) {
	tmpDir, err := exec.Command("mktemp", "-d").Output()
	if err != nil {
		return "", err
	}
	dir := strings.TrimSpace(string(tmpDir))

	// Write flake.nix
	flakePath := dir + "/flake.nix"
	if err := writeFile(flakePath, flakeContent); err != nil {
		cleanupTempDir(dir)
		return "", err
	}

	return dir, nil
}

// cleanupTempDir removes a temporary directory.
func cleanupTempDir(dir string) {
	exec.Command("rm", "-rf", dir).Run()
}

// writeFile writes content to a file.
func writeFile(path, content string) error {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("cat > %s", path))
	cmd.Stdin = strings.NewReader(content)
	return cmd.Run()
}
