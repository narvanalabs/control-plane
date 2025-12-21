package postgres

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/narvanalabs/control-plane/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// UserStore implements store.UserStore using PostgreSQL.
type UserStore struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *slog.Logger
}

func (s *UserStore) conn() queryable {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Create creates a new user with hashed password.
func (s *UserStore) Create(ctx context.Context, email, password string, isAdmin bool) (*store.User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	id := uuid.New().String()
	now := time.Now().Unix()

	query := `
		INSERT INTO users (id, email, password_hash, is_admin, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err = s.conn().ExecContext(ctx, query, id, email, string(hashedPassword), isAdmin, now)
	if err != nil {
		return nil, err
	}

	return &store.User{
		ID:        id,
		Email:     email,
		IsAdmin:   isAdmin,
		CreatedAt: now,
	}, nil
}

// GetByEmail retrieves a user by email.
func (s *UserStore) GetByEmail(ctx context.Context, email string) (*store.User, error) {
	query := `SELECT id, email, is_admin, created_at FROM users WHERE email = $1`

	var user store.User
	err := s.conn().QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &user.IsAdmin, &user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Authenticate verifies credentials and returns the user.
func (s *UserStore) Authenticate(ctx context.Context, email, password string) (*store.User, error) {
	query := `SELECT id, email, password_hash, is_admin, created_at FROM users WHERE email = $1`

	var user store.User
	var passwordHash string
	err := s.conn().QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &passwordHash, &user.IsAdmin, &user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("invalid credentials")
	}
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}

	return &user, nil
}

// List retrieves all users.
func (s *UserStore) List(ctx context.Context) ([]*store.User, error) {
	query := `SELECT id, email, is_admin, created_at FROM users ORDER BY created_at`

	rows, err := s.conn().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*store.User
	for rows.Next() {
		var user store.User
		if err := rows.Scan(&user.ID, &user.Email, &user.IsAdmin, &user.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, &user)
	}

	return users, rows.Err()
}
