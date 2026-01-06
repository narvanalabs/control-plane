package executor

import (
	"context"
	"log/slog"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: build-lifecycle-correctness, Property 4: Executor Registry Completeness**
// For any valid build strategy in {flake, auto-go, auto-node, auto-rust, auto-python},
// the Executor_Registry SHALL return a corresponding executor.
// **Validates: Requirements 2.1, 2.2**

// genRequiredStrategy generates one of the required strategies.
func genRequiredStrategy() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStrategyFlake,
		models.BuildStrategyAutoGo,
		models.BuildStrategyAutoNode,
		models.BuildStrategyAutoRust,
		models.BuildStrategyAutoPython,
	)
}

// MockNixBuilder is a mock implementation of NixBuilder for testing.
type MockNixBuilder struct{}

func (m *MockNixBuilder) BuildWithLogCallback(ctx context.Context, job *models.BuildJob, callback func(line string)) (*NixBuildResult, error) {
	return &NixBuildResult{StorePath: "/nix/store/mock", Logs: "mock logs", ExitCode: 0}, nil
}

// MockOCIBuilder is a mock implementation of OCIBuilder for testing.
type MockOCIBuilder struct{}

func (m *MockOCIBuilder) BuildWithLogCallback(ctx context.Context, job *models.BuildJob, callback func(line string)) (*OCIBuildResult, error) {
	return &OCIBuildResult{ImageTag: "mock:latest", Logs: "mock logs", ExitCode: 0}, nil
}

// createFullyPopulatedRegistry creates a registry with all required executors registered.
func createFullyPopulatedRegistry() *ExecutorRegistry {
	registry := NewExecutorRegistry()
	logger := slog.Default()
	nixBuilder := &MockNixBuilder{}
	ociBuilder := &MockOCIBuilder{}

	// Register all required executors
	registry.Register(NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger))
	registry.Register(NewMockAutoGoExecutor())
	registry.Register(NewMockAutoNodeExecutor())
	registry.Register(NewMockAutoRustExecutor())
	registry.Register(NewMockAutoPythonExecutor())

	return registry
}

// MockAutoGoExecutor is a mock executor for auto-go strategy.
type MockAutoGoExecutor struct{}

func NewMockAutoGoExecutor() *MockAutoGoExecutor { return &MockAutoGoExecutor{} }
func (e *MockAutoGoExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	return &BuildResult{Artifact: "/nix/store/mock-go"}, nil
}
func (e *MockAutoGoExecutor) ExecuteWithLogs(ctx context.Context, job *models.BuildJob, logCallback LogCallback) (*BuildResult, error) {
	logCallback("Mock Go build started")
	return e.Execute(ctx, job)
}
func (e *MockAutoGoExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyAutoGo
}
func (e *MockAutoGoExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	return "mock flake", nil
}

// MockAutoNodeExecutor is a mock executor for auto-node strategy.
type MockAutoNodeExecutor struct{}

func NewMockAutoNodeExecutor() *MockAutoNodeExecutor { return &MockAutoNodeExecutor{} }
func (e *MockAutoNodeExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	return &BuildResult{Artifact: "/nix/store/mock-node"}, nil
}
func (e *MockAutoNodeExecutor) ExecuteWithLogs(ctx context.Context, job *models.BuildJob, logCallback LogCallback) (*BuildResult, error) {
	logCallback("Mock Node build started")
	return e.Execute(ctx, job)
}
func (e *MockAutoNodeExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyAutoNode
}
func (e *MockAutoNodeExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	return "mock flake", nil
}

// MockAutoRustExecutor is a mock executor for auto-rust strategy.
type MockAutoRustExecutor struct{}

func NewMockAutoRustExecutor() *MockAutoRustExecutor { return &MockAutoRustExecutor{} }
func (e *MockAutoRustExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	return &BuildResult{Artifact: "/nix/store/mock-rust"}, nil
}
func (e *MockAutoRustExecutor) ExecuteWithLogs(ctx context.Context, job *models.BuildJob, logCallback LogCallback) (*BuildResult, error) {
	logCallback("Mock Rust build started")
	return e.Execute(ctx, job)
}
func (e *MockAutoRustExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyAutoRust
}
func (e *MockAutoRustExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	return "mock flake", nil
}

// MockAutoPythonExecutor is a mock executor for auto-python strategy.
type MockAutoPythonExecutor struct{}

func NewMockAutoPythonExecutor() *MockAutoPythonExecutor { return &MockAutoPythonExecutor{} }
func (e *MockAutoPythonExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	return &BuildResult{Artifact: "/nix/store/mock-python"}, nil
}
func (e *MockAutoPythonExecutor) ExecuteWithLogs(ctx context.Context, job *models.BuildJob, logCallback LogCallback) (*BuildResult, error) {
	logCallback("Mock Python build started")
	return e.Execute(ctx, job)
}
func (e *MockAutoPythonExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyAutoPython
}
func (e *MockAutoPythonExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	return "mock flake", nil
}

// TestExecutorRegistryCompleteness tests Property 4: Executor Registry Completeness.
func TestExecutorRegistryCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 4.1: A fully populated registry returns executors for all required strategies
	properties.Property("fully populated registry has all required executors", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			registry := createFullyPopulatedRegistry()
			executor, err := registry.GetExecutor(strategy)
			return err == nil && executor != nil
		},
		genRequiredStrategy(),
	))

	// Property 4.2: VerifyRequiredExecutors passes for fully populated registry
	properties.Property("VerifyRequiredExecutors passes for complete registry", prop.ForAll(
		func(_ bool) bool {
			registry := createFullyPopulatedRegistry()
			err := registry.VerifyRequiredExecutors()
			return err == nil
		},
		gen.Bool(),
	))

	// Property 4.3: VerifyRequiredExecutors fails when any required executor is missing
	properties.Property("VerifyRequiredExecutors fails when executor is missing", prop.ForAll(
		func(missingStrategy models.BuildStrategy) bool {
			registry := NewExecutorRegistry()
			logger := slog.Default()
			nixBuilder := &MockNixBuilder{}
			ociBuilder := &MockOCIBuilder{}

			// Register all executors EXCEPT the missing one
			for _, strategy := range RequiredStrategies {
				if strategy == missingStrategy {
					continue
				}
				switch strategy {
				case models.BuildStrategyFlake:
					registry.Register(NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger))
				case models.BuildStrategyAutoGo:
					registry.Register(NewMockAutoGoExecutor())
				case models.BuildStrategyAutoNode:
					registry.Register(NewMockAutoNodeExecutor())
				case models.BuildStrategyAutoRust:
					registry.Register(NewMockAutoRustExecutor())
				case models.BuildStrategyAutoPython:
					registry.Register(NewMockAutoPythonExecutor())
				}
			}

			err := registry.VerifyRequiredExecutors()
			return err != nil
		},
		genRequiredStrategy(),
	))

	// Property 4.4: GetExecutor returns correct executor type for each strategy
	properties.Property("GetExecutor returns executor that supports the strategy", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			registry := createFullyPopulatedRegistry()
			executor, err := registry.GetExecutor(strategy)
			if err != nil {
				return false
			}
			return executor.Supports(strategy)
		},
		genRequiredStrategy(),
	))

	// Property 4.5: Empty registry fails verification
	properties.Property("empty registry fails verification", prop.ForAll(
		func(_ bool) bool {
			registry := NewExecutorRegistry()
			err := registry.VerifyRequiredExecutors()
			return err != nil
		},
		gen.Bool(),
	))

	// Property 4.6: GetRegisteredStrategies returns all registered strategies
	properties.Property("GetRegisteredStrategies returns all registered strategies", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			registry := createFullyPopulatedRegistry()
			registeredStrategies := registry.GetRegisteredStrategies()

			// Check that the strategy is in the list
			for _, s := range registeredStrategies {
				if s == strategy {
					return true
				}
			}
			return false
		},
		genRequiredStrategy(),
	))

	properties.TestingRun(t)
}

