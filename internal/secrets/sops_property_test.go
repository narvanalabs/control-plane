package secrets

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: platform-enhancements, Property 7: Secret Encryption Round-Trip**
// For any secret value, encrypting with SOPS and then decrypting SHALL produce the original value.
// **Validates: Requirements 13.1, 13.2, 13.3**
func TestSecretEncryptionRoundTrip(t *testing.T) {
	// Generate a key pair for testing
	publicKey, privateKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create a service that can both encrypt and decrypt
	svc, err := NewSOPSService(&Config{
		AgePublicKey:  publicKey,
		AgePrivateKey: privateKey,
	}, logger)
	if err != nil {
		t.Fatalf("failed to create SOPS service: %v", err)
	}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("encrypt then decrypt returns original plaintext", prop.ForAll(
		func(plaintext []byte) bool {
			ctx := context.Background()

			// Encrypt the plaintext
			ciphertext, err := svc.Encrypt(ctx, plaintext)
			if err != nil {
				t.Logf("encryption failed: %v", err)
				return false
			}

			// Decrypt the ciphertext
			decrypted, err := svc.Decrypt(ctx, ciphertext)
			if err != nil {
				t.Logf("decryption failed: %v", err)
				return false
			}

			// Verify round-trip preserves the original value
			return bytes.Equal(plaintext, decrypted)
		},
		// Generate arbitrary byte slices (including empty)
		gen.SliceOf(gen.UInt8()).Map(func(vals []uint8) []byte {
			result := make([]byte, len(vals))
			for i, v := range vals {
				result[i] = byte(v)
			}
			return result
		}),
	))

	properties.TestingRun(t)
}

// **Feature: platform-enhancements, Property 8: Secret Encryption Produces Different Output**
// For any non-empty secret value, the encrypted output SHALL differ from the plaintext input.
// **Validates: Requirements 13.1**
func TestSecretEncryptionProducesDifferentOutput(t *testing.T) {
	// Generate a key pair for testing
	publicKey, privateKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	svc, err := NewSOPSService(&Config{
		AgePublicKey:  publicKey,
		AgePrivateKey: privateKey,
	}, logger)
	if err != nil {
		t.Fatalf("failed to create SOPS service: %v", err)
	}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("encrypted output differs from plaintext for non-empty input", prop.ForAll(
		func(plaintext []byte) bool {
			// Skip empty plaintexts as they are a degenerate case
			if len(plaintext) == 0 {
				return true
			}

			ctx := context.Background()

			// Encrypt the plaintext
			ciphertext, err := svc.Encrypt(ctx, plaintext)
			if err != nil {
				t.Logf("encryption failed: %v", err)
				return false
			}

			// Verify the ciphertext is different from plaintext
			// This ensures encryption is actually happening
			return !bytes.Equal(plaintext, ciphertext)
		},
		// Generate non-empty byte slices
		gen.SliceOfN(1, gen.UInt8()).Map(func(vals []uint8) []byte {
			result := make([]byte, len(vals))
			for i, v := range vals {
				result[i] = byte(v)
			}
			return result
		}),
	))

	properties.TestingRun(t)
}

// TestKeyPairGeneration verifies that key pair generation works correctly.
func TestKeyPairGeneration(t *testing.T) {
	publicKey, privateKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	// Verify keys are non-empty
	if publicKey == "" {
		t.Error("public key is empty")
	}
	if privateKey == "" {
		t.Error("private key is empty")
	}

	// Verify public key format (age1...)
	if len(publicKey) < 4 || publicKey[:4] != "age1" {
		t.Errorf("public key has unexpected format: %s", publicKey)
	}

	// Verify private key format (AGE-SECRET-KEY-1...)
	if len(privateKey) < 16 || privateKey[:16] != "AGE-SECRET-KEY-1" {
		t.Errorf("private key has unexpected format: %s", privateKey[:min(16, len(privateKey))])
	}
}

// TestEncryptWithoutPublicKey verifies that encryption fails without a public key.
func TestEncryptWithoutPublicKey(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	svc, err := NewSOPSService(&Config{}, logger)
	if err != nil {
		t.Fatalf("failed to create SOPS service: %v", err)
	}

	_, err = svc.Encrypt(context.Background(), []byte("test"))
	if err != ErrNoPublicKey {
		t.Errorf("expected ErrNoPublicKey, got: %v", err)
	}
}

// TestDecryptWithoutPrivateKey verifies that decryption fails without a private key.
func TestDecryptWithoutPrivateKey(t *testing.T) {
	publicKey, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create service with only public key (like API server)
	svc, err := NewSOPSService(&Config{
		AgePublicKey: publicKey,
	}, logger)
	if err != nil {
		t.Fatalf("failed to create SOPS service: %v", err)
	}

	// Encrypt something
	ciphertext, err := svc.Encrypt(context.Background(), []byte("test"))
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	// Try to decrypt without private key
	_, err = svc.Decrypt(context.Background(), ciphertext)
	if err != ErrNoPrivateKey {
		t.Errorf("expected ErrNoPrivateKey, got: %v", err)
	}
}

