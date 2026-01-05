// Package validation provides validation functions for various configuration types.
package validation

import (
	"strings"
	"time"
)

// LdflagsBuildContext contains build-time values for ldflags variable substitution.
// **Validates: Requirements 18.3**
type LdflagsBuildContext struct {
	Version   string    // Application version (e.g., "1.0.0", "v2.3.4")
	Commit    string    // Git commit hash (short or full)
	BuildTime time.Time // Build timestamp
}

// DefaultLdflagsBuildContext returns a build context with default values.
// This is useful when actual build context is not available.
func DefaultLdflagsBuildContext() LdflagsBuildContext {
	return LdflagsBuildContext{
		Version:   "0.0.0-dev",
		Commit:    "unknown",
		BuildTime: time.Now(),
	}
}

// SubstituteLdflagsVariables replaces placeholder variables in ldflags with actual values.
// Supported variables:
//   - ${version} - replaced with the application version
//   - ${commit} - replaced with the git commit hash
//   - ${buildTime} - replaced with the build timestamp in RFC3339 format
//
// **Validates: Requirements 18.3**
func SubstituteLdflagsVariables(ldflags string, ctx LdflagsBuildContext) string {
	if ldflags == "" {
		return ""
	}

	// Replace ${version}
	result := strings.ReplaceAll(ldflags, "${version}", ctx.Version)

	// Replace ${commit}
	result = strings.ReplaceAll(result, "${commit}", ctx.Commit)

	// Replace ${buildTime} with RFC3339 formatted timestamp
	buildTimeStr := ctx.BuildTime.UTC().Format(time.RFC3339)
	result = strings.ReplaceAll(result, "${buildTime}", buildTimeStr)

	return result
}

// HasLdflagsVariables checks if the ldflags string contains any substitution variables.
// Returns true if any of ${version}, ${commit}, or ${buildTime} are present.
func HasLdflagsVariables(ldflags string) bool {
	if ldflags == "" {
		return false
	}

	return strings.Contains(ldflags, "${version}") ||
		strings.Contains(ldflags, "${commit}") ||
		strings.Contains(ldflags, "${buildTime}")
}

// ValidateLdflags performs basic validation on ldflags string.
// Currently this is a permissive validation that allows most ldflags formats.
// Returns nil if valid, or an error describing the issue.
func ValidateLdflags(ldflags string) error {
	// Empty ldflags is valid (will use defaults)
	if ldflags == "" {
		return nil
	}

	// Basic sanity check - ldflags should not be excessively long
	// This prevents potential issues with command line length limits
	const maxLdflagsLength = 4096
	if len(ldflags) > maxLdflagsLength {
		return &ValidationError{
			Field:   "ldflags",
			Message: "ldflags string exceeds maximum length",
		}
	}

	return nil
}

// ValidationError represents a validation error with field context.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
