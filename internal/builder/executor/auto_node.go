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

// AutoNodeStrategyExecutor executes builds for Node.js applications by generating a flake.nix.
type AutoNodeStrategyExecutor struct {
	detector       detector.Detector
	templateEngine templates.TemplateEngine
	hashCalculator hash.HashCalculator
	nixBuilder     NixBuilder
	ociBuilder     OCIBuilder
	logger         *slog.Logger
}

// NewAutoNodeStrategyExecutor creates a new AutoNodeStrategyExecutor.
func NewAutoNodeStrategyExecutor(
	det detector.Detector,
	tmplEngine templates.TemplateEngine,
	hashCalc hash.HashCalculator,
	nixBuilder NixBuilder,
	ociBuilder OCIBuilder,
	logger *slog.Logger,
) *AutoNodeStrategyExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &AutoNodeStrategyExecutor{
		detector:       det,
		templateEngine: tmplEngine,
		hashCalculator: hashCalc,
		nixBuilder:     nixBuilder,
		ociBuilder:     ociBuilder,
		logger:         logger,
	}
}

// Supports returns true if this executor handles the given strategy.
func (e *AutoNodeStrategyExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyAutoNode
}


// GenerateFlake generates a flake.nix for a Node.js application.
func (e *AutoNodeStrategyExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	// Determine template name based on framework
	templateName := "nodejs.nix"
	if detection.Framework == models.FrameworkNextJS || config.NextJSOptions != nil {
		templateName = "nextjs.nix"
	}

	// Prepare template data
	data := templates.TemplateData{
		AppName:         getNodeAppName(detection),
		Version:         detection.Version,
		Framework:       detection.Framework,
		BuildCommand:    config.BuildCommand,
		StartCommand:    config.StartCommand,
		Config:          config,
		DetectionResult: detection,
	}

	// Render the template
	flakeContent, err := e.templateEngine.Render(ctx, templateName, data)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplateRenderFailed, err)
	}

	return flakeContent, nil
}

// Execute runs the build for a Node.js application.
func (e *AutoNodeStrategyExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	e.logger.Info("executing auto-node strategy",
		"job_id", job.ID,
		"build_type", job.BuildType,
	)

	var logs string
	logCallback := func(line string) {
		logs += line + "\n"
	}

	// If we don't have a generated flake yet, we need to generate one
	if job.GeneratedFlake == "" {
		logCallback("=== Detecting Node.js application ===")

		detection, err := e.detectFromJob(ctx, job)
		if err != nil {
			return &BuildResult{Logs: logs}, fmt.Errorf("%w: %v", ErrDetectionFailed, err)
		}

		logCallback(fmt.Sprintf("Detected Node.js version: %s", detection.Version))
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

// detectFromJob performs Node.js detection based on job information.
func (e *AutoNodeStrategyExecutor) detectFromJob(ctx context.Context, job *models.BuildJob) (*models.DetectionResult, error) {
	// In a real implementation, we'd clone the repo and detect
	// For now, return a basic detection result
	return &models.DetectionResult{
		Strategy:             models.BuildStrategyAutoNode,
		Framework:            models.FrameworkGeneric,
		Version:              "20",
		Confidence:           0.9,
		RecommendedBuildType: models.BuildTypePureNix,
	}, nil
}

// getConfigFromJob extracts build config from the job.
func (e *AutoNodeStrategyExecutor) getConfigFromJob(job *models.BuildJob) models.BuildConfig {
	if job.BuildConfig != nil {
		return *job.BuildConfig
	}
	return models.BuildConfig{}
}

// buildPureNix executes a pure-nix build.
func (e *AutoNodeStrategyExecutor) buildPureNix(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
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
func (e *AutoNodeStrategyExecutor) buildOCI(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
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

// getNodeAppName extracts the application name from detection result.
func getNodeAppName(detection *models.DetectionResult) string {
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

// ValidatePackageJSONExists checks if a package.json exists in the given repository path.
func ValidatePackageJSONExists(repoPath string) error {
	packagePath := filepath.Join(repoPath, "package.json")
	if _, err := os.Stat(packagePath); os.IsNotExist(err) {
		return fmt.Errorf("package.json not found in %s", repoPath)
	}
	return nil
}
