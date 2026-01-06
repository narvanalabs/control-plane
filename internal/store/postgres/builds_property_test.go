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

// setupBuildTestDB creates a test database connection and runs migrations for builds tests.
func setupBuildTestDB(t *testing.T) *sql.DB {
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

	// Run migrations for builds tests
	if err := runBuildMigrations(db); err != nil {
		db.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

// cleanupBuildTestDB cleans up test data and closes the connection.
func cleanupBuildTestDB(t *testing.T, db *sql.DB) {
	t.Helper()
	db.Exec("DELETE FROM builds")
	db.Exec("DELETE FROM deployments")
	db.Exec("DELETE FROM apps")
	db.Close()
}

// runBuildMigrations applies the database schema for builds testing.
func runBuildMigrations(db *sql.DB) error {
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
			resources JSONB,
			config JSONB,
			depends_on JSONB NOT NULL DEFAULT '[]',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			started_at TIMESTAMPTZ,
			finished_at TIMESTAMPTZ
		);

		CREATE TABLE builds (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
			app_id UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
			git_url TEXT NOT NULL,
			git_ref VARCHAR(255) NOT NULL,
			flake_output VARCHAR(255) NOT NULL,
			build_type VARCHAR(20) NOT NULL CHECK (build_type IN ('oci', 'pure-nix')),
			status VARCHAR(20) NOT NULL CHECK (status IN (
				'queued', 'running', 'succeeded', 'failed'
			)),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			started_at TIMESTAMPTZ,
			finished_at TIMESTAMPTZ,
			build_strategy VARCHAR(20),
			timeout_seconds INTEGER NOT NULL DEFAULT 1800,
			retry_count INTEGER NOT NULL DEFAULT 0,
			retry_as_oci BOOLEAN NOT NULL DEFAULT false,
			generated_flake TEXT,
			flake_lock TEXT,
			vendor_hash VARCHAR(255),
			detection_result JSONB,
			detected_at TIMESTAMPTZ
		);
	`
	_, err := db.Exec(schema)
	return err
}


// genBuildBuildStrategy generates a random BuildStrategy for builds tests.
func genBuildBuildStrategy() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStrategyFlake,
		models.BuildStrategyAutoGo,
		models.BuildStrategyAutoRust,
		models.BuildStrategyAutoNode,
		models.BuildStrategyAutoPython,
		models.BuildStrategyDockerfile,
		models.BuildStrategyNixpacks,
		models.BuildStrategyAuto,
	)
}

// genBuildFramework generates a random Framework for builds tests.
func genBuildFramework() gopter.Gen {
	return gen.OneConstOf(
		models.FrameworkGeneric,
		models.FrameworkNextJS,
		models.FrameworkExpress,
		models.FrameworkReact,
		models.FrameworkFastify,
		models.FrameworkDjango,
		models.FrameworkFastAPI,
		models.FrameworkFlask,
	)
}

// genBuildDetectionResult generates a random DetectionResult for builds tests.
func genBuildDetectionResult() gopter.Gen {
	return gopter.CombineGens(
		genBuildBuildStrategy(),
		genBuildFramework(),
		gen.AlphaString(),                                            // Version
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI), // RecommendedBuildType
		gen.SliceOfN(3, gen.AlphaString()),                           // EntryPoints
		gen.Float64Range(0.0, 1.0),                                   // Confidence
		gen.SliceOfN(2, gen.AlphaString()),                           // Warnings
	).Map(func(vals []interface{}) *models.DetectionResult {
		// Create a simple SuggestedConfig map
		suggestedConfig := map[string]interface{}{
			"cgo_enabled": true,
			"go_version":  "1.21",
		}
		return &models.DetectionResult{
			Strategy:             vals[0].(models.BuildStrategy),
			Framework:            vals[1].(models.Framework),
			Version:              vals[2].(string),
			SuggestedConfig:      suggestedConfig,
			RecommendedBuildType: vals[3].(models.BuildType),
			EntryPoints:          vals[4].([]string),
			Confidence:           vals[5].(float64),
			Warnings:             vals[6].([]string),
		}
	})
}

// genBuildOptionalDetectionResult generates an optional DetectionResult pointer for builds tests.
func genBuildOptionalDetectionResult() gopter.Gen {
	return gen.Bool().FlatMap(func(v interface{}) gopter.Gen {
		if v.(bool) {
			return genBuildDetectionResult()
		}
		return gen.Const((*models.DetectionResult)(nil))
	}, reflect.TypeOf((*models.DetectionResult)(nil)))
}

// genBuildJob generates a random BuildJob for testing.
func genBuildJob(appID, deploymentID string) gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString(),                                            // GitURL
		gen.AlphaString(),                                            // GitRef
		gen.AlphaString(),                                            // FlakeOutput
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI), // BuildType
		genBuildBuildStrategy(),                                      // BuildStrategy
		genBuildOptionalDetectionResult(),                            // DetectionResult
	).Map(func(vals []interface{}) models.BuildJob {
		now := time.Now().UTC()
		var detectedAt *time.Time
		if vals[5] != nil {
			detectedAt = &now
		}
		return models.BuildJob{
			ID:              uuid.New().String(),
			DeploymentID:    deploymentID,
			AppID:           appID,
			GitURL:          vals[0].(string),
			GitRef:          vals[1].(string),
			FlakeOutput:     vals[2].(string),
			BuildType:       vals[3].(models.BuildType),
			Status:          models.BuildStatusQueued,
			BuildStrategy:   vals[4].(models.BuildStrategy),
			DetectionResult: vals[5].(*models.DetectionResult),
			DetectedAt:      detectedAt,
			TimeoutSeconds:  1800,
		}
	})
}


// **Feature: build-detection-integration, Property 6: Detection results persisted**
// *For any* completed build, the BuildJob record must contain the DetectionResult that was used.
// **Validates: Requirements 2.1**
func TestDetectionResultPersistence(t *testing.T) {
	db := setupBuildTestDB(t)
	defer cleanupBuildTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	appStore := &AppStore{db: db, logger: logger}
	deploymentStore := &DeploymentStore{db: db, logger: logger}
	buildStore := &BuildStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Detection result round-trip preserves data", prop.ForAll(
		func(detectionResult *models.DetectionResult) bool {
			ctx := context.Background()

			// Create a test app
			app := &models.App{
				ID:          uuid.New().String(),
				OwnerID:     "test-owner",
				Name:        "testapp-" + uuid.New().String()[:8],
				Description: "test app",
				Services:    []models.ServiceConfig{},
			}
			if err := appStore.Create(ctx, app); err != nil {
				t.Logf("Create app error: %v", err)
				return false
			}
			defer appStore.Delete(ctx, app.ID)

			// Create a test deployment
			deployment := &models.Deployment{
				ID:          uuid.New().String(),
				AppID:       app.ID,
				ServiceName: "test-service",
				Version:     1,
				GitRef:      "main",
				BuildType:   models.BuildTypePureNix,
				Status:      models.DeploymentStatusPending,
			}
			if err := deploymentStore.Create(ctx, deployment); err != nil {
				t.Logf("Create deployment error: %v", err)
				return false
			}

			// Create a build with detection result
			now := time.Now().UTC()
			var detectedAt *time.Time
			if detectionResult != nil {
				detectedAt = &now
			}

			build := &models.BuildJob{
				ID:              uuid.New().String(),
				DeploymentID:    deployment.ID,
				AppID:           app.ID,
				GitURL:          "https://github.com/test/repo",
				GitRef:          "main",
				FlakeOutput:     "default",
				BuildType:       models.BuildTypePureNix,
				Status:          models.BuildStatusQueued,
				BuildStrategy:   models.BuildStrategyAutoGo,
				DetectionResult: detectionResult,
				DetectedAt:      detectedAt,
				TimeoutSeconds:  1800,
			}

			// Create the build
			if err := buildStore.Create(ctx, build); err != nil {
				t.Logf("Create build error: %v", err)
				return false
			}

			// Retrieve the build
			retrieved, err := buildStore.Get(ctx, build.ID)
			if err != nil {
				t.Logf("Get build error: %v", err)
				return false
			}

			// Verify detection result is preserved
			if detectionResult == nil {
				if retrieved.DetectionResult != nil {
					t.Logf("Expected nil DetectionResult, got non-nil")
					return false
				}
				if retrieved.DetectedAt != nil {
					t.Logf("Expected nil DetectedAt, got non-nil")
					return false
				}
			} else {
				if retrieved.DetectionResult == nil {
					t.Logf("Expected non-nil DetectionResult, got nil")
					return false
				}

				// Verify key fields match
				if retrieved.DetectionResult.Strategy != detectionResult.Strategy {
					t.Logf("Strategy mismatch: got %s, want %s", retrieved.DetectionResult.Strategy, detectionResult.Strategy)
					return false
				}
				if retrieved.DetectionResult.Framework != detectionResult.Framework {
					t.Logf("Framework mismatch: got %s, want %s", retrieved.DetectionResult.Framework, detectionResult.Framework)
					return false
				}
				if retrieved.DetectionResult.Version != detectionResult.Version {
					t.Logf("Version mismatch: got %s, want %s", retrieved.DetectionResult.Version, detectionResult.Version)
					return false
				}
				if retrieved.DetectionResult.RecommendedBuildType != detectionResult.RecommendedBuildType {
					t.Logf("RecommendedBuildType mismatch: got %s, want %s", retrieved.DetectionResult.RecommendedBuildType, detectionResult.RecommendedBuildType)
					return false
				}
				if retrieved.DetectionResult.Confidence != detectionResult.Confidence {
					t.Logf("Confidence mismatch: got %f, want %f", retrieved.DetectionResult.Confidence, detectionResult.Confidence)
					return false
				}
				if len(retrieved.DetectionResult.EntryPoints) != len(detectionResult.EntryPoints) {
					t.Logf("EntryPoints length mismatch: got %d, want %d", len(retrieved.DetectionResult.EntryPoints), len(detectionResult.EntryPoints))
					return false
				}
				if len(retrieved.DetectionResult.Warnings) != len(detectionResult.Warnings) {
					t.Logf("Warnings length mismatch: got %d, want %d", len(retrieved.DetectionResult.Warnings), len(detectionResult.Warnings))
					return false
				}

				// Verify DetectedAt is set
				if retrieved.DetectedAt == nil {
					t.Logf("Expected non-nil DetectedAt, got nil")
					return false
				}
			}

			return true
		},
		genBuildOptionalDetectionResult(),
	))

	properties.TestingRun(t)
}

// **Feature: build-detection-integration, Property 6: Detection results persisted (Update)**
// *For any* build that is updated with detection results, the updated DetectionResult must be persisted.
// **Validates: Requirements 2.1**
func TestDetectionResultUpdatePersistence(t *testing.T) {
	db := setupBuildTestDB(t)
	defer cleanupBuildTestDB(t, db)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	appStore := &AppStore{db: db, logger: logger}
	deploymentStore := &DeploymentStore{db: db, logger: logger}
	buildStore := &BuildStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Detection result update round-trip preserves data", prop.ForAll(
		func(detectionResult *models.DetectionResult) bool {
			ctx := context.Background()

			// Create a test app
			app := &models.App{
				ID:          uuid.New().String(),
				OwnerID:     "test-owner",
				Name:        "testapp-" + uuid.New().String()[:8],
				Description: "test app",
				Services:    []models.ServiceConfig{},
			}
			if err := appStore.Create(ctx, app); err != nil {
				t.Logf("Create app error: %v", err)
				return false
			}
			defer appStore.Delete(ctx, app.ID)

			// Create a test deployment
			deployment := &models.Deployment{
				ID:          uuid.New().String(),
				AppID:       app.ID,
				ServiceName: "test-service",
				Version:     1,
				GitRef:      "main",
				BuildType:   models.BuildTypePureNix,
				Status:      models.DeploymentStatusPending,
			}
			if err := deploymentStore.Create(ctx, deployment); err != nil {
				t.Logf("Create deployment error: %v", err)
				return false
			}

			// Create a build WITHOUT detection result initially
			build := &models.BuildJob{
				ID:             uuid.New().String(),
				DeploymentID:   deployment.ID,
				AppID:          app.ID,
				GitURL:         "https://github.com/test/repo",
				GitRef:         "main",
				FlakeOutput:    "default",
				BuildType:      models.BuildTypePureNix,
				Status:         models.BuildStatusQueued,
				BuildStrategy:  models.BuildStrategyAutoGo,
				TimeoutSeconds: 1800,
			}

			if err := buildStore.Create(ctx, build); err != nil {
				t.Logf("Create build error: %v", err)
				return false
			}

			// Update the build with detection result
			now := time.Now().UTC()
			build.DetectionResult = detectionResult
			if detectionResult != nil {
				build.DetectedAt = &now
			}
			build.Status = models.BuildStatusRunning

			if err := buildStore.Update(ctx, build); err != nil {
				t.Logf("Update build error: %v", err)
				return false
			}

			// Retrieve the build
			retrieved, err := buildStore.Get(ctx, build.ID)
			if err != nil {
				t.Logf("Get build error: %v", err)
				return false
			}

			// Verify detection result is preserved after update
			if detectionResult == nil {
				if retrieved.DetectionResult != nil {
					t.Logf("Expected nil DetectionResult after update, got non-nil")
					return false
				}
			} else {
				if retrieved.DetectionResult == nil {
					t.Logf("Expected non-nil DetectionResult after update, got nil")
					return false
				}

				// Verify key fields match
				if retrieved.DetectionResult.Strategy != detectionResult.Strategy {
					t.Logf("Strategy mismatch after update: got %s, want %s", retrieved.DetectionResult.Strategy, detectionResult.Strategy)
					return false
				}
				if retrieved.DetectionResult.Framework != detectionResult.Framework {
					t.Logf("Framework mismatch after update: got %s, want %s", retrieved.DetectionResult.Framework, detectionResult.Framework)
					return false
				}
			}

			return true
		},
		genBuildOptionalDetectionResult(),
	))

	properties.TestingRun(t)
}
