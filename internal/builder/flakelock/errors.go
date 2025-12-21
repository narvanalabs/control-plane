// Package flakelock provides flake.lock file management for reproducible builds.
package flakelock

import "errors"

// Errors returned by the flakelock package.
var (
	// ErrEmptyFlakePath is returned when the flake path is empty.
	ErrEmptyFlakePath = errors.New("flake path is empty")

	// ErrFlakeNotFound is returned when flake.nix is not found.
	ErrFlakeNotFound = errors.New("flake.nix not found")

	// ErrLockGenerationFailed is returned when lock generation fails.
	ErrLockGenerationFailed = errors.New("failed to generate flake.lock")

	// ErrLockGenerationTimeout is returned when lock generation times out.
	ErrLockGenerationTimeout = errors.New("flake.lock generation timed out")

	// ErrLockUpdateFailed is returned when lock update fails.
	ErrLockUpdateFailed = errors.New("failed to update flake.lock")

	// ErrInvalidLockFormat is returned when the lock content is not valid JSON.
	ErrInvalidLockFormat = errors.New("invalid flake.lock format")

	// ErrEmptyBuildID is returned when the build ID is empty.
	ErrEmptyBuildID = errors.New("build ID is empty")

	// ErrEmptyLockContent is returned when the lock content is empty.
	ErrEmptyLockContent = errors.New("lock content is empty")

	// ErrLockNotFound is returned when a stored lock is not found.
	ErrLockNotFound = errors.New("flake.lock not found for build ID")
)
