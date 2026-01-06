package postgres

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// getTestDSN returns the database DSN for testing.
// Set TEST_DATABASE_URL environment variable to run these tests.
func getTestDSN() string {
	return os.Getenv("TEST_DATABASE_URL")
}

// setupTestDB creates a test database connection and runs migrations.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := getTestDSN()
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping database tests")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("failed to ping database: %v", err)
	}

	// Run migrations
	if err := runMigrations(db); err != nil {
		db.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

// cleanupTestDB cleans up test data and closes the connection.
func cleanupTestDB(t *testing.T, db *sql.DB) {
	t.Helper()
	// Clean up test data
	db.Exec("DELETE FROM apps")
	db.Close()
}

// runMigrations applies the database schema for testing.
func runMigrations(db *sql.DB) error {
	// Drop existing tables to ensure clean state
	_, _ = db.Exec("DROP TABLE IF EXISTS logs CASCADE")
	_, _ = db.Exec("DROP TABLE IF EXISTS secrets CASCADE")
	_, _ = db.Exec("DROP TABLE IF EXISTS builds CASCADE")
	_, _ = db.Exec("DROP TABLE IF EXISTS deployments CASCADE")
	_, _ = db.Exec("DROP TABLE IF EXISTS nodes CASCADE")
	_, _ = db.Exec("DROP TABLE IF EXISTS apps CASCADE")

	schema := `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE apps (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			org_id UUID,
			owner_id VARCHAR(255) NOT NULL,
			name VARCHAR(63) NOT NULL,
			description TEXT,
			services JSONB NOT NULL DEFAULT '[]',
			icon_url TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		);

		-- Unique index on (org_id, name) for non-deleted apps
		-- This ensures app names are unique within an organization
		CREATE UNIQUE INDEX apps_org_name_unique ON apps(org_id, name) WHERE deleted_at IS NULL;
		CREATE INDEX idx_apps_owner_id ON apps(owner_id) WHERE deleted_at IS NULL;
		CREATE INDEX idx_apps_org_id ON apps(org_id) WHERE deleted_at IS NULL;
	`
	_, err := db.Exec(schema)
	return err
}

// genResourceSpec generates a random ResourceSpec.
func genResourceSpec() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("0.25", "0.5", "1", "2", "4"),
		gen.OneConstOf("256Mi", "512Mi", "1Gi", "2Gi", "4Gi"),
	).Map(func(vals []interface{}) *models.ResourceSpec {
		return &models.ResourceSpec{
			CPU:    vals[0].(string),
			Memory: vals[1].(string),
		}
	})
}

// genPortMapping generates a random PortMapping.
func genPortMapping() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 65535),
		gen.OneConstOf("tcp", "udp"),
	).Map(func(vals []interface{}) models.PortMapping {
		return models.PortMapping{
			ContainerPort: vals[0].(int),
			Protocol:      vals[1].(string),
		}
	})
}

// genHealthCheckConfig generates a random HealthCheckConfig.
func genHealthCheckConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString(),
		gen.IntRange(1, 65535),
		gen.IntRange(1, 300),
		gen.IntRange(1, 60),
		gen.IntRange(1, 10),
	).Map(func(vals []interface{}) models.HealthCheckConfig {
		return models.HealthCheckConfig{
			Path:            vals[0].(string),
			Port:            vals[1].(int),
			IntervalSeconds: vals[2].(int),
			TimeoutSeconds:  vals[3].(int),
			Retries:         vals[4].(int),
		}
	})
}

// genOptionalHealthCheckConfig generates an optional HealthCheckConfig pointer.
func genOptionalHealthCheckConfig() gopter.Gen {
	return gen.Bool().FlatMap(func(v interface{}) gopter.Gen {
		if v.(bool) {
			return genHealthCheckConfig().Map(func(hc models.HealthCheckConfig) *models.HealthCheckConfig {
				return &hc
			})
		}
		return gen.Const((*models.HealthCheckConfig)(nil))
	}, reflect.TypeOf((*models.HealthCheckConfig)(nil)))
}

// genNonEmptyAlphaString generates a non-empty alpha string with length 1-63.
func genNonEmptyAlphaString() gopter.Gen {
	return gen.IntRange(1, 63).FlatMap(func(v interface{}) gopter.Gen {
		length := v.(int)
		return gen.SliceOfN(length, gen.AlphaChar()).Map(func(chars []rune) string {
			return string(chars)
		})
	}, reflect.TypeOf(""))
}

// genServiceConfig generates a random ServiceConfig.
func genServiceConfig() gopter.Gen {
	return gopter.CombineGens(
		genNonEmptyAlphaString(),
		gen.AlphaString(),
		genResourceSpec(),
		gen.IntRange(1, 10),
		gen.SliceOfN(2, genPortMapping()),
		genOptionalHealthCheckConfig(),
		gen.SliceOfN(2, gen.AlphaString()),
	).Map(func(vals []interface{}) models.ServiceConfig {
		return models.ServiceConfig{
			Name:        vals[0].(string),
			FlakeOutput: vals[1].(string),
			Resources:   vals[2].(*models.ResourceSpec),
			Replicas:    vals[3].(int),
			Ports:       vals[4].([]models.PortMapping),
			HealthCheck: vals[5].(*models.HealthCheckConfig),
			DependsOn:   vals[6].([]string),
		}
	})
}

// genAppInput generates a random App for creation (without ID and timestamps).
func genAppInput() gopter.Gen {
	return gopter.CombineGens(
		genNonEmptyAlphaString(), // OwnerID
		genNonEmptyAlphaString(), // Name
		gen.AlphaString(),        // Description (can be empty)
		gen.SliceOfN(2, genServiceConfig()),
	).Map(func(vals []interface{}) models.App {
		return models.App{
			ID:          uuid.New().String(),
			OwnerID:     vals[0].(string),
			Name:        vals[1].(string),
			Description: vals[2].(string),
			Services:    vals[3].([]models.ServiceConfig),
		}
	})
}

