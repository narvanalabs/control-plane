package builder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/narvanalabs/control-plane/internal/builder/detector"
	"github.com/narvanalabs/control-plane/internal/builder/executor"
	"github.com/narvanalabs/control-plane/internal/builder/hash"
	"github.com/narvanalabs/control-plane/internal/builder/retry"
	"github.com/narvanalabs/control-plane/internal/builder/templates"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/queue"
	"github.com/narvanalabs/control-plane/internal/store"
)

// Build timeout errors.
var (
	// ErrBuildTimeout is returned when a build exceeds its configured timeout.
	ErrBuildTimeout = errors.New("build exceeded timeout limit")
	// ErrValidationFailed is returned when build validation fails.
	ErrValidationFailed = errors.New("build validation failed")
	// ErrInvalidStateTransition is returned when an invalid state transition is attempted.
	ErrInvalidStateTransition = errors.New("invalid state transition")
)

// DefaultBuildTimeout is the default build timeout in seconds (30 minutes).
const DefaultBuildTimeout = 1800

// Validation error codes.
// These codes are used to categorize validation errors for programmatic handling.
// **Validates: Requirements 3.4, 3.5, 3.6, 3.7, 3.8, 3.9**
const (
	// ValidationCodeRequiredField indicates a required field is missing or empty.
	ValidationCodeRequiredField = "REQUIRED_FIELD"
	// ValidationCodeInvalidValue indicates a field has an invalid value.
	ValidationCodeInvalidValue = "INVALID_VALUE"
	// ValidationCodeNegativeValue indicates a field has a negative value when it should be non-negative.
	ValidationCodeNegativeValue = "NEGATIVE_VALUE"
)

// transitionJobStatus validates and performs a state transition for a build job.
// It returns an error if the transition is not allowed by the state machine.
// The isRetry parameter indicates if this is a retry operation (allows running → queued).
func transitionJobStatus(job *models.BuildJob, newStatus models.BuildStatus, isRetry bool) error {
	if !models.CanTransition(job.Status, newStatus, isRetry) {
		return fmt.Errorf("%w: cannot transition from %s to %s (isRetry=%v)",
			ErrInvalidStateTransition, job.Status, newStatus, isRetry)
	}
	job.Status = newStatus
	return nil
}

// ValidationError describes a validation failure.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s (code: %s)", e.Field, e.Message, e.Code)
}

// ValidationResult contains validation results.
type ValidationResult struct {
	Valid    bool              `json:"valid"`
	Errors   []ValidationError `json:"errors,omitempty"`
	Warnings []string          `json:"warnings,omitempty"`
}

// BuildValidator validates build configurations before execution.
type BuildValidator interface {
	// Validate checks if a build job configuration is valid.
	Validate(ctx context.Context, job *models.BuildJob) (*ValidationResult, error)
}

// DefaultBuildValidator is the default implementation of BuildValidator.
type DefaultBuildValidator struct {
	logger *slog.Logger
}

// NewDefaultBuildValidator creates a new DefaultBuildValidator.
func NewDefaultBuildValidator(logger *slog.Logger) *DefaultBuildValidator {
	if logger == nil {
		logger = slog.Default()
	}
	return &DefaultBuildValidator{logger: logger}
}

// Validate checks if a build job configuration is valid.
// **Validates: Requirements 3.1, 3.4, 3.5, 3.6, 3.7, 3.8, 3.9**
func (v *DefaultBuildValidator) Validate(ctx context.Context, job *models.BuildJob) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Errors:   make([]ValidationError, 0),
		Warnings: make([]string, 0),
	}

	// Validate required fields
	// **Validates: Requirements 3.4** - empty id field
	if job.ID == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "id",
			Message: "build job ID is required",
			Code:    ValidationCodeRequiredField,
		})
	}

	// **Validates: Requirements 3.5** - empty deployment_id field
	if job.DeploymentID == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "deployment_id",
			Message: "deployment ID is required",
			Code:    ValidationCodeRequiredField,
		})
	}

	// Validate build type
	// **Validates: Requirements 3.6** - empty build_type field
	if job.BuildType == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "build_type",
			Message: "build type is required",
			Code:    ValidationCodeRequiredField,
		})
	} else if job.BuildType != models.BuildTypePureNix && job.BuildType != models.BuildTypeOCI {
		// **Validates: Requirements 3.7** - invalid build_type
		result.Errors = append(result.Errors, ValidationError{
			Field:   "build_type",
			Message: fmt.Sprintf("invalid build type: %s", job.BuildType),
			Code:    ValidationCodeInvalidValue,
		})
	}

	// Validate build strategy if specified
	// **Validates: Requirements 3.8** - invalid build_strategy
	if job.BuildStrategy != "" && !job.BuildStrategy.IsValid() {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "build_strategy",
			Message: fmt.Sprintf("invalid build strategy: %s", job.BuildStrategy),
			Code:    ValidationCodeInvalidValue,
		})
	}

	// Validate strategy-specific requirements
	if job.BuildStrategy == models.BuildStrategyDockerfile || job.BuildStrategy == models.BuildStrategyNixpacks {
		if job.BuildType != models.BuildTypeOCI {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("strategy %s requires OCI build type, will be enforced", job.BuildStrategy))
		}
	}

	// Validate build config if present
	if job.BuildConfig != nil {
		v.validateBuildConfig(job.BuildConfig, result)
	}

	// Validate timeout
	// **Validates: Requirements 3.9** - negative timeout_seconds
	if job.TimeoutSeconds < 0 {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "timeout_seconds",
			Message: "timeout cannot be negative",
			Code:    ValidationCodeNegativeValue,
		})
	}

	// Set valid flag based on errors
	result.Valid = len(result.Errors) == 0

	return result, nil
}

