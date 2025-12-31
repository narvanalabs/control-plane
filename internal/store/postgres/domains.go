package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

type domainStore struct {
	db queryable
}

func NewDomainStore(db queryable) *domainStore {
	return &domainStore{db: db}
}

func (s *domainStore) Create(ctx context.Context, domain *models.Domain) error {
	query := `
		INSERT INTO domains (app_id, service, domain, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`
	now := time.Now().UTC()
	err := s.db.QueryRowContext(ctx, query,
		domain.AppID,
		domain.Service,
		domain.Domain,
		now,
		now,
	).Scan(&domain.ID, &domain.CreatedAt)

	if err != nil {
		return fmt.Errorf("creating domain: %w", err)
	}

	domain.UpdatedAt = now
	return nil
}

func (s *domainStore) Get(ctx context.Context, id string) (*models.Domain, error) {
	query := `
		SELECT id, app_id, service, domain, created_at, updated_at
		FROM domains
		WHERE id = $1
	`
	domain := &models.Domain{}
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&domain.ID,
		&domain.AppID,
		&domain.Service,
		&domain.Domain,
		&domain.CreatedAt,
		&domain.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("domain not found: %w", err)
		}
		return nil, fmt.Errorf("getting domain: %w", err)
	}

	return domain, nil
}

func (s *domainStore) List(ctx context.Context, appID string) ([]*models.Domain, error) {
	query := `
		SELECT id, app_id, service, domain, created_at, updated_at
		FROM domains
		WHERE app_id = $1
		ORDER BY created_at DESC
	`
	rows, err := s.db.QueryContext(ctx, query, appID)
	if err != nil {
		return nil, fmt.Errorf("querying domains: %w", err)
	}
	defer rows.Close()

	var domains []*models.Domain
	for rows.Next() {
		domain := &models.Domain{}
		if err := rows.Scan(
			&domain.ID,
			&domain.AppID,
			&domain.Service,
			&domain.Domain,
			&domain.CreatedAt,
			&domain.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning domain: %w", err)
		}
		domains = append(domains, domain)
	}

	return domains, nil
}

func (s *domainStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM domains WHERE id = $1`
	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting domain: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking affected rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("domain not found")
	}

	return nil
}

func (s *domainStore) GetByDomain(ctx context.Context, domainName string) (*models.Domain, error) {
	query := `
		SELECT id, app_id, service, domain, created_at, updated_at
		FROM domains
		WHERE domain = $1
	`
	domain := &models.Domain{}
	err := s.db.QueryRowContext(ctx, query, domainName).Scan(
		&domain.ID,
		&domain.AppID,
		&domain.Service,
		&domain.Domain,
		&domain.CreatedAt,
		&domain.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Return nil if not found, not an error
		}
		return nil, fmt.Errorf("getting domain by name: %w", err)
	}

	return domain, nil
}