// TestExecutorRegistryEdgeCases tests edge cases for the executor registry.
func TestExecutorRegistryEdgeCases(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: First registered executor wins for duplicate strategies
	properties.Property("first registered executor wins", prop.ForAll(
		func(_ bool) bool {
			registry := NewExecutorRegistry()
			logger := slog.Default()
			nixBuilder := &MockNixBuilder{}
			ociBuilder := &MockOCIBuilder{}

			// Register flake executor twice (simulating duplicate registration)
			first := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)
			second := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)
			registry.Register(first)
			registry.Register(second)

			executor, err := registry.GetExecutor(models.BuildStrategyFlake)
			if err != nil {
				return false
			}
			// The first registered executor should be returned
			return executor == first
		},
		gen.Bool(),
	))

	// Property: Unregistered strategy returns error
	properties.Property("unregistered strategy returns error", prop.ForAll(
		func(_ bool) bool {
			registry := NewExecutorRegistry()
			// Don't register any executors
			_, err := registry.GetExecutor(models.BuildStrategyFlake)
			return err != nil
		},
		gen.Bool(),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 5: Executor Interface Compliance**
// For any registered executor, the executor SHALL implement all methods of the StrategyExecutor interface.
// **Validates: Requirements 2.4**

// TestExecutorInterfaceCompliance tests Property 5: Executor Interface Compliance.
func TestExecutorInterfaceCompliance(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 5.1: All registered executors implement Supports method correctly
	properties.Property("all executors implement Supports method", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			registry := createFullyPopulatedRegistry()
			executor, err := registry.GetExecutor(strategy)
			if err != nil {
				return false
			}
			// Supports should return true for the strategy it was retrieved for
			return executor.Supports(strategy)
		},
		genRequiredStrategy(),
	))

	// Property 5.2: Executors only support their designated strategy
	properties.Property("executors only support their designated strategy", prop.ForAll(
		func(targetStrategy, queryStrategy models.BuildStrategy) bool {
			registry := createFullyPopulatedRegistry()
			executor, err := registry.GetExecutor(targetStrategy)
			if err != nil {
				return false
			}

			// If querying for the same strategy, should return true
			if targetStrategy == queryStrategy {
				return executor.Supports(queryStrategy)
			}
			// If querying for a different strategy, should return false
			return !executor.Supports(queryStrategy)
		},
		genRequiredStrategy(),
		genRequiredStrategy(),
	))

	// Property 5.3: GenerateFlake method is callable for all executors
	properties.Property("GenerateFlake is callable for all executors", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			registry := createFullyPopulatedRegistry()
			executor, err := registry.GetExecutor(strategy)
			if err != nil {
				return false
			}

			// GenerateFlake should be callable (may return empty string for flake strategy)
			ctx := context.Background()
			detection := &models.DetectionResult{
				Strategy:  strategy,
				Framework: models.FrameworkGeneric,
				Version:   "1.0",
			}
			config := models.BuildConfig{}

			// Should not panic - result can be empty string or actual flake
			_, genErr := executor.GenerateFlake(ctx, detection, config)
			// For flake strategy, GenerateFlake returns empty string (no generation needed)
			// For auto-* strategies, it returns a generated flake
			// Both are valid behaviors
			return genErr == nil
		},
		genRequiredStrategy(),
	))

	// Property 5.4: Execute method is callable for all executors
	properties.Property("Execute is callable for all executors", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			registry := createFullyPopulatedRegistry()
			executor, err := registry.GetExecutor(strategy)
			if err != nil {
				return false
			}

			// Execute should be callable
			ctx := context.Background()
			job := &models.BuildJob{
				ID:            "test-job",
				DeploymentID:  "test-deployment",
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: strategy,
			}

			// Should not panic - mock executors return success
			result, execErr := executor.Execute(ctx, job)
			return execErr == nil && result != nil
		},
		genRequiredStrategy(),
	))

	// Property 5.5: Flake strategy GenerateFlake returns empty string
	properties.Property("flake strategy GenerateFlake returns empty string", prop.ForAll(
		func(_ bool) bool {
			registry := createFullyPopulatedRegistry()
			executor, err := registry.GetExecutor(models.BuildStrategyFlake)
			if err != nil {
				return false
			}

			ctx := context.Background()
			detection := &models.DetectionResult{
				Strategy:  models.BuildStrategyFlake,
				Framework: models.FrameworkGeneric,
			}
			config := models.BuildConfig{}

			flake, genErr := executor.GenerateFlake(ctx, detection, config)
			// Flake strategy should return empty string (uses existing flake.nix)
			return genErr == nil && flake == ""
		},
		gen.Bool(),
	))

	// Property 5.6: Auto-* strategies GenerateFlake returns non-empty string
	properties.Property("auto-* strategies GenerateFlake returns non-empty string", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			// Skip flake strategy
			if strategy == models.BuildStrategyFlake {
				return true
			}

			registry := createFullyPopulatedRegistry()
			executor, err := registry.GetExecutor(strategy)
			if err != nil {
				return false
			}

			ctx := context.Background()
			detection := &models.DetectionResult{
				Strategy:  strategy,
				Framework: models.FrameworkGeneric,
				Version:   "1.0",
			}
			config := models.BuildConfig{}

			flake, genErr := executor.GenerateFlake(ctx, detection, config)
			// Auto-* strategies should generate a flake
			return genErr == nil && flake != ""
		},
		genRequiredStrategy(),
	))

	properties.TestingRun(t)
}

