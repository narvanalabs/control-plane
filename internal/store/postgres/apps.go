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

// AppStore implements store.AppStore using PostgreSQL.
type AppStore struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *slog.Logger
}

// conn returns the queryable connection (transaction or database).
func (s *AppStore) conn() queryable {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Create creates a new application.
func (s *AppStore) Create(ctx context.Context, app *models.App) error {
	servicesJSON, err := json.Marshal(app.Services)
	if err != nil {
		return fmt.Errorf("marshaling services: %w", err)
	}

	query := `
		INSERT INTO apps (id, owner_id, name, description, build_type, services, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`

	now := time.Now().UTC()
	if app.CreatedAt.IsZero() {
		app.CreatedAt = now
	}
	if app.UpdatedAt.IsZero() {
		app.UpdatedAt = now
	}

	err = s.conn().QueryRowContext(ctx, query,
		app.ID,
		app.OwnerID,
		app.Name,
		app.Description,
		app.BuildType,
		servicesJSON,
		app.CreatedAt,
		app.UpdatedAt,
	).Scan(&app.ID, &app.CreatedAt, &app.UpdatedAt)

	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateName
		}
		return fmt.Errorf("inserting app: %w", err)
	}

	return nil
}


// Get retrieves an application by ID.
func (s *AppStore) Get(ctx context.Context, id string) (*models.App, error) {
	query := `
		SELECT id, owner_id, name, description, build_type, services, 
		       created_at, updated_at, deleted_at
		FROM apps
		WHERE id = $1 AND deleted_at IS NULL`

	app := &models.App{}
	var servicesJSON []byte
	var deletedAt sql.NullTime

	err := s.conn().QueryRowContext(ctx, query, id).Scan(
		&app.ID,
		&app.OwnerID,
		&app.Name,
		&app.Description,
		&app.BuildType,
		&servicesJSON,
		&app.CreatedAt,
		&app.UpdatedAt,
		&deletedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying app: %w", err)
	}

	if err := json.Unmarshal(servicesJSON, &app.Services); err != nil {
		return nil, fmt.Errorf("unmarshaling services: %w", err)
	}

	if deletedAt.Valid {
		app.DeletedAt = &deletedAt.Time
	}

	return app, nil
}

// GetByName retrieves an application by owner ID and name.
func (s *AppStore) GetByName(ctx context.Context, ownerID, name string) (*models.App, error) {
	query := `
		SELECT id, owner_id, name, description, build_type, services, 
		       created_at, updated_at, deleted_at
		FROM apps
		WHERE owner_id = $1 AND name = $2 AND deleted_at IS NULL`

	app := &models.App{}
	var servicesJSON []byte
	var deletedAt sql.NullTime

	err := s.conn().QueryRowContext(ctx, query, ownerID, name).Scan(
		&app.ID,
		&app.OwnerID,
		&app.Name,
		&app.Description,
		&app.BuildType,
		&servicesJSON,
		&app.CreatedAt,
		&app.UpdatedAt,
		&deletedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying app by name: %w", err)
	}

	if err := json.Unmarshal(servicesJSON, &app.Services); err != nil {
		return nil, fmt.Errorf("unmarshaling services: %w", err)
	}

	if deletedAt.Valid {
		app.DeletedAt = &deletedAt.Time
	}

	return app, nil
}

// List retrieves all applications for a given owner.
func (s *AppStore) List(ctx context.Context, ownerID string) ([]*models.App, error) {
	query := `
		SELECT id, owner_id, name, description, build_type, services, 
		       created_at, updated_at, deleted_at
		FROM apps
		WHERE owner_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC`

	rows, err := s.conn().QueryContext(ctx, query, ownerID)
	if err != nil {
		return nil, fmt.Errorf("querying apps: %w", err)
	}
	defer rows.Close()

	var apps []*models.App
	for rows.Next() {
		app := &models.App{}
		var servicesJSON []byte
		var deletedAt sql.NullTime

		err := rows.Scan(
			&app.ID,
			&app.OwnerID,
			&app.Name,
			&app.Description,
			&app.BuildType,
			&servicesJSON,
			&app.CreatedAt,
			&app.UpdatedAt,
			&deletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning app row: %w", err)
		}

		if err := json.Unmarshal(servicesJSON, &app.Services); err != nil {
			return nil, fmt.Errorf("unmarshaling services: %w", err)
		}

		if deletedAt.Valid {
			app.DeletedAt = &deletedAt.Time
		}

		apps = append(apps, app)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating app rows: %w", err)
	}

	return apps, nil
}


// Update updates an existing application.
func (s *AppStore) Update(ctx context.Context, app *models.App) error {
	servicesJSON, err := json.Marshal(app.Services)
	if err != nil {
		return fmt.Errorf("marshaling services: %w", err)
	}

	query := `
		UPDATE apps
		SET name = $2, description = $3, build_type = $4, services = $5, updated_at = $6
		WHERE id = $1 AND deleted_at IS NULL`

	app.UpdatedAt = time.Now().UTC()

	result, err := s.conn().ExecContext(ctx, query,
		app.ID,
		app.Name,
		app.Description,
		app.BuildType,
		servicesJSON,
		app.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateName
		}
		return fmt.Errorf("updating app: %w", err)
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

// Delete soft-deletes an application by setting deleted_at.
func (s *AppStore) Delete(ctx context.Context, id string) error {
	query := `
		UPDATE apps
		SET deleted_at = $2, updated_at = $2
		WHERE id = $1 AND deleted_at IS NULL`

	now := time.Now().UTC()

	result, err := s.conn().ExecContext(ctx, query, id, now)
	if err != nil {
		return fmt.Errorf("deleting app: %w", err)
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
