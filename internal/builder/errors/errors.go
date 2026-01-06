// Package errors provides enhanced error handling for the build system.
package errors

import (
	"errors"
	"fmt"
	"strings"

	"github.com/narvanalabs/control-plane/internal/models"
)

// Error categories for build failures.
const (
	CategoryDetection = "detection"
	CategoryTemplate  = "template"
	CategoryBuild     = "build"
	CategoryTimeout   = "timeout"
	CategoryResource  = "resource"
	CategoryConfig    = "config"
)

// Error codes for specific failure types.
const (
	CodeNoLanguageDetected   = "NO_LANGUAGE_DETECTED"
	CodeMultipleLanguages    = "MULTIPLE_LANGUAGES"
	CodeUnsupportedLanguage  = "UNSUPPORTED_LANGUAGE"
	CodeRepositoryAccess     = "REPOSITORY_ACCESS_FAILED"
	CodeTemplateNotFound     = "TEMPLATE_NOT_FOUND"
	CodeTemplateRenderFailed = "TEMPLATE_RENDER_FAILED"
	CodeInvalidFlakeSyntax   = "INVALID_FLAKE_SYNTAX"
	CodeBuildFailed          = "BUILD_FAILED"
	CodeBuildTimeout         = "BUILD_TIMEOUT"
	CodeBuildOOM             = "BUILD_OOM"
	CodeVendorHashFailed     = "VENDOR_HASH_FAILED"
	CodeDockerfileNotFound   = "DOCKERFILE_NOT_FOUND"
	CodeFlakeNotFound        = "FLAKE_NOT_FOUND"
	CodeDependencyMissing    = "DEPENDENCY_MISSING"
	CodeInvalidConfig        = "INVALID_CONFIG"
	CodeNixpacksFailed       = "NIXPACKS_FAILED"
)

// BuildErrorResponse provides detailed error information with suggestions and next steps.
type BuildErrorResponse struct {
	// Error is the main error message.
	Error string `json:"error"`

	// Code is a machine-readable error code.
	Code string `json:"code"`

	// Category groups errors by type (detection, template, build, timeout).
	Category string `json:"category"`

	// Strategy is the build strategy that was being used when the error occurred.
	Strategy string `json:"strategy,omitempty"`

	// GeneratedFlake contains the generated flake.nix content for debugging template issues.
	GeneratedFlake string `json:"generated_flake,omitempty"`

	// Suggestions provides actionable suggestions to resolve the error.
	Suggestions []string `json:"suggestions,omitempty"`

	// Documentation is a URL to relevant documentation.
	Documentation string `json:"documentation_url,omitempty"`

	// CanRetryAsOCI indicates whether the build can be retried as an OCI build.
	CanRetryAsOCI bool `json:"can_retry_as_oci"`

	// DetectedIssues lists specific problems found during the build.
	DetectedIssues []string `json:"detected_issues,omitempty"`

	// NextSteps provides guidance on what the user should do next.
	NextSteps []string `json:"next_steps,omitempty"`
}

// BuildError is an error type that can be converted to a BuildErrorResponse.
type BuildError struct {
	Err            error
	Code           string
	Category       string
	Strategy       models.BuildStrategy
	GeneratedFlake string
	Suggestions    []string
	DetectedIssues []string
	NextSteps      []string
	CanRetryAsOCI  bool
}

// Error implements the error interface.
func (e *BuildError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %s", e.Category, e.Code)
}

// Unwrap returns the underlying error.
func (e *BuildError) Unwrap() error {
	return e.Err
}

// ToResponse converts the BuildError to a BuildErrorResponse.
func (e *BuildError) ToResponse() *BuildErrorResponse {
	return &BuildErrorResponse{
		Error:          e.Error(),
		Code:           e.Code,
		Category:       e.Category,
		Strategy:       string(e.Strategy),
		GeneratedFlake: e.GeneratedFlake,
		Suggestions:    e.Suggestions,
		CanRetryAsOCI:  e.CanRetryAsOCI,
		DetectedIssues: e.DetectedIssues,
		NextSteps:      e.NextSteps,
	}
}

// NewBuildError creates a new BuildError with the given parameters.
func NewBuildError(err error, code, category string) *BuildError {
	return &BuildError{
		Err:      err,
		Code:     code,
		Category: category,
	}
}

// WithStrategy sets the build strategy on the error.
func (e *BuildError) WithStrategy(strategy models.BuildStrategy) *BuildError {
	e.Strategy = strategy
	return e
}

