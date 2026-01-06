package executor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/narvanalabs/control-plane/internal/builder/templates"
	"github.com/narvanalabs/control-plane/internal/models"
)

// DockerfileStrategyExecutor executes builds using an existing Dockerfile.
// This strategy always produces OCI images.
type DockerfileStrategyExecutor struct {
	templateEngine templates.TemplateEngine
	ociBuilder     OCIBuilder
	logger         *slog.Logger
}

// NewDockerfileStrategyExecutor creates a new DockerfileStrategyExecutor.
func NewDockerfileStrategyExecutor(
	tmplEngine templates.TemplateEngine,
	ociBuilder OCIBuilder,
	logger *slog.Logger,
) *DockerfileStrategyExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &DockerfileStrategyExecutor{
		templateEngine: tmplEngine,
		ociBuilder:     ociBuilder,
		logger:         logger,
	}
}

// Supports returns true if this executor handles the given strategy.
func (e *DockerfileStrategyExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyDockerfile
}

// GenerateFlake generates a flake.nix wrapper for Dockerfile builds.
// This uses nix2container to build OCI images from the Dockerfile.
func (e *DockerfileStrategyExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	// Prepare template data
	data := templates.TemplateData{
		AppName:         getDockerAppName(detection),
		StartCommand:    config.StartCommand,
		Config:          config,
		DetectionResult: detection,
	}

	// Render the template
	flakeContent, err := e.templateEngine.Render(ctx, "dockerfile.nix", data)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplateRenderFailed, err)
	}

	return flakeContent, nil
}

// Execute runs the build using the Dockerfile.
// Dockerfile strategy always produces OCI images.
func (e *DockerfileStrategyExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	e.logger.Info("executing dockerfile strategy",
		"job_id", job.ID,
	)

	// Dockerfile strategy always produces OCI images
	if job.BuildType != models.BuildTypeOCI {
		e.logger.Warn("dockerfile strategy requires OCI build type, overriding",
			"original_build_type", job.BuildType,
		)
		job.BuildType = models.BuildTypeOCI
	}

	var logs string
	logCallback := func(line string) {
		logs += line + "\n"
	}

	logCallback("=== Building from Dockerfile ===")
	logCallback("Build type: OCI (enforced for dockerfile strategy)")

	// If we don't have a generated flake yet, generate one
	if job.GeneratedFlake == "" {
		logCallback("=== Generating flake.nix wrapper ===")

		detection := &models.DetectionResult{
			Strategy:             models.BuildStrategyDockerfile,
			Framework:            models.FrameworkGeneric,
			RecommendedBuildType: models.BuildTypeOCI,
		}

		config := e.getConfigFromJob(job)
		flakeContent, err := e.GenerateFlake(ctx, detection, config)
		if err != nil {
			return &BuildResult{Logs: logs}, err
		}

		job.GeneratedFlake = flakeContent
		logCallback("Generated flake.nix wrapper successfully")
	}

	// Execute the OCI build
	return e.buildOCI(ctx, job, logCallback, &logs)
}

// getConfigFromJob extracts build config from the job.
func (e *DockerfileStrategyExecutor) getConfigFromJob(job *models.BuildJob) models.BuildConfig {
	if job.BuildConfig != nil {
		return *job.BuildConfig
	}
	return models.BuildConfig{}
}

// buildOCI executes an OCI build.
func (e *DockerfileStrategyExecutor) buildOCI(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
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

// getDockerAppName extracts the application name from detection result.
func getDockerAppName(detection *models.DetectionResult) string {
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

// ValidateDockerfileExists checks if a Dockerfile exists in the given repository path.
func ValidateDockerfileExists(repoPath string) error {
	dockerfilePath := filepath.Join(repoPath, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		return ErrDockerfileNotFound
	}
	return nil
}
