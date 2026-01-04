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

		CREATE UNIQUE INDEX idx_apps_owner_name_unique ON apps(owner_id, name) WHERE deleted_at IS NULL;
		CREATE INDEX idx_apps_owner_id ON apps(owner_id) WHERE deleted_at IS NULL;
		CREATE INDEX idx_apps_org_id ON apps(org_id) WHERE deleted_at IS NULL;
	`
	_, err := db.Exec(schema)
	return err
}


// genResourceTier generates a random ResourceTier.
func genResourceTier() gopter.Gen {
	return gen.OneConstOf(
		models.ResourceTierNano,
		models.ResourceTierSmall,
		models.ResourceTierMedium,
		models.ResourceTierLarge,
		models.ResourceTierXLarge,
	)
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
		genResourceTier(),
		gen.IntRange(1, 10),
		gen.SliceOfN(2, genPortMapping()),
		genOptionalHealthCheckConfig(),
		gen.SliceOfN(2, gen.AlphaString()),
	).Map(func(vals []interface{}) models.ServiceConfig {
		return models.ServiceConfig{
			Name:         vals[0].(string),
			FlakeOutput:  vals[1].(string),
			ResourceTier: vals[2].(models.ResourceTier),
			Replicas:     vals[3].(int),
			Ports:        vals[4].([]models.PortMapping),
			HealthCheck:  vals[5].(*models.HealthCheckConfig),
			DependsOn:    vals[6].([]string),
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
// For any user and application name, attempting to create a second application
// with the same name should be rejected while the first exists.
// **Validates: Requirements 1.5**
func TestAppNameUniqueness(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	store := &AppStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Duplicate app names for same owner are rejected", prop.ForAll(
		func(input models.App) bool {
			ctx := context.Background()

			// Create the first app
			err := store.Create(ctx, &input)
			if err != nil {
				t.Logf("First create error: %v", err)
				return false
			}

			// Try to create a second app with the same owner and name
			duplicate := models.App{
				ID:          uuid.New().String(),
				OwnerID:     input.OwnerID,
				Name:        input.Name,
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
		genAppInput(),
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
