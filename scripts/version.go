// Package scripts provides version extraction utilities for release management.
package scripts

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version represents a semantic version with major, minor, and patch components.
type Version struct {
	Major int
	Minor int
	Patch int
}

// String returns the version as a string in MAJOR.MINOR.PATCH format.
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// semverRegex matches semantic version patterns with optional 'v' prefix.
var semverRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

// ErrInvalidVersion is returned when a version string doesn't match semantic versioning format.
var ErrInvalidVersion = errors.New("invalid semantic version format")

// ExtractVersion extracts a semantic version from a git tag.
// It handles the 'v' prefix removal and validates the semantic version format.
// Valid inputs: "v1.2.3", "1.2.3", "v0.0.1"
// Invalid inputs: "v1.2", "1.2.3.4", "abc", ""
func ExtractVersion(tag string) (Version, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return Version{}, ErrInvalidVersion
	}

	matches := semverRegex.FindStringSubmatch(tag)
	if matches == nil {
		return Version{}, ErrInvalidVersion
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return Version{}, ErrInvalidVersion
	}

	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return Version{}, ErrInvalidVersion
	}

	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return Version{}, ErrInvalidVersion
	}

	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
	}, nil
}

// ExtractVersionString extracts a version string from a git tag without the 'v' prefix.
// Returns the version in MAJOR.MINOR.PATCH format.
func ExtractVersionString(tag string) (string, error) {
	v, err := ExtractVersion(tag)
	if err != nil {
		return "", err
	}
	return v.String(), nil
}

// IsValidSemver checks if a string is a valid semantic version (with or without 'v' prefix).
func IsValidSemver(tag string) bool {
	_, err := ExtractVersion(tag)
	return err == nil
}

// VersionToUnderscore converts a version string to underscore format for filenames.
// Example: "1.2.3" -> "1_2_3"
func VersionToUnderscore(version string) string {
	return strings.ReplaceAll(version, ".", "_")
}