// TestCanEncryptCanDecrypt verifies the capability check methods.
func TestCanEncryptCanDecrypt(t *testing.T) {
	publicKey, privateKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name       string
		config     *Config
		canEncrypt bool
		canDecrypt bool
	}{
		{
			name:       "no keys",
			config:     &Config{},
			canEncrypt: false,
			canDecrypt: false,
		},
		{
			name:       "public key only",
			config:     &Config{AgePublicKey: publicKey},
			canEncrypt: true,
			canDecrypt: false,
		},
		{
			name:       "private key only",
			config:     &Config{AgePrivateKey: privateKey},
			canEncrypt: false,
			canDecrypt: true,
		},
		{
			name:       "both keys",
			config:     &Config{AgePublicKey: publicKey, AgePrivateKey: privateKey},
			canEncrypt: true,
			canDecrypt: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewSOPSService(tt.config, logger)
			if err != nil {
				t.Fatalf("failed to create SOPS service: %v", err)
			}

			if svc.CanEncrypt() != tt.canEncrypt {
				t.Errorf("CanEncrypt() = %v, want %v", svc.CanEncrypt(), tt.canEncrypt)
			}
			if svc.CanDecrypt() != tt.canDecrypt {
				t.Errorf("CanDecrypt() = %v, want %v", svc.CanDecrypt(), tt.canDecrypt)
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestKeyRotation verifies that key rotation works correctly.
// It encrypts secrets with one key, rotates to a new key, and verifies
// the secrets can be decrypted with the new key.
func TestKeyRotation(t *testing.T) {
	// Generate initial key pair
	publicKey1, privateKey1, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate initial key pair: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create service with initial keys
	svc, err := NewSOPSService(&Config{
		AgePublicKey:  publicKey1,
		AgePrivateKey: privateKey1,
	}, logger)
	if err != nil {
		t.Fatalf("failed to create SOPS service: %v", err)
	}

	ctx := context.Background()

	// Create some test secrets
	testSecrets := map[string]string{
		"DB_PASSWORD": "secret123",
		"API_KEY":     "key456",
		"JWT_SECRET":  "jwt789",
	}

	// Encrypt secrets with initial key
	encryptedSecrets := make(map[string]map[string][]byte)
	encryptedSecrets["app1"] = make(map[string][]byte)
	for key, value := range testSecrets {
		ciphertext, err := svc.Encrypt(ctx, []byte(value))
		if err != nil {
			t.Fatalf("failed to encrypt secret %s: %v", key, err)
		}
		encryptedSecrets["app1"][key] = ciphertext
	}

	// Perform key rotation
	result, err := svc.RotateKeys(ctx, encryptedSecrets)
	if err != nil {
		t.Fatalf("key rotation failed: %v", err)
	}

	// Verify new keys were generated
	if result.NewPublicKey == "" {
		t.Error("new public key is empty")
	}
	if result.NewPrivateKey == "" {
		t.Error("new private key is empty")
	}
	if result.NewPublicKey == publicKey1 {
		t.Error("new public key should be different from old key")
	}
	if result.NewPrivateKey == privateKey1 {
		t.Error("new private key should be different from old key")
	}

	// Verify no secrets failed
	if len(result.FailedSecrets) > 0 {
		t.Errorf("some secrets failed to rotate: %v", result.FailedSecrets)
	}

	// Create a new service with the new keys to verify decryption
	newSvc, err := NewSOPSService(&Config{
		AgePublicKey:  result.NewPublicKey,
		AgePrivateKey: result.NewPrivateKey,
	}, logger)
	if err != nil {
		t.Fatalf("failed to create new SOPS service: %v", err)
	}

	// Verify all rotated secrets can be decrypted with new key
	for key, expectedValue := range testSecrets {
		rotatedCiphertext := result.RotatedSecrets["app1"][key]
		decrypted, err := newSvc.Decrypt(ctx, rotatedCiphertext)
		if err != nil {
			t.Errorf("failed to decrypt rotated secret %s: %v", key, err)
			continue
		}
		if string(decrypted) != expectedValue {
			t.Errorf("rotated secret %s mismatch: got %q, want %q", key, decrypted, expectedValue)
		}
	}

	// Verify old key cannot decrypt new secrets
	for key := range testSecrets {
		rotatedCiphertext := result.RotatedSecrets["app1"][key]
		_, err := svc.Decrypt(ctx, rotatedCiphertext)
		if err == nil {
			t.Errorf("old key should not be able to decrypt rotated secret %s", key)
		}
	}
}

// TestUpdateKeys verifies that UpdateKeys correctly updates the service's keys.
func TestUpdateKeys(t *testing.T) {
	// Generate two key pairs
	publicKey1, privateKey1, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate first key pair: %v", err)
	}
	publicKey2, privateKey2, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate second key pair: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create service with first keys
	svc, err := NewSOPSService(&Config{
		AgePublicKey:  publicKey1,
		AgePrivateKey: privateKey1,
	}, logger)
	if err != nil {
		t.Fatalf("failed to create SOPS service: %v", err)
	}

	// Verify initial public key
	if svc.GetPublicKey() != publicKey1 {
		t.Errorf("initial public key mismatch")
	}

	// Update to second keys
	if err := svc.UpdateKeys(publicKey2, privateKey2); err != nil {
		t.Fatalf("failed to update keys: %v", err)
	}

	// Verify updated public key
	if svc.GetPublicKey() != publicKey2 {
		t.Errorf("updated public key mismatch")
	}

	// Verify encryption/decryption works with new keys
	ctx := context.Background()
	plaintext := []byte("test secret")
	ciphertext, err := svc.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("encryption with new key failed: %v", err)
	}
	decrypted, err := svc.Decrypt(ctx, ciphertext)
	if err != nil {
		t.Fatalf("decryption with new key failed: %v", err)
	}
	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("round-trip with new keys failed")
	}
}