// WithGeneratedFlake sets the generated flake content on the error.
func (e *BuildError) WithGeneratedFlake(flake string) *BuildError {
	e.GeneratedFlake = flake
	return e
}

// WithSuggestions sets the suggestions on the error.
func (e *BuildError) WithSuggestions(suggestions ...string) *BuildError {
	e.Suggestions = suggestions
	return e
}

// WithDetectedIssues sets the detected issues on the error.
func (e *BuildError) WithDetectedIssues(issues ...string) *BuildError {
	e.DetectedIssues = issues
	return e
}

// WithNextSteps sets the next steps on the error.
func (e *BuildError) WithNextSteps(steps ...string) *BuildError {
	e.NextSteps = steps
	return e
}

// WithCanRetryAsOCI sets whether the build can be retried as OCI.
func (e *BuildError) WithCanRetryAsOCI(canRetry bool) *BuildError {
	e.CanRetryAsOCI = canRetry
	return e
}

// Detection error constructors.

// NewDetectionError creates a new detection error.
func NewDetectionError(err error, code string) *BuildError {
	return NewBuildError(err, code, CategoryDetection)
}

// NewNoLanguageDetectedError creates an error for when no language can be detected.
func NewNoLanguageDetectedError() *BuildError {
	return NewDetectionError(
		errors.New("could not detect application language"),
		CodeNoLanguageDetected,
	).WithSuggestions(
		"Try specifying a build strategy explicitly (e.g., auto-go, auto-node)",
		"Use the dockerfile strategy if you have a Dockerfile",
		"Use the nixpacks strategy for automatic detection",
		"Ensure your repository contains standard project files (go.mod, package.json, Cargo.toml, etc.)",
	).WithNextSteps(
		"Review your repository structure",
		"Add a build strategy to your service configuration",
		"Consider using nixpacks for automatic builds",
	)
}

// NewMultipleLanguagesError creates an error for when multiple languages are detected.
func NewMultipleLanguagesError(languages []string) *BuildError {
	return NewDetectionError(
		fmt.Errorf("multiple languages detected: %s", strings.Join(languages, ", ")),
		CodeMultipleLanguages,
	).WithSuggestions(
		"Specify a build strategy explicitly to choose which language to build",
		"Use the dockerfile strategy if you have a multi-language project",
		"Consider splitting your project into separate services",
	).WithDetectedIssues(
		fmt.Sprintf("Found project files for: %s", strings.Join(languages, ", ")),
	).WithNextSteps(
		"Choose the primary language for this service",
		"Set build_strategy to the appropriate auto-* strategy",
	)
}

// NewUnsupportedLanguageError creates an error for unsupported languages.
func NewUnsupportedLanguageError(language string) *BuildError {
	return NewDetectionError(
		fmt.Errorf("detected language '%s' is not supported", language),
		CodeUnsupportedLanguage,
	).WithSuggestions(
		"Use the dockerfile strategy with a custom Dockerfile",
		"Use the nixpacks strategy for automatic builds",
		"Use the flake strategy with a custom flake.nix",
	).WithNextSteps(
		"Create a Dockerfile for your application",
		"Or try nixpacks which supports many languages",
	)
}

// NewRepositoryAccessError creates an error for repository access failures.
func NewRepositoryAccessError(err error) *BuildError {
	return NewDetectionError(
		fmt.Errorf("failed to access repository: %w", err),
		CodeRepositoryAccess,
	).WithSuggestions(
		"Verify the repository URL is correct",
		"Check that the repository is accessible",
		"Ensure authentication credentials are valid",
	).WithNextSteps(
		"Verify repository URL and access permissions",
		"Check network connectivity",
	)
}

// Template error constructors.

// NewTemplateError creates a new template error.
func NewTemplateError(err error, code string) *BuildError {
	return NewBuildError(err, code, CategoryTemplate)
}

// NewTemplateNotFoundError creates an error for missing templates.
func NewTemplateNotFoundError(templateName string) *BuildError {
	return NewTemplateError(
		fmt.Errorf("template not found: %s", templateName),
		CodeTemplateNotFound,
	).WithSuggestions(
		"Check that the build strategy is supported",
		"Use a different build strategy",
	).WithNextSteps(
		"Try a different build strategy",
		"Contact support if this strategy should be available",
	)
}

