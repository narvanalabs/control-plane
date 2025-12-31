package postgres

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// setupOrgTestDB creates a test database connection and runs migrations for orgs.
func setupOrgTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := getTestDSN()
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping database tests")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("failed to open database: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("failed to ping database: %v", err)
	}

	// Run migrations
	if err := runOrgMigrations(db); err != nil {
		db.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

// cleanupOrgTestDB cleans up test data and closes the connection.
func cleanupOrgTestDB(t *testing.T, db *sql.DB) {
	t.Helper()
	// Clean up test data
	db.Exec("DELETE FROM org_memberships")
	db.Exec("DELETE FROM organizations")
	db.Close()
}

// runOrgMigrations applies the database schema for organization testing.
func runOrgMigrations(db *sql.DB) error {
	// Drop existing tables to ensure clean state
	_, _ = db.Exec("DROP TABLE IF EXISTS org_memberships CASCADE")
	_, _ = db.Exec("DROP TABLE IF EXISTS organizations CASCADE")

	schema := `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE organizations (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			name VARCHAR(63) NOT NULL,
			slug VARCHAR(63) NOT NULL UNIQUE,
			description TEXT,
			icon_url TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX idx_organizations_slug ON organizations(slug);

		CREATE TABLE org_memberships (
			org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			user_id VARCHAR(255) NOT NULL,
			role VARCHAR(20) NOT NULL CHECK (role IN ('owner', 'member')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (org_id, user_id)
		);

		CREATE INDEX idx_org_memberships_user_id ON org_memberships(user_id);
	`
	_, err := db.Exec(schema)
	return err
}

// genNonEmptySlug generates a valid slug (lowercase alphanumeric with hyphens).
func genNonEmptySlug() gopter.Gen {
	return gen.IntRange(1, 20).FlatMap(func(v interface{}) gopter.Gen {
		length := v.(int)
		return gen.SliceOfN(length, gen.OneConstOf(
			'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
			'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
			'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		)).Map(func(chars []rune) string {
			return string(chars)
		})
	}, reflect.TypeOf(""))
}

// genOrgInput generates a random Organization for creation.
func genOrgInput() gopter.Gen {
	return gopter.CombineGens(
		genNonEmptyAlphaString(), // Name
		genNonEmptySlug(),        // Slug
		gen.AlphaString(),        // Description
	).Map(func(vals []interface{}) models.Organization {
		return models.Organization{
			ID:          uuid.New().String(),
			Name:        vals[0].(string),
			Slug:        vals[1].(string),
			Description: vals[2].(string),
		}
	})
}

// **Feature: platform-enhancements, Property 15: Organization Minimum Constraint**
// For any system state, there SHALL always be at least one organization.
// Attempting to delete the last organization SHALL be rejected.
// **Validates: Requirements 3.4**
func TestOrganizationMinimumConstraint(t *testing.T) {
	db := setupOrgTestDB(t)
	defer cleanupOrgTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	store := &OrgStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Cannot delete the last organization", prop.ForAll(
		func(input models.Organization) bool {
			ctx := context.Background()

			// Clean up any existing organizations first
			db.Exec("DELETE FROM org_memberships")
			db.Exec("DELETE FROM organizations")

			// Create a single organization
			err := store.Create(ctx, &input)
			if err != nil {
				t.Logf("Create error: %v", err)
				return false
			}

			// Verify count is 1
			count, err := store.Count(ctx)
			if err != nil {
				t.Logf("Count error: %v", err)
				return false
			}
			if count != 1 {
				t.Logf("Expected count 1, got %d", count)
				return false
			}

			// Attempt to delete the last organization - should fail
			err = store.Delete(ctx, input.ID)
			if err != models.ErrLastOrgDelete {
				t.Logf("Expected ErrLastOrgDelete, got: %v", err)
				return false
			}

			// Verify the organization still exists
			_, err = store.Get(ctx, input.ID)
			if err != nil {
				t.Logf("Organization was deleted when it shouldn't have been: %v", err)
				return false
			}

			// Verify count is still 1
			count, err = store.Count(ctx)
			if err != nil {
				t.Logf("Count error after delete attempt: %v", err)
				return false
			}
			if count != 1 {
				t.Logf("Expected count 1 after failed delete, got %d", count)
				return false
			}

			return true
		},
		genOrgInput(),
	))

	properties.TestingRun(t)
}

