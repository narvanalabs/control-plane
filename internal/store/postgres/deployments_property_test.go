package postgres

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// setupDeploymentTestDB creates a test database connection and runs migrations for deployments.
func setupDeploymentTestDB(t *testing.T) *sql.DB {
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
	if err := runDeploymentMigrations(db); err != nil {
		db.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

// runDeploymentMigrations applies the database schema for deployment testing.
func runDeploymentMigrations(db *sql.DB) error {
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
			owner_id VARCHAR(255) NOT NULL,
			name VARCHAR(63) NOT NULL,
			description TEXT,
			build_type VARCHAR(20) NOT NULL CHECK (build_type IN ('oci', 'pure-nix')),
			services JSONB NOT NULL DEFAULT '[]',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		);

		CREATE TABLE deployments (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			app_id UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
			service_name VARCHAR(255) NOT NULL,
			version INTEGER NOT NULL,
			git_ref VARCHAR(255) NOT NULL,
			git_commit VARCHAR(40),
			build_type VARCHAR(20) NOT NULL CHECK (build_type IN ('oci', 'pure-nix')),
			artifact TEXT,
			status VARCHAR(20) NOT NULL CHECK (status IN (
				'pending', 'building', 'built', 'scheduled', 
				'starting', 'running', 'stopping', 'stopped', 'failed'
			)),
			node_id UUID,
			resource_tier VARCHAR(20) NOT NULL CHECK (resource_tier IN (
				'nano', 'small', 'medium', 'large', 'xlarge'
			)),
			config JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			started_at TIMESTAMPTZ,
			finished_at TIMESTAMPTZ
		);

		CREATE INDEX idx_deployments_app_id ON deployments(app_id);
		CREATE INDEX idx_deployments_created_at ON deployments(created_at DESC);
	`
	_, err := db.Exec(schema)
	return err
}


// genDeploymentStatus generates a random DeploymentStatus.
func genDeploymentStatus() gopter.Gen {
	return gen.OneConstOf(
		models.DeploymentStatusPending,
		models.DeploymentStatusBuilding,
		models.DeploymentStatusBuilt,
		models.DeploymentStatusScheduled,
		models.DeploymentStatusStarting,
		models.DeploymentStatusRunning,
		models.DeploymentStatusStopping,
		models.DeploymentStatusStopped,
		models.DeploymentStatusFailed,
	)
}

// genRuntimeConfig generates a random RuntimeConfig.
func genRuntimeConfig() gopter.Gen {
	return gen.Bool().FlatMap(func(v interface{}) gopter.Gen {
		if v.(bool) {
			return gopter.CombineGens(
				genResourceTier(),
				gen.MapOf(gen.AlphaString(), gen.AlphaString()),
				gen.SliceOfN(2, genPortMapping()),
				genOptionalHealthCheckConfig(),
			).Map(func(vals []interface{}) *models.RuntimeConfig {
				return &models.RuntimeConfig{
					ResourceTier: vals[0].(models.ResourceTier),
					EnvVars:      vals[1].(map[string]string),
					Ports:        vals[2].([]models.PortMapping),
					HealthCheck:  vals[3].(*models.HealthCheckConfig),
				}
			})
		}
		return gen.Const((*models.RuntimeConfig)(nil))
	}, reflect.TypeOf((*models.RuntimeConfig)(nil)))
}

// genDeploymentInput generates a random Deployment for creation.
func genDeploymentInput(appID string) gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) <= 63 }),
		gen.IntRange(1, 1000),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
		genBuildType(),
		gen.AlphaString(),
		genDeploymentStatus(),
		genResourceTier(),
		genRuntimeConfig(),
	).Map(func(vals []interface{}) models.Deployment {
		return models.Deployment{
			ID:           uuid.New().String(),
			AppID:        appID,
			ServiceName:  vals[0].(string),
			Version:      vals[1].(int),
			GitRef:       vals[2].(string),
			GitCommit:    vals[3].(string),
			BuildType:    vals[4].(models.BuildType),
			Artifact:     vals[5].(string),
			Status:       vals[6].(models.DeploymentStatus),
			ResourceTier: vals[7].(models.ResourceTier),
			Config:       vals[8].(*models.RuntimeConfig),
		}
	})
}

// **Feature: control-plane, Property 5: Deployment list ordering**
// For any set of deployments for an application, listing deployments should
// return them in chronological order (newest to oldest).
// **Validates: Requirements 2.3**
func TestDeploymentListOrdering(t *testing.T) {
	db := setupDeploymentTestDB(t)
	defer func() {
		db.Exec("DELETE FROM deployments")
		db.Exec("DELETE FROM apps")
		db.Close()
	}()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	appStore := &AppStore{db: db, logger: logger}
	deploymentStore := &DeploymentStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Deployment list is ordered by created_at DESC", prop.ForAll(
		func(numDeployments int) bool {
			ctx := context.Background()

			// Create a test app
			app := &models.App{
				ID:        uuid.New().String(),
				OwnerID:   "test-owner",
				Name:      "test-app-" + uuid.New().String()[:8],
				BuildType: models.BuildTypeOCI,
				Services:  []models.ServiceConfig{},
			}
			if err := appStore.Create(ctx, app); err != nil {
				t.Logf("Failed to create app: %v", err)
				return false
			}

			// Create deployments with small delays to ensure different timestamps
			var createdIDs []string
			for i := 0; i < numDeployments; i++ {
				deployment := &models.Deployment{
					ID:           uuid.New().String(),
					AppID:        app.ID,
					ServiceName:  "service",
					Version:      i + 1,
					GitRef:       "main",
					BuildType:    models.BuildTypeOCI,
					Status:       models.DeploymentStatusPending,
					ResourceTier: models.ResourceTierSmall,
					CreatedAt:    time.Now().Add(time.Duration(i) * time.Millisecond),
				}
				if err := deploymentStore.Create(ctx, deployment); err != nil {
					t.Logf("Failed to create deployment: %v", err)
					return false
				}
				createdIDs = append(createdIDs, deployment.ID)
			}

			// List deployments
			deployments, err := deploymentStore.List(ctx, app.ID)
			if err != nil {
				t.Logf("Failed to list deployments: %v", err)
				return false
			}

			// Verify count
			if len(deployments) != numDeployments {
				t.Logf("Expected %d deployments, got %d", numDeployments, len(deployments))
				return false
			}

			// Verify ordering (should be DESC by created_at)
			for i := 1; i < len(deployments); i++ {
				if deployments[i-1].CreatedAt.Before(deployments[i].CreatedAt) {
					t.Logf("Deployments not in DESC order: %v before %v",
						deployments[i-1].CreatedAt, deployments[i].CreatedAt)
					return false
				}
			}

			// Cleanup
			for _, id := range createdIDs {
				db.Exec("DELETE FROM deployments WHERE id = $1", id)
			}
			appStore.Delete(ctx, app.ID)

			return true
		},
		gen.IntRange(2, 10),
	))

	properties.TestingRun(t)
}
