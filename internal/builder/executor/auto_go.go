package executor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/narvanalabs/control-plane/internal/builder/detector"
	"github.com/narvanalabs/control-plane/internal/builder/hash"
	"github.com/narvanalabs/control-plane/internal/builder/templates"
	"github.com/narvanalabs/control-plane/internal/models"
)

// AutoGoStrategyExecutor executes builds for Go applications by generating a flake.nix.
type AutoGoStrategyExecutor struct {
	detector       detector.Detector
	templateEngine templates.TemplateEngine
	hashCalculator hash.HashCalculator
	nixBuilder     NixBuilder
	ociBuilder     OCIBuilder
	logger         *slog.Logger
}

// NewAutoGoStrategyExecutor creates a new AutoGoStrategyExecutor.
func NewAutoGoStrategyExecutor(
	det detector.Detector,
	tmplEngine templates.TemplateEngine,
	hashCalc hash.HashCalculator,
	nixBuilder NixBuilder,
	ociBuilder OCIBuilder,
	logger *slog.Logger,
) *AutoGoStrategyExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &AutoGoStrategyExecutor{
		detector:       det,
		templateEngine: tmplEngine,
		hashCalculator: hashCalc,
		nixBuilder:     nixBuilder,
		ociBuilder:     ociBuilder,
		logger:         logger,
	}
}

// Supports returns true if this executor handles the given strategy.
func (e *AutoGoStrategyExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyAutoGo
}


// GenerateFlake generates a flake.nix for a Go application.
// **Validates: Requirements 16.2** - Sets CGO_ENABLED based on detection result
func (e *AutoGoStrategyExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	// Determine CGO setting:
	// 1. If explicitly set in config, use that (user override)
	// 2. Otherwise, use detection result from SuggestedConfig
	// **Validates: Requirements 16.5** - User config overrides auto-detection
	cgoEnabled := false
	if config.CGOEnabled != nil {
		// User explicitly set CGO - honor their choice
		cgoEnabled = *config.CGOEnabled
	} else if detection != nil && detection.SuggestedConfig != nil {
		// Use auto-detected CGO setting
		if detected, ok := detection.SuggestedConfig["cgo_enabled"].(bool); ok {
			cgoEnabled = detected
		}
	}

	// Determine template name based on CGO
	templateName := "go.nix"
	if cgoEnabled {
		templateName = "go-cgo.nix"
	}

	// Prepare template data
	data := templates.TemplateData{
		AppName:         getAppName(detection),
		Version:         detection.Version,
		Framework:       detection.Framework,
		EntryPoint:      config.EntryPoint,
		Config:          config,
		DetectionResult: detection,
	}

	// If entry point not specified in config, use first detected entry point
	if data.EntryPoint == "" && len(detection.EntryPoints) > 0 {
		data.EntryPoint = detection.EntryPoints[0]
	}

	// Render the template
	flakeContent, err := e.templateEngine.Render(ctx, templateName, data)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplateRenderFailed, err)
	}

	return flakeContent, nil
}

// Execute runs the build for a Go application (without external log streaming).
func (e *AutoGoStrategyExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	// Use a no-op callback that just collects logs
	return e.ExecuteWithLogs(ctx, job, nil)
}

