package builder

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/queue"
	"github.com/narvanalabs/control-plane/internal/store"
)

// Worker processes build jobs from the queue.
type Worker struct {
	store       store.Store
	queue       queue.Queue
	nixBuilder  *NixBuilder
	ociBuilder  *OCIBuilder
	atticClient *AtticClient
	logger      *slog.Logger

	concurrency int
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// WorkerConfig holds configuration for the build worker.
type WorkerConfig struct {
	Concurrency  int
	NixConfig    *NixBuilderConfig
	OCIConfig    *OCIBuilderConfig
	AtticConfig  *AtticConfig
}

// DefaultWorkerConfig returns a WorkerConfig with sensible defaults.
func DefaultWorkerConfig() *WorkerConfig {
	return &WorkerConfig{
		Concurrency: 4,
		NixConfig:   DefaultNixBuilderConfig(),
		OCIConfig:   DefaultOCIBuilderConfig(),
		AtticConfig: DefaultAtticConfig(),
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

	return &Worker{
		store:       s,
		queue:       q,
		nixBuilder:  nixBuilder,
		ociBuilder:  ociBuilder,
		atticClient: atticClient,
		logger:      logger,
		concurrency: cfg.Concurrency,
		stopCh:      make(chan struct{}),
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
	)

	// Update job status to running
	now := time.Now()
	job.Status = models.BuildStatusRunning
	job.StartedAt = &now
	if err := w.store.Builds().Update(ctx, job); err != nil {
		return fmt.Errorf("updating job status to running: %w", err)
	}

	// Update deployment status to building
	deployment, err := w.store.Deployments().Get(ctx, job.DeploymentID)
	if err != nil {
		return fmt.Errorf("getting deployment: %w", err)
	}
	deployment.Status = models.DeploymentStatusBuilding
	deployment.UpdatedAt = now
	if err := w.store.Deployments().Update(ctx, deployment); err != nil {
		return fmt.Errorf("updating deployment status: %w", err)
	}

	// Execute the build based on build type
	var artifact string
	var buildLogs string
	var buildErr error

	// Create a log callback to stream logs to the database
	logCallback := func(line string) {
		w.streamLog(ctx, job.DeploymentID, line)
	}

	switch job.BuildType {
	case models.BuildTypeOCI:
		artifact, buildLogs, buildErr = w.buildOCI(ctx, job, logCallback)
	case models.BuildTypePureNix:
		artifact, buildLogs, buildErr = w.buildPureNix(ctx, job, logCallback)
	default:
		buildErr = fmt.Errorf("unknown build type: %s", job.BuildType)
	}

	// Update job and deployment status based on result
	finishedAt := time.Now()
	job.FinishedAt = &finishedAt

	if buildErr != nil {
		w.logger.Error("build failed",
			"job_id", job.ID,
			"error", buildErr,
		)

		job.Status = models.BuildStatusFailed
		deployment.Status = models.DeploymentStatusFailed

		// Store the build logs even on failure
		w.storeBuildLogs(ctx, job.DeploymentID, buildLogs)
	} else {
		w.logger.Info("build succeeded",
			"job_id", job.ID,
			"artifact", artifact,
		)

		job.Status = models.BuildStatusSucceeded
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

	return buildErr
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

	// Push the closure to Attic
	logCallback("=== Pushing closure to Attic ===")
	pushResult, err := w.atticClient.PushWithDependencies(ctx, result.StorePath)
	if err != nil {
		return "", result.Logs, fmt.Errorf("pushing to Attic: %w", err)
	}

	logCallback(fmt.Sprintf("Pushed to: %s", pushResult.CacheURL))
	logCallback(fmt.Sprintf("Store path: %s", result.StorePath))

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
