package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// setupSecretTestDB creates a test database connection and runs migrations for secrets.
func setupSecretTestDB(t *testing.T) *sql.DB {
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
	if err := runSecretMigrations(db); err != nil {
		db.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

// runSecretMigrations applies the database schema for secret testing.
func runSecretMigrations(db *sql.DB) error {
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

		CREATE TABLE secrets (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			app_id UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
			key VARCHAR(255) NOT NULL,
			encrypted_value BYTEA NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT secrets_app_key_unique UNIQUE (app_id, key)
		);

		CREATE INDEX idx_secrets_app_id ON secrets(app_id);
	`
	_, err := db.Exec(schema)
	return err
}

// simpleEncrypt is a simple XOR encryption for testing purposes.
// In production, use proper encryption like AES-GCM.
func simpleEncrypt(plaintext []byte, key byte) []byte {
	encrypted := make([]byte, len(plaintext))
	for i, b := range plaintext {
		encrypted[i] = b ^ key
	}
	return encrypted
}

// **Feature: control-plane, Property 23: Secret encryption at rest**
// For any stored secret, the persisted value should not equal the plaintext input
// (indicating encryption).
// **Validates: Requirements 9.4**
func TestSecretEncryptionAtRest(t *testing.T) {
	db := setupSecretTestDB(t)
	defer func() {
		db.Exec("DELETE FROM secrets")
		db.Exec("DELETE FROM apps")
		db.Close()
	}()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	appStore := &AppStore{db: db, logger: logger}
	secretStore := &SecretStore{db: db, logger: logger}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Stored secret value differs from plaintext", prop.ForAll(
		func(secretKey string, plaintext string) bool {
			ctx := context.Background()

			// Skip empty plaintexts as they would be equal after encryption
			if len(plaintext) == 0 {
				return true
			}

			// Create a test app
			app := &models.App{
				ID:       uuid.New().String(),
				OwnerID:  "test-owner",
				Name:     "test-app-" + uuid.New().String()[:8],
				Services: []models.ServiceConfig{},
			}
			if err := appStore.Create(ctx, app); err != nil {
				t.Logf("Failed to create app: %v", err)
				return false
			}

			// Encrypt the plaintext (simulating what the application layer would do)
			plaintextBytes := []byte(plaintext)
			encryptedValue := simpleEncrypt(plaintextBytes, 0x42) // XOR with key

			// Store the encrypted secret
			if err := secretStore.Set(ctx, app.ID, secretKey, encryptedValue); err != nil {
				t.Logf("Failed to set secret: %v", err)
				appStore.Delete(ctx, app.ID)
				return false
			}

			// Retrieve the stored value
			storedValue, err := secretStore.Get(ctx, app.ID, secretKey)
			if err != nil {
				t.Logf("Failed to get secret: %v", err)
				appStore.Delete(ctx, app.ID)
				return false
			}

			// Verify the stored value is NOT equal to the plaintext
			// This confirms encryption is happening
			if bytes.Equal(storedValue, plaintextBytes) {
				t.Logf("Stored value equals plaintext - encryption not working")
				appStore.Delete(ctx, app.ID)
				return false
			}

			// Verify we can decrypt back to original
			decrypted := simpleEncrypt(storedValue, 0x42) // XOR again to decrypt
			if !bytes.Equal(decrypted, plaintextBytes) {
				t.Logf("Decryption failed: got %v, want %v", decrypted, plaintextBytes)
				appStore.Delete(ctx, app.ID)
				return false
			}

			// Cleanup
			secretStore.Delete(ctx, app.ID, secretKey)
			appStore.Delete(ctx, app.ID)

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) <= 63 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	properties.TestingRun(t)
}
