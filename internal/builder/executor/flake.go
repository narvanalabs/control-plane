package executor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/narvanalabs/control-plane/internal/models"
)

// FlakeStrategyExecutor executes builds using an existing flake.nix from the repository.
type FlakeStrategyExecutor struct {
	nixBuilder NixBuilder
	ociBuilder OCIBuilder
	logger     *slog.Logger
}

// NixBuilder interface for building pure-nix artifacts.
type NixBuilder interface {
	BuildWithLogCallback(ctx context.Context, job *models.BuildJob, callback func(line string)) (*NixBuildResult, error)
}

// NixBuildResult holds the result of a Nix build.
type NixBuildResult struct {
	StorePath string
	Logs      string
	ExitCode  int
}

// OCIBuilder interface for building OCI images.
type OCIBuilder interface {
	BuildWithLogCallback(ctx context.Context, job *models.BuildJob, callback func(line string)) (*OCIBuildResult, error)
}

// OCIBuildResult holds the result of an OCI build.
type OCIBuildResult struct {
	ImageTag  string
	StorePath string
	Logs      string
	ExitCode  int
}

// NewFlakeStrategyExecutor creates a new FlakeStrategyExecutor.
func NewFlakeStrategyExecutor(nixBuilder NixBuilder, ociBuilder OCIBuilder, logger *slog.Logger) *FlakeStrategyExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &FlakeStrategyExecutor{
		nixBuilder: nixBuilder,
		ociBuilder: ociBuilder,
		logger:     logger,
	}
}

// Supports returns true if this executor handles the given strategy.
func (e *FlakeStrategyExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyFlake
}

// GenerateFlake returns empty string as this strategy uses existing flake.nix.
func (e *FlakeStrategyExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	// Flake strategy uses existing flake.nix, no generation needed
	return "", nil
}

// Execute runs the build using the existing flake.nix from the repository (without external log streaming).
func (e *FlakeStrategyExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	return e.ExecuteWithLogs(ctx, job, nil)
}

// ExecuteWithLogs runs the build using the existing flake.nix with real-time log streaming.
func (e *FlakeStrategyExecutor) ExecuteWithLogs(ctx context.Context, job *models.BuildJob, externalCallback LogCallback) (*BuildResult, error) {
	e.logger.Info("executing flake strategy",
		"job_id", job.ID,
		"build_type", job.BuildType,
	)

	// Create a log collector
	var logs string
	logCallback := func(line string) {
		logs += line + "\n"
		if externalCallback != nil {
			externalCallback(line)
		}
	}

	// Execute based on build type
	switch job.BuildType {
	case models.BuildTypePureNix:
		return e.buildPureNix(ctx, job, logCallback, &logs)
	case models.BuildTypeOCI:
		return e.buildOCI(ctx, job, logCallback, &logs)
	default:
		return nil, fmt.Errorf("%w: unknown build type %s", ErrBuildFailed, job.BuildType)
	}
}

// buildPureNix executes a pure-nix build.
func (e *FlakeStrategyExecutor) buildPureNix(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
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
func (e *FlakeStrategyExecutor) buildOCI(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
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

// ValidateFlakeExists checks if a flake.nix exists in the given repository path.
func ValidateFlakeExists(repoPath string) error {
	flakePath := filepath.Join(repoPath, "flake.nix")
	if _, err := os.Stat(flakePath); os.IsNotExist(err) {
		return ErrFlakeNotFound
	}
	return nil
}
