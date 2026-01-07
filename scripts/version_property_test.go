package main

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: release-changelog-cicd, Property 1: Version Extraction from Tags**
// For any valid semantic version tag (v*.*.*), extracting the version number
// SHALL produce a string matching the pattern MAJOR.MINOR.PATCH without the leading 'v'.
// **Validates: Requirements 3.2**

// genValidVersionComponents generates valid version components (non-negative integers).
func genValidVersionComponents() gopter.Gen {
	return gen.IntRange(0, 999)
}

// genValidSemverTag generates a valid semantic version tag with 'v' prefix.
func genValidSemverTag() gopter.Gen {
	return gen.Struct(reflect.TypeOf(struct {
		Major int
		Minor int
		Patch int
	}{}), map[string]gopter.Gen{
		"Major": genValidVersionComponents(),
		"Minor": genValidVersionComponents(),
		"Patch": genValidVersionComponents(),
	}).Map(func(v interface{}) string {
		s := v.(struct {
			Major int
			Minor int
			Patch int
		})
		return fmt.Sprintf("v%d.%d.%d", s.Major, s.Minor, s.Patch)
	})
}

// genValidSemverTagWithoutPrefix generates a valid semantic version tag without 'v' prefix.
func genValidSemverTagWithoutPrefix() gopter.Gen {
	return gen.Struct(reflect.TypeOf(struct {
		Major int
		Minor int
		Patch int
	}{}), map[string]gopter.Gen{
		"Major": genValidVersionComponents(),
		"Minor": genValidVersionComponents(),
		"Patch": genValidVersionComponents(),
	}).Map(func(v interface{}) string {
		s := v.(struct {
			Major int
			Minor int
			Patch int
		})
		return fmt.Sprintf("%d.%d.%d", s.Major, s.Minor, s.Patch)
	})
}

// genInvalidSemverTag generates invalid semantic version tags.
func genInvalidSemverTag() gopter.Gen {
	return gen.OneGenOf(
		// Empty string
		gen.Const(""),
		// Only 'v'
		gen.Const("v"),
		// Missing patch version
		gen.IntRange(0, 99).Map(func(major int) string {
			return fmt.Sprintf("v%d.%d", major, major+1)
		}),
		// Extra version component
		gen.IntRange(0, 99).Map(func(n int) string {
			return fmt.Sprintf("v%d.%d.%d.%d", n, n+1, n+2, n+3)
		}),
		// Non-numeric version
		gen.AlphaString().Map(func(s string) string {
			if s == "" {
				return "abc"
			}
			return s
		}),
		// Negative numbers (represented as strings)
		gen.Const("v-1.0.0"),
		gen.Const("v1.-1.0"),
		gen.Const("v1.0.-1"),
	)
}

func TestPropertyVersionExtractionFromTags(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 1.1: Valid tags with 'v' prefix produce correct version string
	properties.Property("valid tags with v prefix produce correct version string", prop.ForAll(
		func(major, minor, patch int) bool {
			tag := fmt.Sprintf("v%d.%d.%d", major, minor, patch)
			expected := fmt.Sprintf("%d.%d.%d", major, minor, patch)

			result, err := ExtractVersionString(tag)
			if err != nil {
				return false
			}
			return result == expected
		},
		genValidVersionComponents(),
		genValidVersionComponents(),
		genValidVersionComponents(),
	))

	// Property 1.2: Valid tags without 'v' prefix produce correct version string
	properties.Property("valid tags without v prefix produce correct version string", prop.ForAll(
		func(major, minor, patch int) bool {
			tag := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			expected := fmt.Sprintf("%d.%d.%d", major, minor, patch)

			result, err := ExtractVersionString(tag)
			if err != nil {
				return false
			}
			return result == expected
		},
		genValidVersionComponents(),
		genValidVersionComponents(),
		genValidVersionComponents(),
	))

	// Property 1.3: Extracted version has no 'v' prefix
	properties.Property("extracted version has no v prefix", prop.ForAll(
		func(major, minor, patch int) bool {
			tag := fmt.Sprintf("v%d.%d.%d", major, minor, patch)

			result, err := ExtractVersionString(tag)
			if err != nil {
				return false
			}
			return len(result) > 0 && result[0] != 'v'
		},
		genValidVersionComponents(),
		genValidVersionComponents(),
		genValidVersionComponents(),
	))

	// Property 1.4: Version components are preserved correctly
	properties.Property("version components are preserved correctly", prop.ForAll(
		func(major, minor, patch int) bool {
			tag := fmt.Sprintf("v%d.%d.%d", major, minor, patch)

			v, err := ExtractVersion(tag)
			if err != nil {
				return false
			}
			return v.Major == major && v.Minor == minor && v.Patch == patch
		},
		genValidVersionComponents(),
		genValidVersionComponents(),
		genValidVersionComponents(),
	))

	// Property 1.5: Invalid tags return error
	properties.Property("invalid tags return error", prop.ForAll(
		func(tag string) bool {
			_, err := ExtractVersion(tag)
			return err != nil
		},
		genInvalidSemverTag(),
	))

	// Property 1.6: IsValidSemver returns true for valid tags
	properties.Property("IsValidSemver returns true for valid tags", prop.ForAll(
		func(major, minor, patch int) bool {
			tag := fmt.Sprintf("v%d.%d.%d", major, minor, patch)
			return IsValidSemver(tag)
		},
		genValidVersionComponents(),
		genValidVersionComponents(),
		genValidVersionComponents(),
	))

	// Property 1.7: IsValidSemver returns false for invalid tags
	properties.Property("IsValidSemver returns false for invalid tags", prop.ForAll(
		func(tag string) bool {
			return !IsValidSemver(tag)
		},
		genInvalidSemverTag(),
	))

	// Property 1.8: Version.String() produces MAJOR.MINOR.PATCH format
	properties.Property("Version.String produces MAJOR.MINOR.PATCH format", prop.ForAll(
		func(major, minor, patch int) bool {
			v := Version{Major: major, Minor: minor, Patch: patch}
			expected := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			return v.String() == expected
		},
		genValidVersionComponents(),
		genValidVersionComponents(),
		genValidVersionComponents(),
	))

	// Property 1.9: VersionToUnderscore replaces dots with underscores
	properties.Property("VersionToUnderscore replaces dots with underscores", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			expected := fmt.Sprintf("%d_%d_%d", major, minor, patch)
			return VersionToUnderscore(version) == expected
		},
		genValidVersionComponents(),
		genValidVersionComponents(),
		genValidVersionComponents(),
	))

	properties.TestingRun(t)
}
