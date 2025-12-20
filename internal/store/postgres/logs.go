package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

// LogStore implements store.LogStore using PostgreSQL.
type LogStore struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *slog.Logger
}

// conn returns the queryable connection (transaction or database).
func (s *LogStore) conn() queryable {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Create creates a new log entry.
func (s *LogStore) Create(ctx context.Context, entry *models.LogEntry) error {
	query := `
		INSERT INTO logs (id, deployment_id, source, level, message, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	err := s.conn().QueryRowContext(ctx, query,
		entry.ID,
		entry.DeploymentID,
		entry.Source,
		entry.Level,
		entry.Message,
		entry.Timestamp,
	).Scan(&entry.ID)

	if err != nil {
		return fmt.Errorf("inserting log entry: %w", err)
	}

	return nil
}

// List retrieves log entries for a deployment.
func (s *LogStore) List(ctx context.Context, deploymentID string, limit int) ([]*models.LogEntry, error) {
	query := `
		SELECT id, deployment_id, source, level, message, timestamp
		FROM logs
		WHERE deployment_id = $1
		ORDER BY timestamp DESC
		LIMIT $2`

	rows, err := s.conn().QueryContext(ctx, query, deploymentID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying logs: %w", err)
	}
	defer rows.Close()

	return s.scanLogs(rows)
}

// ListBySource retrieves log entries filtered by source (build/runtime).
func (s *LogStore) ListBySource(ctx context.Context, deploymentID, source string, limit int) ([]*models.LogEntry, error) {
	query := `
		SELECT id, deployment_id, source, level, message, timestamp
		FROM logs
		WHERE deployment_id = $1 AND source = $2
		ORDER BY timestamp DESC
		LIMIT $3`

	rows, err := s.conn().QueryContext(ctx, query, deploymentID, source, limit)
	if err != nil {
		return nil, fmt.Errorf("querying logs by source: %w", err)
	}
	defer rows.Close()

	return s.scanLogs(rows)
}

// DeleteOlderThan removes log entries older than the specified timestamp.
func (s *LogStore) DeleteOlderThan(ctx context.Context, deploymentID string, before int64) error {
	query := `DELETE FROM logs WHERE deployment_id = $1 AND timestamp < $2`

	beforeTime := time.Unix(before, 0).UTC()

	_, err := s.conn().ExecContext(ctx, query, deploymentID, beforeTime)
	if err != nil {
		return fmt.Errorf("deleting old logs: %w", err)
	}

	return nil
}

// scanLogs scans multiple log entry rows.
func (s *LogStore) scanLogs(rows *sql.Rows) ([]*models.LogEntry, error) {
	var entries []*models.LogEntry

	for rows.Next() {
		entry := &models.LogEntry{}

		err := rows.Scan(
			&entry.ID,
			&entry.DeploymentID,
			&entry.Source,
			&entry.Level,
			&entry.Message,
			&entry.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning log row: %w", err)
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating log rows: %w", err)
	}

	return entries, nil
}
