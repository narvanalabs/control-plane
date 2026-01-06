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

// OrgStore implements store.OrgStore using PostgreSQL.
type OrgStore struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *slog.Logger
}

// conn returns the queryable connection (transaction or database).
func (s *OrgStore) conn() queryable {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Create creates a new organization.
func (s *OrgStore) Create(ctx context.Context, org *models.Organization) error {
	if err := org.Validate(); err != nil {
		return fmt.Errorf("validating organization: %w", err)
	}

	query := `
		INSERT INTO organizations (id, name, slug, description, icon_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	now := time.Now().UTC()
	if org.CreatedAt.IsZero() {
		org.CreatedAt = now
	}
	if org.UpdatedAt.IsZero() {
		org.UpdatedAt = now
	}

	err := s.conn().QueryRowContext(ctx, query,
		org.ID,
		org.Name,
		org.Slug,
		org.Description,
		org.IconURL,
		org.CreatedAt,
		org.UpdatedAt,
	).Scan(&org.ID, &org.CreatedAt, &org.UpdatedAt)

	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateName
		}
		return fmt.Errorf("inserting organization: %w", err)
	}

	return nil
}

// Get retrieves an organization by ID.
func (s *OrgStore) Get(ctx context.Context, id string) (*models.Organization, error) {
	query := `
		SELECT id, name, slug, COALESCE(description, ''), COALESCE(icon_url, ''),
		       created_at, updated_at
		FROM organizations
		WHERE id = $1`

	org := &models.Organization{}
	err := s.conn().QueryRowContext(ctx, query, id).Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.Description,
		&org.IconURL,
		&org.CreatedAt,
		&org.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying organization: %w", err)
	}

	return org, nil
}

// GetBySlug retrieves an organization by slug.
func (s *OrgStore) GetBySlug(ctx context.Context, slug string) (*models.Organization, error) {
	query := `
		SELECT id, name, slug, COALESCE(description, ''), COALESCE(icon_url, ''),
		       created_at, updated_at
		FROM organizations
		WHERE slug = $1`

	org := &models.Organization{}
	err := s.conn().QueryRowContext(ctx, query, slug).Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.Description,
		&org.IconURL,
		&org.CreatedAt,
		&org.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying organization by slug: %w", err)
	}

	return org, nil
}

// List retrieves all organizations for a user.
func (s *OrgStore) List(ctx context.Context, userID string) ([]*models.Organization, error) {
	query := `
		SELECT o.id, o.name, o.slug, COALESCE(o.description, ''), COALESCE(o.icon_url, ''),
		       o.created_at, o.updated_at
		FROM organizations o
		INNER JOIN org_memberships m ON o.id = m.org_id
		WHERE m.user_id = $1
		ORDER BY o.created_at ASC`

	rows, err := s.conn().QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("querying organizations: %w", err)
	}
	defer rows.Close()

	var orgs []*models.Organization
	for rows.Next() {
		org := &models.Organization{}
		err := rows.Scan(
			&org.ID,
			&org.Name,
			&org.Slug,
			&org.Description,
			&org.IconURL,
			&org.CreatedAt,
			&org.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning organization row: %w", err)
		}
		orgs = append(orgs, org)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating organization rows: %w", err)
	}

	return orgs, nil
}

// Update updates an organization.
func (s *OrgStore) Update(ctx context.Context, org *models.Organization) error {
	if err := org.Validate(); err != nil {
		return fmt.Errorf("validating organization: %w", err)
	}

	query := `
		UPDATE organizations
		SET name = $2, slug = $3, description = $4, icon_url = $5, updated_at = $6
		WHERE id = $1`

	org.UpdatedAt = time.Now().UTC()

	result, err := s.conn().ExecContext(ctx, query,
		org.ID,
		org.Name,
		org.Slug,
		org.Description,
		org.IconURL,
		org.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateName
		}
		return fmt.Errorf("updating organization: %w", err)
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

// Delete deletes an organization (only if not the last one).
func (s *OrgStore) Delete(ctx context.Context, id string) error {
	// First check if this is the last organization
	count, err := s.Count(ctx)
	if err != nil {
		return fmt.Errorf("counting organizations: %w", err)
	}

	if count <= 1 {
		return models.ErrLastOrgDelete
	}

	query := `DELETE FROM organizations WHERE id = $1`

	result, err := s.conn().ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting organization: %w", err)
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

// AddMember adds a user to an organization with a role.
func (s *OrgStore) AddMember(ctx context.Context, orgID, userID string, role models.Role) error {
	query := `
		INSERT INTO org_memberships (org_id, user_id, role, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (org_id, user_id) DO UPDATE SET role = $3`

	_, err := s.conn().ExecContext(ctx, query, orgID, userID, string(role), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("adding member to organization: %w", err)
	}

	return nil
}

// RemoveMember removes a user from an organization.
func (s *OrgStore) RemoveMember(ctx context.Context, orgID, userID string) error {
	query := `DELETE FROM org_memberships WHERE org_id = $1 AND user_id = $2`

	result, err := s.conn().ExecContext(ctx, query, orgID, userID)
	if err != nil {
		return fmt.Errorf("removing member from organization: %w", err)
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

// IsMember checks if a user is a member of an organization.
func (s *OrgStore) IsMember(ctx context.Context, orgID, userID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM org_memberships WHERE org_id = $1 AND user_id = $2)`

	var exists bool
	err := s.conn().QueryRowContext(ctx, query, orgID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking organization membership: %w", err)
	}

	return exists, nil
}

// GetDefault returns the default organization (first created).
func (s *OrgStore) GetDefault(ctx context.Context) (*models.Organization, error) {
	query := `
		SELECT id, name, slug, COALESCE(description, ''), COALESCE(icon_url, ''),
		       created_at, updated_at
		FROM organizations
		ORDER BY created_at ASC
		LIMIT 1`

	org := &models.Organization{}
	err := s.conn().QueryRowContext(ctx, query).Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.Description,
		&org.IconURL,
		&org.CreatedAt,
		&org.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying default organization: %w", err)
	}

	return org, nil
}

// GetDefaultForUser returns the user's default organization.
// Returns the first organization the user is a member of (ordered by creation date).
func (s *OrgStore) GetDefaultForUser(ctx context.Context, userID string) (*models.Organization, error) {
	query := `
		SELECT o.id, o.name, o.slug, COALESCE(o.description, ''), COALESCE(o.icon_url, ''),
		       o.created_at, o.updated_at
		FROM organizations o
		INNER JOIN org_memberships m ON o.id = m.org_id
		WHERE m.user_id = $1
		ORDER BY o.created_at ASC
		LIMIT 1`

	org := &models.Organization{}
	err := s.conn().QueryRowContext(ctx, query, userID).Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.Description,
		&org.IconURL,
		&org.CreatedAt,
		&org.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying default organization for user: %w", err)
	}

	return org, nil
}

// Count returns the total number of organizations.
func (s *OrgStore) Count(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM organizations`

	var count int
	err := s.conn().QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting organizations: %w", err)
	}

	return count, nil
}

// ListMembers retrieves all members of an organization.
func (s *OrgStore) ListMembers(ctx context.Context, orgID string) ([]*models.OrgMembership, error) {
	query := `
		SELECT org_id, user_id, role, created_at
		FROM org_memberships
		WHERE org_id = $1
		ORDER BY created_at ASC`

	rows, err := s.conn().QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("querying organization members: %w", err)
	}
	defer rows.Close()

	var members []*models.OrgMembership
	for rows.Next() {
		m := &models.OrgMembership{}
		var role string
		err := rows.Scan(
			&m.OrgID,
			&m.UserID,
			&role,
			&m.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning membership row: %w", err)
		}
		m.Role = models.Role(role)
		members = append(members, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating membership rows: %w", err)
	}

	return members, nil
}
