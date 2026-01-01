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
// Deprecated: Use CreateWithRole instead.
func (s *UserStore) Create(ctx context.Context, email, password string, isAdmin bool) (*store.User, error) {
	role := store.RoleMember
	if isAdmin {
		role = store.RoleOwner
	}
	return s.CreateWithRole(ctx, email, password, role, "")
}

// CreateWithRole creates a new user with hashed password and specified role.
func (s *UserStore) CreateWithRole(ctx context.Context, email, password string, role store.Role, invitedBy string) (*store.User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	id := uuid.New().String()
	now := time.Now().Unix()
	isAdmin := role == store.RoleOwner

	query := `
		INSERT INTO users (id, email, password_hash, is_admin, role, invited_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	var invitedByVal interface{}
	if invitedBy != "" {
		invitedByVal = invitedBy
	}

	_, err = s.conn().ExecContext(ctx, query, id, email, string(hashedPassword), isAdmin, string(role), invitedByVal, now)
	if err != nil {
		return nil, err
	}

	return &store.User{
		ID:        id,
		Email:     email,
		Role:      role,
		InvitedBy: invitedBy,
		IsAdmin:   isAdmin,
		CreatedAt: now,
	}, nil
}

// GetByEmail retrieves a user by email.
func (s *UserStore) GetByEmail(ctx context.Context, email string) (*store.User, error) {
	query := `SELECT id, email, name, avatar_url, is_admin, role, invited_by, created_at FROM users WHERE email = $1`

	var user store.User
	var name, avatarURL, role, invitedBy sql.NullString
	err := s.conn().QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &name, &avatarURL, &user.IsAdmin, &role, &invitedBy, &user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	user.Name = name.String
	user.AvatarURL = avatarURL.String
	user.InvitedBy = invitedBy.String
	if role.Valid {
		user.Role = store.Role(role.String)
	} else {
		// Fallback for existing users without role
		if user.IsAdmin {
			user.Role = store.RoleOwner
		} else {
			user.Role = store.RoleMember
		}
	}
	return &user, nil
}

// GetByID retrieves a user by ID.
func (s *UserStore) GetByID(ctx context.Context, id string) (*store.User, error) {
	query := `SELECT id, email, name, avatar_url, is_admin, role, invited_by, created_at FROM users WHERE id = $1`

	var user store.User
	var name, avatarURL, role, invitedBy sql.NullString
	err := s.conn().QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Email, &name, &avatarURL, &user.IsAdmin, &role, &invitedBy, &user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	user.Name = name.String
	user.AvatarURL = avatarURL.String
	user.InvitedBy = invitedBy.String
	if role.Valid {
		user.Role = store.Role(role.String)
	} else {
		// Fallback for existing users without role
		if user.IsAdmin {
			user.Role = store.RoleOwner
		} else {
			user.Role = store.RoleMember
		}
	}
	return &user, nil
}

// Authenticate verifies credentials and returns the user.
func (s *UserStore) Authenticate(ctx context.Context, email, password string) (*store.User, error) {
	query := `SELECT id, email, name, avatar_url, password_hash, is_admin, role, invited_by, created_at FROM users WHERE email = $1`

	var user store.User
	var name, avatarURL, role, invitedBy sql.NullString
	var passwordHash string
	err := s.conn().QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &name, &avatarURL, &passwordHash, &user.IsAdmin, &role, &invitedBy, &user.CreatedAt,
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
	user.InvitedBy = invitedBy.String
	if role.Valid {
		user.Role = store.Role(role.String)
	} else {
		// Fallback for existing users without role
		if user.IsAdmin {
			user.Role = store.RoleOwner
		} else {
			user.Role = store.RoleMember
		}
	}
	return &user, nil
}

// Update updates an existing user's information.
func (s *UserStore) Update(ctx context.Context, user *store.User) error {
	query := `
		UPDATE users 
		SET email = $1, name = $2, avatar_url = $3, is_admin = $4, role = $5
		WHERE id = $6
	`
	_, err := s.conn().ExecContext(ctx, query, user.Email, user.Name, user.AvatarURL, user.IsAdmin, string(user.Role), user.ID)
	return err
}

// List retrieves all users.
func (s *UserStore) List(ctx context.Context) ([]*store.User, error) {
	query := `SELECT id, email, name, avatar_url, is_admin, role, invited_by, created_at FROM users ORDER BY created_at`

	rows, err := s.conn().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*store.User
	for rows.Next() {
		var user store.User
		var name, avatarURL, role, invitedBy sql.NullString
		if err := rows.Scan(&user.ID, &user.Email, &name, &avatarURL, &user.IsAdmin, &role, &invitedBy, &user.CreatedAt); err != nil {
			return nil, err
		}
		user.Name = name.String
		user.AvatarURL = avatarURL.String
		user.InvitedBy = invitedBy.String
		if role.Valid {
			user.Role = store.Role(role.String)
		} else {
			// Fallback for existing users without role
			if user.IsAdmin {
				user.Role = store.RoleOwner
			} else {
				user.Role = store.RoleMember
			}
		}
		users = append(users, &user)
	}

	return users, rows.Err()
}

// Delete removes a user by ID.
func (s *UserStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM users WHERE id = $1`
	_, err := s.conn().ExecContext(ctx, query, id)
	return err
}

// CountByRole returns the number of users with a specific role.
func (s *UserStore) CountByRole(ctx context.Context, role store.Role) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE role = $1`
	var count int
	err := s.conn().QueryRowContext(ctx, query, string(role)).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetFirstOwner returns the first user with owner role, if any.
func (s *UserStore) GetFirstOwner(ctx context.Context) (*store.User, error) {
	query := `SELECT id, email, name, avatar_url, is_admin, role, invited_by, created_at FROM users WHERE role = $1 ORDER BY created_at LIMIT 1`

	var user store.User
	var name, avatarURL, role, invitedBy sql.NullString
	err := s.conn().QueryRowContext(ctx, query, string(store.RoleOwner)).Scan(
		&user.ID, &user.Email, &name, &avatarURL, &user.IsAdmin, &role, &invitedBy, &user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	user.Name = name.String
	user.AvatarURL = avatarURL.String
	user.InvitedBy = invitedBy.String
	if role.Valid {
		user.Role = store.Role(role.String)
	} else {
		user.Role = store.RoleOwner
	}
	return &user, nil
}
