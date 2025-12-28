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

// AutoRustStrategyExecutor executes builds for Rust applications by generating a flake.nix.
type AutoRustStrategyExecutor struct {
	detector       detector.Detector
	templateEngine templates.TemplateEngine
	hashCalculator hash.HashCalculator
	nixBuilder     NixBuilder
	ociBuilder     OCIBuilder
	logger         *slog.Logger
}

// NewAutoRustStrategyExecutor creates a new AutoRustStrategyExecutor.
func NewAutoRustStrategyExecutor(
	det detector.Detector,
	tmplEngine templates.TemplateEngine,
	hashCalc hash.HashCalculator,
	nixBuilder NixBuilder,
	ociBuilder OCIBuilder,
	logger *slog.Logger,
) *AutoRustStrategyExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &AutoRustStrategyExecutor{
		detector:       det,
		templateEngine: tmplEngine,
		hashCalculator: hashCalc,
		nixBuilder:     nixBuilder,
		ociBuilder:     ociBuilder,
		logger:         logger,
	}
}

// Supports returns true if this executor handles the given strategy.
func (e *AutoRustStrategyExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyAutoRust
}


// GenerateFlake generates a flake.nix for a Rust application using crane.
func (e *AutoRustStrategyExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	// Prepare template data
	data := templates.TemplateData{
		AppName:         getRustAppName(detection),
		Version:         detection.Version,
		Framework:       detection.Framework,
		EntryPoint:      config.EntryPoint,
		Config:          config,
		DetectionResult: detection,
	}

	// Render the template
	flakeContent, err := e.templateEngine.Render(ctx, "rust.nix", data)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplateRenderFailed, err)
	}

	return flakeContent, nil
}

// Execute runs the build for a Rust application (without external log streaming).
func (e *AutoRustStrategyExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	return e.ExecuteWithLogs(ctx, job, nil)
}

// ExecuteWithLogs runs the build for a Rust application with real-time log streaming.
func (e *AutoRustStrategyExecutor) ExecuteWithLogs(ctx context.Context, job *models.BuildJob, externalCallback LogCallback) (*BuildResult, error) {
	e.logger.Info("executing auto-rust strategy",
		"job_id", job.ID,
		"build_type", job.BuildType,
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
		logCallback("=== Detecting Rust application ===")

		detection, err := e.detectFromJob(ctx, job)
		if err != nil {
			return &BuildResult{Logs: logs}, fmt.Errorf("%w: %v", ErrDetectionFailed, err)
		}

		logCallback(fmt.Sprintf("Detected Rust edition: %s", detection.Version))

		// Generate the flake
		logCallback("=== Generating flake.nix ===")
		config := e.getConfigFromJob(job)
		flakeContent, err := e.GenerateFlake(ctx, detection, config)
		if err != nil {
			return &BuildResult{Logs: logs}, err
		}

		job.GeneratedFlake = flakeContent
		logCallback("Generated flake.nix successfully")
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

// detectFromJob performs Rust detection based on job information.
func (e *AutoRustStrategyExecutor) detectFromJob(ctx context.Context, job *models.BuildJob) (*models.DetectionResult, error) {
	// In a real implementation, we'd clone the repo and detect
	// For now, return a basic detection result
	return &models.DetectionResult{
		Strategy:             models.BuildStrategyAutoRust,
		Framework:            models.FrameworkGeneric,
		Version:              "2021", // Rust edition
		Confidence:           0.9,
		RecommendedBuildType: models.BuildTypePureNix,
	}, nil
}

// getConfigFromJob extracts build config from the job.
func (e *AutoRustStrategyExecutor) getConfigFromJob(job *models.BuildJob) models.BuildConfig {
	if job.BuildConfig != nil {
		return *job.BuildConfig
	}
	return models.BuildConfig{}
}

// buildPureNix executes a pure-nix build.
func (e *AutoRustStrategyExecutor) buildPureNix(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
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
func (e *AutoRustStrategyExecutor) buildOCI(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
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

// getRustAppName extracts the application name from detection result.
func getRustAppName(detection *models.DetectionResult) string {
	if detection == nil {
		return "app"
	}
	// Check suggested config for name (from Cargo.toml)
	if detection.SuggestedConfig != nil {
		if name, ok := detection.SuggestedConfig["name"].(string); ok && name != "" {
			return name
		}
	}
	return "app"
}

// ValidateCargoTomlExists checks if a Cargo.toml exists in the given repository path.
func ValidateCargoTomlExists(repoPath string) error {
	cargoPath := filepath.Join(repoPath, "Cargo.toml")
	if _, err := os.Stat(cargoPath); os.IsNotExist(err) {
		return fmt.Errorf("Cargo.toml not found in %s", repoPath)
	}
	return nil
}