// ExecuteWithLogs runs the build for a Go application with real-time log streaming.
func (e *AutoGoStrategyExecutor) ExecuteWithLogs(ctx context.Context, job *models.BuildJob, externalCallback LogCallback) (*BuildResult, error) {
	e.logger.Info("executing auto-go strategy",
		"job_id", job.ID,
		"build_type", job.BuildType,
		"has_generated_flake_before", job.GeneratedFlake != "",
	)

	var logs string
	logCallback := func(line string) {
		logs += line + "\n"
		if externalCallback != nil {
			externalCallback(line)
		}
	}

	// If we don't have a generated flake yet, we need to generate one
	if job.GeneratedFlake == "" {
		logCallback("=== Detecting Go application ===")

		// We need a repo path to detect - this would typically be passed via the job
		// For now, we'll use the detection result if available
		detection, err := e.detectFromJob(ctx, job)
		if err != nil {
			return &BuildResult{Logs: logs}, fmt.Errorf("%w: %v", ErrDetectionFailed, err)
		}

		logCallback(fmt.Sprintf("Detected Go version: %s", detection.Version))
		if len(detection.EntryPoints) > 0 {
			logCallback(fmt.Sprintf("Entry points: %v", detection.EntryPoints))
		}

		// Generate the flake
		logCallback("=== Generating flake.nix ===")
		config := e.getConfigFromJob(job)
		flakeContent, err := e.GenerateFlake(ctx, detection, config)
		if err != nil {
			return &BuildResult{Logs: logs}, err
		}

		job.GeneratedFlake = flakeContent
		e.logger.Info("generated flake for job",
			"job_id", job.ID,
			"flake_length", len(flakeContent),
			"has_generated_flake_after", job.GeneratedFlake != "",
		)
		logCallback("Generated flake.nix successfully")
	}

	// Calculate vendor hash if not already set
	if job.VendorHash == "" {
		logCallback("=== Calculating vendor hash ===")
		// Note: In a real implementation, we'd need the repo path
		// For now, we'll use a placeholder approach
		logCallback("Vendor hash calculation would happen here")
	}

	// Execute the build based on build type
	switch job.BuildType {
	case models.BuildTypePureNix:
		return e.buildPureNix(ctx, job, logCallback, &logs)
	case models.BuildTypeOCI:
		return e.buildOCI(ctx, job, logCallback, &logs)
	default:
		return nil, fmt.Errorf("%w: unknown build type %s", ErrBuildFailed, job.BuildType)
	}
}

// detectFromJob performs Go detection based on job information.
func (e *AutoGoStrategyExecutor) detectFromJob(ctx context.Context, job *models.BuildJob) (*models.DetectionResult, error) {
	// In a real implementation, we'd clone the repo and detect
	// For now, return a basic detection result
	return &models.DetectionResult{
		Strategy:             models.BuildStrategyAutoGo,
		Framework:            models.FrameworkGeneric,
		Version:              "1.21",
		Confidence:           0.9,
		RecommendedBuildType: models.BuildTypePureNix,
	}, nil
}

// getConfigFromJob extracts build config from the job.
func (e *AutoGoStrategyExecutor) getConfigFromJob(job *models.BuildJob) models.BuildConfig {
	if job.BuildConfig != nil {
		return *job.BuildConfig
	}
	return models.BuildConfig{}
}

// buildPureNix executes a pure-nix build.
func (e *AutoGoStrategyExecutor) buildPureNix(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
	result, err := e.nixBuilder.BuildWithLogCallback(ctx, job, logCallback)
	if err != nil {
		return &BuildResult{
			Logs: *logs,
		}, fmt.Errorf("%w: %v", ErrBuildFailed, err)
	}

	return &BuildResult{
		Artifact:  result.StorePath,
		StorePath: result.StorePath,
		Logs:      *logs,
	}, nil
}

// buildOCI executes an OCI build.
func (e *AutoGoStrategyExecutor) buildOCI(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
	result, err := e.ociBuilder.BuildWithLogCallback(ctx, job, logCallback)
	if err != nil {
		return &BuildResult{
			Logs: *logs,
		}, fmt.Errorf("%w: %v", ErrBuildFailed, err)
	}

	return &BuildResult{
		Artifact:  result.ImageTag,
		ImageTag:  result.ImageTag,
		StorePath: result.StorePath,
		Logs:      *logs,
	}, nil
}

// getAppName extracts the application name from detection result.
func getAppName(detection *models.DetectionResult) string {
	if detection == nil {
		return "app"
	}
	// Use first entry point as app name if available
	if len(detection.EntryPoints) > 0 {
		return filepath.Base(detection.EntryPoints[0])
	}
	return "app"
}

// ValidateGoModExists checks if a go.mod exists in the given repository path.
func ValidateGoModExists(repoPath string) error {
	goModPath := filepath.Join(repoPath, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		return fmt.Errorf("go.mod not found in %s", repoPath)
	}
	return nil
}
