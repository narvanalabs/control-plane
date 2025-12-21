package entrypoint

import "errors"

var (
	// ErrEmptyRepoPath is returned when the repository path is empty.
	ErrEmptyRepoPath = errors.New("repository path is empty")

	// ErrRepoNotFound is returned when the repository path does not exist.
	ErrRepoNotFound = errors.New("repository not found")

	// ErrEmptyEntryPoint is returned when the entry point is empty.
	ErrEmptyEntryPoint = errors.New("entry point is empty")

	// ErrEntryPointNotFound is returned when the specified entry point does not exist.
	ErrEntryPointNotFound = errors.New("entry point not found")

	// ErrInvalidEntryPoint is returned when the entry point is not valid.
	ErrInvalidEntryPoint = errors.New("invalid entry point")

	// ErrNoEntryPointsFound is returned when no entry points are detected.
	ErrNoEntryPointsFound = errors.New("no entry points found")

	// ErrUnsupportedLanguage is returned when the language is not supported.
	ErrUnsupportedLanguage = errors.New("unsupported language")
)