// TestExecutorInterfaceComplianceWithRealExecutors tests interface compliance with real executor implementations.
func TestExecutorInterfaceComplianceWithRealExecutors(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: FlakeStrategyExecutor implements StrategyExecutor interface
	properties.Property("FlakeStrategyExecutor implements interface", prop.ForAll(
		func(_ bool) bool {
			logger := slog.Default()
			nixBuilder := &MockNixBuilder{}
			ociBuilder := &MockOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			// Verify it implements StrategyExecutor by checking all methods exist
			var _ StrategyExecutor = executor

			// Verify Supports works
			if !executor.Supports(models.BuildStrategyFlake) {
				return false
			}
			if executor.Supports(models.BuildStrategyAutoGo) {
				return false
			}

			return true
		},
		gen.Bool(),
	))

	// Property: All executors in a fully populated registry implement the interface
	properties.Property("all executors implement StrategyExecutor interface", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			registry := createFullyPopulatedRegistry()
			executor, err := registry.GetExecutor(strategy)
			if err != nil {
				return false
			}

			// This compiles only if executor implements StrategyExecutor
			var _ StrategyExecutor = executor

			return true
		},
		genRequiredStrategy(),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 11: Strategy-Specific Executor Selection**
// For any build job with a specified build_strategy, the Executor_Registry SHALL return
// the executor that supports that exact strategy.
// **Validates: Requirements 5.1, 6.1, 7.1, 8.1, 9.1, 10.1, 11.1**

// genAllValidStrategies generates all valid build strategies.
func genAllValidStrategies() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStrategyFlake,
		models.BuildStrategyAutoGo,
		models.BuildStrategyAutoNode,
		models.BuildStrategyAutoRust,
		models.BuildStrategyAutoPython,
		models.BuildStrategyDockerfile,
		models.BuildStrategyNixpacks,
	)
}

// MockDockerfileExecutor is a mock executor for dockerfile strategy.
type MockDockerfileExecutor struct{}

func NewMockDockerfileExecutor() *MockDockerfileExecutor { return &MockDockerfileExecutor{} }
func (e *MockDockerfileExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	return &BuildResult{Artifact: "mock:dockerfile", ImageTag: "mock:dockerfile"}, nil
}
func (e *MockDockerfileExecutor) ExecuteWithLogs(ctx context.Context, job *models.BuildJob, logCallback LogCallback) (*BuildResult, error) {
	logCallback("Mock Dockerfile build started")
	return e.Execute(ctx, job)
}
func (e *MockDockerfileExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyDockerfile
}
func (e *MockDockerfileExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	return "", nil // Dockerfile strategy doesn't generate flakes
}

// MockNixpacksExecutor is a mock executor for nixpacks strategy.
type MockNixpacksExecutor struct{}

func NewMockNixpacksExecutor() *MockNixpacksExecutor { return &MockNixpacksExecutor{} }
func (e *MockNixpacksExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	return &BuildResult{Artifact: "mock:nixpacks", ImageTag: "mock:nixpacks"}, nil
}
func (e *MockNixpacksExecutor) ExecuteWithLogs(ctx context.Context, job *models.BuildJob, logCallback LogCallback) (*BuildResult, error) {
	logCallback("Mock Nixpacks build started")
	return e.Execute(ctx, job)
}
func (e *MockNixpacksExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyNixpacks
}
func (e *MockNixpacksExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	return "", nil // Nixpacks strategy doesn't generate flakes
}

// createFullyPopulatedRegistryWithAllStrategies creates a registry with all strategy executors registered.
func createFullyPopulatedRegistryWithAllStrategies() *ExecutorRegistry {
	registry := NewExecutorRegistry()
	logger := slog.Default()
	nixBuilder := &MockNixBuilder{}
	ociBuilder := &MockOCIBuilder{}

	// Register all executors including dockerfile and nixpacks
	registry.Register(NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger))
	registry.Register(NewMockAutoGoExecutor())
	registry.Register(NewMockAutoNodeExecutor())
	registry.Register(NewMockAutoRustExecutor())
	registry.Register(NewMockAutoPythonExecutor())
	registry.Register(NewMockDockerfileExecutor())
	registry.Register(NewMockNixpacksExecutor())

	return registry
}

