package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// SecretStore implements store.SecretStore using PostgreSQL.
type SecretStore struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *slog.Logger
}

// conn returns the queryable connection (transaction or database).
func (s *SecretStore) conn() queryable {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Set creates or updates a secret for an application.
func (s *SecretStore) Set(ctx context.Context, appID, key string, encryptedValue []byte) error {
	query := `
		INSERT INTO secrets (app_id, key, encrypted_value, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (app_id, key) DO UPDATE SET
			encrypted_value = EXCLUDED.encrypted_value,
			updated_at = EXCLUDED.updated_at`

	now := time.Now().UTC()

	_, err := s.conn().ExecContext(ctx, query, appID, key, encryptedValue, now)
	if err != nil {
		return fmt.Errorf("setting secret: %w", err)
	}

	return nil
}

// Get retrieves a secret by app ID and key.
func (s *SecretStore) Get(ctx context.Context, appID, key string) ([]byte, error) {
	query := `
		SELECT encrypted_value
		FROM secrets
		WHERE app_id = $1 AND key = $2`

	var encryptedValue []byte

	err := s.conn().QueryRowContext(ctx, query, appID, key).Scan(&encryptedValue)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying secret: %w", err)
	}

	return encryptedValue, nil
}

// List retrieves all secret keys for an application.
func (s *SecretStore) List(ctx context.Context, appID string) ([]string, error) {
	query := `
		SELECT key
		FROM secrets
		WHERE app_id = $1
		ORDER BY key`

	rows, err := s.conn().QueryContext(ctx, query, appID)
	if err != nil {
		return nil, fmt.Errorf("querying secret keys: %w", err)
	}
	defer rows.Close()

	var keys []string

	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scanning key: %w", err)
		}
		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating keys: %w", err)
	}

	return keys, nil
}


// Delete removes a secret.
func (s *SecretStore) Delete(ctx context.Context, appID, key string) error {
	query := `DELETE FROM secrets WHERE app_id = $1 AND key = $2`

	result, err := s.conn().ExecContext(ctx, query, appID, key)
	if err != nil {
		return fmt.Errorf("deleting secret: %w", err)
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

// GetAll retrieves all secrets for an application as a map.
func (s *SecretStore) GetAll(ctx context.Context, appID string) (map[string][]byte, error) {
	query := `
		SELECT key, encrypted_value
		FROM secrets
		WHERE app_id = $1`

	rows, err := s.conn().QueryContext(ctx, query, appID)
	if err != nil {
		return nil, fmt.Errorf("querying secrets: %w", err)
	}
	defer rows.Close()

	secrets := make(map[string][]byte)

	for rows.Next() {
		var key string
		var encryptedValue []byte
		if err := rows.Scan(&key, &encryptedValue); err != nil {
			return nil, fmt.Errorf("scanning secret: %w", err)
		}
		secrets[key] = encryptedValue
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating secrets: %w", err)
	}

	return secrets, nil
}
