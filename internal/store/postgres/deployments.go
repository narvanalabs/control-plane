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
)

// DeploymentStore implements store.DeploymentStore using PostgreSQL.
type DeploymentStore struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *slog.Logger
}

// conn returns the queryable connection (transaction or database).
func (s *DeploymentStore) conn() queryable {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Create creates a new deployment.
func (s *DeploymentStore) Create(ctx context.Context, deployment *models.Deployment) error {
	configJSON, err := json.Marshal(deployment.Config)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	dependsOnJSON, err := json.Marshal(deployment.DependsOn)
	if err != nil {
		return fmt.Errorf("marshaling depends_on: %w", err)
	}

	query := `
		INSERT INTO deployments (id, app_id, service_name, version, git_ref, git_commit, 
			build_type, artifact, status, node_id, resource_tier, config, depends_on,
			created_at, updated_at, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING id, created_at, updated_at`

	now := time.Now().UTC()
	if deployment.CreatedAt.IsZero() {
		deployment.CreatedAt = now
	}
	if deployment.UpdatedAt.IsZero() {
		deployment.UpdatedAt = now
	}

	var nodeID *string
	if deployment.NodeID != "" {
		nodeID = &deployment.NodeID
	}

	err = s.conn().QueryRowContext(ctx, query,
		deployment.ID,
		deployment.AppID,
		deployment.ServiceName,
		deployment.Version,
		deployment.GitRef,
		deployment.GitCommit,
		deployment.BuildType,
		deployment.Artifact,
		deployment.Status,
		nodeID,
		deployment.ResourceTier,
		configJSON,
		dependsOnJSON,
		deployment.CreatedAt,
		deployment.UpdatedAt,
		deployment.StartedAt,
		deployment.FinishedAt,
	).Scan(&deployment.ID, &deployment.CreatedAt, &deployment.UpdatedAt)

	if err != nil {
		return fmt.Errorf("inserting deployment: %w", err)
	}

	return nil
}


// Get retrieves a deployment by ID.
func (s *DeploymentStore) Get(ctx context.Context, id string) (*models.Deployment, error) {
	query := `
		SELECT id, app_id, service_name, version, git_ref, git_commit, 
			build_type, artifact, status, node_id, resource_tier, config, depends_on,
			created_at, updated_at, started_at, finished_at
		FROM deployments
		WHERE id = $1`

	deployment := &models.Deployment{}
	var configJSON []byte
	var dependsOnJSON []byte
	var nodeID sql.NullString
	var startedAt, finishedAt sql.NullTime

	err := s.conn().QueryRowContext(ctx, query, id).Scan(
		&deployment.ID,
		&deployment.AppID,
		&deployment.ServiceName,
		&deployment.Version,
		&deployment.GitRef,
		&deployment.GitCommit,
		&deployment.BuildType,
		&deployment.Artifact,
		&deployment.Status,
		&nodeID,
		&deployment.ResourceTier,
		&configJSON,
		&dependsOnJSON,
		&deployment.CreatedAt,
		&deployment.UpdatedAt,
		&startedAt,
		&finishedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying deployment: %w", err)
	}

	if nodeID.Valid {
		deployment.NodeID = nodeID.String
	}
	if startedAt.Valid {
		deployment.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		deployment.FinishedAt = &finishedAt.Time
	}

	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &deployment.Config); err != nil {
			return nil, fmt.Errorf("unmarshaling config: %w", err)
		}
	}

	if len(dependsOnJSON) > 0 {
		if err := json.Unmarshal(dependsOnJSON, &deployment.DependsOn); err != nil {
			return nil, fmt.Errorf("unmarshaling depends_on: %w", err)
		}
	}

	return deployment, nil
}

// List retrieves all deployments for a given application, ordered by created_at DESC.
func (s *DeploymentStore) List(ctx context.Context, appID string) ([]*models.Deployment, error) {
	query := `
		SELECT id, app_id, service_name, version, git_ref, git_commit, 
			build_type, artifact, status, node_id, resource_tier, config, depends_on,
			created_at, updated_at, started_at, finished_at
		FROM deployments
		WHERE app_id = $1
		ORDER BY created_at DESC`

	rows, err := s.conn().QueryContext(ctx, query, appID)
	if err != nil {
		return nil, fmt.Errorf("querying deployments: %w", err)
	}
	defer rows.Close()

	return s.scanDeployments(rows)
}

// ListByNode retrieves all deployments assigned to a given node.
func (s *DeploymentStore) ListByNode(ctx context.Context, nodeID string) ([]*models.Deployment, error) {
	query := `
		SELECT id, app_id, service_name, version, git_ref, git_commit, 
			build_type, artifact, status, node_id, resource_tier, config, depends_on,
			created_at, updated_at, started_at, finished_at
		FROM deployments
		WHERE node_id = $1
		ORDER BY created_at DESC`

	rows, err := s.conn().QueryContext(ctx, query, nodeID)
	if err != nil {
		return nil, fmt.Errorf("querying deployments by node: %w", err)
	}
	defer rows.Close()

	return s.scanDeployments(rows)
}

