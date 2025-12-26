package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

// BuildStore implements store.BuildStore using PostgreSQL.
type BuildStore struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *slog.Logger
}

// conn returns the queryable connection (transaction or database).
func (s *BuildStore) conn() queryable {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Create creates a new build job.
func (s *BuildStore) Create(ctx context.Context, build *models.BuildJob) error {
	query := `
		INSERT INTO builds (id, deployment_id, app_id, git_url, git_ref, 
			flake_output, build_type, status, created_at, started_at, finished_at,
			build_strategy, timeout_seconds, retry_count, retry_as_oci,
			generated_flake, flake_lock, vendor_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING id, created_at`

	now := time.Now().UTC()
	if build.CreatedAt.IsZero() {
		build.CreatedAt = now
	}

	// Handle nullable build_strategy
	var buildStrategy sql.NullString
	if build.BuildStrategy != "" {
		buildStrategy = sql.NullString{String: string(build.BuildStrategy), Valid: true}
	}

	// Handle nullable generated_flake
	var generatedFlake sql.NullString
	if build.GeneratedFlake != "" {
		generatedFlake = sql.NullString{String: build.GeneratedFlake, Valid: true}
	}

	// Handle nullable flake_lock
	var flakeLock sql.NullString
	if build.FlakeLock != "" {
		flakeLock = sql.NullString{String: build.FlakeLock, Valid: true}
	}

	// Handle nullable vendor_hash
	var vendorHash sql.NullString
	if build.VendorHash != "" {
		vendorHash = sql.NullString{String: build.VendorHash, Valid: true}
	}

	err := s.conn().QueryRowContext(ctx, query,
		build.ID,
		build.DeploymentID,
		build.AppID,
		build.GitURL,
		build.GitRef,
		build.FlakeOutput,
		build.BuildType,
		build.Status,
		build.CreatedAt,
		build.StartedAt,
		build.FinishedAt,
		buildStrategy,
		build.TimeoutSeconds,
		build.RetryCount,
		build.RetryAsOCI,
		generatedFlake,
		flakeLock,
		vendorHash,
	).Scan(&build.ID, &build.CreatedAt)

	if err != nil {
		return fmt.Errorf("inserting build: %w", err)
	}

	return nil
}

// Get retrieves a build job by ID.
func (s *BuildStore) Get(ctx context.Context, id string) (*models.BuildJob, error) {
	query := `
		SELECT id, deployment_id, app_id, git_url, git_ref, 
			flake_output, build_type, status, created_at, started_at, finished_at,
			build_strategy, timeout_seconds, retry_count, retry_as_oci,
			generated_flake, flake_lock, vendor_hash
		FROM builds
		WHERE id = $1`

	build := &models.BuildJob{}
	var startedAt, finishedAt sql.NullTime
	var buildStrategy, generatedFlake, flakeLock, vendorHash sql.NullString

	err := s.conn().QueryRowContext(ctx, query, id).Scan(
		&build.ID,
		&build.DeploymentID,
		&build.AppID,
		&build.GitURL,
		&build.GitRef,
		&build.FlakeOutput,
		&build.BuildType,
		&build.Status,
		&build.CreatedAt,
		&startedAt,
		&finishedAt,
		&buildStrategy,
		&build.TimeoutSeconds,
		&build.RetryCount,
		&build.RetryAsOCI,
		&generatedFlake,
		&flakeLock,
		&vendorHash,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying build: %w", err)
	}

	if startedAt.Valid {
		build.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		build.FinishedAt = &finishedAt.Time
	}
	if buildStrategy.Valid {
		build.BuildStrategy = models.BuildStrategy(buildStrategy.String)
	}
	if generatedFlake.Valid {
		build.GeneratedFlake = generatedFlake.String
	}
	if flakeLock.Valid {
		build.FlakeLock = flakeLock.String
	}
	if vendorHash.Valid {
		build.VendorHash = vendorHash.String
	}

	return build, nil
}


// GetByDeployment retrieves a build job by deployment ID.
func (s *BuildStore) GetByDeployment(ctx context.Context, deploymentID string) (*models.BuildJob, error) {
	query := `
		SELECT id, deployment_id, app_id, git_url, git_ref, 
			flake_output, build_type, status, created_at, started_at, finished_at,
			build_strategy, timeout_seconds, retry_count, retry_as_oci,
			generated_flake, flake_lock, vendor_hash
		FROM builds
		WHERE deployment_id = $1
		ORDER BY created_at DESC
		LIMIT 1`

	build := &models.BuildJob{}
	var startedAt, finishedAt sql.NullTime
	var buildStrategy, generatedFlake, flakeLock, vendorHash sql.NullString

	err := s.conn().QueryRowContext(ctx, query, deploymentID).Scan(
		&build.ID,
		&build.DeploymentID,
		&build.AppID,
		&build.GitURL,
		&build.GitRef,
		&build.FlakeOutput,
		&build.BuildType,
		&build.Status,
		&build.CreatedAt,
		&startedAt,
		&finishedAt,
		&buildStrategy,
		&build.TimeoutSeconds,
		&build.RetryCount,
		&build.RetryAsOCI,
		&generatedFlake,
		&flakeLock,
		&vendorHash,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying build by deployment: %w", err)
	}

	if startedAt.Valid {
		build.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		build.FinishedAt = &finishedAt.Time
	}
	if buildStrategy.Valid {
		build.BuildStrategy = models.BuildStrategy(buildStrategy.String)
	}
	if generatedFlake.Valid {
		build.GeneratedFlake = generatedFlake.String
	}
	if flakeLock.Valid {
		build.FlakeLock = flakeLock.String
	}
	if vendorHash.Valid {
		build.VendorHash = vendorHash.String
	}

	return build, nil
}

// Update updates an existing build job.
func (s *BuildStore) Update(ctx context.Context, build *models.BuildJob) error {
	query := `
		UPDATE builds
		SET status = $2, started_at = $3, finished_at = $4,
			build_strategy = $5, retry_count = $6, retry_as_oci = $7,
			generated_flake = $8, flake_lock = $9, vendor_hash = $10
		WHERE id = $1`

	// Handle nullable build_strategy
	var buildStrategy sql.NullString
	if build.BuildStrategy != "" {
		buildStrategy = sql.NullString{String: string(build.BuildStrategy), Valid: true}
	}

	// Handle nullable generated_flake
	var generatedFlake sql.NullString
	if build.GeneratedFlake != "" {
		generatedFlake = sql.NullString{String: build.GeneratedFlake, Valid: true}
	}

	// Handle nullable flake_lock
	var flakeLock sql.NullString
	if build.FlakeLock != "" {
		flakeLock = sql.NullString{String: build.FlakeLock, Valid: true}
	}

	// Handle nullable vendor_hash
	var vendorHash sql.NullString
	if build.VendorHash != "" {
		vendorHash = sql.NullString{String: build.VendorHash, Valid: true}
	}

	result, err := s.conn().ExecContext(ctx, query,
		build.ID,
		build.Status,
		build.StartedAt,
		build.FinishedAt,
		buildStrategy,
		build.RetryCount,
		build.RetryAsOCI,
		generatedFlake,
		flakeLock,
		vendorHash,
	)
	if err != nil {
		return fmt.Errorf("updating build: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// ListPending retrieves all pending build jobs.
func (s *BuildStore) ListPending(ctx context.Context) ([]*models.BuildJob, error) {
	query := `
		SELECT id, deployment_id, app_id, git_url, git_ref, 
			flake_output, build_type, status, created_at, started_at, finished_at,
			build_strategy, timeout_seconds, retry_count, retry_as_oci,
			generated_flake, flake_lock, vendor_hash
		FROM builds
		WHERE status = 'queued'
		ORDER BY created_at ASC`

	rows, err := s.conn().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying pending builds: %w", err)
	}
	defer rows.Close()

	var builds []*models.BuildJob

	for rows.Next() {
		build := &models.BuildJob{}
		var startedAt, finishedAt sql.NullTime
		var buildStrategy, generatedFlake, flakeLock, vendorHash sql.NullString

		err := rows.Scan(
			&build.ID,
			&build.DeploymentID,
			&build.AppID,
			&build.GitURL,
			&build.GitRef,
			&build.FlakeOutput,
			&build.BuildType,
			&build.Status,
			&build.CreatedAt,
			&startedAt,
			&finishedAt,
			&buildStrategy,
			&build.TimeoutSeconds,
			&build.RetryCount,
			&build.RetryAsOCI,
			&generatedFlake,
			&flakeLock,
			&vendorHash,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning build row: %w", err)
		}

		if startedAt.Valid {
			build.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			build.FinishedAt = &finishedAt.Time
		}
		if buildStrategy.Valid {
			build.BuildStrategy = models.BuildStrategy(buildStrategy.String)
		}
		if generatedFlake.Valid {
			build.GeneratedFlake = generatedFlake.String
		}
		if flakeLock.Valid {
			build.FlakeLock = flakeLock.String
		}
		if vendorHash.Valid {
			build.VendorHash = vendorHash.String
		}

		builds = append(builds, build)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating build rows: %w", err)
	}

	return builds, nil
}
