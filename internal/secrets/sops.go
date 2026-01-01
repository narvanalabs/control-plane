// Package secrets provides SOPS-Nix compatible secrets management using age encryption.
// This implements the secrets management as per https://github.com/Mic92/sops-nix
package secrets

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"filippo.io/age"
)

var (
	// ErrNoPublicKey is returned when no public key is configured for encryption.
	ErrNoPublicKey = errors.New("no public key configured for encryption")
	// ErrNoPrivateKey is returned when no private key is configured for decryption.
	ErrNoPrivateKey = errors.New("no private key configured for decryption")
	// ErrDecryptionFailed is returned when decryption fails.
	ErrDecryptionFailed = errors.New("decryption failed")
	// ErrEncryptionFailed is returned when encryption fails.
	ErrEncryptionFailed = errors.New("encryption failed")
	// ErrInvalidKey is returned when a key is invalid.
	ErrInvalidKey = errors.New("invalid key format")
)

// SOPSService provides SOPS-compatible encryption and decryption using age.
// It follows the sops-nix pattern where secrets are encrypted with age public keys
// and can only be decrypted by nodes with the corresponding private keys.
type SOPSService struct {
	publicKey    *age.X25519Recipient // For encryption (API server)
	privateKey   *age.X25519Identity  // For decryption (node agents only)
	logger       *slog.Logger
}

// Config holds the configuration for the SOPS service.
type Config struct {
	// AgePublicKey is the age public key for encryption (required for API server).
	// Format: age1... (Bech32 encoded)
	AgePublicKey string
	// AgePrivateKey is the age private key for decryption (required for node agents).
	// Format: AGE-SECRET-KEY-1... (Bech32 encoded)
	AgePrivateKey string
}

// NewSOPSService creates a new SOPS service with the given configuration.
// At least one of public key (for encryption) or private key (for decryption) must be provided.
func NewSOPSService(cfg *Config, logger *slog.Logger) (*SOPSService, error) {
	if logger == nil {
		logger = slog.Default()
	}

	svc := &SOPSService{
		logger: logger,
	}

	// Parse public key if provided
	if cfg.AgePublicKey != "" {
		recipient, err := age.ParseX25519Recipient(cfg.AgePublicKey)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid public key: %v", ErrInvalidKey, err)
		}
		svc.publicKey = recipient
	}

	// Parse private key if provided
	if cfg.AgePrivateKey != "" {
		identity, err := age.ParseX25519Identity(cfg.AgePrivateKey)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid private key: %v", ErrInvalidKey, err)
		}
		svc.privateKey = identity
	}

	return svc, nil
}

// Encrypt encrypts plaintext using age encryption with the configured public key.
// The output is the age-encrypted ciphertext that can only be decrypted with
// the corresponding private key.
func (s *SOPSService) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	if s.publicKey == nil {
		return nil, ErrNoPublicKey
	}

	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, s.publicKey)
	if err != nil {
		s.logger.Error("failed to create age encryptor", "error", err)
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}

	if _, err := w.Write(plaintext); err != nil {
		s.logger.Error("failed to write plaintext to encryptor", "error", err)
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}

	if err := w.Close(); err != nil {
		s.logger.Error("failed to close encryptor", "error", err)
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}

	return buf.Bytes(), nil
}

// Decrypt decrypts age-encrypted ciphertext using the configured private key.
// This is typically only available on node agents that have the private key.
func (s *SOPSService) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	if s.privateKey == nil {
		return nil, ErrNoPrivateKey
	}

	r, err := age.Decrypt(bytes.NewReader(ciphertext), s.privateKey)
	if err != nil {
		s.logger.Error("failed to create age decryptor", "error", err)
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	plaintext, err := io.ReadAll(r)
	if err != nil {
		s.logger.Error("failed to read decrypted data", "error", err)
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return plaintext, nil
}

// CanEncrypt returns true if the service is configured for encryption.
func (s *SOPSService) CanEncrypt() bool {
	return s.publicKey != nil
}

// CanDecrypt returns true if the service is configured for decryption.
func (s *SOPSService) CanDecrypt() bool {
	return s.privateKey != nil
}

// GenerateKeyPair generates a new age key pair for use with SOPS.
// Returns the public key (for encryption) and private key (for decryption).
func GenerateKeyPair() (publicKey, privateKey string, err error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate age key pair: %w", err)
	}

	return identity.Recipient().String(), identity.String(), nil
}

