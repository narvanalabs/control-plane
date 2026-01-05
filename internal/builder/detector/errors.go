package detector

import "errors"

// Detection errors.
var (
	// ErrNoLanguageDetected is returned when no language can be detected.
	ErrNoLanguageDetected = errors.New("could not detect application language")

	// ErrMultipleLanguages is returned when multiple languages are detected.
	ErrMultipleLanguages = errors.New("multiple languages detected, please specify strategy")

	// ErrUnsupportedLanguage is returned when the detected language is not supported.
	ErrUnsupportedLanguage = errors.New("detected language is not supported")

	// ErrRepositoryAccessFailed is returned when the repository cannot be accessed.
	ErrRepositoryAccessFailed = errors.New("failed to access repository")

	// ErrInvalidGoMod is returned when go.mod cannot be parsed.
	ErrInvalidGoMod = errors.New("failed to parse go.mod")

	// ErrInvalidPackageJSON is returned when package.json cannot be parsed.
	ErrInvalidPackageJSON = errors.New("failed to parse package.json")

	// ErrInvalidCargoToml is returned when Cargo.toml cannot be parsed.
	ErrInvalidCargoToml = errors.New("failed to parse Cargo.toml")

	// ErrInvalidPyProject is returned when pyproject.toml cannot be parsed.
	ErrInvalidPyProject = errors.New("failed to parse pyproject.toml")

	// ErrNoEntryPointsFound is returned when no entry points are detected in the repository.
	// **Validates: Requirements 19.6**
	ErrNoEntryPointsFound = errors.New("no entry points found in repository")
)