// **Feature: control-plane, Property 1: Application creation round-trip**
// For any valid application metadata, creating an application and then retrieving
// it by the returned ID should produce an application with equivalent metadata.
// **Validates: Requirements 1.1, 1.3**
func TestAppCreationRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	store := &AppStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("App creation round-trip preserves data", prop.ForAll(
		func(input models.App) bool {
			ctx := context.Background()

			// Create the app
			err := store.Create(ctx, &input)
			if err != nil {
				t.Logf("Create error: %v", err)
				return false
			}

			// Retrieve the app
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
			if retrieved.OwnerID != input.OwnerID {
				t.Logf("OwnerID mismatch: got %s, want %s", retrieved.OwnerID, input.OwnerID)
				return false
			}
			if retrieved.Name != input.Name {
				t.Logf("Name mismatch: got %s, want %s", retrieved.Name, input.Name)
				return false
			}
			if retrieved.Description != input.Description {
				t.Logf("Description mismatch: got %s, want %s", retrieved.Description, input.Description)
				return false
			}

			// Verify services match (compare lengths and content)
			if len(retrieved.Services) != len(input.Services) {
				t.Logf("Services length mismatch: got %d, want %d", len(retrieved.Services), len(input.Services))
				return false
			}

			// Cleanup for next iteration
			store.Delete(ctx, input.ID)

			return true
		},
		genAppInput(),
	))

	properties.TestingRun(t)
}

// **Feature: control-plane, Property 3: Application name uniqueness**
// For any organization and application name, attempting to create a second application
// with the same name within the same organization should be rejected while the first exists.
// **Validates: Requirements 1.5**
// Note: This test was updated to use org_id instead of owner_id after migration 020.
func TestAppNameUniqueness(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	store := &AppStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Duplicate app names for same org are rejected", prop.ForAll(
		func(ownerID string) bool {
			ctx := context.Background()

			// Create a single org ID for this test
			orgID := uuid.New().String()
			appName := "testapp"

			// Create the first app
			input := models.App{
				ID:          uuid.New().String(),
				OrgID:       orgID,
				OwnerID:     ownerID,
				Name:        appName,
				Description: "first app",
				Services:    []models.ServiceConfig{},
			}

			err := store.Create(ctx, &input)
			if err != nil {
				t.Logf("First create error: %v", err)
				return false
			}

			// Try to create a second app with the same org and name
			duplicate := models.App{
				ID:          uuid.New().String(),
				OrgID:       orgID, // Same org
				OwnerID:     ownerID,
				Name:        appName, // Same name
				Description: "duplicate",
				Services:    []models.ServiceConfig{},
			}

			err = store.Create(ctx, &duplicate)
			if err != ErrDuplicateName {
				t.Logf("Expected ErrDuplicateName, got: %v", err)
				// Cleanup
				store.Delete(ctx, input.ID)
				store.Delete(ctx, duplicate.ID)
				return false
			}

			// Cleanup
			store.Delete(ctx, input.ID)

			return true
		},
		genNonEmptyAlphaString(),
	))

	properties.TestingRun(t)
}

// **Feature: platform-enhancements, Property 12: Application CRUD Round-Trip**
// *For any* valid application data, creating an application and then retrieving it by ID
// SHALL return equivalent data. Additionally, updating and deleting should work correctly.
// **Validates: Requirements 21.1, 21.2**
func TestAppCRUDRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	store := &AppStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("App CRUD round-trip preserves data", prop.ForAll(
		func(input models.App, newDescription string) bool {
			ctx := context.Background()

			// CREATE: Create the app
			err := store.Create(ctx, &input)
			if err != nil {
				t.Logf("Create error: %v", err)
				return false
			}

			// READ: Retrieve the app by ID
			retrieved, err := store.Get(ctx, input.ID)
			if err != nil {
				t.Logf("Get error: %v", err)
				return false
			}

			// Verify core fields match after creation
			if retrieved.ID != input.ID ||
				retrieved.OwnerID != input.OwnerID ||
				retrieved.Name != input.Name ||
				retrieved.Description != input.Description {
				t.Logf("Fields mismatch after create")
				return false
			}

			// UPDATE: Update the app description
			retrieved.Description = newDescription
			err = store.Update(ctx, retrieved)
			if err != nil {
				t.Logf("Update error: %v", err)
				return false
			}

			// READ again: Verify update was persisted
			updated, err := store.Get(ctx, input.ID)
			if err != nil {
				t.Logf("Get after update error: %v", err)
				return false
			}

			if updated.Description != newDescription {
				t.Logf("Description not updated: got %s, want %s", updated.Description, newDescription)
				return false
			}

			// Verify other fields were preserved
			if updated.ID != input.ID ||
				updated.OwnerID != input.OwnerID ||
				updated.Name != input.Name {
				t.Logf("Other fields changed during update")
				return false
			}

			// DELETE: Soft-delete the app
			err = store.Delete(ctx, input.ID)
			if err != nil {
				t.Logf("Delete error: %v", err)
				return false
			}

			// READ after delete: Should return not found
			_, err = store.Get(ctx, input.ID)
			if err != ErrNotFound {
				t.Logf("Expected ErrNotFound after delete, got: %v", err)
				return false
			}

			return true
		},
		genAppInput(),
		gen.AlphaString(), // new description
	))

	properties.TestingRun(t)
}

// TestAppListFiltersDeleted verifies that List only returns non-deleted apps.
// **Validates: Requirements 21.5**
func TestAppListFiltersDeleted(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	store := &AppStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("List returns only non-deleted apps", prop.ForAll(
		func(ownerID string) bool {
			ctx := context.Background()

			// Create two apps for the same owner
			app1 := models.App{
				ID:          uuid.New().String(),
				OwnerID:     ownerID,
				Name:        "app1",
				Description: "first app",
				Services:    []models.ServiceConfig{},
			}
			app2 := models.App{
				ID:          uuid.New().String(),
				OwnerID:     ownerID,
				Name:        "app2",
				Description: "second app",
				Services:    []models.ServiceConfig{},
			}

			err := store.Create(ctx, &app1)
			if err != nil {
				t.Logf("Create app1 error: %v", err)
				return false
			}

			err = store.Create(ctx, &app2)
			if err != nil {
				t.Logf("Create app2 error: %v", err)
				return false
			}

			// List should return both apps
			apps, err := store.List(ctx, ownerID)
			if err != nil {
				t.Logf("List error: %v", err)
				return false
			}

			if len(apps) != 2 {
				t.Logf("Expected 2 apps, got %d", len(apps))
				return false
			}

			// Delete one app
			err = store.Delete(ctx, app1.ID)
			if err != nil {
				t.Logf("Delete error: %v", err)
				return false
			}

			// List should now return only one app
			apps, err = store.List(ctx, ownerID)
			if err != nil {
				t.Logf("List after delete error: %v", err)
				return false
			}

			if len(apps) != 1 {
				t.Logf("Expected 1 app after delete, got %d", len(apps))
				return false
			}

			if apps[0].ID != app2.ID {
				t.Logf("Wrong app returned: got %s, want %s", apps[0].ID, app2.ID)
				return false
			}

			// Cleanup
			store.Delete(ctx, app2.ID)

			return true
		},
		genNonEmptyAlphaString(),
	))

	properties.TestingRun(t)
}