// GenerateRandomBytes generates cryptographically secure random bytes.
// Useful for generating secrets like database passwords.
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return b, nil
}

// GetPublicKey returns the configured public key string, or empty if not configured.
func (s *SOPSService) GetPublicKey() string {
	if s.publicKey == nil {
		return ""
	}
	return s.publicKey.String()
}


// ReEncrypt decrypts ciphertext with the current private key and re-encrypts
// with a new public key. This is used during key rotation.
func (s *SOPSService) ReEncrypt(ctx context.Context, ciphertext []byte, newPublicKey *age.X25519Recipient) ([]byte, error) {
	// Decrypt with current private key
	plaintext, err := s.Decrypt(ctx, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt for re-encryption: %w", err)
	}

	// Encrypt with new public key
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, newPublicKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}

	if _, err := w.Write(plaintext); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}

	return buf.Bytes(), nil
}

// KeyRotationResult contains the result of a key rotation operation.
type KeyRotationResult struct {
	// NewPublicKey is the new public key to use for encryption.
	NewPublicKey string
	// NewPrivateKey is the new private key to use for decryption.
	NewPrivateKey string
	// RotatedSecrets is a map of app_id -> key -> new_encrypted_value.
	RotatedSecrets map[string]map[string][]byte
	// FailedSecrets is a map of app_id -> key -> error message for secrets that failed to rotate.
	FailedSecrets map[string]map[string]string
}

// RotateKeys generates a new key pair and re-encrypts all provided secrets.
// This function requires both encryption (public key) and decryption (private key) capabilities.
// The caller is responsible for persisting the new keys and updated secrets.
//
// Parameters:
//   - secrets: map of app_id -> key -> encrypted_value
//
// Returns:
//   - KeyRotationResult containing the new keys and re-encrypted secrets
//   - error if key generation fails or if the service cannot decrypt
func (s *SOPSService) RotateKeys(ctx context.Context, secrets map[string]map[string][]byte) (*KeyRotationResult, error) {
	if !s.CanDecrypt() {
		return nil, ErrNoPrivateKey
	}

	// Generate new key pair
	newIdentity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("failed to generate new key pair: %w", err)
	}

	newPublicKey := newIdentity.Recipient()

	result := &KeyRotationResult{
		NewPublicKey:   newPublicKey.String(),
		NewPrivateKey:  newIdentity.String(),
		RotatedSecrets: make(map[string]map[string][]byte),
		FailedSecrets:  make(map[string]map[string]string),
	}

	// Re-encrypt all secrets with the new key
	for appID, appSecrets := range secrets {
		result.RotatedSecrets[appID] = make(map[string][]byte)
		for key, ciphertext := range appSecrets {
			newCiphertext, err := s.ReEncrypt(ctx, ciphertext, newPublicKey)
			if err != nil {
				s.logger.Error("failed to rotate secret",
					"app_id", appID,
					"key", key,
					"error", err,
				)
				if result.FailedSecrets[appID] == nil {
					result.FailedSecrets[appID] = make(map[string]string)
				}
				result.FailedSecrets[appID][key] = err.Error()
				continue
			}
			result.RotatedSecrets[appID][key] = newCiphertext
		}
	}

	s.logger.Info("key rotation completed",
		"total_apps", len(secrets),
		"failed_secrets", len(result.FailedSecrets),
	)

	return result, nil
}

// UpdateKeys updates the service's keys to use a new key pair.
// This should be called after key rotation to update the service configuration.
func (s *SOPSService) UpdateKeys(publicKey, privateKey string) error {
	if publicKey != "" {
		recipient, err := age.ParseX25519Recipient(publicKey)
		if err != nil {
			return fmt.Errorf("%w: invalid public key: %v", ErrInvalidKey, err)
		}
		s.publicKey = recipient
	}

	if privateKey != "" {
		identity, err := age.ParseX25519Identity(privateKey)
		if err != nil {
			return fmt.Errorf("%w: invalid private key: %v", ErrInvalidKey, err)
		}
		s.privateKey = identity
	}

	return nil
}
