// Package postgres provides a PostgreSQL-backed implementation of the build queue.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/queue"
)

// PostgresQueue implements queue.Queue using PostgreSQL.
type PostgresQueue struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewPostgresQueue creates a new PostgreSQL-backed queue.
func NewPostgresQueue(db *sql.DB, logger *slog.Logger) *PostgresQueue {
	if logger == nil {
		logger = slog.Default()
	}
	return &PostgresQueue{
		db:     db,
		logger: logger,
	}
}

// Enqueue adds a new build job to the queue.
// The job is serialized to JSON and stored in the build_queue table.
func (q *PostgresQueue) Enqueue(ctx context.Context, job *models.BuildJob) error {
	// Serialize the job to JSON
	jobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshaling job to JSON: %w", err)
	}

	query := `
		INSERT INTO build_queue (id, job_data, status, created_at)
		VALUES ($1, $2, 'pending', $3)`

	now := time.Now().UTC()
	_, err = q.db.ExecContext(ctx, query, job.ID, jobData, now)
	if err != nil {
		return fmt.Errorf("inserting job into queue: %w", err)
	}

	q.logger.Debug("enqueued build job", "job_id", job.ID)
	return nil
}

// Dequeue retrieves and locks the next available build job from the queue.
// Uses SELECT FOR UPDATE SKIP LOCKED for concurrent worker safety.
func (q *PostgresQueue) Dequeue(ctx context.Context) (*models.BuildJob, error) {
	// Use a transaction to atomically select and update the job status
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Select the oldest pending job and lock it
	selectQuery := `
		SELECT id, job_data
		FROM build_queue
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED`

	var jobID string
	var jobData []byte
	err = tx.QueryRowContext(ctx, selectQuery).Scan(&jobID, &jobData)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, queue.ErrNoJobs
		}
		return nil, fmt.Errorf("selecting job from queue: %w", err)
	}

	// Update the job status to processing
	updateQuery := `
		UPDATE build_queue
		SET status = 'processing', started_at = $2
		WHERE id = $1`

	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, updateQuery, jobID, now)
	if err != nil {
		return nil, fmt.Errorf("updating job status: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	// Deserialize the job from JSON
	var job models.BuildJob
	if err := json.Unmarshal(jobData, &job); err != nil {
		return nil, fmt.Errorf("unmarshaling job from JSON: %w", err)
	}

	q.logger.Debug("dequeued build job", "job_id", job.ID)
	return &job, nil
}

// Ack acknowledges successful processing of a job, removing it from the queue.
func (q *PostgresQueue) Ack(ctx context.Context, jobID string) error {
	query := `
		DELETE FROM build_queue
		WHERE id = $1 AND status = 'processing'`

	result, err := q.db.ExecContext(ctx, query, jobID)
	if err != nil {
		return fmt.Errorf("deleting job from queue: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return queue.ErrJobNotFound
	}

	q.logger.Debug("acknowledged build job", "job_id", jobID)
	return nil
}

// Nack indicates that job processing failed, making the job available for retry.
func (q *PostgresQueue) Nack(ctx context.Context, jobID string) error {
	query := `
		UPDATE build_queue
		SET status = 'pending', started_at = NULL, retry_count = retry_count + 1
		WHERE id = $1 AND status = 'processing'`

	result, err := q.db.ExecContext(ctx, query, jobID)
	if err != nil {
		return fmt.Errorf("updating job status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return queue.ErrJobNotFound
	}

	q.logger.Debug("nacked build job", "job_id", jobID)
	return nil
}
