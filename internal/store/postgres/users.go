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
	query := `SELECT id, email, name, avatar_url, is_admin, created_at FROM users WHERE email = $1`

	var user store.User
	var name, avatarURL sql.NullString
	err := s.conn().QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &name, &avatarURL, &user.IsAdmin, &user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	user.Name = name.String
	user.AvatarURL = avatarURL.String
	return &user, nil
}

// GetByID retrieves a user by ID.
func (s *UserStore) GetByID(ctx context.Context, id string) (*store.User, error) {
	query := `SELECT id, email, name, avatar_url, is_admin, created_at FROM users WHERE id = $1`

	var user store.User
	var name, avatarURL sql.NullString
	err := s.conn().QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Email, &name, &avatarURL, &user.IsAdmin, &user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	user.Name = name.String
	user.AvatarURL = avatarURL.String
	return &user, nil
}

// Authenticate verifies credentials and returns the user.
func (s *UserStore) Authenticate(ctx context.Context, email, password string) (*store.User, error) {
	query := `SELECT id, email, name, avatar_url, password_hash, is_admin, created_at FROM users WHERE email = $1`

	var user store.User
	var name, avatarURL sql.NullString
	var passwordHash string
	err := s.conn().QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &name, &avatarURL, &passwordHash, &user.IsAdmin, &user.CreatedAt,
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

	user.Name = name.String
	user.AvatarURL = avatarURL.String
	return &user, nil
}

// Update updates an existing user's information.
func (s *UserStore) Update(ctx context.Context, user *store.User) error {
	query := `
		UPDATE users 
		SET email = $1, name = $2, avatar_url = $3, is_admin = $4
		WHERE id = $5
	`
	_, err := s.conn().ExecContext(ctx, query, user.Email, user.Name, user.AvatarURL, user.IsAdmin, user.ID)
	return err
}

// List retrieves all users.
func (s *UserStore) List(ctx context.Context) ([]*store.User, error) {
	query := `SELECT id, email, name, avatar_url, is_admin, created_at FROM users ORDER BY created_at`

	rows, err := s.conn().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*store.User
	for rows.Next() {
		var user store.User
		var name, avatarURL sql.NullString
		if err := rows.Scan(&user.ID, &user.Email, &name, &avatarURL, &user.IsAdmin, &user.CreatedAt); err != nil {
			return nil, err
		}
		user.Name = name.String
		user.AvatarURL = avatarURL.String
		users = append(users, &user)
	}

	return users, rows.Err()
}
