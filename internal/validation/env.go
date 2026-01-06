package validation

import (
	"regexp"

	"github.com/narvanalabs/control-plane/internal/models"
)

// envKeyRegex validates environment variable key format:
// - Must start with a letter or underscore
// - Can contain letters, numbers, and underscores
// - Must not be empty
var envKeyRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// MaxEnvKeyLength is the maximum allowed length for an environment variable key.
const MaxEnvKeyLength = 256

// MaxEnvValueLength is the maximum allowed length for an environment variable value (32KB).
const MaxEnvValueLength = 32 * 1024

// ValidateEnvKey validates that an environment variable key is valid.
// Requirements: 5.1, 5.2
//
// Environment variable key rules:
// - Must not be empty
// - Must start with a letter (A-Z, a-z) or underscore (_)
// - Can contain letters, numbers, and underscores
// - Must not exceed 256 characters
func ValidateEnvKey(key string) error {
	if key == "" {
		return &models.ValidationError{
			Field:   "key",
			Message: "environment variable key is required",
		}
	}

	if len(key) > MaxEnvKeyLength {
		return &models.ValidationError{
			Field:   "key",
			Message: "environment variable key must be 256 characters or less",
		}
	}

	if !envKeyRegex.MatchString(key) {
		return &models.ValidationError{
			Field:   "key",
			Message: "environment variable key must start with a letter or underscore and contain only letters, numbers, and underscores",
		}
	}

	return nil
}

// ValidateEnvValue validates that an environment variable value is valid.
// Requirements: 5.1, 5.3
//
// Environment variable value rules:
// - Must not exceed 32KB
func ValidateEnvValue(value string) error {
	if len(value) > MaxEnvValueLength {
		return &models.ValidationError{
			Field:   "value",
			Message: "environment variable value must be 32KB or less",
		}
	}

	return nil
}
