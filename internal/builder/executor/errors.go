package executor

import "errors"

// Executor errors.
var (
	// ErrNoExecutorFound is returned when no executor supports the given strategy.
	ErrNoExecutorFound = errors.New("no executor found for strategy")

	// ErrFlakeNotFound is returned when a flake.nix is required but not found.
	ErrFlakeNotFound = errors.New("flake.nix not found in repository")

	// ErrDockerfileNotFound is returned when a Dockerfile is required but not found.
	ErrDockerfileNotFound = errors.New("Dockerfile not found in repository")

	// ErrBuildFailed is returned when the build process fails.
	ErrBuildFailed = errors.New("build failed")

	// ErrDetectionFailed is returned when language/framework detection fails.
	ErrDetectionFailed = errors.New("detection failed")

	// ErrTemplateRenderFailed is returned when template rendering fails.
	ErrTemplateRenderFailed = errors.New("template rendering failed")

	// ErrHashCalculationFailed is returned when vendor hash calculation fails.
	ErrHashCalculationFailed = errors.New("hash calculation failed")

	// ErrNixpacksFailed is returned when Nixpacks build fails.
	ErrNixpacksFailed = errors.New("nixpacks build failed")

	// ErrUnsupportedStrategy is returned when a strategy is not supported.
	ErrUnsupportedStrategy = errors.New("unsupported build strategy")
)