// **Feature: platform-enhancements, Property: Organization CRUD Round-Trip**
// For any valid organization data, creating an organization and then retrieving
// it by ID should produce an organization with equivalent data.
func TestOrgCreationRoundTrip(t *testing.T) {
	db := setupOrgTestDB(t)
	defer cleanupOrgTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	store := &OrgStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Org creation round-trip preserves data", prop.ForAll(
		func(input models.Organization) bool {
			ctx := context.Background()

			// Clean up any existing organizations first
			db.Exec("DELETE FROM org_memberships")
			db.Exec("DELETE FROM organizations")

			// Create the org
			err := store.Create(ctx, &input)
			if err != nil {
				t.Logf("Create error: %v", err)
				return false
			}

			// Retrieve the org
			retrieved, err := store.Get(ctx, input.ID)
			if err != nil {
				t.Logf("Get error: %v", err)
				return false
			}

			// Verify core fields match
			if retrieved.ID != input.ID {
				t.Logf("ID mismatch: got %s, want %s", retrieved.ID, input.ID)
				return false
			}
			if retrieved.Name != input.Name {
				t.Logf("Name mismatch: got %s, want %s", retrieved.Name, input.Name)
				return false
			}
			if retrieved.Slug != input.Slug {
				t.Logf("Slug mismatch: got %s, want %s", retrieved.Slug, input.Slug)
				return false
			}
			if retrieved.Description != input.Description {
				t.Logf("Description mismatch: got %s, want %s", retrieved.Description, input.Description)
				return false
			}

			return true
		},
		genOrgInput(),
	))

	properties.TestingRun(t)
}

// TestOrgDeleteWithMultiple verifies that deletion works when multiple orgs exist.
func TestOrgDeleteWithMultiple(t *testing.T) {
	db := setupOrgTestDB(t)
	defer cleanupOrgTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	store := &OrgStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50
	properties := gopter.NewProperties(parameters)

	properties.Property("Can delete org when multiple exist", prop.ForAll(
		func(input1, input2 models.Organization) bool {
			ctx := context.Background()

			// Clean up any existing organizations first
			db.Exec("DELETE FROM org_memberships")
			db.Exec("DELETE FROM organizations")

			// Ensure unique slugs
			input2.Slug = input2.Slug + "2"

			// Create two organizations
			err := store.Create(ctx, &input1)
			if err != nil {
				t.Logf("Create org1 error: %v", err)
				return false
			}

			err = store.Create(ctx, &input2)
			if err != nil {
				t.Logf("Create org2 error: %v", err)
				return false
			}

			// Verify count is 2
			count, err := store.Count(ctx)
			if err != nil {
				t.Logf("Count error: %v", err)
				return false
			}
			if count != 2 {
				t.Logf("Expected count 2, got %d", count)
				return false
			}

			// Delete the first organization - should succeed
			err = store.Delete(ctx, input1.ID)
			if err != nil {
				t.Logf("Delete error: %v", err)
				return false
			}

			// Verify count is now 1
			count, err = store.Count(ctx)
			if err != nil {
				t.Logf("Count error after delete: %v", err)
				return false
			}
			if count != 1 {
				t.Logf("Expected count 1 after delete, got %d", count)
				return false
			}

			// Verify the deleted org no longer exists
			_, err = store.Get(ctx, input1.ID)
			if err != ErrNotFound {
				t.Logf("Expected ErrNotFound for deleted org, got: %v", err)
				return false
			}

			// Verify the second org still exists
			_, err = store.Get(ctx, input2.ID)
			if err != nil {
				t.Logf("Second org should still exist: %v", err)
				return false
			}

			return true
		},
		genOrgInput(),
		genOrgInput(),
	))

	properties.TestingRun(t)
}
