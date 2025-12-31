package databases

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: platform-enhancements, Property 5: Database Service Build Type**
// For any database service (PostgreSQL, MySQL, MariaDB, MongoDB, Redis),
// the build_type SHALL always be "pure-nix", never "oci".
// **Validates: Requirements 11.2**

// genDatabaseType generates a random valid database type.
func genDatabaseType() gopter.Gen {
	return gen.OneConstOf(
		DatabaseTypePostgres,
		DatabaseTypeMySQL,
		DatabaseTypeMariaDB,
		DatabaseTypeMongoDB,
		DatabaseTypeRedis,
	)
}

// TestDatabaseServiceBuildType tests Property 5: Database Service Build Type.
func TestDatabaseServiceBuildType(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: GetBuildType always returns pure-nix for database services
	properties.Property("Database services always use pure-nix build type", prop.ForAll(
		func(dbType DatabaseType) bool {
			// The GetBuildType function should always return pure-nix
			buildType := GetBuildType()
			return buildType == models.BuildTypePureNix
		},
		genDatabaseType(),
	))

	// Property: Database build type is never OCI
	properties.Property("Database build type is never OCI", prop.ForAll(
		func(dbType DatabaseType) bool {
			buildType := GetBuildType()
			return buildType != models.BuildTypeOCI
		},
		genDatabaseType(),
	))

	// Property: All valid database types are recognized
	properties.Property("All valid database types are recognized", prop.ForAll(
		func(dbType DatabaseType) bool {
			return dbType.IsValid()
		},
		genDatabaseType(),
	))

	// Property: All database types have templates in registry
	properties.Property("All database types have templates in registry", prop.ForAll(
		func(dbType DatabaseType) bool {
			template, err := GetTemplate(dbType)
			return err == nil && template != nil
		},
		genDatabaseType(),
	))

	properties.TestingRun(t)
}


// **Feature: platform-enhancements, Property 6: Database Template Selection**
// For any database type, the correct Database_Flake template SHALL be selected
// with the appropriate default version:
// - postgres → PostgreSQL flake, default version 16
// - mysql → MySQL flake, default version 8.0
// - mariadb → MariaDB flake, default version 11
// - mongodb → MongoDB flake, default version 7.0
// - redis → Redis flake, default version 7
// **Validates: Requirements 11.3, 11.4, 11.5, 11.6, 11.7**

// expectedDefaultVersion returns the expected default version for a database type.
func expectedDefaultVersion(dbType DatabaseType) string {
	switch dbType {
	case DatabaseTypePostgres:
		return "16"
	case DatabaseTypeMySQL:
		return "8.0"
	case DatabaseTypeMariaDB:
		return "11"
	case DatabaseTypeMongoDB:
		return "7.0"
	case DatabaseTypeRedis:
		return "7"
	default:
		return ""
	}
}

// expectedTemplateName returns the expected template name for a database type.
func expectedTemplateName(dbType DatabaseType) string {
	switch dbType {
	case DatabaseTypePostgres:
		return "postgres.nix"
	case DatabaseTypeMySQL:
		return "mysql.nix"
	case DatabaseTypeMariaDB:
		return "mariadb.nix"
	case DatabaseTypeMongoDB:
		return "mongodb.nix"
	case DatabaseTypeRedis:
		return "redis.nix"
	default:
		return ""
	}
}

