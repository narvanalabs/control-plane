// Package builder provides build job processing and recovery functionality.
package builder

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/queue"
	"github.com/narvanalabs/control-plane/internal/store"
)

// RecoveryService handles startup recovery for pending and interrupted builds.
// It ensures that builds survive API restarts by:
// 1. Marking interrupted builds (status = "running") as failed
// 2. Re-queuing pending builds from the builds table
// **Validates: Requirements 15.1, 15.2**
type RecoveryService struct {
	store  store.Store
	queue  queue.Queue
	logger *slog.Logger
}

// RecoveryResult contains the results of a startup recovery operation.
type RecoveryResult struct {
	// InterruptedBuilds is the number of builds marked as failed due to interruption.
	InterruptedBuilds int
	// ResumedBuilds is the number of builds re-queued for processing.
	ResumedBuilds int
	// Errors contains any errors encountered during recovery.
	Errors []error
}

// NewRecoveryService creates a new RecoveryService.
func NewRecoveryService(s store.Store, q queue.Queue, logger *slog.Logger) *RecoveryService {
	if logger == nil {
		logger = slog.Default()
	}
	return &RecoveryService{
		store:  s,
		queue:  q,
		logger: logger,
	}
}

// RecoverOnStartup performs startup recovery for builds.
// This should be called when the API server or worker starts.
// **Validates: Requirements 15.1, 15.2**
func (r *RecoveryService) RecoverOnStartup(ctx context.Context) (*RecoveryResult, error) {
	result := &RecoveryResult{
		Errors: make([]error, 0),
	}

	r.logger.Info("starting build queue recovery")

	// Step 1: Mark interrupted builds as failed
	// **Validates: Requirements 15.2**
	interruptedCount, err := r.markInterruptedBuildsAsFailed(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("marking interrupted builds: %w", err))
		r.logger.Error("failed to mark interrupted builds", "error", err)
	} else {
		result.InterruptedBuilds = interruptedCount
		if interruptedCount > 0 {
			r.logger.Info("marked interrupted builds as failed", "count", interruptedCount)
		}
	}

	// Step 2: Resume pending builds by re-queuing them
	// **Validates: Requirements 15.1**
	resumedCount, err := r.resumePendingBuilds(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("resuming pending builds: %w", err))
		r.logger.Error("failed to resume pending builds", "error", err)
	} else {
		result.ResumedBuilds = resumedCount
		if resumedCount > 0 {
			r.logger.Info("resumed pending builds", "count", resumedCount)
		}
	}

	r.logger.Info("build queue recovery completed",
		"interrupted_builds", result.InterruptedBuilds,
		"resumed_builds", result.ResumedBuilds,
		"errors", len(result.Errors),
	)

	return result, nil
}

// markInterruptedBuildsAsFailed marks all builds with status "running" as failed.
// These builds were interrupted by a server restart.
// **Validates: Requirements 15.2**
func (r *RecoveryService) markInterruptedBuildsAsFailed(ctx context.Context) (int, error) {
	// Get all running builds
	runningBuilds, err := r.store.Builds().ListRunning(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing running builds: %w", err)
	}

	if len(runningBuilds) == 0 {
		return 0, nil
	}

	count := 0
	now := time.Now()

	for _, build := range runningBuilds {
		r.logger.Info("marking interrupted build as failed",
			"build_id", build.ID,
			"deployment_id", build.DeploymentID,
		)

		// Update build status to failed
		build.Status = models.BuildStatusFailed
		build.FinishedAt = &now

		if err := r.store.Builds().Update(ctx, build); err != nil {
			r.logger.Error("failed to update interrupted build",
				"build_id", build.ID,
				"error", err,
			)
			continue
		}

		// Update deployment status to failed
		// **Validates: Requirements 15.3**
		if err := r.updateDeploymentStatusToFailed(ctx, build.DeploymentID); err != nil {
			r.logger.Error("failed to update deployment status for interrupted build",
				"build_id", build.ID,
				"deployment_id", build.DeploymentID,
				"error", err,
			)
		}

		count++
	}

	return count, nil
}

// resumePendingBuilds re-queues all builds with status "queued".
// **Validates: Requirements 15.1**
func (r *RecoveryService) resumePendingBuilds(ctx context.Context) (int, error) {
	// Get all queued builds
	queuedBuilds, err := r.store.Builds().ListQueued(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing queued builds: %w", err)
	}

	if len(queuedBuilds) == 0 {
		return 0, nil
	}

	count := 0

	for _, build := range queuedBuilds {
		r.logger.Info("re-queuing pending build",
			"build_id", build.ID,
			"deployment_id", build.DeploymentID,
		)

		// Enqueue the build job
		if err := r.queue.Enqueue(ctx, build); err != nil {
			r.logger.Error("failed to re-queue build",
				"build_id", build.ID,
				"error", err,
			)
			continue
		}

		count++
	}

	return count, nil
}

// updateDeploymentStatusToFailed updates a deployment's status to failed.
// **Validates: Requirements 15.3**
func (r *RecoveryService) updateDeploymentStatusToFailed(ctx context.Context, deploymentID string) error {
	deployment, err := r.store.Deployments().Get(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("getting deployment: %w", err)
	}

	deployment.Status = models.DeploymentStatusFailed
	deployment.UpdatedAt = time.Now()

	if err := r.store.Deployments().Update(ctx, deployment); err != nil {
		return fmt.Errorf("updating deployment: %w", err)
	}

	return nil
}