// validateBuildConfig validates the build configuration.
func (v *DefaultBuildValidator) validateBuildConfig(config *models.BuildConfig, result *ValidationResult) {
	// Validate Go version format if specified
	if config.GoVersion != "" {
		if !isValidVersionFormat(config.GoVersion) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Go version '%s' may not be a valid version format", config.GoVersion))
		}
	}

	// Validate Node version format if specified
	if config.NodeVersion != "" {
		if !isValidVersionFormat(config.NodeVersion) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Node version '%s' may not be a valid version format", config.NodeVersion))
		}
	}

	// Validate Python version format if specified
	if config.PythonVersion != "" {
		if !isValidVersionFormat(config.PythonVersion) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Python version '%s' may not be a valid version format", config.PythonVersion))
		}
	}

	// Validate package manager if specified
	if config.PackageManager != "" {
		validManagers := []string{"npm", "yarn", "pnpm"}
		isValid := false
		for _, m := range validManagers {
			if config.PackageManager == m {
				isValid = true
				break
			}
		}
		if !isValid {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("package manager '%s' may not be supported", config.PackageManager))
		}
	}

	// Validate build timeout - negative values are not allowed
	if config.BuildTimeout < 0 {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "build_config.build_timeout",
			Message: "build timeout cannot be negative",
			Code:    ValidationCodeNegativeValue,
		})
	}
}

// isValidVersionFormat checks if a version string has a valid format.
func isValidVersionFormat(version string) bool {
	if version == "" {
		return false
	}
	// Basic check: version should start with a digit or 'v'
	if len(version) > 0 {
		first := version[0]
		if (first >= '0' && first <= '9') || first == 'v' {
			return true
		}
	}
	return false
}

// ValidateBuildJob validates a build job and returns any validation errors.
// This is a convenience function for external callers.
func ValidateBuildJob(ctx context.Context, job *models.BuildJob) (*ValidationResult, error) {
	validator := NewDefaultBuildValidator(nil)
	return validator.Validate(ctx, job)
}

// BuildStage represents a phase in the build process.
type BuildStage string

const (
	StageCloning         BuildStage = "cloning"
	StageDetecting       BuildStage = "detecting"
	StageGenerating      BuildStage = "generating"
	StageCalculatingHash BuildStage = "calculating_hash"
	StageBuilding        BuildStage = "building"
	StagePushing         BuildStage = "pushing"
	StageCompleted       BuildStage = "completed"
	StageFailed          BuildStage = "failed"
)

// BuildProgressTracker reports build progress to users.
type BuildProgressTracker interface {
	// ReportStage reports current build stage.
	ReportStage(ctx context.Context, buildID string, stage BuildStage) error
	// ReportProgress reports percentage completion.
	ReportProgress(ctx context.Context, buildID string, percent int, message string) error
}

// ProgressTrackerWithHistory extends BuildProgressTracker with history retrieval.
// **Validates: Requirements 4.1, 4.2, 4.3**
type ProgressTrackerWithHistory interface {
	BuildProgressTracker
	// GetProgressHistory returns the progress history for a build.
	GetProgressHistory(buildID string) []ProgressRecord
	// GetStageHistory returns the stage history for a build.
	GetStageHistory(buildID string) []StageRecord
}

// ProgressRecord tracks a single progress report for verification.
// **Validates: Requirements 4.1, 4.2, 4.3**
type ProgressRecord struct {
	BuildID   string
	Percent   int
	Message   string
	Timestamp time.Time
}

// StageRecord tracks a single stage report for verification.
// **Validates: Requirements 4.1, 4.2**
type StageRecord struct {
	BuildID   string
	Stage     BuildStage
	Timestamp time.Time
}

// DefaultProgressTracker is a progress tracker that logs progress and maintains history.
// **Validates: Requirements 4.1, 4.2, 4.3**
type DefaultProgressTracker struct {
	logger          *slog.Logger
	mu              sync.RWMutex
	progressHistory map[string][]ProgressRecord
	stageHistory    map[string][]StageRecord
}

// NewDefaultProgressTracker creates a new DefaultProgressTracker.
func NewDefaultProgressTracker(logger *slog.Logger) *DefaultProgressTracker {
	if logger == nil {
		logger = slog.Default()
	}
	return &DefaultProgressTracker{
		logger:          logger,
		progressHistory: make(map[string][]ProgressRecord),
		stageHistory:    make(map[string][]StageRecord),
	}
}

