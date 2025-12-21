package retry

import "errors"

// Retry manager errors.
var (
	// ErrMaxRetriesExceeded is returned when the maximum number of retries has been exceeded.
	ErrMaxRetriesExceeded = errors.New("maximum retry attempts exceeded")

	// ErrNonRetryableError is returned when an error is not retryable.
	ErrNonRetryableError = errors.New("error is not retryable")

	// ErrInvalidJob is returned when a job is invalid for retry.
	ErrInvalidJob = errors.New("invalid job for retry")

	// ErrBuildIDRequired is returned when a build ID is required but not provided.
	ErrBuildIDRequired = errors.New("build ID is required")
)