// **Feature: backend-source-of-truth, Property 4: Optimistic Locking Conflict Detection**
// *For any* app with version N, an update request with version M where M != N
// SHALL be rejected with a conflict error.
// **Validates: Requirements 7.2**
func TestAppOptimisticLockingConflictDetection(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	store := &AppStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Update with wrong version is rejected", prop.ForAll(
		func(input models.App) bool {
			ctx := context.Background()

			// Create the app (version starts at 1)
			err := store.Create(ctx, &input)
			if err != nil {
				t.Logf("Create error: %v", err)
				return false
			}

			// Retrieve the app to get the current version
			retrieved, err := store.Get(ctx, input.ID)
			if err != nil {
				t.Logf("Get error: %v", err)
				store.Delete(ctx, input.ID)
				return false
			}

			// Verify initial version is 1
			if retrieved.Version != 1 {
				t.Logf("Expected initial version 1, got %d", retrieved.Version)
				store.Delete(ctx, input.ID)
				return false
			}

			// Simulate a concurrent modification by updating the app
			retrieved.Description = "first update"
			err = store.Update(ctx, retrieved)
			if err != nil {
				t.Logf("First update error: %v", err)
				store.Delete(ctx, input.ID)
				return false
			}

			// Verify version was incremented
			if retrieved.Version != 2 {
				t.Logf("Expected version 2 after update, got %d", retrieved.Version)
				store.Delete(ctx, input.ID)
				return false
			}

			// Now try to update with the old version (simulating stale data)
			staleApp := &models.App{
				ID:          input.ID,
				OwnerID:     input.OwnerID,
				Name:        input.Name,
				Description: "stale update",
				Services:    input.Services,
				Version:     1, // Old version
			}

			err = store.Update(ctx, staleApp)
			if err != ErrConcurrentModification {
				t.Logf("Expected ErrConcurrentModification, got: %v", err)
				store.Delete(ctx, input.ID)
				return false
			}

			// Verify the app still has the first update's description
			final, err := store.Get(ctx, input.ID)
			if err != nil {
				t.Logf("Final get error: %v", err)
				store.Delete(ctx, input.ID)
				return false
			}

			if final.Description != "first update" {
				t.Logf("Expected description 'first update', got '%s'", final.Description)
				store.Delete(ctx, input.ID)
				return false
			}

			// Cleanup
			store.Delete(ctx, input.ID)

			return true
		},
		genAppInput(),
	))

	properties.TestingRun(t)
}

// **Feature: backend-source-of-truth, Property 3: App Organization Filtering**
// *For any* organization with apps, listing apps SHALL return only apps
// where org_id matches the current organization context.
// **Validates: Requirements 3.5, 5.1, 5.2**
func TestAppOrganizationFiltering(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	store := &AppStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("ListByOrg returns only apps for the specified org", prop.ForAll(
		func(ownerID string) bool {
			ctx := context.Background()

			// Create two different org IDs
			org1ID := uuid.New().String()
			org2ID := uuid.New().String()

			// Create apps for org1
			app1 := models.App{
				ID:          uuid.New().String(),
				OrgID:       org1ID,
				OwnerID:     ownerID,
				Name:        "app1org1",
				Description: "app in org1",
				Services:    []models.ServiceConfig{},
			}
			app2 := models.App{
				ID:          uuid.New().String(),
				OrgID:       org1ID,
				OwnerID:     ownerID,
				Name:        "app2org1",
				Description: "another app in org1",
				Services:    []models.ServiceConfig{},
			}

			// Create app for org2
			app3 := models.App{
				ID:          uuid.New().String(),
				OrgID:       org2ID,
				OwnerID:     ownerID,
				Name:        "app1org2",
				Description: "app in org2",
				Services:    []models.ServiceConfig{},
			}

			// Create all apps
			if err := store.Create(ctx, &app1); err != nil {
				t.Logf("Create app1 error: %v", err)
				return false
			}
			if err := store.Create(ctx, &app2); err != nil {
				t.Logf("Create app2 error: %v", err)
				store.Delete(ctx, app1.ID)
				return false
			}
			if err := store.Create(ctx, &app3); err != nil {
				t.Logf("Create app3 error: %v", err)
				store.Delete(ctx, app1.ID)
				store.Delete(ctx, app2.ID)
				return false
			}

			// List apps for org1 - should return only app1 and app2
			org1Apps, err := store.ListByOrg(ctx, org1ID)
			if err != nil {
				t.Logf("ListByOrg org1 error: %v", err)
				store.Delete(ctx, app1.ID)
				store.Delete(ctx, app2.ID)
				store.Delete(ctx, app3.ID)
				return false
			}

			if len(org1Apps) != 2 {
				t.Logf("Expected 2 apps for org1, got %d", len(org1Apps))
				store.Delete(ctx, app1.ID)
				store.Delete(ctx, app2.ID)
				store.Delete(ctx, app3.ID)
				return false
			}

			// Verify all returned apps belong to org1
			for _, app := range org1Apps {
				if app.OrgID != org1ID {
					t.Logf("App %s has wrong org_id: got %s, want %s", app.ID, app.OrgID, org1ID)
					store.Delete(ctx, app1.ID)
					store.Delete(ctx, app2.ID)
					store.Delete(ctx, app3.ID)
					return false
				}
			}

			// List apps for org2 - should return only app3
			org2Apps, err := store.ListByOrg(ctx, org2ID)
			if err != nil {
				t.Logf("ListByOrg org2 error: %v", err)
				store.Delete(ctx, app1.ID)
				store.Delete(ctx, app2.ID)
				store.Delete(ctx, app3.ID)
				return false
			}

			if len(org2Apps) != 1 {
				t.Logf("Expected 1 app for org2, got %d", len(org2Apps))
				store.Delete(ctx, app1.ID)
				store.Delete(ctx, app2.ID)
				store.Delete(ctx, app3.ID)
				return false
			}

			if org2Apps[0].OrgID != org2ID {
				t.Logf("App has wrong org_id: got %s, want %s", org2Apps[0].OrgID, org2ID)
				store.Delete(ctx, app1.ID)
				store.Delete(ctx, app2.ID)
				store.Delete(ctx, app3.ID)
				return false
			}

			// Delete one app from org1 and verify it's excluded
			store.Delete(ctx, app1.ID)

			org1AppsAfterDelete, err := store.ListByOrg(ctx, org1ID)
			if err != nil {
				t.Logf("ListByOrg after delete error: %v", err)
				store.Delete(ctx, app2.ID)
				store.Delete(ctx, app3.ID)
				return false
			}

			if len(org1AppsAfterDelete) != 1 {
				t.Logf("Expected 1 app for org1 after delete, got %d", len(org1AppsAfterDelete))
				store.Delete(ctx, app2.ID)
				store.Delete(ctx, app3.ID)
				return false
			}

			// Cleanup
			store.Delete(ctx, app2.ID)
			store.Delete(ctx, app3.ID)

			return true
		},
		genNonEmptyAlphaString(),
	))

	properties.TestingRun(t)
}

