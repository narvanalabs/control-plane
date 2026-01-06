package postgres

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// setupNodeTestDB creates a test database connection and runs migrations for nodes.
func setupNodeTestDB(t *testing.T) *sql.DB {
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
	if err := runNodeMigrations(db); err != nil {
		db.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

// runNodeMigrations applies the database schema for node testing.
func runNodeMigrations(db *sql.DB) error {
	// Drop existing tables to ensure clean state
	_, _ = db.Exec("DROP TABLE IF EXISTS logs CASCADE")
	_, _ = db.Exec("DROP TABLE IF EXISTS secrets CASCADE")
	_, _ = db.Exec("DROP TABLE IF EXISTS builds CASCADE")
	_, _ = db.Exec("DROP TABLE IF EXISTS deployments CASCADE")
	_, _ = db.Exec("DROP TABLE IF EXISTS nodes CASCADE")
	_, _ = db.Exec("DROP TABLE IF EXISTS apps CASCADE")

	schema := `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE nodes (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			hostname VARCHAR(255) NOT NULL,
			address VARCHAR(255) NOT NULL,
			grpc_port INTEGER NOT NULL DEFAULT 9090,
			healthy BOOLEAN NOT NULL DEFAULT true,
			cpu_total DOUBLE PRECISION NOT NULL DEFAULT 0,
			cpu_available DOUBLE PRECISION NOT NULL DEFAULT 0,
			memory_total BIGINT NOT NULL DEFAULT 0,
			memory_available BIGINT NOT NULL DEFAULT 0,
			disk_total BIGINT NOT NULL DEFAULT 0,
			disk_available BIGINT NOT NULL DEFAULT 0,
			nix_store_total BIGINT DEFAULT 0,
			nix_store_used BIGINT DEFAULT 0,
			nix_store_available BIGINT DEFAULT 0,
			nix_store_usage_percent DOUBLE PRECISION DEFAULT 0,
			container_storage_total BIGINT DEFAULT 0,
			container_storage_used BIGINT DEFAULT 0,
			container_storage_available BIGINT DEFAULT 0,
			container_storage_usage_percent DOUBLE PRECISION DEFAULT 0,
			cached_paths TEXT[] DEFAULT '{}',
			last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX idx_nodes_healthy ON nodes(healthy);
		CREATE INDEX idx_nodes_last_heartbeat ON nodes(last_heartbeat);
	`
	_, err := db.Exec(schema)
	return err
}

// genNodeResources generates random NodeResources.
func genNodeResources() gopter.Gen {
	return gopter.CombineGens(
		gen.Float64Range(1, 64),
		gen.Float64Range(0, 64),
		gen.Int64Range(1<<30, 256<<30),
		gen.Int64Range(0, 256<<30),
		gen.Int64Range(1<<30, 1<<40),
		gen.Int64Range(0, 1<<40),
	).Map(func(vals []interface{}) *models.NodeResources {
		return &models.NodeResources{
			CPUTotal:        vals[0].(float64),
			CPUAvailable:    vals[1].(float64),
			MemoryTotal:     vals[2].(int64),
			MemoryAvailable: vals[3].(int64),
			DiskTotal:       vals[4].(int64),
			DiskAvailable:   vals[5].(int64),
		}
	})
}

// **Feature: control-plane, Property 15: Heartbeat updates node state**
// For any heartbeat received, the node's last-seen timestamp should be updated
// to a value >= the previous timestamp.
// **Validates: Requirements 5.1**
func TestHeartbeatUpdatesNodeState(t *testing.T) {
	db := setupNodeTestDB(t)
	defer func() {
		db.Exec("DELETE FROM nodes")
		db.Close()
	}()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	nodeStore := &NodeStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Heartbeat updates last_heartbeat to >= previous value", prop.ForAll(
		func(resources *models.NodeResources) bool {
			ctx := context.Background()

			// Create a test node with an old heartbeat
			oldHeartbeat := time.Now().Add(-1 * time.Hour).UTC()
			node := &models.Node{
				ID:            uuid.New().String(),
				Hostname:      "test-node",
				Address:       "192.168.1.1",
				GRPCPort:      9090,
				Healthy:       false, // Start unhealthy
				Resources:     resources,
				CachedPaths:   []string{},
				LastHeartbeat: oldHeartbeat,
			}

			if err := nodeStore.Register(ctx, node); err != nil {
				t.Logf("Failed to register node: %v", err)
				return false
			}

			// Get the node to verify initial state
			initialNode, err := nodeStore.Get(ctx, node.ID)
			if err != nil {
				t.Logf("Failed to get initial node: %v", err)
				return false
			}

			// Update heartbeat with new resources
			newResources := &models.NodeResources{
				CPUTotal:        resources.CPUTotal,
				CPUAvailable:    resources.CPUAvailable * 0.9, // Slightly different
				MemoryTotal:     resources.MemoryTotal,
				MemoryAvailable: resources.MemoryAvailable,
				DiskTotal:       resources.DiskTotal,
				DiskAvailable:   resources.DiskAvailable,
			}

			if err := nodeStore.UpdateHeartbeat(ctx, node.ID, newResources); err != nil {
				t.Logf("Failed to update heartbeat: %v", err)
				return false
			}

			// Get the node again
			updatedNode, err := nodeStore.Get(ctx, node.ID)
			if err != nil {
				t.Logf("Failed to get updated node: %v", err)
				return false
			}

			// Verify last_heartbeat is >= previous
			if updatedNode.LastHeartbeat.Before(initialNode.LastHeartbeat) {
				t.Logf("LastHeartbeat went backwards: %v < %v",
					updatedNode.LastHeartbeat, initialNode.LastHeartbeat)
				return false
			}

			// Verify node is now healthy
			if !updatedNode.Healthy {
				t.Logf("Node should be healthy after heartbeat")
				return false
			}

			// Verify resources were updated
			if updatedNode.Resources.CPUAvailable != newResources.CPUAvailable {
				t.Logf("CPUAvailable not updated: got %v, want %v",
					updatedNode.Resources.CPUAvailable, newResources.CPUAvailable)
				return false
			}

			// Cleanup
			db.Exec("DELETE FROM nodes WHERE id = $1", node.ID)

			return true
		},
		genNodeResources(),
	))

	properties.TestingRun(t)
}
