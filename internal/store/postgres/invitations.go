package postgres

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/narvanalabs/control-plane/internal/models"
)

// InvitationStore implements store.InvitationStore using PostgreSQL.
type InvitationStore struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *slog.Logger
}

func (s *InvitationStore) conn() queryable {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Create creates a new invitation.
func (s *InvitationStore) Create(ctx context.Context, invitation *models.Invitation) error {
	if invitation.ID == "" {
		invitation.ID = uuid.New().String()
	}
	if invitation.CreatedAt.IsZero() {
		invitation.CreatedAt = time.Now()
	}
	if invitation.Status == "" {
		invitation.Status = models.InvitationStatusPending
	}

	query := `
		INSERT INTO invitations (id, email, token, invited_by, role, status, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := s.conn().ExecContext(ctx, query,
		invitation.ID,
		invitation.Email,
		invitation.Token,
		invitation.InvitedBy,
		string(invitation.Role),
		string(invitation.Status),
		invitation.ExpiresAt,
		invitation.CreatedAt,
	)
	return err
}

// Get retrieves an invitation by ID.
func (s *InvitationStore) Get(ctx context.Context, id string) (*models.Invitation, error) {
	query := `
		SELECT id, email, token, invited_by, role, status, expires_at, accepted_at, created_at
		FROM invitations WHERE id = $1
	`

	var inv models.Invitation
	var role, status string
	var acceptedAt sql.NullTime

	err := s.conn().QueryRowContext(ctx, query, id).Scan(
		&inv.ID, &inv.Email, &inv.Token, &inv.InvitedBy,
		&role, &status, &inv.ExpiresAt, &acceptedAt, &inv.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	inv.Role = models.Role(role)
	inv.Status = models.InvitationStatus(status)
	if acceptedAt.Valid {
		inv.AcceptedAt = &acceptedAt.Time
	}

	return &inv, nil
}


// GetByToken retrieves an invitation by its token.
func (s *InvitationStore) GetByToken(ctx context.Context, token string) (*models.Invitation, error) {
	query := `
		SELECT id, email, token, invited_by, role, status, expires_at, accepted_at, created_at
		FROM invitations WHERE token = $1
	`

	var inv models.Invitation
	var role, status string
	var acceptedAt sql.NullTime

	err := s.conn().QueryRowContext(ctx, query, token).Scan(
		&inv.ID, &inv.Email, &inv.Token, &inv.InvitedBy,
		&role, &status, &inv.ExpiresAt, &acceptedAt, &inv.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	inv.Role = models.Role(role)
	inv.Status = models.InvitationStatus(status)
	if acceptedAt.Valid {
		inv.AcceptedAt = &acceptedAt.Time
	}

	return &inv, nil
}

// GetByEmail retrieves a pending invitation by email.
func (s *InvitationStore) GetByEmail(ctx context.Context, email string) (*models.Invitation, error) {
	query := `
		SELECT id, email, token, invited_by, role, status, expires_at, accepted_at, created_at
		FROM invitations WHERE email = $1 AND status = 'pending'
		ORDER BY created_at DESC LIMIT 1
	`

	var inv models.Invitation
	var role, status string
	var acceptedAt sql.NullTime

	err := s.conn().QueryRowContext(ctx, query, email).Scan(
		&inv.ID, &inv.Email, &inv.Token, &inv.InvitedBy,
		&role, &status, &inv.ExpiresAt, &acceptedAt, &inv.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	inv.Role = models.Role(role)
	inv.Status = models.InvitationStatus(status)
	if acceptedAt.Valid {
		inv.AcceptedAt = &acceptedAt.Time
	}

	return &inv, nil
}

// List retrieves all invitations.
func (s *InvitationStore) List(ctx context.Context) ([]*models.Invitation, error) {
	query := `
		SELECT id, email, token, invited_by, role, status, expires_at, accepted_at, created_at
		FROM invitations ORDER BY created_at DESC
	`

	rows, err := s.conn().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invitations []*models.Invitation
	for rows.Next() {
		var inv models.Invitation
		var role, status string
		var acceptedAt sql.NullTime

		if err := rows.Scan(
			&inv.ID, &inv.Email, &inv.Token, &inv.InvitedBy,
			&role, &status, &inv.ExpiresAt, &acceptedAt, &inv.CreatedAt,
		); err != nil {
			return nil, err
		}

		inv.Role = models.Role(role)
		inv.Status = models.InvitationStatus(status)
		if acceptedAt.Valid {
			inv.AcceptedAt = &acceptedAt.Time
		}

		invitations = append(invitations, &inv)
	}

	return invitations, rows.Err()
}

// Update updates an invitation.
func (s *InvitationStore) Update(ctx context.Context, invitation *models.Invitation) error {
	query := `
		UPDATE invitations
		SET status = $1, accepted_at = $2
		WHERE id = $3
	`

	_, err := s.conn().ExecContext(ctx, query,
		string(invitation.Status),
		invitation.AcceptedAt,
		invitation.ID,
	)
	return err
}

// Delete removes an invitation.
func (s *InvitationStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM invitations WHERE id = $1`
	_, err := s.conn().ExecContext(ctx, query, id)
	return err
}