// ListByStatus retrieves all deployments with a given status.
func (s *DeploymentStore) ListByStatus(ctx context.Context, status models.DeploymentStatus) ([]*models.Deployment, error) {
	query := `
		SELECT id, app_id, service_name, version, git_ref, git_commit, 
			build_type, artifact, status, node_id, resource_tier, config, depends_on,
			created_at, updated_at, started_at, finished_at
		FROM deployments
		WHERE status = $1
		ORDER BY created_at ASC`

	rows, err := s.conn().QueryContext(ctx, query, status)
	if err != nil {
		return nil, fmt.Errorf("querying deployments by status: %w", err)
	}
	defer rows.Close()

	return s.scanDeployments(rows)
}


// Update updates an existing deployment.
func (s *DeploymentStore) Update(ctx context.Context, deployment *models.Deployment) error {
	configJSON, err := json.Marshal(deployment.Config)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	dependsOnJSON, err := json.Marshal(deployment.DependsOn)
	if err != nil {
		return fmt.Errorf("marshaling depends_on: %w", err)
	}

	query := `
		UPDATE deployments
		SET service_name = $2, version = $3, git_ref = $4, git_commit = $5,
			build_type = $6, artifact = $7, status = $8, node_id = $9,
			resource_tier = $10, config = $11, depends_on = $12, updated_at = $13,
			started_at = $14, finished_at = $15
		WHERE id = $1`

	deployment.UpdatedAt = time.Now().UTC()

	var nodeID *string
	if deployment.NodeID != "" {
		nodeID = &deployment.NodeID
	}

	result, err := s.conn().ExecContext(ctx, query,
		deployment.ID,
		deployment.ServiceName,
		deployment.Version,
		deployment.GitRef,
		deployment.GitCommit,
		deployment.BuildType,
		deployment.Artifact,
		deployment.Status,
		nodeID,
		deployment.ResourceTier,
		configJSON,
		dependsOnJSON,
		deployment.UpdatedAt,
		deployment.StartedAt,
		deployment.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("updating deployment: %w", err)
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

// GetLatestSuccessful retrieves the most recent successful deployment for an app.
func (s *DeploymentStore) GetLatestSuccessful(ctx context.Context, appID string) (*models.Deployment, error) {
	query := `
		SELECT id, app_id, service_name, version, git_ref, git_commit, 
			build_type, artifact, status, node_id, resource_tier, config, depends_on,
			created_at, updated_at, started_at, finished_at
		FROM deployments
		WHERE app_id = $1 AND status = 'running'
		ORDER BY created_at DESC
		LIMIT 1`

	deployment := &models.Deployment{}
	var configJSON []byte
	var dependsOnJSON []byte
	var nodeID sql.NullString
	var startedAt, finishedAt sql.NullTime

	err := s.conn().QueryRowContext(ctx, query, appID).Scan(
		&deployment.ID,
		&deployment.AppID,
		&deployment.ServiceName,
		&deployment.Version,
		&deployment.GitRef,
		&deployment.GitCommit,
		&deployment.BuildType,
		&deployment.Artifact,
		&deployment.Status,
		&nodeID,
		&deployment.ResourceTier,
		&configJSON,
		&dependsOnJSON,
		&deployment.CreatedAt,
		&deployment.UpdatedAt,
		&startedAt,
		&finishedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying latest successful deployment: %w", err)
	}

	if nodeID.Valid {
		deployment.NodeID = nodeID.String
	}
	if startedAt.Valid {
		deployment.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		deployment.FinishedAt = &finishedAt.Time
	}

	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &deployment.Config); err != nil {
			return nil, fmt.Errorf("unmarshaling config: %w", err)
		}
	}

	if len(dependsOnJSON) > 0 {
		if err := json.Unmarshal(dependsOnJSON, &deployment.DependsOn); err != nil {
			return nil, fmt.Errorf("unmarshaling depends_on: %w", err)
		}
	}

	return deployment, nil
}

// scanDeployments scans multiple deployment rows.
func (s *DeploymentStore) scanDeployments(rows *sql.Rows) ([]*models.Deployment, error) {
	var deployments []*models.Deployment

	for rows.Next() {
		deployment := &models.Deployment{}
		var configJSON []byte
		var dependsOnJSON []byte
		var nodeID sql.NullString
		var startedAt, finishedAt sql.NullTime

		err := rows.Scan(
			&deployment.ID,
			&deployment.AppID,
			&deployment.ServiceName,
			&deployment.Version,
			&deployment.GitRef,
			&deployment.GitCommit,
			&deployment.BuildType,
			&deployment.Artifact,
			&deployment.Status,
			&nodeID,
			&deployment.ResourceTier,
			&configJSON,
			&dependsOnJSON,
			&deployment.CreatedAt,
			&deployment.UpdatedAt,
			&startedAt,
			&finishedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning deployment row: %w", err)
		}

		if nodeID.Valid {
			deployment.NodeID = nodeID.String
		}
		if startedAt.Valid {
			deployment.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			deployment.FinishedAt = &finishedAt.Time
		}

		if len(configJSON) > 0 {
			if err := json.Unmarshal(configJSON, &deployment.Config); err != nil {
				return nil, fmt.Errorf("unmarshaling config: %w", err)
			}
		}

		if len(dependsOnJSON) > 0 {
			if err := json.Unmarshal(dependsOnJSON, &deployment.DependsOn); err != nil {
				return nil, fmt.Errorf("unmarshaling depends_on: %w", err)
			}
		}

		deployments = append(deployments, deployment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating deployment rows: %w", err)
	}

	return deployments, nil
}
