package detector

import (
	"context"
	"os"
	"path/filepath"

	"github.com/narvanalabs/control-plane/internal/models"
)

// AutoDetector provides priority-based detection for repositories.
// Detection priority: flake > language-specific > dockerfile
type AutoDetector struct {
	detector *DefaultDetector
}

// NewAutoDetector creates a new AutoDetector.
func NewAutoDetector() *AutoDetector {
	return &AutoDetector{
		detector: NewDetector(),
	}
}

// Detect performs auto-detection on a repository.
// It follows the priority: flake > language > dockerfile.
func (a *AutoDetector) Detect(ctx context.Context, repoPath string) (*models.DetectionResult, error) {
	return a.detector.Detect(ctx, repoPath)
}

// DetectAll returns all detected languages/frameworks in the repository.
// This is useful for handling mixed-language repositories.
func (a *AutoDetector) DetectAll(ctx context.Context, repoPath string) ([]*models.DetectionResult, error) {
	var results []*models.DetectionResult

	// Check for flake.nix first
	if a.detector.HasFlake(ctx, repoPath) {
		results = append(results, &models.DetectionResult{
			Strategy:             models.BuildStrategyFlake,
			Framework:            models.FrameworkGeneric,
			Confidence:           1.0,
			RecommendedBuildType: models.BuildTypePureNix,
		})
	}

	// Detect all languages
	if result, err := a.detector.DetectGo(ctx, repoPath); err == nil && result != nil {
		results = append(results, result)
	}

	if result, err := a.detector.DetectNode(ctx, repoPath); err == nil && result != nil {
		results = append(results, result)
	}

	if result, err := a.detector.DetectRust(ctx, repoPath); err == nil && result != nil {
		results = append(results, result)
	}

	if result, err := a.detector.DetectPython(ctx, repoPath); err == nil && result != nil {
		results = append(results, result)
	}

	// Check for Dockerfile
	if a.detector.HasDockerfile(ctx, repoPath) {
		results = append(results, &models.DetectionResult{
			Strategy:             models.BuildStrategyDockerfile,
			Framework:            models.FrameworkGeneric,
			Confidence:           0.8,
			RecommendedBuildType: models.BuildTypeOCI,
		})
	}

	if len(results) == 0 {
		return nil, ErrNoLanguageDetected
	}

	return results, nil
}

// DetectWithPreference performs detection with a user preference.
// If the user prefers a specific strategy, it validates that strategy is applicable.
func (a *AutoDetector) DetectWithPreference(ctx context.Context, repoPath string, preference models.BuildStrategy) (*models.DetectionResult, error) {
	switch preference {
	case models.BuildStrategyFlake:
		if a.detector.HasFlake(ctx, repoPath) {
			return &models.DetectionResult{
				Strategy:             models.BuildStrategyFlake,
				Framework:            models.FrameworkGeneric,
				Confidence:           1.0,
				RecommendedBuildType: models.BuildTypePureNix,
			}, nil
		}
		return nil, ErrRepositoryAccessFailed

	case models.BuildStrategyDockerfile:
		if a.detector.HasDockerfile(ctx, repoPath) {
			return &models.DetectionResult{
				Strategy:             models.BuildStrategyDockerfile,
				Framework:            models.FrameworkGeneric,
				Confidence:           1.0,
				RecommendedBuildType: models.BuildTypeOCI,
			}, nil
		}
		return nil, ErrRepositoryAccessFailed

	case models.BuildStrategyAutoGo:
		return a.detector.DetectGo(ctx, repoPath)

	case models.BuildStrategyAutoNode:
		return a.detector.DetectNode(ctx, repoPath)

	case models.BuildStrategyAutoRust:
		return a.detector.DetectRust(ctx, repoPath)

	case models.BuildStrategyAutoPython:
		return a.detector.DetectPython(ctx, repoPath)

	case models.BuildStrategyNixpacks:
		// Nixpacks can work with any repository
		return &models.DetectionResult{
			Strategy:             models.BuildStrategyNixpacks,
			Framework:            models.FrameworkGeneric,
			Confidence:           0.7,
			RecommendedBuildType: models.BuildTypeOCI,
		}, nil

	case models.BuildStrategyAuto:
		return a.Detect(ctx, repoPath)

	default:
		return nil, ErrUnsupportedLanguage
	}
}