// TestDatabaseTemplateSelection tests Property 6: Database Template Selection.
func TestDatabaseTemplateSelection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Each database type has the correct default version
	properties.Property("Database type has correct default version", prop.ForAll(
		func(dbType DatabaseType) bool {
			expected := expectedDefaultVersion(dbType)
			actual := GetDefaultVersion(dbType)
			return expected == actual
		},
		genDatabaseType(),
	))

	// Property: Each database type has the correct template name
	properties.Property("Database type has correct template name", prop.ForAll(
		func(dbType DatabaseType) bool {
			expected := expectedTemplateName(dbType)
			actual := GetTemplateName(dbType)
			return expected == actual
		},
		genDatabaseType(),
	))

	// Property: PostgreSQL has correct configuration
	properties.Property("PostgreSQL has correct default version 16", prop.ForAll(
		func(_ int) bool {
			template, err := GetTemplate(DatabaseTypePostgres)
			if err != nil {
				return false
			}
			return template.DefaultVersion == "16" &&
				template.TemplateName == "postgres.nix" &&
				template.DefaultPort == 5432
		},
		gen.IntRange(0, 100),
	))

	// Property: MySQL has correct configuration
	properties.Property("MySQL has correct default version 8.0", prop.ForAll(
		func(_ int) bool {
			template, err := GetTemplate(DatabaseTypeMySQL)
			if err != nil {
				return false
			}
			return template.DefaultVersion == "8.0" &&
				template.TemplateName == "mysql.nix" &&
				template.DefaultPort == 3306
		},
		gen.IntRange(0, 100),
	))

	// Property: MariaDB has correct configuration
	properties.Property("MariaDB has correct default version 11", prop.ForAll(
		func(_ int) bool {
			template, err := GetTemplate(DatabaseTypeMariaDB)
			if err != nil {
				return false
			}
			return template.DefaultVersion == "11" &&
				template.TemplateName == "mariadb.nix" &&
				template.DefaultPort == 3306
		},
		gen.IntRange(0, 100),
	))

	// Property: MongoDB has correct configuration
	properties.Property("MongoDB has correct default version 7.0", prop.ForAll(
		func(_ int) bool {
			template, err := GetTemplate(DatabaseTypeMongoDB)
			if err != nil {
				return false
			}
			return template.DefaultVersion == "7.0" &&
				template.TemplateName == "mongodb.nix" &&
				template.DefaultPort == 27017
		},
		gen.IntRange(0, 100),
	))

	// Property: Redis has correct configuration
	properties.Property("Redis has correct default version 7", prop.ForAll(
		func(_ int) bool {
			template, err := GetTemplate(DatabaseTypeRedis)
			if err != nil {
				return false
			}
			return template.DefaultVersion == "7" &&
				template.TemplateName == "redis.nix" &&
				template.DefaultPort == 6379
		},
		gen.IntRange(0, 100),
	))

	// Property: Default version is always in available versions
	properties.Property("Default version is in available versions", prop.ForAll(
		func(dbType DatabaseType) bool {
			template, err := GetTemplate(dbType)
			if err != nil {
				return false
			}
			for _, v := range template.AvailableVersions {
				if v == template.DefaultVersion {
					return true
				}
			}
			return false
		},
		genDatabaseType(),
	))

	// Property: IsValidVersion returns true for default version
	properties.Property("IsValidVersion returns true for default version", prop.ForAll(
		func(dbType DatabaseType) bool {
			defaultVersion := GetDefaultVersion(dbType)
			return IsValidVersion(dbType, defaultVersion)
		},
		genDatabaseType(),
	))

	// Property: IsValidVersion returns false for invalid version
	properties.Property("IsValidVersion returns false for invalid version", prop.ForAll(
		func(dbType DatabaseType) bool {
			return !IsValidVersion(dbType, "invalid-version-999")
		},
		genDatabaseType(),
	))

	// Property: All database types have config options
	properties.Property("All database types have config options", prop.ForAll(
		func(dbType DatabaseType) bool {
			options := GetConfigOptions(dbType)
			return len(options) > 0
		},
		genDatabaseType(),
	))

	// Property: GetDefaultConfig returns non-empty config
	properties.Property("GetDefaultConfig returns non-empty config", prop.ForAll(
		func(dbType DatabaseType) bool {
			config := GetDefaultConfig(dbType)
			return len(config) > 0
		},
		genDatabaseType(),
	))

	properties.TestingRun(t)
}
