package executor

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/narvanalabs/control-plane/internal/models"
)

// NixpacksStrategyExecutor executes builds using Nixpacks.
// This strategy always produces OCI images.
type NixpacksStrategyExecutor struct {
	ociBuilder OCIBuilder
	logger     *slog.Logger
}

// NewNixpacksStrategyExecutor creates a new NixpacksStrategyExecutor.
func NewNixpacksStrategyExecutor(
	ociBuilder OCIBuilder,
	logger *slog.Logger,
) *NixpacksStrategyExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &NixpacksStrategyExecutor{
		ociBuilder: ociBuilder,
		logger:     logger,
	}
}

// Supports returns true if this executor handles the given strategy.
func (e *NixpacksStrategyExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyNixpacks
}

// GenerateFlake returns empty string as Nixpacks doesn't use flakes.
// Nixpacks generates its own build plan.
func (e *NixpacksStrategyExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	// Nixpacks doesn't use flakes - it generates its own build plan
	return "", nil
}

// Execute runs the build using Nixpacks.
// Nixpacks strategy always produces OCI images.
func (e *NixpacksStrategyExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	e.logger.Info("executing nixpacks strategy",
		"job_id", job.ID,
	)

	// Nixpacks strategy always produces OCI images
	if job.BuildType != models.BuildTypeOCI {
		e.logger.Warn("nixpacks strategy requires OCI build type, overriding",
			"original_build_type", job.BuildType,
		)
		job.BuildType = models.BuildTypeOCI
	}

	var logs string
	logCallback := func(line string) {
		logs += line + "\n"
	}

	logCallback("=== Building with Nixpacks ===")
	logCallback("Build type: OCI (enforced for nixpacks strategy)")

	// Generate the build plan first
	logCallback("=== Generating Nixpacks build plan ===")
	plan, err := e.generateBuildPlan(ctx, job)
	if err != nil {
		logCallback(fmt.Sprintf("Failed to generate build plan: %v", err))
		return &BuildResult{Logs: logs}, fmt.Errorf("%w: %v", ErrNixpacksFailed, err)
	}
	logCallback(fmt.Sprintf("Build plan generated:\n%s", plan))

	// Execute the Nixpacks build
	logCallback("=== Executing Nixpacks build ===")
	imageTag, buildLogs, err := e.runNixpacksBuild(ctx, job)
	logs += buildLogs

	if err != nil {
		return &BuildResult{Logs: logs}, fmt.Errorf("%w: %v", ErrNixpacksFailed, err)
	}

	logCallback(fmt.Sprintf("Successfully built image: %s", imageTag))

	return &BuildResult{
		Artifact: imageTag,
		ImageTag: imageTag,
		Logs:     logs,
	}, nil
}

// generateBuildPlan generates a Nixpacks build plan for the repository.
func (e *NixpacksStrategyExecutor) generateBuildPlan(ctx context.Context, job *models.BuildJob) (string, error) {
	// Build the nixpacks plan command
	args := []string{"plan", job.GitURL}

	// Add any custom configuration from build config
	if job.BuildConfig != nil {
		if job.BuildConfig.BuildCommand != "" {
			args = append(args, "--build-cmd", job.BuildConfig.BuildCommand)
		}
		if job.BuildConfig.StartCommand != "" {
			args = append(args, "--start-cmd", job.BuildConfig.StartCommand)
		}
	}

	cmd := exec.CommandContext(ctx, "nixpacks", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("nixpacks plan failed: %s: %w", stderr.String(), err)
	}

	return stdout.String(), nil
}

// runNixpacksBuild executes the Nixpacks build and returns the image tag.
func (e *NixpacksStrategyExecutor) runNixpacksBuild(ctx context.Context, job *models.BuildJob) (string, string, error) {
	// Generate image name
	imageName := sanitizeImageName(job.AppID)
	imageTag := fmt.Sprintf("%s:%s", imageName, job.DeploymentID)
	if job.DeploymentID == "" {
		imageTag = fmt.Sprintf("%s:%s", imageName, job.ID)
	}

	// Build the nixpacks build command
	args := []string{"build", job.GitURL, "--name", imageTag}

	// Add any custom configuration from build config
	if job.BuildConfig != nil {
		if job.BuildConfig.BuildCommand != "" {
			args = append(args, "--build-cmd", job.BuildConfig.BuildCommand)
		}
		if job.BuildConfig.StartCommand != "" {
			args = append(args, "--start-cmd", job.BuildConfig.StartCommand)
		}
		// Add environment variables
		for key, value := range job.BuildConfig.EnvironmentVars {
			args = append(args, "--env", fmt.Sprintf("%s=%s", key, value))
		}
	}

	cmd := exec.CommandContext(ctx, "nixpacks", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		logs := stdout.String() + "\n" + stderr.String()
		return "", logs, fmt.Errorf("nixpacks build failed: %w", err)
	}

	logs := stdout.String() + "\n" + stderr.String()
	return imageTag, logs, nil
}

// sanitizeImageName converts a string to a valid Docker image name.
func sanitizeImageName(name string) string {
	name = strings.ToLower(name)
	// Replace invalid characters with dashes
	var result strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '.' || ch == '_' || ch == '-' || ch == '/' {
			result.WriteRune(ch)
		} else {
			result.WriteRune('-')
		}
	}
	// Remove leading/trailing dashes
	return strings.Trim(result.String(), "-")
}

// IsNixpacksAvailable checks if nixpacks is installed and available.
func IsNixpacksAvailable() bool {
	cmd := exec.Command("nixpacks", "--version")
	return cmd.Run() == nil
}