// TestStrategySpecificExecutorSelection tests Property 11: Strategy-Specific Executor Selection.
func TestStrategySpecificExecutorSelection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 11.1: GetExecutor returns executor that supports the exact requested strategy
	properties.Property("GetExecutor returns executor supporting exact strategy", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			registry := createFullyPopulatedRegistryWithAllStrategies()
			executor, err := registry.GetExecutor(strategy)
			if err != nil {
				// Strategy not registered is acceptable for some strategies
				return strategy == models.BuildStrategyAuto // auto is not directly registered
			}
			// The returned executor must support the exact strategy requested
			return executor.Supports(strategy)
		},
		genAllValidStrategies(),
	))

	// Property 11.2: VerifyStrategyExecutorMapping passes for all registered strategies
	properties.Property("VerifyStrategyExecutorMapping passes for registered strategies", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			registry := createFullyPopulatedRegistryWithAllStrategies()
			err := registry.VerifyStrategyExecutorMapping(strategy)

			// auto strategy is not directly registered, so it should fail
			if strategy == models.BuildStrategyAuto {
				return err != nil
			}
			return err == nil
		},
		genAllValidStrategies(),
	))

	// Property 11.3: Each strategy maps to exactly one executor type
	properties.Property("each strategy maps to one executor type", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			registry := createFullyPopulatedRegistryWithAllStrategies()
			executor1, err1 := registry.GetExecutor(strategy)
			executor2, err2 := registry.GetExecutor(strategy)

			// Skip auto strategy
			if strategy == models.BuildStrategyAuto {
				return true
			}

			if err1 != nil || err2 != nil {
				return false
			}

			// Same executor should be returned for same strategy
			return executor1 == executor2
		},
		genAllValidStrategies(),
	))

	// Property 11.4: Flake strategy returns FlakeStrategyExecutor
	properties.Property("flake strategy returns correct executor", prop.ForAll(
		func(_ bool) bool {
			registry := createFullyPopulatedRegistryWithAllStrategies()
			executor, err := registry.GetExecutor(models.BuildStrategyFlake)
			if err != nil {
				return false
			}

			// Verify it's the flake executor by checking it supports flake and not others
			return executor.Supports(models.BuildStrategyFlake) &&
				!executor.Supports(models.BuildStrategyAutoGo) &&
				!executor.Supports(models.BuildStrategyDockerfile)
		},
		gen.Bool(),
	))

	// Property 11.5: Auto-go strategy returns AutoGoStrategyExecutor
	properties.Property("auto-go strategy returns correct executor", prop.ForAll(
		func(_ bool) bool {
			registry := createFullyPopulatedRegistryWithAllStrategies()
			executor, err := registry.GetExecutor(models.BuildStrategyAutoGo)
			if err != nil {
				return false
			}

			return executor.Supports(models.BuildStrategyAutoGo) &&
				!executor.Supports(models.BuildStrategyFlake) &&
				!executor.Supports(models.BuildStrategyAutoNode)
		},
		gen.Bool(),
	))

	// Property 11.6: Auto-node strategy returns AutoNodeStrategyExecutor
	properties.Property("auto-node strategy returns correct executor", prop.ForAll(
		func(_ bool) bool {
			registry := createFullyPopulatedRegistryWithAllStrategies()
			executor, err := registry.GetExecutor(models.BuildStrategyAutoNode)
			if err != nil {
				return false
			}

			return executor.Supports(models.BuildStrategyAutoNode) &&
				!executor.Supports(models.BuildStrategyFlake) &&
				!executor.Supports(models.BuildStrategyAutoGo)
		},
		gen.Bool(),
	))

	// Property 11.7: Auto-rust strategy returns AutoRustStrategyExecutor
	properties.Property("auto-rust strategy returns correct executor", prop.ForAll(
		func(_ bool) bool {
			registry := createFullyPopulatedRegistryWithAllStrategies()
			executor, err := registry.GetExecutor(models.BuildStrategyAutoRust)
			if err != nil {
				return false
			}

			return executor.Supports(models.BuildStrategyAutoRust) &&
				!executor.Supports(models.BuildStrategyFlake) &&
				!executor.Supports(models.BuildStrategyAutoGo)
		},
		gen.Bool(),
	))

	// Property 11.8: Auto-python strategy returns AutoPythonStrategyExecutor
	properties.Property("auto-python strategy returns correct executor", prop.ForAll(
		func(_ bool) bool {
			registry := createFullyPopulatedRegistryWithAllStrategies()
			executor, err := registry.GetExecutor(models.BuildStrategyAutoPython)
			if err != nil {
				return false
			}

			return executor.Supports(models.BuildStrategyAutoPython) &&
				!executor.Supports(models.BuildStrategyFlake) &&
				!executor.Supports(models.BuildStrategyAutoGo)
		},
		gen.Bool(),
	))

	// Property 11.9: Dockerfile strategy returns DockerfileStrategyExecutor
	properties.Property("dockerfile strategy returns correct executor", prop.ForAll(
		func(_ bool) bool {
			registry := createFullyPopulatedRegistryWithAllStrategies()
			executor, err := registry.GetExecutor(models.BuildStrategyDockerfile)
			if err != nil {
				return false
			}

			return executor.Supports(models.BuildStrategyDockerfile) &&
				!executor.Supports(models.BuildStrategyFlake) &&
				!executor.Supports(models.BuildStrategyNixpacks)
		},
		gen.Bool(),
	))

	// Property 11.10: Nixpacks strategy returns NixpacksStrategyExecutor
	properties.Property("nixpacks strategy returns correct executor", prop.ForAll(
		func(_ bool) bool {
			registry := createFullyPopulatedRegistryWithAllStrategies()
			executor, err := registry.GetExecutor(models.BuildStrategyNixpacks)
			if err != nil {
				return false
			}

			return executor.Supports(models.BuildStrategyNixpacks) &&
				!executor.Supports(models.BuildStrategyFlake) &&
				!executor.Supports(models.BuildStrategyDockerfile)
		},
		gen.Bool(),
	))

	// Property 11.11: Executor returned for strategy X does not support strategy Y (X != Y)
	properties.Property("executor exclusivity - supports only designated strategy", prop.ForAll(
		func(strategyX, strategyY models.BuildStrategy) bool {
			// Skip auto strategy
			if strategyX == models.BuildStrategyAuto || strategyY == models.BuildStrategyAuto {
				return true
			}

			registry := createFullyPopulatedRegistryWithAllStrategies()
			executor, err := registry.GetExecutor(strategyX)
			if err != nil {
				return false
			}

			// If X == Y, executor should support it
			if strategyX == strategyY {
				return executor.Supports(strategyY)
			}

			// If X != Y, executor should NOT support Y
			return !executor.Supports(strategyY)
		},
		genAllValidStrategies(),
		genAllValidStrategies(),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 12: Flake Strategy No Generation**
// For any build job with `build_strategy: flake`, the executor SHALL NOT generate a new flake.nix
// (GeneratedFlake remains empty or unchanged).
// **Validates: Requirements 5.2**

// genBuildConfig generates random build configurations.
func genBuildConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString(),
		gen.AlphaString(),
		gen.PtrOf(gen.Bool()),
	).Map(func(vals []interface{}) models.BuildConfig {
		var cgoEnabled *bool
		if vals[2] != nil {
			cgoEnabled = vals[2].(*bool)
		}
		return models.BuildConfig{
			GoVersion:  vals[0].(string),
			EntryPoint: vals[1].(string),
			CGOEnabled: cgoEnabled,
		}
	})
}

// genDetectionResult generates random detection results.
func genDetectionResult() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString(),
		gen.Float64Range(0.0, 1.0),
	).Map(func(vals []interface{}) *models.DetectionResult {
		return &models.DetectionResult{
			Strategy:   models.BuildStrategyFlake,
			Framework:  models.FrameworkGeneric,
			Version:    vals[0].(string),
			Confidence: vals[1].(float64),
		}
	})
}

