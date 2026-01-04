package validation

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: backend-source-of-truth, Property 20: Database Type and Version Validation**
// For any database service creation, the database type SHALL be one of the supported types,
// and the version SHALL be one of the supported versions for that type.
// **Validates: Requirements 29.1, 29.2**

// genSupportedDatabaseType generates a supported database type.
func genSupportedDatabaseType() gopter.Gen {
	return gen.OneConstOf("postgres", "mysql", "mariadb", "mongodb", "redis", "sqlite")
}

// genUnsupportedDatabaseType generates an unsupported database type.
func genUnsupportedDatabaseType() gopter.Gen {
	return gen.OneConstOf(
		"oracle",
		"sqlserver",
		"cassandra",
		"couchdb",
		"neo4j",
		"dynamodb",
		"firestore",
		"unknown",
		"",
	).SuchThat(func(s string) bool {
		// Ensure it's not accidentally a supported type
		_, ok := SupportedDatabaseTypes[s]
		return !ok
	})
}

// genValidVersionForType generates a valid version for a given database type.
func genValidVersionForType(dbType string) gopter.Gen {
	versions := SupportedDatabaseTypes[dbType]
	if len(versions) == 0 {
		return gen.Const("")
	}
	gens := make([]gopter.Gen, len(versions))
	for i, v := range versions {
		gens[i] = gen.Const(v)
	}
	return gen.OneGenOf(gens...)
}

// genInvalidVersionForType generates an invalid version for a given database type.
func genInvalidVersionForType(dbType string) gopter.Gen {
	return gen.OneConstOf(
		"0.0",
		"99.99",
		"invalid",
		"latest",
		"1.0",
		"2.0",
	).SuchThat(func(v string) bool {
		// Ensure it's not accidentally a valid version
		versions := SupportedDatabaseTypes[dbType]
		for _, valid := range versions {
			if valid == v {
				return false
			}
		}
		return true
	})
}

// TestDatabaseTypeAndVersionValidation tests Property 20: Database Type and Version Validation.
func TestDatabaseTypeAndVersionValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 20.1: Supported database types are accepted
	properties.Property("supported database types are accepted", prop.ForAll(
		func(dbType string) bool {
			config := &models.DatabaseConfig{
				Type:    dbType,
				Version: "", // Empty version uses default
			}
			err := ValidateDatabaseConfig(config)
			return err == nil
		},
		genSupportedDatabaseType(),
	))

	// Property 20.2: Unsupported database types are rejected
	properties.Property("unsupported database types are rejected", prop.ForAll(
		func(dbType string) bool {
			if dbType == "" {
				// Empty type has a different error message
				config := &models.DatabaseConfig{
					Type:    dbType,
					Version: "",
				}
				err := ValidateDatabaseConfig(config)
				if err == nil {
					return false
				}
				validationErr, ok := err.(*models.ValidationError)
				if !ok {
					return false
				}
				return validationErr.Field == "database.type" &&
					containsSubstring(validationErr.Message, "required")
			}

			config := &models.DatabaseConfig{
				Type:    dbType,
				Version: "",
			}
			err := ValidateDatabaseConfig(config)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "database.type" &&
				containsSubstring(validationErr.Message, "unsupported database type")
		},
		genUnsupportedDatabaseType(),
	))

	// Property 20.3: Valid versions for each type are accepted
	properties.Property("valid versions are accepted for postgres", prop.ForAll(
		func(version string) bool {
			config := &models.DatabaseConfig{
				Type:    "postgres",
				Version: version,
			}
			err := ValidateDatabaseConfig(config)
			return err == nil
		},
		genValidVersionForType("postgres"),
	))

	properties.Property("valid versions are accepted for mysql", prop.ForAll(
		func(version string) bool {
			config := &models.DatabaseConfig{
				Type:    "mysql",
				Version: version,
			}
			err := ValidateDatabaseConfig(config)
			return err == nil
		},
		genValidVersionForType("mysql"),
	))

	properties.Property("valid versions are accepted for redis", prop.ForAll(
		func(version string) bool {
			config := &models.DatabaseConfig{
				Type:    "redis",
				Version: version,
			}
			err := ValidateDatabaseConfig(config)
			return err == nil
		},
		genValidVersionForType("redis"),
	))

	// Property 20.4: Invalid versions are rejected
	properties.Property("invalid versions are rejected for postgres", prop.ForAll(
		func(version string) bool {
			config := &models.DatabaseConfig{
				Type:    "postgres",
				Version: version,
			}
			err := ValidateDatabaseConfig(config)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "database.version" &&
				containsSubstring(validationErr.Message, "unsupported version")
		},
		genInvalidVersionForType("postgres"),
	))

	properties.Property("invalid versions are rejected for mysql", prop.ForAll(
		func(version string) bool {
			config := &models.DatabaseConfig{
				Type:    "mysql",
				Version: version,
			}
			err := ValidateDatabaseConfig(config)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "database.version" &&
				containsSubstring(validationErr.Message, "unsupported version")
		},
		genInvalidVersionForType("mysql"),
	))

	// Property 20.5: Empty version is valid (uses default)
	properties.Property("empty version uses default", prop.ForAll(
		func(dbType string) bool {
			config := &models.DatabaseConfig{
				Type:    dbType,
				Version: "",
			}
			err := ValidateDatabaseConfig(config)
			return err == nil
		},
		genSupportedDatabaseType(),
	))

	// Property 20.6: Nil config is rejected
	properties.Property("nil config is rejected", prop.ForAll(
		func(_ int) bool {
			err := ValidateDatabaseConfig(nil)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "database" &&
				containsSubstring(validationErr.Message, "required")
		},
		gen.IntRange(0, 1),
	))

	// Property 20.7: IsSupportedDatabaseType returns true for supported types
	properties.Property("IsSupportedDatabaseType returns true for supported types", prop.ForAll(
		func(dbType string) bool {
			return IsSupportedDatabaseType(dbType)
		},
		genSupportedDatabaseType(),
	))

	// Property 20.8: IsSupportedDatabaseType returns false for unsupported types
	properties.Property("IsSupportedDatabaseType returns false for unsupported types", prop.ForAll(
		func(dbType string) bool {
			return !IsSupportedDatabaseType(dbType)
		},
		genUnsupportedDatabaseType(),
	))

	// Property 20.9: GetDefaultVersion returns non-empty for supported types
	properties.Property("GetDefaultVersion returns non-empty for supported types", prop.ForAll(
		func(dbType string) bool {
			version := GetDefaultVersion(dbType)
			return version != ""
		},
		genSupportedDatabaseType(),
	))

	// Property 20.10: GetSupportedVersions returns non-empty for supported types
	properties.Property("GetSupportedVersions returns non-empty for supported types", prop.ForAll(
		func(dbType string) bool {
			versions := GetSupportedVersions(dbType)
			return len(versions) > 0
		},
		genSupportedDatabaseType(),
	))

	properties.TestingRun(t)
}
