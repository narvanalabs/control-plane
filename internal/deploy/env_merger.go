// Package deploy provides deployment-related utilities for the control plane.
package deploy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/narvanalabs/control-plane/internal/secrets"
	"github.com/narvanalabs/control-plane/internal/store"
)

// EnvMerger merges app-level secrets with service-level environment variables.
// Service-level variables take precedence over app-level secrets when both have the same key.
// **Validates: Requirements 3.2, 6.1, 6.3**
type EnvMerger struct {
	store       store.Store
	sopsService *secrets.SOPSService
	logger      *slog.Logger
}

// NewEnvMerger creates a new EnvMerger instance.
func NewEnvMerger(st store.Store, sopsService *secrets.SOPSService, logger *slog.Logger) *EnvMerger {
	if logger == nil {
		logger = slog.Default()
	}
	return &EnvMerger{
		store:       st,
		sopsService: sopsService,
		logger:      logger,
	}
}

// MergeForDeployment fetches app-level secrets (decrypted) and service-level env vars,
// then merges them with service-level taking precedence.
// **Validates: Requirements 6.1, 6.3**
func (m *EnvMerger) MergeForDeployment(ctx context.Context, appID, serviceName string, serviceEnvVars map[string]string) (map[string]string, error) {
	m.logger.Debug("merging environment variables for deployment",
		"app_id", appID,
		"service_name", serviceName,
	)

	// Start with app-level secrets (decrypted)
	appSecrets, err := m.getDecryptedAppSecrets(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("getting app secrets: %w", err)
	}

	// Merge with service-level env vars taking precedence
	merged := MergeEnvVars(appSecrets, serviceEnvVars)

	m.logger.Debug("environment variables merged",
		"app_id", appID,
		"service_name", serviceName,
		"app_secrets_count", len(appSecrets),
		"service_env_vars_count", len(serviceEnvVars),
		"merged_count", len(merged),
	)

	return merged, nil
}

// getDecryptedAppSecrets fetches and decrypts all app-level secrets.
func (m *EnvMerger) getDecryptedAppSecrets(ctx context.Context, appID string) (map[string]string, error) {
	// Get all encrypted secrets for the app
	encryptedSecrets, err := m.store.Secrets().GetAll(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("fetching secrets: %w", err)
	}

	decrypted := make(map[string]string, len(encryptedSecrets))

	for key, encryptedValue := range encryptedSecrets {
		var value string

		// Decrypt the value if SOPS is configured and can decrypt
		if m.sopsService != nil && m.sopsService.CanDecrypt() {
			decryptedBytes, err := m.sopsService.Decrypt(ctx, encryptedValue)
			if err != nil {
				m.logger.Warn("failed to decrypt secret, using as-is",
					"key", key,
					"error", err,
				)
				value = string(encryptedValue)
			} else {
				value = string(decryptedBytes)
			}
		} else {
			// No encryption configured or no private key, value is stored as plaintext
			value = string(encryptedValue)
		}

		decrypted[key] = value
	}

	return decrypted, nil
}

// MergeEnvVars merges two maps of environment variables.
// The second map (serviceEnvVars) takes precedence over the first (appSecrets).
// This is a pure function for easy testing.
// **Validates: Requirements 3.2, 6.1, 6.3**
func MergeEnvVars(appSecrets, serviceEnvVars map[string]string) map[string]string {
	// Start with app-level secrets
	merged := make(map[string]string, len(appSecrets)+len(serviceEnvVars))

	for k, v := range appSecrets {
		merged[k] = v
	}

	// Override with service-level env vars
	for k, v := range serviceEnvVars {
		merged[k] = v
	}

	return merged
}