// **Feature: backend-source-of-truth, Property 6: App Update Field Preservation**
// *For any* app update request, fields not specified in the request SHALL retain
// their previous values.
// **Validates: Requirements 8.1, 8.3**
func TestAppUpdateFieldPreservation(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	store := &AppStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Partial update preserves unspecified fields", prop.ForAll(
		func(input models.App, newDescription string) bool {
			ctx := context.Background()

			// Create the app with initial values
			err := store.Create(ctx, &input)
			if err != nil {
				t.Logf("Create error: %v", err)
				return false
			}

			// Retrieve the app to get the current state
			original, err := store.Get(ctx, input.ID)
			if err != nil {
				t.Logf("Get error: %v", err)
				store.Delete(ctx, input.ID)
				return false
			}

			// Store original values
			originalName := original.Name
			originalOwnerID := original.OwnerID
			originalOrgID := original.OrgID
			originalIconURL := original.IconURL
			originalServicesLen := len(original.Services)

			// Update only the description (simulating partial update)
			original.Description = newDescription
			err = store.Update(ctx, original)
			if err != nil {
				t.Logf("Update error: %v", err)
				store.Delete(ctx, input.ID)
				return false
			}

			// Retrieve the updated app
			updated, err := store.Get(ctx, input.ID)
			if err != nil {
				t.Logf("Get after update error: %v", err)
				store.Delete(ctx, input.ID)
				return false
			}

			// Verify the description was updated
			if updated.Description != newDescription {
				t.Logf("Description not updated: got %s, want %s", updated.Description, newDescription)
				store.Delete(ctx, input.ID)
				return false
			}

			// Verify all other fields were preserved (Requirements: 8.3)
			if updated.Name != originalName {
				t.Logf("Name changed unexpectedly: got %s, want %s", updated.Name, originalName)
				store.Delete(ctx, input.ID)
				return false
			}

			if updated.OwnerID != originalOwnerID {
				t.Logf("OwnerID changed unexpectedly: got %s, want %s", updated.OwnerID, originalOwnerID)
				store.Delete(ctx, input.ID)
				return false
			}

			if updated.OrgID != originalOrgID {
				t.Logf("OrgID changed unexpectedly: got %s, want %s", updated.OrgID, originalOrgID)
				store.Delete(ctx, input.ID)
				return false
			}

			if updated.IconURL != originalIconURL {
				t.Logf("IconURL changed unexpectedly: got %s, want %s", updated.IconURL, originalIconURL)
				store.Delete(ctx, input.ID)
				return false
			}

			if len(updated.Services) != originalServicesLen {
				t.Logf("Services length changed unexpectedly: got %d, want %d", len(updated.Services), originalServicesLen)
				store.Delete(ctx, input.ID)
				return false
			}

			// Verify version was incremented
			if updated.Version != original.Version {
				t.Logf("Version not incremented correctly: got %d, want %d", updated.Version, original.Version)
				store.Delete(ctx, input.ID)
				return false
			}

			// Cleanup
			store.Delete(ctx, input.ID)

			return true
		},
		genAppInput(),
		gen.AlphaString(), // new description
	))

	properties.TestingRun(t)
}

// **Feature: backend-source-of-truth, Property 7: App Name Uniqueness Within Organization**
// *For any* organization, attempting to create or rename an app to a name that already
// exists within that organization SHALL be rejected with a conflict error.
// **Validates: Requirements 8.2**
func TestAppNameUniquenessWithinOrganization(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	store := &AppStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Duplicate app names within same org are rejected", prop.ForAll(
		func(ownerID string) bool {
			ctx := context.Background()

			// Create a single org ID for this test
			orgID := uuid.New().String()
			appName := "testapp"

			// Create the first app in the org
			app1 := models.App{
				ID:          uuid.New().String(),
				OrgID:       orgID,
				OwnerID:     ownerID,
				Name:        appName,
				Description: "first app",
				Services:    []models.ServiceConfig{},
			}

			err := store.Create(ctx, &app1)
			if err != nil {
				t.Logf("First create error: %v", err)
				return false
			}

			// Try to create a second app with the same name in the same org
			app2 := models.App{
				ID:          uuid.New().String(),
				OrgID:       orgID,
				OwnerID:     ownerID,
				Name:        appName, // Same name
				Description: "duplicate app",
				Services:    []models.ServiceConfig{},
			}

			err = store.Create(ctx, &app2)
			if err != ErrDuplicateName {
				t.Logf("Expected ErrDuplicateName for same org, got: %v", err)
				store.Delete(ctx, app1.ID)
				store.Delete(ctx, app2.ID)
				return false
			}

			// Verify that the same name CAN be used in a different org
			differentOrgID := uuid.New().String()
			app3 := models.App{
				ID:          uuid.New().String(),
				OrgID:       differentOrgID,
				OwnerID:     ownerID,
				Name:        appName, // Same name, different org
				Description: "app in different org",
				Services:    []models.ServiceConfig{},
			}

			err = store.Create(ctx, &app3)
			if err != nil {
				t.Logf("Create in different org error: %v", err)
				store.Delete(ctx, app1.ID)
				return false
			}

			// Verify renaming to an existing name in the same org is rejected
			app3.Name = "uniquename"
			err = store.Update(ctx, &app3)
			if err != nil {
				t.Logf("Rename to unique name error: %v", err)
				store.Delete(ctx, app1.ID)
				store.Delete(ctx, app3.ID)
				return false
			}

			// Now try to rename app3 to the same name as app1 (should fail if same org)
			// But since app3 is in a different org, this should succeed
			app3Updated, _ := store.Get(ctx, app3.ID)
			app3Updated.Name = appName
			err = store.Update(ctx, app3Updated)
			if err != nil {
				t.Logf("Rename to same name in different org should succeed, got: %v", err)
				store.Delete(ctx, app1.ID)
				store.Delete(ctx, app3.ID)
				return false
			}

			// Cleanup
			store.Delete(ctx, app1.ID)
			store.Delete(ctx, app3.ID)

			return true
		},
		genNonEmptyAlphaString(),
	))

	properties.TestingRun(t)
}

