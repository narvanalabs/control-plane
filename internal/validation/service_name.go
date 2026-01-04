package validation

import (
	"regexp"

	"github.com/narvanalabs/control-plane/internal/models"
)

// dnsLabelRegex validates DNS label format:
// - Must start with a lowercase letter
// - Can contain lowercase letters, numbers, and hyphens
// - Must end with a lowercase letter or number
// - Single character names must be a letter
var dnsLabelRegex = regexp.MustCompile(`^[a-z]([a-z0-9-]*[a-z0-9])?$`)

// ValidateServiceName validates that a service name is a valid DNS label.
// Requirements: 10.1, 10.2, 10.3
//
// DNS label rules:
// - Must be 1-63 characters long
// - Must start with a lowercase letter
// - Can contain lowercase letters, numbers, and hyphens
// - Cannot start or end with a hyphen
func ValidateServiceName(name string) error {
	if name == "" {
		return &models.ValidationError{
			Field:   "name",
			Message: "service name is required",
		}
	}

	if len(name) > 63 {
		return &models.ValidationError{
			Field:   "name",
			Message: "service name must be 63 characters or less",
		}
	}

	// Check for leading hyphen
	if name[0] == '-' {
		return &models.ValidationError{
			Field:   "name",
			Message: "service name cannot start with a hyphen",
		}
	}

	// Check for trailing hyphen
	if name[len(name)-1] == '-' {
		return &models.ValidationError{
			Field:   "name",
			Message: "service name cannot end with a hyphen",
		}
	}

	// Check DNS label format
	if !dnsLabelRegex.MatchString(name) {
		return &models.ValidationError{
			Field:   "name",
			Message: "service name must be a valid DNS label (lowercase letters, numbers, and hyphens, starting with a letter)",
		}
	}

	return nil
}
