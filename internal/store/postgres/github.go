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

// GitHubStore implements store.GitHubStore using PostgreSQL.
type GitHubStore struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *slog.Logger
}

func (s *GitHubStore) conn() queryable {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// GetConfig retrieves the GitHub App configuration.
func (s *GitHubStore) GetConfig(ctx context.Context) (*models.GitHubAppConfig, error) {
	query := `
		SELECT id, config_type, app_id, client_id, client_secret, webhook_secret, private_key, slug, created_at, updated_at
		FROM github_app_settings
		WHERE id = 'default'
	`

	var config models.GitHubAppConfig
	var createdAt, updatedAt int64
	err := s.conn().QueryRowContext(ctx, query).Scan(
		&config.ID, &config.ConfigType, &config.AppID, &config.ClientID, &config.ClientSecret,
		&config.WebhookSecret, &config.PrivateKey, &config.Slug,
		&createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	config.CreatedAt = time.Unix(createdAt, 0)
	config.UpdatedAt = time.Unix(updatedAt, 0)

	return &config, nil
}

// SaveConfig saves the GitHub App configuration.
func (s *GitHubStore) SaveConfig(ctx context.Context, config *models.GitHubAppConfig) error {
	now := time.Now().Unix()
	config.ID = "default"

	query := `
		INSERT INTO github_app_settings (id, config_type, app_id, client_id, client_secret, webhook_secret, private_key, slug, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			config_type = EXCLUDED.config_type,
			app_id = EXCLUDED.app_id,
			client_id = EXCLUDED.client_id,
			client_secret = EXCLUDED.client_secret,
			webhook_secret = EXCLUDED.webhook_secret,
			private_key = EXCLUDED.private_key,
			slug = EXCLUDED.slug,
			updated_at = EXCLUDED.updated_at
	`

	_, err := s.conn().ExecContext(ctx, query,
		config.ID, config.ConfigType, config.AppID, config.ClientID, config.ClientSecret,
		config.WebhookSecret, config.PrivateKey, config.Slug,
		now, now,
	)
	return err
}

// CreateInstallation saves a new GitHub App installation.
func (s *GitHubStore) CreateInstallation(ctx context.Context, inst *models.GitHubInstallation) error {
	now := time.Now().Unix()

	query := `
		INSERT INTO github_installations (id, account_id, account_login, account_type, access_tokens_url, repositories_url, html_url, created_at, updated_at, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			account_login = EXCLUDED.account_login,
			access_tokens_url = EXCLUDED.access_tokens_url,
			repositories_url = EXCLUDED.repositories_url,
			updated_at = EXCLUDED.updated_at
	`

	_, err := s.conn().ExecContext(ctx, query,
		inst.ID, inst.AccountID, inst.AccountLogin, inst.AccountType,
		inst.AccessTokensURL, inst.RepositoriesURL, inst.HTMLURL,
		now, now, inst.UserID,
	)
	return err
}

// GetInstallation retrieves an installation by its ID.
func (s *GitHubStore) GetInstallation(ctx context.Context, id int64) (*models.GitHubInstallation, error) {
	query := `
		SELECT id, account_id, account_login, account_type, access_tokens_url, repositories_url, html_url, created_at, updated_at, user_id
		FROM github_installations
		WHERE id = $1
	`

	var inst models.GitHubInstallation
	var createdAt, updatedAt int64
	err := s.conn().QueryRowContext(ctx, query, id).Scan(
		&inst.ID, &inst.AccountID, &inst.AccountLogin, &inst.AccountType,
		&inst.AccessTokensURL, &inst.RepositoriesURL, &inst.HTMLURL,
		&createdAt, &updatedAt, &inst.UserID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	inst.CreatedAt = time.Unix(createdAt, 0)
	inst.UpdatedAt = time.Unix(updatedAt, 0)

	return &inst, nil
}

// ListInstallations retrieves all installations for a given user.
func (s *GitHubStore) ListInstallations(ctx context.Context, userID string) ([]*models.GitHubInstallation, error) {
	query := `
		SELECT id, account_id, account_login, account_type, access_tokens_url, repositories_url, html_url, created_at, updated_at, user_id
		FROM github_installations
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := s.conn().QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var installations []*models.GitHubInstallation
	for rows.Next() {
		var inst models.GitHubInstallation
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&inst.ID, &inst.AccountID, &inst.AccountLogin, &inst.AccountType,
			&inst.AccessTokensURL, &inst.RepositoriesURL, &inst.HTMLURL,
			&createdAt, &updatedAt, &inst.UserID,
		); err != nil {
			return nil, err
		}
		inst.CreatedAt = time.Unix(createdAt, 0)
		inst.UpdatedAt = time.Unix(updatedAt, 0)
		installations = append(installations, &inst)
	}

	return installations, rows.Err()
}

// DeleteInstallation removes an installation.
func (s *GitHubStore) DeleteInstallation(ctx context.Context, id int64) error {
	query := `DELETE FROM github_installations WHERE id = $1`
	_, err := s.conn().ExecContext(ctx, query, id)
	return err
}
// ResetConfig clears the GitHub App configuration and all installations.
func (s *GitHubStore) ResetConfig(ctx context.Context) error {
	// 1. Delete all installations
	_, err := s.conn().ExecContext(ctx, "DELETE FROM github_installations")
	if err != nil {
		return fmt.Errorf("deleting installations: %w", err)
	}

	// 2. Delete all OAuth accounts
	_, err = s.conn().ExecContext(ctx, "DELETE FROM github_accounts")
	if err != nil {
		return fmt.Errorf("deleting accounts: %w", err)
	}

	// 3. Clear the main config
	_, err = s.conn().ExecContext(ctx, "DELETE FROM github_app_settings WHERE id = 'default'")
	if err != nil {
		return fmt.Errorf("deleting config: %w", err)
	}

	return nil
}
