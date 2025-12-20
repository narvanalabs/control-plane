// Package queue provides build job queue interfaces and implementations.
package queue

import (
	"context"
	"errors"

	"github.com/narvanalabs/control-plane/internal/models"
)

// Common errors returned by queue operations.
var (
	// ErrNoJobs is returned when no jobs are available in the queue.
	ErrNoJobs = errors.New("no jobs available")
	// ErrJobNotFound is returned when a job cannot be found.
	ErrJobNotFound = errors.New("job not found")
)

// Queue defines the interface for build job queue operations.
type Queue interface {
	// Enqueue adds a new build job to the queue.
	// The job will be serialized to JSON for storage.
	Enqueue(ctx context.Context, job *models.BuildJob) error

	// Dequeue retrieves and locks the next available build job from the queue.
	// Returns ErrNoJobs if no jobs are available.
	// The job is deserialized from JSON storage.
	Dequeue(ctx context.Context) (*models.BuildJob, error)

	// Ack acknowledges successful processing of a job, removing it from the queue.
	Ack(ctx context.Context, jobID string) error

	// Nack indicates that job processing failed, making the job available for retry.
	Nack(ctx context.Context, jobID string) error
}