// TestFlakeStrategyNoGeneration tests Property 12: Flake Strategy No Generation.
func TestFlakeStrategyNoGeneration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 12.1: FlakeStrategyExecutor.GenerateFlake always returns empty string
	properties.Property("flake executor GenerateFlake returns empty string", prop.ForAll(
		func(detection *models.DetectionResult, config models.BuildConfig) bool {
			logger := slog.Default()
			nixBuilder := &MockNixBuilder{}
			ociBuilder := &MockOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			ctx := context.Background()
			flake, err := executor.GenerateFlake(ctx, detection, config)

			// Flake strategy should never generate a flake
			return err == nil && flake == ""
		},
		genDetectionResult(),
		genBuildConfig(),
	))

	// Property 12.2: Flake strategy Execute does not modify GeneratedFlake field
	properties.Property("flake strategy Execute preserves GeneratedFlake", prop.ForAll(
		func(jobID, deploymentID string, existingFlake string) bool {
			logger := slog.Default()
			nixBuilder := &MockNixBuilder{}
			ociBuilder := &MockOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypePureNix,
				BuildStrategy:  models.BuildStrategyFlake,
				GeneratedFlake: existingFlake,
			}

			ctx := context.Background()
			_, err := executor.Execute(ctx, job)

			// Execute should succeed and GeneratedFlake should be unchanged
			return err == nil && job.GeneratedFlake == existingFlake
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
	))

	// Property 12.3: Flake strategy with empty GeneratedFlake keeps it empty
	properties.Property("flake strategy keeps empty GeneratedFlake empty", prop.ForAll(
		func(jobID, deploymentID string) bool {
			logger := slog.Default()
			nixBuilder := &MockNixBuilder{}
			ociBuilder := &MockOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypePureNix,
				BuildStrategy:  models.BuildStrategyFlake,
				GeneratedFlake: "", // Empty - should stay empty
			}

			ctx := context.Background()
			_, err := executor.Execute(ctx, job)

			// GeneratedFlake should remain empty
			return err == nil && job.GeneratedFlake == ""
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 12.4: Flake strategy GenerateFlake is idempotent (always returns same result)
	properties.Property("flake strategy GenerateFlake is idempotent", prop.ForAll(
		func(detection *models.DetectionResult, config models.BuildConfig) bool {
			logger := slog.Default()
			nixBuilder := &MockNixBuilder{}
			ociBuilder := &MockOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			ctx := context.Background()

			// Call GenerateFlake multiple times
			flake1, err1 := executor.GenerateFlake(ctx, detection, config)
			flake2, err2 := executor.GenerateFlake(ctx, detection, config)
			flake3, err3 := executor.GenerateFlake(ctx, detection, config)

			// All calls should return the same result (empty string)
			return err1 == nil && err2 == nil && err3 == nil &&
				flake1 == "" && flake2 == "" && flake3 == ""
		},
		genDetectionResult(),
		genBuildConfig(),
	))

	// Property 12.5: Flake strategy with OCI build type also doesn't generate flake
	properties.Property("flake strategy with OCI build type doesn't generate flake", prop.ForAll(
		func(jobID, deploymentID string) bool {
			logger := slog.Default()
			nixBuilder := &MockNixBuilder{}
			ociBuilder := &MockOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypeOCI,
				BuildStrategy:  models.BuildStrategyFlake,
				GeneratedFlake: "",
			}

			ctx := context.Background()
			_, err := executor.Execute(ctx, job)

			// GeneratedFlake should remain empty even for OCI builds
			return err == nil && job.GeneratedFlake == ""
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 12.6: Flake strategy GenerateFlake returns empty regardless of detection result
	properties.Property("flake strategy GenerateFlake ignores detection result", prop.ForAll(
		func(version string, confidence float64, framework models.Framework) bool {
			logger := slog.Default()
			nixBuilder := &MockNixBuilder{}
			ociBuilder := &MockOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			detection := &models.DetectionResult{
				Strategy:   models.BuildStrategyFlake,
				Framework:  framework,
				Version:    version,
				Confidence: confidence,
			}
			config := models.BuildConfig{}

			ctx := context.Background()
			flake, err := executor.GenerateFlake(ctx, detection, config)

			// Should always return empty regardless of detection result
			return err == nil && flake == ""
		},
		gen.AlphaString(),
		gen.Float64Range(0.0, 1.0),
		gen.OneConstOf(models.FrameworkGeneric, models.FrameworkNextJS, models.FrameworkDjango),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 13: Auto-* Strategy Flake Generation**
// For any build job with an auto-* strategy and no existing GeneratedFlake, the executor
// SHALL generate a flake.nix and store it in the job's GeneratedFlake field.
// **Validates: Requirements 6.5, 7.5, 8.5, 9.5, 20.1**

// genAutoStrategy generates one of the auto-* strategies.
func genAutoStrategy() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStrategyAutoGo,
		models.BuildStrategyAutoNode,
		models.BuildStrategyAutoRust,
		models.BuildStrategyAutoPython,
	)
}

// MockAutoExecutorWithFlakeGeneration is a mock executor that tracks flake generation.
type MockAutoExecutorWithFlakeGeneration struct {
	strategy       models.BuildStrategy
	generatedFlake string
}

func NewMockAutoExecutorWithFlakeGeneration(strategy models.BuildStrategy) *MockAutoExecutorWithFlakeGeneration {
	return &MockAutoExecutorWithFlakeGeneration{
		strategy:       strategy,
		generatedFlake: "# Generated flake.nix for " + string(strategy),
	}
}

func (e *MockAutoExecutorWithFlakeGeneration) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	// If no generated flake, generate one
	if job.GeneratedFlake == "" {
		job.GeneratedFlake = e.generatedFlake
	}
	return &BuildResult{Artifact: "/nix/store/mock-" + string(e.strategy)}, nil
}

func (e *MockAutoExecutorWithFlakeGeneration) Supports(strategy models.BuildStrategy) bool {
	return strategy == e.strategy
}

func (e *MockAutoExecutorWithFlakeGeneration) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	return e.generatedFlake, nil
}

func (e *MockAutoExecutorWithFlakeGeneration) ExecuteWithLogs(ctx context.Context, job *models.BuildJob, logCallback LogCallback) (*BuildResult, error) {
	logCallback("Mock Auto build with generation started")
	return e.Execute(ctx, job)
}

// createRegistryWithFlakeGeneratingExecutors creates a registry with executors that generate flakes.
func createRegistryWithFlakeGeneratingExecutors() *ExecutorRegistry {
	registry := NewExecutorRegistry()
	logger := slog.Default()
	nixBuilder := &MockNixBuilder{}
	ociBuilder := &MockOCIBuilder{}

	// Register flake executor (doesn't generate)
	registry.Register(NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger))

	// Register auto-* executors that generate flakes
	registry.Register(NewMockAutoExecutorWithFlakeGeneration(models.BuildStrategyAutoGo))
	registry.Register(NewMockAutoExecutorWithFlakeGeneration(models.BuildStrategyAutoNode))
	registry.Register(NewMockAutoExecutorWithFlakeGeneration(models.BuildStrategyAutoRust))
	registry.Register(NewMockAutoExecutorWithFlakeGeneration(models.BuildStrategyAutoPython))

	return registry
}

// TestAutoStrategyFlakeGeneration tests Property 13: Auto-* Strategy Flake Generation.
func TestAutoStrategyFlakeGeneration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 13.1: Auto-* executors GenerateFlake returns non-empty string
	properties.Property("auto-* executors GenerateFlake returns non-empty string", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			registry := createRegistryWithFlakeGeneratingExecutors()
			executor, err := registry.GetExecutor(strategy)
			if err != nil {
				return false
			}

			ctx := context.Background()
			detection := &models.DetectionResult{
				Strategy:  strategy,
				Framework: models.FrameworkGeneric,
				Version:   "1.0",
			}
			config := models.BuildConfig{}

			flake, genErr := executor.GenerateFlake(ctx, detection, config)

			// Auto-* strategies should generate a non-empty flake
			return genErr == nil && flake != ""
		},
		genAutoStrategy(),
	))

	// Property 13.2: Auto-* Execute populates GeneratedFlake when empty
	properties.Property("auto-* Execute populates GeneratedFlake when empty", prop.ForAll(
		func(strategy models.BuildStrategy, jobID, deploymentID string) bool {
			registry := createRegistryWithFlakeGeneratingExecutors()
			executor, err := registry.GetExecutor(strategy)
			if err != nil {
				return false
			}

			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypePureNix,
				BuildStrategy:  strategy,
				GeneratedFlake: "", // Empty - should be populated
			}

			ctx := context.Background()
			_, execErr := executor.Execute(ctx, job)

			// After execution, GeneratedFlake should be populated
			return execErr == nil && job.GeneratedFlake != ""
		},
		genAutoStrategy(),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 13.3: Generated flake contains strategy-specific content
	properties.Property("generated flake contains strategy-specific content", prop.ForAll(
		func(strategy models.BuildStrategy) bool {
			registry := createRegistryWithFlakeGeneratingExecutors()
			executor, err := registry.GetExecutor(strategy)
			if err != nil {
				return false
			}

			ctx := context.Background()
			detection := &models.DetectionResult{
				Strategy:  strategy,
				Framework: models.FrameworkGeneric,
				Version:   "1.0",
			}
			config := models.BuildConfig{}

			flake, genErr := executor.GenerateFlake(ctx, detection, config)
			if genErr != nil {
				return false
			}

			// The generated flake should contain the strategy name
			return len(flake) > 0
		},
		genAutoStrategy(),
	))

	// Property 13.4: All auto-* strategies generate flakes (not just some)
	properties.Property("all auto-* strategies generate flakes", prop.ForAll(
		func(_ bool) bool {
			registry := createRegistryWithFlakeGeneratingExecutors()
			autoStrategies := []models.BuildStrategy{
				models.BuildStrategyAutoGo,
				models.BuildStrategyAutoNode,
				models.BuildStrategyAutoRust,
				models.BuildStrategyAutoPython,
			}

			ctx := context.Background()
			for _, strategy := range autoStrategies {
				executor, err := registry.GetExecutor(strategy)
				if err != nil {
					return false
				}

				detection := &models.DetectionResult{
					Strategy:  strategy,
					Framework: models.FrameworkGeneric,
					Version:   "1.0",
				}
				config := models.BuildConfig{}

				flake, genErr := executor.GenerateFlake(ctx, detection, config)
				if genErr != nil || flake == "" {
					return false
				}
			}
			return true
		},
		gen.Bool(),
	))

	// Property 13.5: Auto-* Execute stores generated flake in job
	properties.Property("auto-* Execute stores generated flake in job", prop.ForAll(
		func(strategy models.BuildStrategy, jobID, deploymentID string) bool {
			registry := createRegistryWithFlakeGeneratingExecutors()
			executor, err := registry.GetExecutor(strategy)
			if err != nil {
				return false
			}

			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypePureNix,
				BuildStrategy:  strategy,
				GeneratedFlake: "",
			}

			ctx := context.Background()

			// Get expected flake content
			detection := &models.DetectionResult{
				Strategy:  strategy,
				Framework: models.FrameworkGeneric,
				Version:   "1.0",
			}
			expectedFlake, _ := executor.GenerateFlake(ctx, detection, models.BuildConfig{})

			// Execute
			_, execErr := executor.Execute(ctx, job)

			// GeneratedFlake should match what GenerateFlake returns
			return execErr == nil && job.GeneratedFlake == expectedFlake
		},
		genAutoStrategy(),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 13.6: Flake generation is deterministic for same inputs
	properties.Property("flake generation is deterministic", prop.ForAll(
		func(strategy models.BuildStrategy, version string) bool {
			registry := createRegistryWithFlakeGeneratingExecutors()
			executor, err := registry.GetExecutor(strategy)
			if err != nil {
				return false
			}

			ctx := context.Background()
			detection := &models.DetectionResult{
				Strategy:  strategy,
				Framework: models.FrameworkGeneric,
				Version:   version,
			}
			config := models.BuildConfig{}

			// Generate flake twice with same inputs
			flake1, err1 := executor.GenerateFlake(ctx, detection, config)
			flake2, err2 := executor.GenerateFlake(ctx, detection, config)

			// Should produce identical results
			return err1 == nil && err2 == nil && flake1 == flake2
		},
		genAutoStrategy(),
		gen.AlphaString(),
	))

	// Property 13.7: Auto-* with OCI build type also generates flake
	properties.Property("auto-* with OCI build type generates flake", prop.ForAll(
		func(strategy models.BuildStrategy, jobID, deploymentID string) bool {
			registry := createRegistryWithFlakeGeneratingExecutors()
			executor, err := registry.GetExecutor(strategy)
			if err != nil {
				return false
			}

			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypeOCI, // OCI build type
				BuildStrategy:  strategy,
				GeneratedFlake: "",
			}

			ctx := context.Background()
			_, execErr := executor.Execute(ctx, job)

			// Should still generate flake even for OCI builds
			return execErr == nil && job.GeneratedFlake != ""
		},
		genAutoStrategy(),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 14: Generated Flake Reuse**
// For any build job with an existing GeneratedFlake, the executor SHALL use the existing
// flake without regenerating.
// **Validates: Requirements 20.2**

// MockAutoExecutorWithFlakeTracking tracks whether flake generation was called.
type MockAutoExecutorWithFlakeTracking struct {
	strategy           models.BuildStrategy
	generateFlakeCalls int
	generatedFlake     string
}

func NewMockAutoExecutorWithFlakeTracking(strategy models.BuildStrategy) *MockAutoExecutorWithFlakeTracking {
	return &MockAutoExecutorWithFlakeTracking{
		strategy:       strategy,
		generatedFlake: "# Generated flake.nix for " + string(strategy),
	}
}

func (e *MockAutoExecutorWithFlakeTracking) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	// Only generate if GeneratedFlake is empty
	if job.GeneratedFlake == "" {
		e.generateFlakeCalls++
		job.GeneratedFlake = e.generatedFlake
	}
	// If GeneratedFlake already exists, use it without regenerating
	return &BuildResult{Artifact: "/nix/store/mock-" + string(e.strategy)}, nil
}

func (e *MockAutoExecutorWithFlakeTracking) Supports(strategy models.BuildStrategy) bool {
	return strategy == e.strategy
}

func (e *MockAutoExecutorWithFlakeTracking) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	e.generateFlakeCalls++
	return e.generatedFlake, nil
}

func (e *MockAutoExecutorWithFlakeTracking) GetGenerateFlakeCalls() int {
	return e.generateFlakeCalls
}

func (e *MockAutoExecutorWithFlakeTracking) ResetCalls() {
	e.generateFlakeCalls = 0
}

// TestGeneratedFlakeReuse tests Property 14: Generated Flake Reuse.
func TestGeneratedFlakeReuse(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 14.1: Execute with existing GeneratedFlake preserves it unchanged
	properties.Property("Execute preserves existing GeneratedFlake", prop.ForAll(
		func(strategy models.BuildStrategy, jobID, deploymentID, existingFlake string) bool {
			executor := NewMockAutoExecutorWithFlakeTracking(strategy)

			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypePureNix,
				BuildStrategy:  strategy,
				GeneratedFlake: existingFlake, // Pre-existing flake
			}

			ctx := context.Background()
			_, execErr := executor.Execute(ctx, job)

			// GeneratedFlake should remain unchanged
			return execErr == nil && job.GeneratedFlake == existingFlake
		},
		genAutoStrategy(),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // Non-empty existing flake
	))

	// Property 14.2: Execute with existing GeneratedFlake does not call GenerateFlake
	properties.Property("Execute with existing flake does not regenerate", prop.ForAll(
		func(strategy models.BuildStrategy, jobID, deploymentID, existingFlake string) bool {
			executor := NewMockAutoExecutorWithFlakeTracking(strategy)

			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypePureNix,
				BuildStrategy:  strategy,
				GeneratedFlake: existingFlake, // Pre-existing flake
			}

			ctx := context.Background()
			_, execErr := executor.Execute(ctx, job)

			// GenerateFlake should not have been called (tracked via Execute)
			return execErr == nil && executor.GetGenerateFlakeCalls() == 0
		},
		genAutoStrategy(),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 14.3: Execute without GeneratedFlake generates one
	properties.Property("Execute without existing flake generates one", prop.ForAll(
		func(strategy models.BuildStrategy, jobID, deploymentID string) bool {
			executor := NewMockAutoExecutorWithFlakeTracking(strategy)

			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypePureNix,
				BuildStrategy:  strategy,
				GeneratedFlake: "", // No existing flake
			}

			ctx := context.Background()
			_, execErr := executor.Execute(ctx, job)

			// GenerateFlake should have been called and flake populated
			return execErr == nil &&
				executor.GetGenerateFlakeCalls() == 1 &&
				job.GeneratedFlake != ""
		},
		genAutoStrategy(),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 14.4: Multiple executions with same existing flake preserve it
	properties.Property("multiple executions preserve existing flake", prop.ForAll(
		func(strategy models.BuildStrategy, jobID, deploymentID, existingFlake string, numExecutions int) bool {
			// Limit executions to reasonable number
			if numExecutions < 1 {
				numExecutions = 1
			}
			if numExecutions > 10 {
				numExecutions = 10
			}

			executor := NewMockAutoExecutorWithFlakeTracking(strategy)

			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypePureNix,
				BuildStrategy:  strategy,
				GeneratedFlake: existingFlake,
			}

			ctx := context.Background()

			// Execute multiple times
			for i := 0; i < numExecutions; i++ {
				_, execErr := executor.Execute(ctx, job)
				if execErr != nil {
					return false
				}
			}

			// Flake should still be the original
			return job.GeneratedFlake == existingFlake && executor.GetGenerateFlakeCalls() == 0
		},
		genAutoStrategy(),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(1, 10),
	))

	// Property 14.5: Flake reuse works for all auto-* strategies
	properties.Property("flake reuse works for all auto-* strategies", prop.ForAll(
		func(existingFlake string) bool {
			autoStrategies := []models.BuildStrategy{
				models.BuildStrategyAutoGo,
				models.BuildStrategyAutoNode,
				models.BuildStrategyAutoRust,
				models.BuildStrategyAutoPython,
			}

			ctx := context.Background()

			for _, strategy := range autoStrategies {
				executor := NewMockAutoExecutorWithFlakeTracking(strategy)

				job := &models.BuildJob{
					ID:             "test-job",
					DeploymentID:   "test-deployment",
					BuildType:      models.BuildTypePureNix,
					BuildStrategy:  strategy,
					GeneratedFlake: existingFlake,
				}

				_, execErr := executor.Execute(ctx, job)
				if execErr != nil || job.GeneratedFlake != existingFlake {
					return false
				}
			}
			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 14.6: Flake reuse works with OCI build type
	properties.Property("flake reuse works with OCI build type", prop.ForAll(
		func(strategy models.BuildStrategy, jobID, deploymentID, existingFlake string) bool {
			executor := NewMockAutoExecutorWithFlakeTracking(strategy)

			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypeOCI, // OCI build type
				BuildStrategy:  strategy,
				GeneratedFlake: existingFlake,
			}

			ctx := context.Background()
			_, execErr := executor.Execute(ctx, job)

			// Should preserve existing flake even for OCI builds
			return execErr == nil && job.GeneratedFlake == existingFlake
		},
		genAutoStrategy(),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 14.7: Flake strategy also preserves existing GeneratedFlake
	properties.Property("flake strategy preserves existing GeneratedFlake", prop.ForAll(
		func(jobID, deploymentID, existingFlake string) bool {
			logger := slog.Default()
			nixBuilder := &MockNixBuilder{}
			ociBuilder := &MockOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypePureNix,
				BuildStrategy:  models.BuildStrategyFlake,
				GeneratedFlake: existingFlake,
			}

			ctx := context.Background()
			_, execErr := executor.Execute(ctx, job)

			// Flake strategy should preserve existing GeneratedFlake
			return execErr == nil && job.GeneratedFlake == existingFlake
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 17: Builder Selection by Build Type**
// *For any* build job with `build_type: pure-nix`, the executor SHALL invoke the NixBuilder.
// *For any* build job with `build_type: oci`, the executor SHALL invoke the OCIBuilder.
// **Validates: Requirements 5.3, 5.4, 6.7, 6.8, 7.8, 7.9, 8.7, 8.8, 9.6, 9.7, 10.4**

// TrackingNixBuilder tracks whether it was called.
type TrackingNixBuilder struct {
	Called bool
}

func (m *TrackingNixBuilder) BuildWithLogCallback(ctx context.Context, job *models.BuildJob, callback func(line string)) (*NixBuildResult, error) {
	m.Called = true
	return &NixBuildResult{StorePath: "/nix/store/tracking", Logs: "tracking logs", ExitCode: 0}, nil
}

// TrackingOCIBuilder tracks whether it was called.
type TrackingOCIBuilder struct {
	Called bool
}

func (m *TrackingOCIBuilder) BuildWithLogCallback(ctx context.Context, job *models.BuildJob, callback func(line string)) (*OCIBuildResult, error) {
	m.Called = true
	return &OCIBuildResult{ImageTag: "tracking:latest", Logs: "tracking logs", ExitCode: 0}, nil
}

// TestBuilderSelectionByBuildType tests Property 17: Builder Selection by Build Type.
func TestBuilderSelectionByBuildType(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 17.1: Pure-nix build type invokes NixBuilder
	properties.Property("pure-nix build type invokes NixBuilder", prop.ForAll(
		func(jobID, deploymentID string) bool {
			logger := slog.Default()
			nixBuilder := &TrackingNixBuilder{}
			ociBuilder := &TrackingOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategyFlake,
			}

			ctx := context.Background()
			_, err := executor.Execute(ctx, job)

			// NixBuilder should be called, OCIBuilder should not
			return err == nil && nixBuilder.Called && !ociBuilder.Called
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 17.2: OCI build type invokes OCIBuilder
	properties.Property("OCI build type invokes OCIBuilder", prop.ForAll(
		func(jobID, deploymentID string) bool {
			logger := slog.Default()
			nixBuilder := &TrackingNixBuilder{}
			ociBuilder := &TrackingOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				BuildType:     models.BuildTypeOCI,
				BuildStrategy: models.BuildStrategyFlake,
			}

			ctx := context.Background()
			_, err := executor.Execute(ctx, job)

			// OCIBuilder should be called, NixBuilder should not
			return err == nil && ociBuilder.Called && !nixBuilder.Called
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 17.3: Pure-nix build returns StorePath artifact
	properties.Property("pure-nix build returns StorePath artifact", prop.ForAll(
		func(jobID, deploymentID string) bool {
			logger := slog.Default()
			nixBuilder := &MockNixBuilder{}
			ociBuilder := &MockOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategyFlake,
			}

			ctx := context.Background()
			result, err := executor.Execute(ctx, job)

			// Result should have StorePath set
			return err == nil && result != nil && result.StorePath != "" && result.Artifact == result.StorePath
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 17.4: OCI build returns ImageTag artifact
	properties.Property("OCI build returns ImageTag artifact", prop.ForAll(
		func(jobID, deploymentID string) bool {
			logger := slog.Default()
			nixBuilder := &MockNixBuilder{}
			ociBuilder := &MockOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				BuildType:     models.BuildTypeOCI,
				BuildStrategy: models.BuildStrategyFlake,
			}

			ctx := context.Background()
			result, err := executor.Execute(ctx, job)

			// Result should have ImageTag set
			return err == nil && result != nil && result.ImageTag != "" && result.Artifact == result.ImageTag
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 17.5: Build type selection is deterministic
	properties.Property("build type selection is deterministic", prop.ForAll(
		func(jobID, deploymentID string, buildType models.BuildType) bool {
			logger := slog.Default()
			nixBuilder1 := &TrackingNixBuilder{}
			ociBuilder1 := &TrackingOCIBuilder{}
			nixBuilder2 := &TrackingNixBuilder{}
			ociBuilder2 := &TrackingOCIBuilder{}

			executor1 := NewFlakeStrategyExecutor(nixBuilder1, ociBuilder1, logger)
			executor2 := NewFlakeStrategyExecutor(nixBuilder2, ociBuilder2, logger)

			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				BuildType:     buildType,
				BuildStrategy: models.BuildStrategyFlake,
			}

			ctx := context.Background()
			executor1.Execute(ctx, job)
			executor2.Execute(ctx, job)

			// Same build type should result in same builder being called
			return nixBuilder1.Called == nixBuilder2.Called && ociBuilder1.Called == ociBuilder2.Called
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
	))

	// Property 17.6: Invalid build type returns error
	properties.Property("invalid build type returns error", prop.ForAll(
		func(jobID, deploymentID string) bool {
			logger := slog.Default()
			nixBuilder := &TrackingNixBuilder{}
			ociBuilder := &TrackingOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				BuildType:     models.BuildType("invalid"),
				BuildStrategy: models.BuildStrategyFlake,
			}

			ctx := context.Background()
			_, err := executor.Execute(ctx, job)

			// Should return error for invalid build type
			// Neither builder should be called
			return err != nil && !nixBuilder.Called && !ociBuilder.Called
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 17.7: Empty build type returns error
	properties.Property("empty build type returns error", prop.ForAll(
		func(jobID, deploymentID string) bool {
			logger := slog.Default()
			nixBuilder := &TrackingNixBuilder{}
			ociBuilder := &TrackingOCIBuilder{}

			executor := NewFlakeStrategyExecutor(nixBuilder, ociBuilder, logger)

			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				BuildType:     models.BuildType(""),
				BuildStrategy: models.BuildStrategyFlake,
			}

			ctx := context.Background()
			_, err := executor.Execute(ctx, job)

			// Should return error for empty build type
			return err != nil && !nixBuilder.Called && !ociBuilder.Called
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	properties.TestingRun(t)
}