// ReportStage logs the current build stage and stores it in history.
func (t *DefaultProgressTracker) ReportStage(ctx context.Context, buildID string, stage BuildStage) error {
	t.logger.Info("build stage", "build_id", buildID, "stage", stage)

	t.mu.Lock()
	defer t.mu.Unlock()

	record := StageRecord{
		BuildID:   buildID,
		Stage:     stage,
		Timestamp: time.Now(),
	}
	t.stageHistory[buildID] = append(t.stageHistory[buildID], record)

	return nil
}

// ReportProgress logs the build progress, validates monotonicity, and stores it in history.
// **Validates: Requirements 4.3** - Progress monotonicity validation
func (t *DefaultProgressTracker) ReportProgress(ctx context.Context, buildID string, percent int, message string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check for monotonicity violation
	// **Validates: Requirements 4.3**
	history := t.progressHistory[buildID]
	if len(history) > 0 {
		lastPercent := history[len(history)-1].Percent
		if percent < lastPercent {
			t.logger.Warn("non-monotonic progress detected",
				"build_id", buildID,
				"previous_percent", lastPercent,
				"new_percent", percent,
				"message", message,
			)
		}
	}

	t.logger.Info("build progress", "build_id", buildID, "percent", percent, "message", message)

	record := ProgressRecord{
		BuildID:   buildID,
		Percent:   percent,
		Message:   message,
		Timestamp: time.Now(),
	}
	t.progressHistory[buildID] = append(t.progressHistory[buildID], record)

	return nil
}

// GetProgressHistory returns the progress history for a build.
// **Validates: Requirements 4.1, 4.2, 4.3**
func (t *DefaultProgressTracker) GetProgressHistory(buildID string) []ProgressRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()

	history := t.progressHistory[buildID]
	if history == nil {
		return []ProgressRecord{}
	}
	// Return a copy to prevent external modification
	result := make([]ProgressRecord, len(history))
	copy(result, history)
	return result
}

// GetStageHistory returns the stage history for a build.
// **Validates: Requirements 4.1, 4.2**
func (t *DefaultProgressTracker) GetStageHistory(buildID string) []StageRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()

	history := t.stageHistory[buildID]
	if history == nil {
		return []StageRecord{}
	}
	// Return a copy to prevent external modification
	result := make([]StageRecord, len(history))
	copy(result, history)
	return result
}

// IsProgressMonotonic checks if all progress reports for a build are monotonically increasing.
// **Validates: Requirements 4.3**
func (t *DefaultProgressTracker) IsProgressMonotonic(buildID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	history := t.progressHistory[buildID]
	if len(history) <= 1 {
		return true
	}

	for i := 1; i < len(history); i++ {
		if history[i].Percent < history[i-1].Percent {
			return false
		}
	}
	return true
}

// HasTerminalStage checks if the build has reported a terminal stage (completed or failed).
// **Validates: Requirements 4.5, 4.6**
func (t *DefaultProgressTracker) HasTerminalStage(buildID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	history := t.stageHistory[buildID]
	for _, record := range history {
		if record.Stage == StageCompleted || record.Stage == StageFailed {
			return true
		}
	}
	return false
}

// GetLastStage returns the last reported stage for a build.
func (t *DefaultProgressTracker) GetLastStage(buildID string) (BuildStage, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	history := t.stageHistory[buildID]
	if len(history) == 0 {
		return "", false
	}
	return history[len(history)-1].Stage, true
}

// ClearHistory clears the history for a specific build (useful for testing).
func (t *DefaultProgressTracker) ClearHistory(buildID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.progressHistory, buildID)
	delete(t.stageHistory, buildID)
}

// ClearAllHistory clears all history (useful for testing).
func (t *DefaultProgressTracker) ClearAllHistory() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.progressHistory = make(map[string][]ProgressRecord)
	t.stageHistory = make(map[string][]StageRecord)
}

