package postgres

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

// GitHubAccountStore implements store.GitHubAccountStore using PostgreSQL.
type GitHubAccountStore struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *slog.Logger
}

func (s *GitHubAccountStore) conn() queryable {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Create saves a new GitHub account.
func (s *GitHubAccountStore) Create(ctx context.Context, account *models.GitHubAccount) error {
	now := time.Now().Unix()
	var expiry int64
	if !account.Expiry.IsZero() {
		expiry = account.Expiry.Unix()
	}

	query := `
		INSERT INTO github_accounts (id, login, name, email, avatar_url, access_token, refresh_token, expiry, token_type, created_at, updated_at, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			login = EXCLUDED.login,
			name = EXCLUDED.name,
			email = EXCLUDED.email,
			avatar_url = EXCLUDED.avatar_url,
			access_token = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			expiry = EXCLUDED.expiry,
			token_type = EXCLUDED.token_type,
			updated_at = EXCLUDED.updated_at
	`

	_, err := s.conn().ExecContext(ctx, query,
		account.ID, account.Login, account.Name, account.Email, account.AvatarURL,
		account.AccessToken, account.RefreshToken, expiry, account.TokenType,
		now, now, account.UserID,
	)
	return err
}

// Get retrieves a GitHub account by its ID.
func (s *GitHubAccountStore) Get(ctx context.Context, id int64) (*models.GitHubAccount, error) {
	query := `
		SELECT id, login, name, email, avatar_url, access_token, refresh_token, expiry, token_type, created_at, updated_at, user_id
		FROM github_accounts
		WHERE id = $1
	`

	var account models.GitHubAccount
	var expiry, createdAt, updatedAt int64
	err := s.conn().QueryRowContext(ctx, query, id).Scan(
		&account.ID, &account.Login, &account.Name, &account.Email, &account.AvatarURL,
		&account.AccessToken, &account.RefreshToken, &expiry, &account.TokenType,
		&createdAt, &updatedAt, &account.UserID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if expiry > 0 {
		account.Expiry = time.Unix(expiry, 0)
	}
	account.CreatedAt = time.Unix(createdAt, 0)
	account.UpdatedAt = time.Unix(updatedAt, 0)

	return &account, nil
}

// GetByUserID retrieves a GitHub account by its associated user ID.
func (s *GitHubAccountStore) GetByUserID(ctx context.Context, userID string) (*models.GitHubAccount, error) {
	query := `
		SELECT id, login, name, email, avatar_url, access_token, refresh_token, expiry, token_type, created_at, updated_at, user_id
		FROM github_accounts
		WHERE user_id = $1
	`

	var account models.GitHubAccount
	var expiry, createdAt, updatedAt int64
	err := s.conn().QueryRowContext(ctx, query, userID).Scan(
		&account.ID, &account.Login, &account.Name, &account.Email, &account.AvatarURL,
		&account.AccessToken, &account.RefreshToken, &expiry, &account.TokenType,
		&createdAt, &updatedAt, &account.UserID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if expiry > 0 {
		account.Expiry = time.Unix(expiry, 0)
	}
	account.CreatedAt = time.Unix(createdAt, 0)
	account.UpdatedAt = time.Unix(updatedAt, 0)

	return &account, nil
}

// Update updates an existing GitHub account.
func (s *GitHubAccountStore) Update(ctx context.Context, account *models.GitHubAccount) error {
	now := time.Now().Unix()
	var expiry int64
	if !account.Expiry.IsZero() {
		expiry = account.Expiry.Unix()
	}

	query := `
		UPDATE github_accounts SET
			login = $1,
			name = $2,
			email = $3,
			avatar_url = $4,
			access_token = $5,
			refresh_token = $6,
			expiry = $7,
			token_type = $8,
			updated_at = $9
		WHERE id = $10
	`

	_, err := s.conn().ExecContext(ctx, query,
		account.Login, account.Name, account.Email, account.AvatarURL,
		account.AccessToken, account.RefreshToken, expiry, account.TokenType,
		now, account.ID,
	)
	return err
}

// Delete removes a GitHub account.
func (s *GitHubAccountStore) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM github_accounts WHERE id = $1`
	_, err := s.conn().ExecContext(ctx, query, id)
	return err
}