// NewTemplateRenderError creates an error for template rendering failures.
func NewTemplateRenderError(err error, templateName string) *BuildError {
	return NewTemplateError(
		fmt.Errorf("failed to render template %s: %w", templateName, err),
		CodeTemplateRenderFailed,
	).WithSuggestions(
		"Check your build configuration for invalid values",
		"Try using default configuration values",
		"Use the dockerfile strategy as an alternative",
	).WithCanRetryAsOCI(true).WithNextSteps(
		"Review your build configuration",
		"Try with default settings",
		"Consider using dockerfile or nixpacks strategy",
	)
}

// NewInvalidFlakeSyntaxError creates an error for invalid flake syntax.
func NewInvalidFlakeSyntaxError(err error, flakeContent string) *BuildError {
	return NewTemplateError(
		fmt.Errorf("generated flake has invalid syntax: %w", err),
		CodeInvalidFlakeSyntax,
	).WithGeneratedFlake(flakeContent).WithSuggestions(
		"This is likely a bug in the template - please report it",
		"Try using the dockerfile or nixpacks strategy instead",
		"Provide a custom flake.nix in your repository",
	).WithCanRetryAsOCI(true).WithNextSteps(
		"Report this issue with the generated flake content",
		"Use an alternative build strategy",
	)
}

// Build error constructors.

// NewBuildFailedError creates a new build error.
func NewBuildFailedError(err error) *BuildError {
	return NewBuildError(err, CodeBuildFailed, CategoryBuild).WithCanRetryAsOCI(true).WithSuggestions(
		"Check the build logs for specific errors",
		"Verify all dependencies are available",
		"Try building locally to reproduce the issue",
	).WithNextSteps(
		"Review build logs",
		"Fix any identified issues",
		"Retry the build",
	)
}

// NewBuildTimeoutError creates an error for build timeouts.
func NewBuildTimeoutError(timeout string) *BuildError {
	return NewBuildError(
		fmt.Errorf("build exceeded timeout limit of %s", timeout),
		CodeBuildTimeout,
		CategoryTimeout,
	).WithSuggestions(
		"Increase the build timeout in your configuration",
		"Optimize your build process to be faster",
		"Consider using build caching",
		"Split large builds into smaller components",
	).WithNextSteps(
		"Increase build_config.build_timeout",
		"Review build process for optimization opportunities",
	)
}

// NewBuildOOMError creates an error for out-of-memory failures.
func NewBuildOOMError() *BuildError {
	return NewBuildError(
		errors.New("build exceeded memory limit"),
		CodeBuildOOM,
		CategoryResource,
	).WithSuggestions(
		"Reduce memory usage during build",
		"Use a higher resource tier",
		"Split the build into smaller steps",
	).WithNextSteps(
		"Optimize build memory usage",
		"Contact support for higher resource limits",
	)
}

// NewVendorHashError creates an error for vendor hash calculation failures.
func NewVendorHashError(err error) *BuildError {
	return NewBuildError(
		fmt.Errorf("failed to calculate vendor hash: %w", err),
		CodeVendorHashFailed,
		CategoryBuild,
	).WithCanRetryAsOCI(true).WithSuggestions(
		"Ensure all dependencies are properly specified",
		"Check that dependency sources are accessible",
		"Try using the dockerfile strategy instead",
	).WithNextSteps(
		"Verify dependency specifications",
		"Check network access to dependency sources",
	)
}

// NewDockerfileNotFoundError creates an error for missing Dockerfile.
func NewDockerfileNotFoundError() *BuildError {
	return NewBuildError(
		errors.New("Dockerfile not found in repository"),
		CodeDockerfileNotFound,
		CategoryConfig,
	).WithSuggestions(
		"Add a Dockerfile to your repository root",
		"Use a different build strategy (auto-*, nixpacks)",
		"Specify the correct path if Dockerfile is in a subdirectory",
	).WithNextSteps(
		"Create a Dockerfile in your repository",
		"Or change to a different build strategy",
	)
}

// NewFlakeNotFoundError creates an error for missing flake.nix.
func NewFlakeNotFoundError() *BuildError {
	return NewBuildError(
		errors.New("flake.nix not found in repository"),
		CodeFlakeNotFound,
		CategoryConfig,
	).WithSuggestions(
		"Add a flake.nix to your repository root",
		"Use an auto-* strategy to generate a flake automatically",
		"Use the dockerfile or nixpacks strategy instead",
	).WithNextSteps(
		"Create a flake.nix in your repository",
		"Or change to a different build strategy",
	)
}