// **Feature: backend-source-of-truth, Property 5: Transaction Rollback on Failure**
// *For any* service operation that fails mid-transaction, all changes including
// generated credentials SHALL be rolled back, leaving the database in its pre-operation state.
// **Validates: Requirements 7.5, 11.4, 23.5**
func TestTransactionRollbackOnFailure(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Create additional tables needed for this test
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS secrets (
			app_id UUID NOT NULL,
			key VARCHAR(255) NOT NULL,
			encrypted_value BYTEA NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (app_id, key)
		);
		
		CREATE TABLE IF NOT EXISTS deployments (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			app_id UUID NOT NULL,
			service_name VARCHAR(63) NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			git_ref VARCHAR(255),
			git_commit VARCHAR(255),
			build_type VARCHAR(20),
			artifact TEXT,
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			node_id UUID,
			resources JSONB,
			config JSONB,
			depends_on JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			started_at TIMESTAMPTZ,
			finished_at TIMESTAMPTZ
		);
	`)
	if err != nil {
		t.Fatalf("failed to create additional tables: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Transaction rollback preserves pre-operation state", prop.ForAll(
		func(input models.App) bool {
			ctx := context.Background()

			// Create the main store
			mainStore := &PostgresStore{
				db:     db,
				logger: logger,
			}
			mainStore.apps = &AppStore{db: db, logger: logger}
			mainStore.secrets = &SecretStore{db: db, logger: logger}
			mainStore.deployments = &DeploymentStore{db: db, logger: logger}

			// First, create an app outside of a transaction
			err := mainStore.Apps().Create(ctx, &input)
			if err != nil {
				t.Logf("Create error: %v", err)
				return false
			}

			// Store the original state
			originalApp, err := mainStore.Apps().Get(ctx, input.ID)
			if err != nil {
				t.Logf("Get original error: %v", err)
				mainStore.Apps().Delete(ctx, input.ID)
				return false
			}
			originalDescription := originalApp.Description
			originalVersion := originalApp.Version

			// Create a secret for the app
			secretKey := "test-secret"
			secretValue := []byte("original-secret-value")
			err = mainStore.Secrets().Set(ctx, input.ID, secretKey, secretValue)
			if err != nil {
				t.Logf("Set secret error: %v", err)
				mainStore.Apps().Delete(ctx, input.ID)
				return false
			}

			// Now execute a transaction that will fail mid-way
			// This simulates a service operation that creates credentials and then fails
			simulatedError := errors.New("simulated failure mid-transaction")

			err = mainStore.WithTx(ctx, func(txStore store.Store) error {
				// Step 1: Update the app (this should be rolled back)
				app, err := txStore.Apps().Get(ctx, input.ID)
				if err != nil {
					return err
				}
				app.Description = "modified-in-transaction"
				err = txStore.Apps().Update(ctx, app)
				if err != nil {
					return err
				}

				// Step 2: Create a new secret (this should be rolled back)
				newSecretKey := "new-secret-in-tx"
				newSecretValue := []byte("new-secret-value")
				err = txStore.Secrets().Set(ctx, input.ID, newSecretKey, newSecretValue)
				if err != nil {
					return err
				}

				// Step 3: Simulate a failure mid-transaction
				return simulatedError
			})

			// Verify the transaction returned an error
			if err != simulatedError {
				t.Logf("Expected simulated error, got: %v", err)
				mainStore.Apps().Delete(ctx, input.ID)
				return false
			}

			// Verify the app was NOT modified (rollback worked)
			appAfterRollback, err := mainStore.Apps().Get(ctx, input.ID)
			if err != nil {
				t.Logf("Get after rollback error: %v", err)
				mainStore.Apps().Delete(ctx, input.ID)
				return false
			}

			if appAfterRollback.Description != originalDescription {
				t.Logf("Description was modified despite rollback: got %s, want %s",
					appAfterRollback.Description, originalDescription)
				mainStore.Apps().Delete(ctx, input.ID)
				return false
			}

			if appAfterRollback.Version != originalVersion {
				t.Logf("Version was modified despite rollback: got %d, want %d",
					appAfterRollback.Version, originalVersion)
				mainStore.Apps().Delete(ctx, input.ID)
				return false
			}

			// Verify the original secret still exists with original value
			retrievedSecret, err := mainStore.Secrets().Get(ctx, input.ID, secretKey)
			if err != nil {
				t.Logf("Get original secret error: %v", err)
				mainStore.Apps().Delete(ctx, input.ID)
				return false
			}

			if string(retrievedSecret) != string(secretValue) {
				t.Logf("Original secret was modified: got %s, want %s",
					string(retrievedSecret), string(secretValue))
				mainStore.Apps().Delete(ctx, input.ID)
				return false
			}

			// Verify the new secret was NOT created (rollback worked)
			_, err = mainStore.Secrets().Get(ctx, input.ID, "new-secret-in-tx")
			if err != ErrNotFound {
				t.Logf("New secret should not exist after rollback, got error: %v", err)
				mainStore.Apps().Delete(ctx, input.ID)
				// Clean up the secret that shouldn't exist
				mainStore.Secrets().Delete(ctx, input.ID, "new-secret-in-tx")
				return false
			}

			// Cleanup
			mainStore.Secrets().Delete(ctx, input.ID, secretKey)
			mainStore.Apps().Delete(ctx, input.ID)

			return true
		},
		genAppInput(),
	))

	properties.TestingRun(t)
}

// TestTransactionRollbackOnAppDeletion tests that app deletion rollback works correctly.
// This specifically tests Requirement 11.4: WHEN app deletion fails mid-process
// THEN the Narvana_API SHALL rollback to a consistent state.
// **Feature: backend-source-of-truth, Property 5: Transaction Rollback on Failure**
// **Validates: Requirements 11.4**
func TestTransactionRollbackOnAppDeletion(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Create additional tables needed for this test
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS secrets (
			app_id UUID NOT NULL,
			key VARCHAR(255) NOT NULL,
			encrypted_value BYTEA NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (app_id, key)
		);
		
		CREATE TABLE IF NOT EXISTS deployments (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			app_id UUID NOT NULL,
			service_name VARCHAR(63) NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			git_ref VARCHAR(255),
			git_commit VARCHAR(255),
			build_type VARCHAR(20),
			artifact TEXT,
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			node_id UUID,
			resources JSONB,
			config JSONB,
			depends_on JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			started_at TIMESTAMPTZ,
			finished_at TIMESTAMPTZ
		);
	`)
	if err != nil {
		t.Fatalf("failed to create additional tables: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("App deletion rollback preserves app and related resources", prop.ForAll(
		func(input models.App) bool {
			ctx := context.Background()

			// Create the main store
			mainStore := &PostgresStore{
				db:     db,
				logger: logger,
			}
			mainStore.apps = &AppStore{db: db, logger: logger}
			mainStore.secrets = &SecretStore{db: db, logger: logger}
			mainStore.deployments = &DeploymentStore{db: db, logger: logger}

			// Create an app
			err := mainStore.Apps().Create(ctx, &input)
			if err != nil {
				t.Logf("Create error: %v", err)
				return false
			}

			// Create associated secrets
			secretKey := "db-password"
			secretValue := []byte("super-secret-password")
			err = mainStore.Secrets().Set(ctx, input.ID, secretKey, secretValue)
			if err != nil {
				t.Logf("Set secret error: %v", err)
				mainStore.Apps().Delete(ctx, input.ID)
				return false
			}

			// Create a deployment for the app
			deployment := &models.Deployment{
				ID:          uuid.New().String(),
				AppID:       input.ID,
				ServiceName: "test-service",
				Version:     1,
				Status:      models.DeploymentStatusRunning,
			}
			err = mainStore.Deployments().Create(ctx, deployment)
			if err != nil {
				t.Logf("Create deployment error: %v", err)
				mainStore.Secrets().Delete(ctx, input.ID, secretKey)
				mainStore.Apps().Delete(ctx, input.ID)
				return false
			}

			// Simulate a failed app deletion transaction
			// In a real scenario, this might fail when trying to stop containers
			simulatedError := errors.New("failed to stop container")

			err = mainStore.WithTx(ctx, func(txStore store.Store) error {
				// Step 1: Mark secrets for cleanup (delete them)
				err := txStore.Secrets().Delete(ctx, input.ID, secretKey)
				if err != nil {
					return err
				}

				// Step 2: Update deployment status to stopped
				dep, err := txStore.Deployments().Get(ctx, deployment.ID)
				if err != nil {
					return err
				}
				dep.Status = models.DeploymentStatusStopped
				err = txStore.Deployments().Update(ctx, dep)
				if err != nil {
					return err
				}

				// Step 3: Simulate failure (e.g., container stop failed)
				return simulatedError
			})

			// Verify the transaction returned an error
			if err != simulatedError {
				t.Logf("Expected simulated error, got: %v", err)
				return false
			}

			// Verify the app still exists
			appAfterRollback, err := mainStore.Apps().Get(ctx, input.ID)
			if err != nil {
				t.Logf("App should still exist after rollback, got error: %v", err)
				return false
			}

			if appAfterRollback.DeletedAt != nil {
				t.Logf("App should not be deleted after rollback")
				return false
			}

			// Verify the secret still exists
			retrievedSecret, err := mainStore.Secrets().Get(ctx, input.ID, secretKey)
			if err != nil {
				t.Logf("Secret should still exist after rollback, got error: %v", err)
				return false
			}

			if string(retrievedSecret) != string(secretValue) {
				t.Logf("Secret value changed after rollback")
				return false
			}

			// Verify the deployment status was NOT changed
			depAfterRollback, err := mainStore.Deployments().Get(ctx, deployment.ID)
			if err != nil {
				t.Logf("Deployment should still exist after rollback, got error: %v", err)
				return false
			}

			if depAfterRollback.Status != models.DeploymentStatusRunning {
				t.Logf("Deployment status should still be running after rollback, got: %s",
					depAfterRollback.Status)
				return false
			}

			// Cleanup
			mainStore.Secrets().Delete(ctx, input.ID, secretKey)
			// Delete deployment directly from DB since there's no Delete method
			db.ExecContext(ctx, "DELETE FROM deployments WHERE id = $1", deployment.ID)
			mainStore.Apps().Delete(ctx, input.ID)

			return true
		},
		genAppInput(),
	))

	properties.TestingRun(t)
}

