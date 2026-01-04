package validation

import (
	"fmt"
	"strings"

	"github.com/narvanalabs/control-plane/internal/models"
)

// SupportedDatabaseTypes defines the supported database types and their versions.
var SupportedDatabaseTypes = map[string][]string{
	"postgres": {"14", "15", "16"},
	"mysql":    {"8.0", "8.4"},
	"mariadb":  {"10.11", "11.4"},
	"mongodb":  {"6.0", "7.0"},
	"redis":    {"7.0", "7.2"},
	"sqlite":   {"3"},
}

// DefaultDatabaseVersions defines the default version for each database type.
var DefaultDatabaseVersions = map[string]string{
	"postgres": "16",
	"mysql":    "8.4",
	"mariadb":  "11.4",
	"mongodb":  "7.0",
	"redis":    "7.2",
	"sqlite":   "3",
}

// ValidateDatabaseConfig validates a database configuration.
// Requirements: 29.1, 29.2
func ValidateDatabaseConfig(config *models.DatabaseConfig) error {
	if config == nil {
		return &models.ValidationError{
			Field:   "database",
			Message: "database configuration is required",
		}
	}

	// Validate database type
	if config.Type == "" {
		return &models.ValidationError{
			Field:   "database.type",
			Message: "database type is required",
		}
	}

	supportedVersions, ok := SupportedDatabaseTypes[config.Type]
	if !ok {
		supportedTypes := getSupportedTypesList()
		return &models.ValidationError{
			Field:   "database.type",
			Message: fmt.Sprintf("unsupported database type '%s'; supported types are: %s", config.Type, supportedTypes),
		}
	}

	// If version is empty, it will use the default - that's valid
	if config.Version == "" {
		return nil
	}

	// Validate version is supported for this type
	if !isVersionSupported(config.Version, supportedVersions) {
		return &models.ValidationError{
			Field:   "database.version",
			Message: fmt.Sprintf("unsupported version '%s' for database type '%s'; supported versions are: %s", config.Version, config.Type, strings.Join(supportedVersions, ", ")),
		}
	}

	return nil
}

// getSupportedTypesList returns a comma-separated list of supported database types.
func getSupportedTypesList() string {
	types := make([]string, 0, len(SupportedDatabaseTypes))
	for t := range SupportedDatabaseTypes {
		types = append(types, t)
	}
	return strings.Join(types, ", ")
}

// isVersionSupported checks if a version is in the list of supported versions.
func isVersionSupported(version string, supportedVersions []string) bool {
	for _, v := range supportedVersions {
		if v == version {
			return true
		}
	}
	return false
}

// GetDefaultVersion returns the default version for a database type.
func GetDefaultVersion(dbType string) string {
	return DefaultDatabaseVersions[dbType]
}

// IsSupportedDatabaseType checks if a database type is supported.
func IsSupportedDatabaseType(dbType string) bool {
	_, ok := SupportedDatabaseTypes[dbType]
	return ok
}

// GetSupportedVersions returns the supported versions for a database type.
func GetSupportedVersions(dbType string) []string {
	return SupportedDatabaseTypes[dbType]
}
