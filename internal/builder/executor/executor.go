// Package executor provides strategy-specific build execution.
package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/narvanalabs/control-plane/internal/models"
)

// RequiredStrategies lists strategies that MUST have executors registered.
// These are the core strategies that the build system must support.
// **Validates: Requirements 2.1**
var RequiredStrategies = []models.BuildStrategy{
	models.BuildStrategyFlake,
	models.BuildStrategyAutoGo,
	models.BuildStrategyAutoNode,
	models.BuildStrategyAutoRust,
	models.BuildStrategyAutoPython,
}

// BuildResult represents the output of a completed build.
type BuildResult struct {
	Artifact  string `json:"artifact"`
	StorePath string `json:"store_path,omitempty"` // For pure-nix
	ImageTag  string `json:"image_tag,omitempty"`  // For OCI
	Logs      string `json:"logs"`
}

// LogCallback is a function that receives log lines during build execution.
type LogCallback func(line string)

// StrategyExecutor executes a specific build strategy.
type StrategyExecutor interface {
	// Execute runs the build strategy.
	Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error)

	// ExecuteWithLogs runs the build strategy with real-time log streaming.
	// The logCallback will be invoked for each log line produced during the build.
	ExecuteWithLogs(ctx context.Context, job *models.BuildJob, logCallback LogCallback) (*BuildResult, error)

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

// VerifyRequiredExecutors checks that all required strategies have registered executors.
// Returns an error listing any missing executors.
// **Validates: Requirements 2.1, 2.2**
func (r *ExecutorRegistry) VerifyRequiredExecutors() error {
	var missing []string
	
	for _, strategy := range RequiredStrategies {
		_, err := r.GetExecutor(strategy)
		if err != nil {
			missing = append(missing, string(strategy))
		}
	}
	
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing executors for strategies: %s", 
			ErrMissingRequiredExecutors, strings.Join(missing, ", "))
	}
	
	return nil
}

// GetRegisteredStrategies returns a list of all strategies that have registered executors.
func (r *ExecutorRegistry) GetRegisteredStrategies() []models.BuildStrategy {
	var strategies []models.BuildStrategy
	seen := make(map[models.BuildStrategy]bool)
	
	for _, executor := range r.executors {
		for _, strategy := range models.ValidBuildStrategies() {
			if executor.Supports(strategy) && !seen[strategy] {
				strategies = append(strategies, strategy)
				seen[strategy] = true
			}
		}
	}
	
	return strategies
}

// VerifyStrategyExecutorMapping verifies that GetExecutor returns the correct executor type
// for each strategy. It checks that the returned executor actually supports the requested strategy.
// **Validates: Requirements 5.1, 6.1, 7.1, 8.1, 9.1, 10.1, 11.1**
func (r *ExecutorRegistry) VerifyStrategyExecutorMapping(strategy models.BuildStrategy) error {
	executor, err := r.GetExecutor(strategy)
	if err != nil {
		return fmt.Errorf("no executor found for strategy %s: %w", strategy, err)
	}
	
	// Verify the executor actually supports the strategy
	if !executor.Supports(strategy) {
		return fmt.Errorf("executor returned for strategy %s does not support that strategy", strategy)
	}
	
	return nil
}

// VerifyAllStrategyMappings verifies that all valid strategies have correct executor mappings.
// Returns a map of strategy to error for any strategies that fail verification.
// **Validates: Requirements 5.1, 6.1, 7.1, 8.1, 9.1, 10.1, 11.1**
func (r *ExecutorRegistry) VerifyAllStrategyMappings() map[models.BuildStrategy]error {
	results := make(map[models.BuildStrategy]error)
	
	for _, strategy := range models.ValidBuildStrategies() {
		if err := r.VerifyStrategyExecutorMapping(strategy); err != nil {
			results[strategy] = err
		}
	}
	
	return results
}
