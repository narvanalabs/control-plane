// Package validation provides validation functions for various configuration types.
package validation

import (
	"regexp"
	"strings"

	"github.com/narvanalabs/control-plane/internal/models"
)

// buildTagRegex validates Go build tag format:
// - Must contain only alphanumeric characters, underscores, and dots
// - Cannot contain spaces or special characters
// - Cannot be empty
// Go build tags follow the format defined in: https://pkg.go.dev/cmd/go#hdr-Build_constraints
var buildTagRegex = regexp.MustCompile(`^[a-zA-Z0-9_\.]+$`)

// ValidateBuildTag validates a single Go build tag.
// Requirements: 17.5
//
// Build tag rules:
// - Cannot be empty
// - Cannot contain spaces
// - Can only contain alphanumeric characters, underscores, and dots
func ValidateBuildTag(tag string) error {
	if tag == "" {
		return &models.ValidationError{
			Field:   "build_tag",
			Message: "build tag cannot be empty",
		}
	}

	// Check for spaces
	if strings.Contains(tag, " ") {
		return &models.ValidationError{
			Field:   "build_tag",
			Message: "build tag cannot contain spaces",
		}
	}

	// Check for valid characters
	if !buildTagRegex.MatchString(tag) {
		return &models.ValidationError{
			Field:   "build_tag",
			Message: "build tag can only contain alphanumeric characters, underscores, and dots",
		}
	}

	return nil
}

// ValidateBuildTags validates a slice of Go build tags.
// Requirements: 17.5
//
// Returns an error if any tag is invalid.
func ValidateBuildTags(tags []string) error {
	if len(tags) == 0 {
		return nil // Empty tags list is valid
	}

	for i, tag := range tags {
		if err := ValidateBuildTag(tag); err != nil {
			validationErr, ok := err.(*models.ValidationError)
			if ok {
				// Add index to the field for better error reporting
				validationErr.Field = "build_tags[" + itoa(i) + "]"
				return validationErr
			}
			return err
		}
	}

	return nil
}

// FormatBuildTags formats build tags as a comma-separated string for the -tags flag.
// Requirements: 17.2, 17.3
//
// Returns an empty string if no tags are provided.
func FormatBuildTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return strings.Join(tags, ",")
}

// itoa converts an integer to a string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