// IsMixedLanguageRepo checks if a repository contains multiple languages.
func (a *AutoDetector) IsMixedLanguageRepo(ctx context.Context, repoPath string) (bool, []models.BuildStrategy) {
	var strategies []models.BuildStrategy

	// Check for each language marker
	if fileExists(filepath.Join(repoPath, "go.mod")) {
		strategies = append(strategies, models.BuildStrategyAutoGo)
	}
	if fileExists(filepath.Join(repoPath, "package.json")) {
		strategies = append(strategies, models.BuildStrategyAutoNode)
	}
	if fileExists(filepath.Join(repoPath, "Cargo.toml")) {
		strategies = append(strategies, models.BuildStrategyAutoRust)
	}
	if fileExists(filepath.Join(repoPath, "requirements.txt")) ||
		fileExists(filepath.Join(repoPath, "pyproject.toml")) ||
		fileExists(filepath.Join(repoPath, "setup.py")) {
		strategies = append(strategies, models.BuildStrategyAutoPython)
	}

	return len(strategies) > 1, strategies
}

// GetRecommendedStrategy returns the recommended strategy for a repository.
// It considers the detected languages and provides reasoning.
func (a *AutoDetector) GetRecommendedStrategy(ctx context.Context, repoPath string) (*StrategyRecommendation, error) {
	// Check for flake first
	if a.detector.HasFlake(ctx, repoPath) {
		return &StrategyRecommendation{
			Strategy: models.BuildStrategyFlake,
			Reason:   "Repository contains a flake.nix file",
			Confidence: 1.0,
		}, nil
	}

	// Check for mixed languages
	isMixed, strategies := a.IsMixedLanguageRepo(ctx, repoPath)
	if isMixed {
		return &StrategyRecommendation{
			Strategy:     models.BuildStrategyAuto,
			Reason:       "Multiple languages detected, please specify a strategy",
			Confidence:   0.5,
			Alternatives: strategies,
		}, nil
	}

	// Perform auto-detection
	result, err := a.Detect(ctx, repoPath)
	if err != nil {
		// Fall back to Dockerfile or Nixpacks
		if a.detector.HasDockerfile(ctx, repoPath) {
			return &StrategyRecommendation{
				Strategy:   models.BuildStrategyDockerfile,
				Reason:     "No language detected, but Dockerfile found",
				Confidence: 0.8,
			}, nil
		}
		return &StrategyRecommendation{
			Strategy:   models.BuildStrategyNixpacks,
			Reason:     "No language or Dockerfile detected, using Nixpacks as fallback",
			Confidence: 0.5,
		}, nil
	}

	return &StrategyRecommendation{
		Strategy:   result.Strategy,
		Reason:     "Detected " + string(result.Strategy) + " project",
		Confidence: result.Confidence,
	}, nil
}

// StrategyRecommendation contains a recommended strategy with reasoning.
type StrategyRecommendation struct {
	Strategy     models.BuildStrategy   `json:"strategy"`
	Reason       string                 `json:"reason"`
	Confidence   float64                `json:"confidence"`
	Alternatives []models.BuildStrategy `json:"alternatives,omitempty"`
}

// ValidateStrategy checks if a strategy is valid for the given repository.
func (a *AutoDetector) ValidateStrategy(ctx context.Context, repoPath string, strategy models.BuildStrategy) error {
	switch strategy {
	case models.BuildStrategyFlake:
		if !a.detector.HasFlake(ctx, repoPath) {
			return ErrRepositoryAccessFailed
		}
	case models.BuildStrategyDockerfile:
		if !a.detector.HasDockerfile(ctx, repoPath) {
			return ErrRepositoryAccessFailed
		}
	case models.BuildStrategyAutoGo:
		if !fileExists(filepath.Join(repoPath, "go.mod")) {
			return ErrNoLanguageDetected
		}
	case models.BuildStrategyAutoNode:
		if !fileExists(filepath.Join(repoPath, "package.json")) {
			return ErrNoLanguageDetected
		}
	case models.BuildStrategyAutoRust:
		if !fileExists(filepath.Join(repoPath, "Cargo.toml")) {
			return ErrNoLanguageDetected
		}
	case models.BuildStrategyAutoPython:
		hasPython := fileExists(filepath.Join(repoPath, "requirements.txt")) ||
			fileExists(filepath.Join(repoPath, "pyproject.toml")) ||
			fileExists(filepath.Join(repoPath, "setup.py"))
		if !hasPython {
			return ErrNoLanguageDetected
		}
	case models.BuildStrategyNixpacks, models.BuildStrategyAuto:
		// These strategies can work with any repository
		return nil
	default:
		return ErrUnsupportedLanguage
	}
	return nil
}

// fileExistsInDir is a helper to check if a file exists in a directory.
func fileExistsInDir(dir, filename string) bool {
	_, err := os.Stat(filepath.Join(dir, filename))
	return err == nil
}
