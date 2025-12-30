package postgres

import (
	"context"
	"database/sql"
	"log/slog"
)

// SettingsStore implements store.SettingsStore for PostgreSQL.
type SettingsStore struct {
	db     queryable
	logger *slog.Logger
}

// Get retrieves a setting by key.
func (s *SettingsStore) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = $1", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// Set sets a setting key-value pair.
func (s *SettingsStore) Set(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at) 
		VALUES ($1, $2, CURRENT_TIMESTAMP) 
		ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = CURRENT_TIMESTAMP
	`, key, value)
	return err
}

// GetAll retrieves all global settings.
func (s *SettingsStore) GetAll(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		settings[key] = value
	}
	return settings, nil
}