// NewDependencyMissingError creates an error for missing dependencies.
func NewDependencyMissingError(dependencies []string) *BuildError {
	return NewBuildError(
		fmt.Errorf("missing dependencies: %s", strings.Join(dependencies, ", ")),
		CodeDependencyMissing,
		CategoryBuild,
	).WithDetectedIssues(
		fmt.Sprintf("Missing: %s", strings.Join(dependencies, ", ")),
	).WithCanRetryAsOCI(true).WithSuggestions(
		"Add the missing dependencies to your project",
		"Check that all dependencies are properly specified",
		"Try using the dockerfile strategy for more control",
	).WithNextSteps(
		"Install missing dependencies",
		"Update dependency specifications",
	)
}

// NewNixpacksError creates an error for Nixpacks failures.
func NewNixpacksError(err error) *BuildError {
	return NewBuildError(
		fmt.Errorf("nixpacks build failed: %w", err),
		CodeNixpacksFailed,
		CategoryBuild,
	).WithSuggestions(
		"Check that your project structure is standard",
		"Try using the dockerfile strategy instead",
		"Review Nixpacks documentation for your language",
	).WithNextSteps(
		"Review build logs for specific errors",
		"Consider using dockerfile strategy",
	)
}

// NewInvalidConfigError creates an error for invalid configuration.
func NewInvalidConfigError(field, message string) *BuildError {
	return NewBuildError(
		fmt.Errorf("invalid configuration: %s - %s", field, message),
		CodeInvalidConfig,
		CategoryConfig,
	).WithDetectedIssues(
		fmt.Sprintf("Invalid field: %s", field),
	).WithSuggestions(
		"Review the configuration documentation",
		"Check the field value format",
		"Use default values if unsure",
	).WithNextSteps(
		"Fix the configuration value",
		"Retry the build",
	)
}

// ErrorFromCode creates a BuildError from an error code.
func ErrorFromCode(code string, err error) *BuildError {
	switch code {
	case CodeNoLanguageDetected:
		return NewNoLanguageDetectedError()
	case CodeMultipleLanguages:
		return NewMultipleLanguagesError(nil)
	case CodeUnsupportedLanguage:
		return NewUnsupportedLanguageError("unknown")
	case CodeRepositoryAccess:
		return NewRepositoryAccessError(err)
	case CodeTemplateNotFound:
		return NewTemplateNotFoundError("unknown")
	case CodeTemplateRenderFailed:
		return NewTemplateRenderError(err, "unknown")
	case CodeInvalidFlakeSyntax:
		return NewInvalidFlakeSyntaxError(err, "")
	case CodeBuildFailed:
		return NewBuildFailedError(err)
	case CodeBuildTimeout:
		return NewBuildTimeoutError("unknown")
	case CodeBuildOOM:
		return NewBuildOOMError()
	case CodeVendorHashFailed:
		return NewVendorHashError(err)
	case CodeDockerfileNotFound:
		return NewDockerfileNotFoundError()
	case CodeFlakeNotFound:
		return NewFlakeNotFoundError()
	case CodeDependencyMissing:
		return NewDependencyMissingError(nil)
	case CodeNixpacksFailed:
		return NewNixpacksError(err)
	case CodeInvalidConfig:
		return NewInvalidConfigError("unknown", err.Error())
	default:
		return NewBuildError(err, code, CategoryBuild)
	}
}

// IsBuildError checks if an error is a BuildError.
func IsBuildError(err error) bool {
	var buildErr *BuildError
	return errors.As(err, &buildErr)
}

// AsBuildError attempts to convert an error to a BuildError.
func AsBuildError(err error) (*BuildError, bool) {
	var buildErr *BuildError
	if errors.As(err, &buildErr) {
		return buildErr, true
	}
	return nil, false
}

// ToErrorResponse converts any error to a BuildErrorResponse.
// If the error is already a BuildError, it uses its response.
// Otherwise, it creates a generic error response.
func ToErrorResponse(err error) *BuildErrorResponse {
	if buildErr, ok := AsBuildError(err); ok {
		return buildErr.ToResponse()
	}

	// Create a generic error response
	return &BuildErrorResponse{
		Error:    err.Error(),
		Code:     CodeBuildFailed,
		Category: CategoryBuild,
		NextSteps: []string{
			"Review the error message",
			"Check build logs for more details",
		},
	}
}