// Worker processes build jobs from the queue.
type Worker struct {
	store           store.Store
	queue           queue.Queue
	nixBuilder      *NixBuilder
	ociBuilder      *OCIBuilder
	atticClient     *AtticClient
	executorRegistry *executor.ExecutorRegistry
	retryManager    *retry.Manager
	progressTracker BuildProgressTracker
	validator       BuildValidator
	logger          *slog.Logger

	concurrency    int
	defaultTimeout int // Default build timeout in seconds
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

// WorkerConfig holds configuration for the build worker.
type WorkerConfig struct {
	Concurrency     int
	NixConfig       *NixBuilderConfig
	OCIConfig       *OCIBuilderConfig
	AtticConfig     *AtticConfig
	DefaultTimeout  int // Default build timeout in seconds (default: 1800 = 30 minutes)
}

// DefaultWorkerConfig returns a WorkerConfig with sensible defaults.
func DefaultWorkerConfig() *WorkerConfig {
	return &WorkerConfig{
		Concurrency:    4,
		NixConfig:      DefaultNixBuilderConfig(),
		OCIConfig:      DefaultOCIBuilderConfig(),
		AtticConfig:    DefaultAtticConfig(),
		DefaultTimeout: 1800, // 30 minutes
	}
}

// NewWorker creates a new build worker.
func NewWorker(cfg *WorkerConfig, s store.Store, q queue.Queue, logger *slog.Logger) (*Worker, error) {
	if logger == nil {
		logger = slog.Default()
	}

	nixBuilder, err := NewNixBuilder(cfg.NixConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("creating nix builder: %w", err)
	}

	ociBuilder, err := NewOCIBuilder(cfg.OCIConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("creating oci builder: %w", err)
	}

	atticClient := NewAtticClient(cfg.AtticConfig, logger)

	// Create executor registry and register all strategy executors
	registry := executor.NewExecutorRegistry()

	// Create adapters for the builders
	nixBuilderAdapter := &nixBuilderAdapter{nixBuilder}
	ociBuilderAdapter := &ociBuilderAdapter{ociBuilder}

	// Register flake strategy executor
	flakeExecutor := executor.NewFlakeStrategyExecutor(nixBuilderAdapter, ociBuilderAdapter, logger)
	registry.Register(flakeExecutor)

	// Create shared dependencies for auto-* executors
	det := detector.NewDetector()
	tmplEngine, tmplErr := templates.NewTemplateEngine()
	if tmplErr != nil {
		logger.Warn("failed to create template engine, auto-* strategies will not be available", "error", tmplErr)
	} else {
		hashCalc := hash.NewCalculator()

		// Register auto-go executor
		autoGoExecutor := executor.NewAutoGoStrategyExecutor(det, tmplEngine, hashCalc, nixBuilderAdapter, ociBuilderAdapter, logger)
		registry.Register(autoGoExecutor)

		// Register auto-node executor
		autoNodeExecutor := executor.NewAutoNodeStrategyExecutor(det, tmplEngine, hashCalc, nixBuilderAdapter, ociBuilderAdapter, logger)
		registry.Register(autoNodeExecutor)

		// Register auto-rust executor
		autoRustExecutor := executor.NewAutoRustStrategyExecutor(det, tmplEngine, hashCalc, nixBuilderAdapter, ociBuilderAdapter, logger)
		registry.Register(autoRustExecutor)

		// Register auto-python executor (doesn't use hash calculator)
		autoPythonExecutor := executor.NewAutoPythonStrategyExecutor(det, tmplEngine, nixBuilderAdapter, ociBuilderAdapter, logger)
		registry.Register(autoPythonExecutor)

		logger.Info("registered auto-* strategy executors")
	}

	// Create retry manager with notification callback
	retryMgr := retry.NewManager(
		retry.WithNotificationCallback(func(notification *retry.FallbackNotification) {
			logger.Warn("build fallback occurred",
				"build_id", notification.BuildID,
				"original_type", notification.OriginalType,
				"fallback_type", notification.FallbackType,
				"reason", notification.Reason,
			)
		}),
	)

	// Create progress tracker
	progressTracker := NewDefaultProgressTracker(logger)

	// Create build validator
	validator := NewDefaultBuildValidator(logger)

	// Verify all required executors are registered
	// **Validates: Requirements 2.1**
	if err := registry.VerifyRequiredExecutors(); err != nil {
		return nil, fmt.Errorf("verifying required executors: %w", err)
	}

	// Log registered executors on success
	registeredStrategies := registry.GetRegisteredStrategies()
	logger.Info("executor registry verified",
		"registered_strategies", registeredStrategies,
		"required_strategies", executor.RequiredStrategies,
	)

	return &Worker{
		store:            s,
		queue:            q,
		nixBuilder:       nixBuilder,
		ociBuilder:       ociBuilder,
		atticClient:      atticClient,
		executorRegistry: registry,
		retryManager:     retryMgr,
		progressTracker:  progressTracker,
		validator:        validator,
		logger:           logger,
		concurrency:      cfg.Concurrency,
		defaultTimeout:   cfg.DefaultTimeout,
		stopCh:           make(chan struct{}),
	}, nil
}

// nixBuilderAdapter adapts NixBuilder to the executor.NixBuilder interface.
type nixBuilderAdapter struct {
	builder *NixBuilder
}

func (a *nixBuilderAdapter) BuildWithLogCallback(ctx context.Context, job *models.BuildJob, callback func(line string)) (*executor.NixBuildResult, error) {
	result, err := a.builder.BuildWithLogCallback(ctx, job, callback)
	if err != nil {
		// Return the result even on error so logs are preserved
		if result != nil {
			return &executor.NixBuildResult{
				StorePath: result.StorePath,
				Logs:      result.Logs,
				ExitCode:  result.ExitCode,
			}, err
		}
		return nil, err
	}
	return &executor.NixBuildResult{
		StorePath: result.StorePath,
		Logs:      result.Logs,
		ExitCode:  result.ExitCode,
	}, nil
}

// ociBuilderAdapter adapts OCIBuilder to the executor.OCIBuilder interface.
type ociBuilderAdapter struct {
	builder *OCIBuilder
}

func (a *ociBuilderAdapter) BuildWithLogCallback(ctx context.Context, job *models.BuildJob, callback func(line string)) (*executor.OCIBuildResult, error) {
	result, err := a.builder.BuildWithLogCallback(ctx, job, callback)
	if err != nil {
		return nil, err
	}
	return &executor.OCIBuildResult{
		ImageTag:  result.ImageTag,
		StorePath: result.StorePath,
		Logs:      result.Logs,
		ExitCode:  result.ExitCode,
	}, nil
}


// Start begins processing build jobs from the queue.
// It spawns multiple goroutines based on the configured concurrency.
func (w *Worker) Start(ctx context.Context) error {
	w.logger.Info("starting build worker", "concurrency", w.concurrency)

	for i := 0; i < w.concurrency; i++ {
		w.wg.Add(1)
		go w.workerLoop(ctx, i)
	}

	return nil
}

// Stop gracefully stops the worker and waits for all jobs to complete.
func (w *Worker) Stop() {
	w.logger.Info("stopping build worker")
	close(w.stopCh)
	w.wg.Wait()
	w.logger.Info("build worker stopped")
}

// workerLoop is the main loop for a single worker goroutine.
func (w *Worker) workerLoop(ctx context.Context, workerID int) {
	defer w.wg.Done()

	logger := w.logger.With("worker_id", workerID)
	logger.Debug("worker started")

	for {
		select {
		case <-ctx.Done():
			logger.Debug("worker context cancelled")
			return
		case <-w.stopCh:
			logger.Debug("worker stop signal received")
			return
		default:
			// Try to dequeue a job
			job, err := w.queue.Dequeue(ctx)
			if err != nil {
				if err == queue.ErrNoJobs {
					// No jobs available, wait a bit before trying again
					time.Sleep(1 * time.Second)
					continue
				}
				logger.Error("failed to dequeue job", "error", err)
				time.Sleep(5 * time.Second)
				continue
			}

			// Process the job
			if err := w.processJob(ctx, job); err != nil {
				logger.Error("failed to process job",
					"job_id", job.ID,
					"error", err,
				)
				// Nack the job so it can be retried
				if nackErr := w.queue.Nack(ctx, job.ID); nackErr != nil {
					logger.Error("failed to nack job", "job_id", job.ID, "error", nackErr)
				}
				continue
			}

			// Ack the job
			if err := w.queue.Ack(ctx, job.ID); err != nil {
				logger.Error("failed to ack job", "job_id", job.ID, "error", err)
			}
		}
	}
}

// processJob executes a single build job.
func (w *Worker) processJob(ctx context.Context, job *models.BuildJob) error {
	w.logger.Info("processing build job",
		"job_id", job.ID,
		"deployment_id", job.DeploymentID,
		"build_type", job.BuildType,
		"build_strategy", job.BuildStrategy,
	)

	// First, verify the build record exists in the database
	existingJob, err := w.store.Builds().Get(ctx, job.ID)
	if err != nil {
		// If the build record doesn't exist, this is an orphaned queue entry
		// We should ack it to remove it from the queue and not retry
		w.logger.Warn("build record not found, removing orphaned job from queue",
			"job_id", job.ID,
			"deployment_id", job.DeploymentID,
		)
		// Return nil to trigger Ack instead of Nack
		return nil
	}

	// Use the existing job from the database (it has the correct state)
	job = existingJob

	// Validate the build job configuration before starting
	validationResult, err := w.validator.Validate(ctx, job)
	if err != nil {
		w.logger.Error("failed to validate build job",
			"job_id", job.ID,
			"error", err,
		)
		return fmt.Errorf("validating build job: %w", err)
	}

	if !validationResult.Valid {
		w.logger.Error("build job validation failed",
			"job_id", job.ID,
			"errors", validationResult.Errors,
		)
		// Transition to running first (job was picked up), then to failed
		// This follows the state machine: queued → running → failed
		if err := transitionJobStatus(job, models.BuildStatusRunning, false); err != nil {
			w.logger.Error("failed to transition job status to running",
				"job_id", job.ID,
				"error", err,
			)
		}
		now := time.Now()
		job.StartedAt = &now
		if err := transitionJobStatus(job, models.BuildStatusFailed, false); err != nil {
			w.logger.Error("failed to transition job status to failed",
				"job_id", job.ID,
				"error", err,
			)
		}
		job.FinishedAt = &now
		w.store.Builds().Update(ctx, job)
		return fmt.Errorf("%w: %v", ErrValidationFailed, validationResult.Errors)
	}

	// Log any validation warnings
	for _, warning := range validationResult.Warnings {
		w.logger.Warn("build job validation warning",
			"job_id", job.ID,
			"warning", warning,
		)
	}

	// Enforce build type based on strategy
	// **Validates: Requirements 10.2, 11.2, 18.1, 18.2, 18.5**
	if job.BuildStrategy != "" {
		enforcedType, wasChanged := models.EnforceBuildType(job.BuildStrategy, job.BuildType)
		if wasChanged {
			// **Validates: Requirements 18.5** - Log warning when build type is enforced
			w.logger.Warn("build type enforced by strategy",
				"job_id", job.ID,
				"strategy", job.BuildStrategy,
				"requested_type", job.BuildType,
				"enforced_type", enforcedType,
			)
			job.BuildType = enforcedType
		}
	}

	// Update job status to running
	now := time.Now()
	if err := transitionJobStatus(job, models.BuildStatusRunning, false); err != nil {
		return fmt.Errorf("transitioning job status to running: %w", err)
	}
	job.StartedAt = &now
	if err := w.store.Builds().Update(ctx, job); err != nil {
		return fmt.Errorf("updating job status to running: %w", err)
	}

	// Update deployment status to building
	deployment, err := w.store.Deployments().Get(ctx, job.DeploymentID)
	if err != nil {
		// If deployment doesn't exist, mark job as failed and return nil to ack
		w.logger.Warn("deployment not found for build job",
			"job_id", job.ID,
			"deployment_id", job.DeploymentID,
		)
		if err := transitionJobStatus(job, models.BuildStatusFailed, false); err != nil {
			w.logger.Error("failed to transition job status to failed",
				"job_id", job.ID,
				"error", err,
			)
		}
		finishedAt := time.Now()
		job.FinishedAt = &finishedAt
		w.store.Builds().Update(ctx, job)
		return nil
	}
	deployment.Status = models.DeploymentStatusBuilding
	deployment.UpdatedAt = now
	if err := w.store.Deployments().Update(ctx, deployment); err != nil {
		return fmt.Errorf("updating deployment status: %w", err)
	}

	// Report build stage
	w.progressTracker.ReportStage(ctx, job.ID, StageBuilding)

	// Execute the build using strategy router
	var artifact string
	var buildLogs string
	var buildErr error

	// Create a log callback to stream logs to the database
	logCallback := func(line string) {
		w.streamLog(ctx, job.DeploymentID, line)
	}

	// Route to appropriate strategy executor or fall back to legacy build
	artifact, buildLogs, buildErr = w.executeWithStrategy(ctx, job, logCallback)

	// Update job and deployment status based on result
	finishedAt := time.Now()
	job.FinishedAt = &finishedAt

	if buildErr != nil {
		w.logger.Error("build failed",
			"job_id", job.ID,
			"error", buildErr,
		)

		// Check if we should retry
		if w.retryManager.ShouldRetry(ctx, job, buildErr) {
			w.logger.Info("preparing build retry",
				"job_id", job.ID,
				"retry_count", job.RetryCount,
			)

			// Record the failed attempt
			w.retryManager.RecordAttempt(ctx, job.ID, retry.BuildAttempt{
				AttemptNumber: job.RetryCount + 1,
				Strategy:      job.BuildStrategy,
				BuildType:     job.BuildType,
				StartedAt:     *job.StartedAt,
				CompletedAt:   &finishedAt,
				Success:       false,
				Error:         buildErr.Error(),
			})

			// Prepare retry job
			retryJob, retryErr := w.retryManager.PrepareRetry(ctx, job)
			if retryErr == nil {
				// Update job for retry - use isRetry=true to allow running → queued transition
				job.RetryCount = retryJob.RetryCount
				job.BuildType = retryJob.BuildType
				job.RetryAsOCI = retryJob.RetryAsOCI
				if err := transitionJobStatus(job, models.BuildStatusQueued, true); err != nil {
					w.logger.Error("failed to transition job status for retry",
						"job_id", job.ID,
						"error", err,
					)
				} else {
					job.StartedAt = nil
					job.FinishedAt = nil

					if err := w.store.Builds().Update(ctx, job); err != nil {
						w.logger.Error("failed to update job for retry", "job_id", job.ID, "error", err)
					}

					// Re-enqueue the job
					if err := w.queue.Enqueue(ctx, job); err != nil {
						w.logger.Error("failed to re-enqueue job for retry", "job_id", job.ID, "error", err)
					} else {
						w.logger.Info("job re-enqueued for retry",
							"job_id", job.ID,
							"retry_count", job.RetryCount,
							"build_type", job.BuildType,
						)
						return nil
					}
				}
			}
		}

		w.progressTracker.ReportStage(ctx, job.ID, StageFailed)
		if err := transitionJobStatus(job, models.BuildStatusFailed, false); err != nil {
			w.logger.Error("failed to transition job status to failed",
				"job_id", job.ID,
				"error", err,
			)
		}
		deployment.Status = models.DeploymentStatusFailed

		// Store the build logs even on failure
		w.storeBuildLogs(ctx, job.DeploymentID, buildLogs)
	} else {
		w.logger.Info("build succeeded",
			"job_id", job.ID,
			"artifact", artifact,
		)

		w.progressTracker.ReportStage(ctx, job.ID, StageCompleted)
		if err := transitionJobStatus(job, models.BuildStatusSucceeded, false); err != nil {
			w.logger.Error("failed to transition job status to succeeded",
				"job_id", job.ID,
				"error", err,
			)
		}
		deployment.Status = models.DeploymentStatusBuilt
		deployment.Artifact = artifact
	}

	deployment.UpdatedAt = finishedAt

	// Update the job
	if err := w.store.Builds().Update(ctx, job); err != nil {
		w.logger.Error("failed to update job status", "job_id", job.ID, "error", err)
	}

	// Update the deployment
	if err := w.store.Deployments().Update(ctx, deployment); err != nil {
		w.logger.Error("failed to update deployment status", "deployment_id", deployment.ID, "error", err)
	}

	// Return nil to acknowledge the job - build failures are recorded in the database
	// and should not be retried via the queue
	return nil
}

// executeWithStrategy routes the build to the appropriate strategy executor.
func (w *Worker) executeWithStrategy(ctx context.Context, job *models.BuildJob, logCallback func(string)) (string, string, error) {
	// Determine the timeout for this build
	timeout := w.getBuildTimeout(job)
	
	// Create a context with timeout
	buildCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create a channel to receive the build result
	type buildResult struct {
		artifact string
		logs     string
		err      error
	}
	resultCh := make(chan buildResult, 1)

	// Execute the build in a goroutine
	go func() {
		artifact, logs, err := w.executeBuild(buildCtx, job, logCallback)
		resultCh <- buildResult{artifact, logs, err}
	}()

	// Wait for either the build to complete or timeout
	select {
	case result := <-resultCh:
		return result.artifact, result.logs, result.err
	case <-buildCtx.Done():
		if buildCtx.Err() == context.DeadlineExceeded {
			logCallback(fmt.Sprintf("=== Build timeout exceeded (%v) ===", timeout))
			return "", "", fmt.Errorf("%w: build exceeded %v timeout", ErrBuildTimeout, timeout)
		}
		return "", "", buildCtx.Err()
	}
}

// getBuildTimeout returns the timeout duration for a build job.
func (w *Worker) getBuildTimeout(job *models.BuildJob) time.Duration {
	// Use job-specific timeout if configured
	if job.TimeoutSeconds > 0 {
		return time.Duration(job.TimeoutSeconds) * time.Second
	}
	
	// Use build config timeout if available
	if job.BuildConfig != nil && job.BuildConfig.BuildTimeout > 0 {
		return time.Duration(job.BuildConfig.BuildTimeout) * time.Second
	}
	
	// Use worker default timeout
	if w.defaultTimeout > 0 {
		return time.Duration(w.defaultTimeout) * time.Second
	}
	
	// Fall back to global default
	return time.Duration(DefaultBuildTimeout) * time.Second
}

// executeBuild performs the actual build execution.
func (w *Worker) executeBuild(ctx context.Context, job *models.BuildJob, logCallback func(string)) (string, string, error) {
	// Report initial progress
	w.progressTracker.ReportProgress(ctx, job.ID, 10, "Starting build execution")

	// If a build strategy is specified, try to use the strategy executor
	if job.BuildStrategy != "" {
		w.logger.Info("looking up executor for strategy", "strategy", job.BuildStrategy)
		w.progressTracker.ReportStage(ctx, job.ID, StageDetecting)
		w.progressTracker.ReportProgress(ctx, job.ID, 20, "Detecting build strategy")

		strategyExecutor, err := w.executorRegistry.GetExecutor(job.BuildStrategy)
		if err == nil {
			w.logger.Info("found executor for strategy", "strategy", job.BuildStrategy)
			logCallback(fmt.Sprintf("=== Using strategy: %s ===", job.BuildStrategy))
			w.progressTracker.ReportProgress(ctx, job.ID, 30, fmt.Sprintf("Using strategy: %s", job.BuildStrategy))

			// Report generating stage if the strategy generates flakes
			if job.GeneratedFlake == "" {
				w.progressTracker.ReportStage(ctx, job.ID, StageGenerating)
				w.progressTracker.ReportProgress(ctx, job.ID, 40, "Generating build configuration")
			}

			w.progressTracker.ReportStage(ctx, job.ID, StageBuilding)
			w.progressTracker.ReportProgress(ctx, job.ID, 50, "Building application")

			result, execErr := strategyExecutor.Execute(ctx, job)
			if result != nil {
				if execErr == nil {
					w.progressTracker.ReportProgress(ctx, job.ID, 90, "Build completed successfully")
				}
				return result.Artifact, result.Logs, execErr
			}
			return "", "", execErr
		}
		// If no executor found for the strategy, log and fall back to legacy
		w.logger.Warn("no executor found for strategy, falling back to legacy build",
			"strategy", job.BuildStrategy,
		)
	}

	w.progressTracker.ReportStage(ctx, job.ID, StageBuilding)
	w.progressTracker.ReportProgress(ctx, job.ID, 50, "Building application")

	// Fall back to legacy build based on build type
	switch job.BuildType {
	case models.BuildTypeOCI:
		return w.buildOCI(ctx, job, logCallback)
	case models.BuildTypePureNix:
		return w.buildPureNix(ctx, job, logCallback)
	default:
		return "", "", fmt.Errorf("unknown build type: %s", job.BuildType)
	}
}

// buildOCI executes an OCI mode build.
func (w *Worker) buildOCI(ctx context.Context, job *models.BuildJob, logCallback func(string)) (string, string, error) {
	result, err := w.ociBuilder.BuildWithLogCallback(ctx, job, logCallback)
	if err != nil {
		logs := ""
		if result != nil {
			logs = result.Logs
		}
		return "", logs, err
	}
	return result.ImageTag, result.Logs, nil
}

// buildPureNix executes a Pure Nix mode build.
func (w *Worker) buildPureNix(ctx context.Context, job *models.BuildJob, logCallback func(string)) (string, string, error) {
	// First, run the Nix build
	result, err := w.nixBuilder.BuildWithLogCallback(ctx, job, logCallback)
	if err != nil {
		logs := ""
		if result != nil {
			logs = result.Logs
		}
		return "", logs, err
	}

	// Report pushing stage
	w.progressTracker.ReportStage(ctx, job.ID, StagePushing)
	w.progressTracker.ReportProgress(ctx, job.ID, 80, "Pushing to cache")

	// Push the closure to Attic
	logCallback("=== Pushing closure to Attic ===")
	pushResult, err := w.atticClient.PushWithDependencies(ctx, result.StorePath)
	if err != nil {
		return "", result.Logs, fmt.Errorf("pushing to Attic: %w", err)
	}

	logCallback(fmt.Sprintf("Pushed to: %s", pushResult.CacheURL))
	logCallback(fmt.Sprintf("Store path: %s", result.StorePath))

	w.progressTracker.ReportProgress(ctx, job.ID, 95, "Push completed")

	return result.StorePath, result.Logs, nil
}

// streamLog streams a single log line to the database.
func (w *Worker) streamLog(ctx context.Context, deploymentID, line string) {
	entry := &models.LogEntry{
		ID:           uuid.New().String(),
		DeploymentID: deploymentID,
		Source:       "build",
		Level:        "info",
		Message:      line,
		Timestamp:    time.Now(),
	}

	if err := w.store.Logs().Create(ctx, entry); err != nil {
		w.logger.Error("failed to stream log entry",
			"deployment_id", deploymentID,
			"error", err,
		)
	}
}

// storeBuildLogs stores the complete build logs in the database.
func (w *Worker) storeBuildLogs(ctx context.Context, deploymentID, logs string) {
	if logs == "" {
		return
	}

	entry := &models.LogEntry{
		ID:           uuid.New().String(),
		DeploymentID: deploymentID,
		Source:       "build",
		Level:        "info",
		Message:      logs,
		Timestamp:    time.Now(),
	}

	if err := w.store.Logs().Create(ctx, entry); err != nil {
		w.logger.Error("failed to store build logs",
			"deployment_id", deploymentID,
			"error", err,
		)
	}
}

// ProcessSingleJob processes a single job without the worker loop.
// This is useful for testing or one-off builds.
func (w *Worker) ProcessSingleJob(ctx context.Context, job *models.BuildJob) error {
	return w.processJob(ctx, job)
}

// IsBuildTimeoutError checks if an error is a build timeout error.
func IsBuildTimeoutError(err error) bool {
	return errors.Is(err, ErrBuildTimeout)
}

// GetEffectiveTimeout returns the effective timeout for a build job.
// The timeout priority is: job.TimeoutSeconds > job.BuildConfig.BuildTimeout > defaultTimeout > DefaultBuildTimeout
// **Validates: Requirements 12.1, 12.2, 12.3**
func GetEffectiveTimeout(job *models.BuildJob, defaultTimeout int) time.Duration {
	// Priority 1: Use job-specific timeout if configured
	// **Validates: Requirements 12.1**
	if job.TimeoutSeconds > 0 {
		return time.Duration(job.TimeoutSeconds) * time.Second
	}

	// Priority 2: Use build config timeout if available
	// **Validates: Requirements 12.2**
	if job.BuildConfig != nil && job.BuildConfig.BuildTimeout > 0 {
		return time.Duration(job.BuildConfig.BuildTimeout) * time.Second
	}

	// Priority 3: Use provided default timeout
	// **Validates: Requirements 12.3**
	if defaultTimeout > 0 {
		return time.Duration(defaultTimeout) * time.Second
	}

	// Priority 4: Fall back to global default (1800 seconds = 30 minutes)
	// **Validates: Requirements 12.3**
	return time.Duration(DefaultBuildTimeout) * time.Second
}

// GetBuildTimeout is an alias for GetEffectiveTimeout for backward compatibility.
// Deprecated: Use GetEffectiveTimeout instead.
func GetBuildTimeout(job *models.BuildJob, defaultTimeout int) time.Duration {
	return GetEffectiveTimeout(job, defaultTimeout)
}

// SetProgressTracker sets a custom progress tracker for the worker.
// This is useful for integrating with external progress reporting systems.
func (w *Worker) SetProgressTracker(tracker BuildProgressTracker) {
	if tracker != nil {
		w.progressTracker = tracker
	}
}

// GetProgressTracker returns the current progress tracker.
func (w *Worker) GetProgressTracker() BuildProgressTracker {
	return w.progressTracker
}

// StageDescription returns a human-readable description of a build stage.
func StageDescription(stage BuildStage) string {
	switch stage {
	case StageCloning:
		return "Cloning repository"
	case StageDetecting:
		return "Detecting build strategy"
	case StageGenerating:
		return "Generating build configuration"
	case StageCalculatingHash:
		return "Calculating dependency hashes"
	case StageBuilding:
		return "Building application"
	case StagePushing:
		return "Pushing to cache"
	case StageCompleted:
		return "Build completed"
	case StageFailed:
		return "Build failed"
	default:
		return string(stage)
	}
}
