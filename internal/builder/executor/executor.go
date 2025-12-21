// Package executor provides strategy-specific build execution.
package executor

import (
	"context"
	"fmt"

	"github.com/narvanalabs/control-plane/internal/models"
)

// BuildResult represents the output of a completed build.
type BuildResult struct {
	Artifact  string `json:"artifact"`
	StorePath string `json:"store_path,omitempty"` // For pure-nix
	ImageTag  string `json:"image_tag,omitempty"`  // For OCI
	Logs      string `json:"logs"`
}

// StrategyExecutor executes a specific build strategy.
type StrategyExecutor interface {
	// Execute runs the build strategy.
	Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error)

	// Supports returns true if this executor handles the given strategy.
	Supports(strategy models.BuildStrategy) bool

	// GenerateFlake generates a flake.nix for this strategy (if applicable).
	// Returns empty string if the strategy doesn't generate flakes.
	GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error)
}

// ExecutorRegistry manages multiple strategy executors.
type ExecutorRegistry struct {
	executors []StrategyExecutor
}

// NewExecutorRegistry creates a new ExecutorRegistry.
func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{
		executors: make([]StrategyExecutor, 0),
	}
}

// Register adds an executor to the registry.
func (r *ExecutorRegistry) Register(executor StrategyExecutor) {
	r.executors = append(r.executors, executor)
}

// GetExecutor returns the executor that supports the given strategy.
func (r *ExecutorRegistry) GetExecutor(strategy models.BuildStrategy) (StrategyExecutor, error) {
	for _, executor := range r.executors {
		if executor.Supports(strategy) {
			return executor, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNoExecutorFound, strategy)
}
