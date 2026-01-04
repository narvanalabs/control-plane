package postgres

import (
	"errors"
	"strings"
)

// Common store errors.
var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = errors.New("resource not found")

	// ErrDuplicateName is returned when attempting to create a resource with a duplicate name.
	ErrDuplicateName = errors.New("duplicate name")

	// ErrDuplicateKey is returned when attempting to create a resource with a duplicate key.
	ErrDuplicateKey = errors.New("duplicate key")

	// ErrConcurrentModification is returned when an optimistic locking conflict is detected.
	// This occurs when the version field doesn't match during an update operation.
	ErrConcurrentModification = errors.New("resource was modified by another request")
)

// isUniqueViolation checks if the error is a PostgreSQL unique constraint violation.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL error code 23505 is unique_violation
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "unique constraint") ||
		strings.Contains(err.Error(), "duplicate key")
}
