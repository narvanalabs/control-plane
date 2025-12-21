package executor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/narvanalabs/control-plane/internal/builder/detector"
	"github.com/narvanalabs/control-plane/internal/builder/templates"
	"github.com/narvanalabs/control-plane/internal/models"
)

// AutoPythonStrategyExecutor executes builds for Python applications by generating a flake.nix.
type AutoPythonStrategyExecutor struct {
	detector       detector.Detector
	templateEngine templates.TemplateEngine
	nixBuilder     NixBuilder
	ociBuilder     OCIBuilder
	logger         *slog.Logger
}

// NewAutoPythonStrategyExecutor creates a new AutoPythonStrategyExecutor.
func NewAutoPythonStrategyExecutor(
	det detector.Detector,
	tmplEngine templates.TemplateEngine,
	nixBuilder NixBuilder,
	ociBuilder OCIBuilder,
	logger *slog.Logger,
) *AutoPythonStrategyExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &AutoPythonStrategyExecutor{
		detector:       det,
		templateEngine: tmplEngine,
		nixBuilder:     nixBuilder,
		ociBuilder:     ociBuilder,
		logger:         logger,
	}
}

// Supports returns true if this executor handles the given strategy.
func (e *AutoPythonStrategyExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyAutoPython
}


// GenerateFlake generates a flake.nix for a Python application.
func (e *AutoPythonStrategyExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	// Prepare template data
	data := templates.TemplateData{
		AppName:         getPythonAppName(detection),
		Version:         detection.Version,
		Framework:       detection.Framework,
		StartCommand:    config.StartCommand,
		Config:          config,
		DetectionResult: detection,
	}

	// Render the template
	flakeContent, err := e.templateEngine.Render(ctx, "python.nix", data)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplateRenderFailed, err)
	}

	return flakeContent, nil
}

// Execute runs the build for a Python application.
func (e *AutoPythonStrategyExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	e.logger.Info("executing auto-python strategy",
		"job_id", job.ID,
		"build_type", job.BuildType,
	)

	var logs string
	logCallback := func(line string) {
		logs += line + "\n"
	}

	// If we don't have a generated flake yet, we need to generate one
	if job.GeneratedFlake == "" {
		logCallback("=== Detecting Python application ===")

		detection, err := e.detectFromJob(ctx, job)
		if err != nil {
			return &BuildResult{Logs: logs}, fmt.Errorf("%w: %v", ErrDetectionFailed, err)
		}

		logCallback(fmt.Sprintf("Detected Python version: %s", detection.Version))
		logCallback(fmt.Sprintf("Framework: %s", detection.Framework))

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

// detectFromJob performs Python detection based on job information.
func (e *AutoPythonStrategyExecutor) detectFromJob(ctx context.Context, job *models.BuildJob) (*models.DetectionResult, error) {
	// In a real implementation, we'd clone the repo and detect
	// For now, return a basic detection result
	return &models.DetectionResult{
		Strategy:             models.BuildStrategyAutoPython,
		Framework:            models.FrameworkGeneric,
		Version:              "3.11",
		Confidence:           0.9,
		RecommendedBuildType: models.BuildTypePureNix,
	}, nil
}

// getConfigFromJob extracts build config from the job.
func (e *AutoPythonStrategyExecutor) getConfigFromJob(job *models.BuildJob) models.BuildConfig {
	if job.BuildConfig != nil {
		return *job.BuildConfig
	}
	return models.BuildConfig{}
}

// buildPureNix executes a pure-nix build.
func (e *AutoPythonStrategyExecutor) buildPureNix(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
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
func (e *AutoPythonStrategyExecutor) buildOCI(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
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

// getPythonAppName extracts the application name from detection result.
func getPythonAppName(detection *models.DetectionResult) string {
	if detection == nil {
		return "app"
	}
	// Check suggested config for name
	if detection.SuggestedConfig != nil {
		if name, ok := detection.SuggestedConfig["name"].(string); ok && name != "" {
			return name
		}
	}
	return "app"
}

// ValidatePythonProjectExists checks if Python project files exist in the given repository path.
func ValidatePythonProjectExists(repoPath string) error {
	// Check for requirements.txt
	reqPath := filepath.Join(repoPath, "requirements.txt")
	if _, err := os.Stat(reqPath); err == nil {
		return nil
	}

	// Check for pyproject.toml
	pyprojectPath := filepath.Join(repoPath, "pyproject.toml")
	if _, err := os.Stat(pyprojectPath); err == nil {
		return nil
	}

	// Check for setup.py
	setupPath := filepath.Join(repoPath, "setup.py")
	if _, err := os.Stat(setupPath); err == nil {
		return nil
	}

	return fmt.Errorf("no Python project files found in %s (requirements.txt, pyproject.toml, or setup.py)", repoPath)
}