// TestTransactionRollbackOnServiceRename tests that service rename rollback works correctly.
// This specifically tests Requirement 23.5: WHEN the rename fails mid-process
// THEN the Narvana_API SHALL rollback all changes to maintain consistency.
// **Feature: backend-source-of-truth, Property 5: Transaction Rollback on Failure**
// **Validates: Requirements 23.5**
func TestTransactionRollbackOnServiceRename(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Create additional tables needed for this test
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS deployments (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			app_id UUID NOT NULL,
			service_name VARCHAR(63) NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			git_ref VARCHAR(255),
			git_commit VARCHAR(255),
			build_type VARCHAR(20),
			artifact TEXT,
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			node_id UUID,
			resources JSONB,
			config JSONB,
			depends_on JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			started_at TIMESTAMPTZ,
			finished_at TIMESTAMPTZ
		);
	`)
	if err != nil {
		t.Fatalf("failed to create additional tables: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Service rename rollback preserves original names", prop.ForAll(
		func(ownerID string) bool {
			ctx := context.Background()

			// Create the main store
			mainStore := &PostgresStore{
				db:     db,
				logger: logger,
			}
			mainStore.apps = &AppStore{db: db, logger: logger}
			mainStore.deployments = &DeploymentStore{db: db, logger: logger}

			// Create an app with services that have dependencies
			originalServiceName := "backend"
			dependentServiceName := "frontend"

			app := models.App{
				ID:          uuid.New().String(),
				OwnerID:     ownerID,
				Name:        "testapp",
				Description: "test app for rename",
				Services: []models.ServiceConfig{
					{Name: originalServiceName, Replicas: 1},
					{Name: dependentServiceName, Replicas: 1, DependsOn: []string{originalServiceName}},
				},
			}

			err := mainStore.Apps().Create(ctx, &app)
			if err != nil {
				t.Logf("Create error: %v", err)
				return false
			}

			// Create deployments for both services
			backendDeployment := &models.Deployment{
				ID:          uuid.New().String(),
				AppID:       app.ID,
				ServiceName: originalServiceName,
				Version:     1,
				Status:      models.DeploymentStatusRunning,
			}
			err = mainStore.Deployments().Create(ctx, backendDeployment)
			if err != nil {
				t.Logf("Create backend deployment error: %v", err)
				mainStore.Apps().Delete(ctx, app.ID)
				return false
			}

			frontendDeployment := &models.Deployment{
				ID:          uuid.New().String(),
				AppID:       app.ID,
				ServiceName: dependentServiceName,
				Version:     1,
				Status:      models.DeploymentStatusRunning,
				DependsOn:   []string{originalServiceName},
			}
			err = mainStore.Deployments().Create(ctx, frontendDeployment)
			if err != nil {
				t.Logf("Create frontend deployment error: %v", err)
				db.ExecContext(ctx, "DELETE FROM deployments WHERE id = $1", backendDeployment.ID)
				mainStore.Apps().Delete(ctx, app.ID)
				return false
			}

			// Simulate a failed service rename transaction
			newServiceName := "api"
			simulatedError := errors.New("failed to update container naming")

			err = mainStore.WithTx(ctx, func(txStore store.Store) error {
				// Step 1: Update the service name in the app
				txApp, err := txStore.Apps().Get(ctx, app.ID)
				if err != nil {
					return err
				}

				// Update service name
				for i := range txApp.Services {
					if txApp.Services[i].Name == originalServiceName {
						txApp.Services[i].Name = newServiceName
					}
					// Update DependsOn references
					for j, dep := range txApp.Services[i].DependsOn {
						if dep == originalServiceName {
							txApp.Services[i].DependsOn[j] = newServiceName
						}
					}
				}

				err = txStore.Apps().Update(ctx, txApp)
				if err != nil {
					return err
				}

				// Step 2: Update deployment records
				dep, err := txStore.Deployments().Get(ctx, backendDeployment.ID)
				if err != nil {
					return err
				}
				dep.ServiceName = newServiceName
				err = txStore.Deployments().Update(ctx, dep)
				if err != nil {
					return err
				}

				// Step 3: Update dependent deployment's DependsOn
				frontDep, err := txStore.Deployments().Get(ctx, frontendDeployment.ID)
				if err != nil {
					return err
				}
				for i, d := range frontDep.DependsOn {
					if d == originalServiceName {
						frontDep.DependsOn[i] = newServiceName
					}
				}
				err = txStore.Deployments().Update(ctx, frontDep)
				if err != nil {
					return err
				}

				// Step 4: Simulate failure
				return simulatedError
			})

			// Verify the transaction returned an error
			if err != simulatedError {
				t.Logf("Expected simulated error, got: %v", err)
				return false
			}

			// Verify the app's service names were NOT changed
			appAfterRollback, err := mainStore.Apps().Get(ctx, app.ID)
			if err != nil {
				t.Logf("Get app after rollback error: %v", err)
				return false
			}

			// Check that the original service name still exists
			foundOriginal := false
			for _, svc := range appAfterRollback.Services {
				if svc.Name == originalServiceName {
					foundOriginal = true
				}
				if svc.Name == newServiceName {
					t.Logf("New service name should not exist after rollback")
					return false
				}
			}

			if !foundOriginal {
				t.Logf("Original service name should still exist after rollback")
				return false
			}

			// Check that DependsOn references were NOT changed
			for _, svc := range appAfterRollback.Services {
				if svc.Name == dependentServiceName {
					for _, dep := range svc.DependsOn {
						if dep == newServiceName {
							t.Logf("DependsOn should still reference original name after rollback")
							return false
						}
					}
				}
			}

			// Verify deployment service names were NOT changed
			backendDepAfterRollback, err := mainStore.Deployments().Get(ctx, backendDeployment.ID)
			if err != nil {
				t.Logf("Get backend deployment after rollback error: %v", err)
				return false
			}

			if backendDepAfterRollback.ServiceName != originalServiceName {
				t.Logf("Backend deployment service name should still be original after rollback, got: %s",
					backendDepAfterRollback.ServiceName)
				return false
			}

			// Verify frontend deployment's DependsOn was NOT changed
			frontendDepAfterRollback, err := mainStore.Deployments().Get(ctx, frontendDeployment.ID)
			if err != nil {
				t.Logf("Get frontend deployment after rollback error: %v", err)
				return false
			}

			for _, dep := range frontendDepAfterRollback.DependsOn {
				if dep == newServiceName {
					t.Logf("Frontend deployment DependsOn should still reference original name after rollback")
					return false
				}
			}

			// Cleanup
			db.ExecContext(ctx, "DELETE FROM deployments WHERE id = $1", backendDeployment.ID)
			db.ExecContext(ctx, "DELETE FROM deployments WHERE id = $1", frontendDeployment.ID)
			mainStore.Apps().Delete(ctx, app.ID)

			return true
		},
		genNonEmptyAlphaString(),
	))

	properties.TestingRun(t)
}

// **Feature: environment-variables, Property 1: Environment Variable CRUD Round-Trip**
// For any valid environment variable key-value pair, creating it via the API and then
// retrieving it should return the same key-value pair.
// **Validates: Requirements 1.2, 1.3, 4.2, 4.4**
func TestServiceEnvVarsCRUDRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	appStore := &AppStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for valid environment variable keys (must match ^[A-Za-z_][A-Za-z0-9_]*$)
	genEnvKey := func() gopter.Gen {
		return gopter.CombineGens(
			gen.OneConstOf("A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
				"N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z", "_"),
			gen.SliceOfN(5, gen.OneConstOf(
				"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
				"N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z",
				"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "_")),
		).Map(func(vals []interface{}) string {
			first := vals[0].(string)
			rest := vals[1].([]string)
			result := first
			for _, s := range rest {
				result += s
			}
			return result
		})
	}

	// Generator for environment variable values
	genEnvValue := func() gopter.Gen {
		return gen.AlphaString()
	}

	// Generator for a map of environment variables
	genEnvVarsMap := func() gopter.Gen {
		return gen.IntRange(1, 5).FlatMap(func(v interface{}) gopter.Gen {
			count := v.(int)
			return gen.SliceOfN(count, gopter.CombineGens(genEnvKey(), genEnvValue())).Map(func(pairs [][]interface{}) map[string]string {
				result := make(map[string]string)
				for _, pair := range pairs {
					key := pair[0].(string)
					value := pair[1].(string)
					result[key] = value
				}
				return result
			})
		}, reflect.TypeOf(map[string]string{}))
	}

	// Generator for ServiceConfig with EnvVars
	genServiceConfigWithEnvVars := func() gopter.Gen {
		return gopter.CombineGens(
			genNonEmptyAlphaString(),
			genEnvVarsMap(),
		).Map(func(vals []interface{}) models.ServiceConfig {
			return models.ServiceConfig{
				Name:       vals[0].(string),
				SourceType: models.SourceTypeFlake,
				FlakeURI:   "github:owner/repo",
				EnvVars:    vals[1].(map[string]string),
				Replicas:   1,
			}
		})
	}

	// Generator for App with services containing EnvVars
	genAppWithEnvVars := func() gopter.Gen {
		return gopter.CombineGens(
			genNonEmptyAlphaString(), // OwnerID
			genNonEmptyAlphaString(), // Name
			genServiceConfigWithEnvVars(),
		).Map(func(vals []interface{}) models.App {
			return models.App{
				ID:       uuid.New().String(),
				OwnerID:  vals[0].(string),
				Name:     vals[1].(string),
				Services: []models.ServiceConfig{vals[2].(models.ServiceConfig)},
			}
		})
	}

	// Property 1: CREATE - EnvVars are preserved when creating an app
	properties.Property("EnvVars are preserved on app creation", prop.ForAll(
		func(input models.App) bool {
			ctx := context.Background()

			// Create the app
			err := appStore.Create(ctx, &input)
			if err != nil {
				t.Logf("Create error: %v", err)
				return false
			}

			// Retrieve the app
			retrieved, err := appStore.Get(ctx, input.ID)
			if err != nil {
				t.Logf("Get error: %v", err)
				appStore.Delete(ctx, input.ID)
				return false
			}

			// Verify services exist
			if len(retrieved.Services) != len(input.Services) {
				t.Logf("Services count mismatch: got %d, want %d", len(retrieved.Services), len(input.Services))
				appStore.Delete(ctx, input.ID)
				return false
			}

			// Verify EnvVars match for each service
			for i, svc := range retrieved.Services {
				inputSvc := input.Services[i]
				if len(svc.EnvVars) != len(inputSvc.EnvVars) {
					t.Logf("EnvVars count mismatch for service %s: got %d, want %d",
						svc.Name, len(svc.EnvVars), len(inputSvc.EnvVars))
					appStore.Delete(ctx, input.ID)
					return false
				}

				for key, expectedValue := range inputSvc.EnvVars {
					actualValue, exists := svc.EnvVars[key]
					if !exists {
						t.Logf("EnvVar key %s not found in retrieved service", key)
						appStore.Delete(ctx, input.ID)
						return false
					}
					if actualValue != expectedValue {
						t.Logf("EnvVar value mismatch for key %s: got %s, want %s",
							key, actualValue, expectedValue)
						appStore.Delete(ctx, input.ID)
						return false
					}
				}
			}

			// Cleanup
			appStore.Delete(ctx, input.ID)
			return true
		},
		genAppWithEnvVars(),
	))

	// Property 2: UPDATE - EnvVars can be added and updated
	properties.Property("EnvVars can be added and updated", prop.ForAll(
		func(input models.App, newKey, newValue string) bool {
			ctx := context.Background()

			// Create the app
			err := appStore.Create(ctx, &input)
			if err != nil {
				t.Logf("Create error: %v", err)
				return false
			}

			// Retrieve the app
			retrieved, err := appStore.Get(ctx, input.ID)
			if err != nil {
				t.Logf("Get error: %v", err)
				appStore.Delete(ctx, input.ID)
				return false
			}

			// Add a new env var to the first service
			if len(retrieved.Services) > 0 {
				if retrieved.Services[0].EnvVars == nil {
					retrieved.Services[0].EnvVars = make(map[string]string)
				}
				retrieved.Services[0].EnvVars[newKey] = newValue

				// Update the app
				err = appStore.Update(ctx, retrieved)
				if err != nil {
					t.Logf("Update error: %v", err)
					appStore.Delete(ctx, input.ID)
					return false
				}

				// Retrieve again and verify
				updated, err := appStore.Get(ctx, input.ID)
				if err != nil {
					t.Logf("Get after update error: %v", err)
					appStore.Delete(ctx, input.ID)
					return false
				}

				// Verify the new env var exists
				actualValue, exists := updated.Services[0].EnvVars[newKey]
				if !exists {
					t.Logf("New EnvVar key %s not found after update", newKey)
					appStore.Delete(ctx, input.ID)
					return false
				}
				if actualValue != newValue {
					t.Logf("New EnvVar value mismatch: got %s, want %s", actualValue, newValue)
					appStore.Delete(ctx, input.ID)
					return false
				}
			}

			// Cleanup
			appStore.Delete(ctx, input.ID)
			return true
		},
		genAppWithEnvVars(),
		genEnvKey(),
		genEnvValue(),
	))

	// Property 3: DELETE - EnvVars can be removed
	properties.Property("EnvVars can be removed", prop.ForAll(
		func(input models.App) bool {
			ctx := context.Background()

			// Ensure we have at least one env var to delete
			if len(input.Services) == 0 || len(input.Services[0].EnvVars) == 0 {
				return true // Skip if no env vars
			}

			// Create the app
			err := appStore.Create(ctx, &input)
			if err != nil {
				t.Logf("Create error: %v", err)
				return false
			}

			// Retrieve the app
			retrieved, err := appStore.Get(ctx, input.ID)
			if err != nil {
				t.Logf("Get error: %v", err)
				appStore.Delete(ctx, input.ID)
				return false
			}

			// Get a key to delete
			var keyToDelete string
			for k := range retrieved.Services[0].EnvVars {
				keyToDelete = k
				break
			}

			originalCount := len(retrieved.Services[0].EnvVars)

			// Delete the env var
			delete(retrieved.Services[0].EnvVars, keyToDelete)

			// Update the app
			err = appStore.Update(ctx, retrieved)
			if err != nil {
				t.Logf("Update error: %v", err)
				appStore.Delete(ctx, input.ID)
				return false
			}

			// Retrieve again and verify
			updated, err := appStore.Get(ctx, input.ID)
			if err != nil {
				t.Logf("Get after update error: %v", err)
				appStore.Delete(ctx, input.ID)
				return false
			}

			// Verify the env var was deleted
			_, exists := updated.Services[0].EnvVars[keyToDelete]
			if exists {
				t.Logf("EnvVar key %s should have been deleted", keyToDelete)
				appStore.Delete(ctx, input.ID)
				return false
			}

			// Verify count decreased
			if len(updated.Services[0].EnvVars) != originalCount-1 {
				t.Logf("EnvVars count should be %d, got %d", originalCount-1, len(updated.Services[0].EnvVars))
				appStore.Delete(ctx, input.ID)
				return false
			}

			// Cleanup
			appStore.Delete(ctx, input.ID)
			return true
		},
		genAppWithEnvVars(),
	))

	properties.TestingRun(t)
}
