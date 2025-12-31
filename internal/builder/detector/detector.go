// Package detector provides build strategy detection for repositories.
package detector

import (
	"context"
	"os"
	"path/filepath"

	builderrors "github.com/narvanalabs/control-plane/internal/builder/errors"
	"github.com/narvanalabs/control-plane/internal/models"
)

// Detector analyzes repositories to determine build strategy.
type Detector interface {
	// Detect analyzes the repository and returns detection results.
	Detect(ctx context.Context, repoPath string) (*models.DetectionResult, error)

	// DetectGo checks for Go application markers.
	DetectGo(ctx context.Context, repoPath string) (*models.DetectionResult, error)

	// DetectNode checks for Node.js application markers.
	DetectNode(ctx context.Context, repoPath string) (*models.DetectionResult, error)

	// DetectRust checks for Rust application markers.
	DetectRust(ctx context.Context, repoPath string) (*models.DetectionResult, error)

	// DetectPython checks for Python application markers.
	DetectPython(ctx context.Context, repoPath string) (*models.DetectionResult, error)

	// HasFlake checks if repository has a flake.nix.
	HasFlake(ctx context.Context, repoPath string) bool

	// HasDockerfile checks if repository has a Dockerfile.
	HasDockerfile(ctx context.Context, repoPath string) bool
}

// DefaultDetector is the default implementation of the Detector interface.
type DefaultDetector struct{}

// NewDetector creates a new DefaultDetector.
func NewDetector() *DefaultDetector {
	return &DefaultDetector{}
}

// Detect analyzes the repository and returns detection results.
// It uses priority-based detection: flake > language > dockerfile.
func (d *DefaultDetector) Detect(ctx context.Context, repoPath string) (*models.DetectionResult, error) {
	// Priority 1: Check for existing flake.nix
	if d.HasFlake(ctx, repoPath) {
		return &models.DetectionResult{
			Strategy:             models.BuildStrategyFlake,
			Framework:            models.FrameworkGeneric,
			Confidence:           1.0,
			RecommendedBuildType: models.BuildTypePureNix,
		}, nil
	}

	// Priority 2: Detect language-specific markers
	// Try Go first
	if result, err := d.DetectGo(ctx, repoPath); err == nil && result != nil {
		return result, nil
	}

	// Try Node.js
	if result, err := d.DetectNode(ctx, repoPath); err == nil && result != nil {
		return result, nil
	}

	// Try Rust
	if result, err := d.DetectRust(ctx, repoPath); err == nil && result != nil {
		return result, nil
	}

	// Try Python
	if result, err := d.DetectPython(ctx, repoPath); err == nil && result != nil {
		return result, nil
	}

	// Priority 3: Check for Dockerfile
	if d.HasDockerfile(ctx, repoPath) {
		return &models.DetectionResult{
			Strategy:             models.BuildStrategyDockerfile,
			Framework:            models.FrameworkGeneric,
			Confidence:           0.8,
			RecommendedBuildType: models.BuildTypeOCI,
		}, nil
	}

	// No detection possible - return enhanced error with suggestions
	return nil, builderrors.NewNoLanguageDetectedError()
}

// HasFlake checks if repository has a flake.nix.
func (d *DefaultDetector) HasFlake(ctx context.Context, repoPath string) bool {
	flakePath := filepath.Join(repoPath, "flake.nix")
	_, err := os.Stat(flakePath)
	return err == nil
}

// HasDockerfile checks if repository has a Dockerfile.
func (d *DefaultDetector) HasDockerfile(ctx context.Context, repoPath string) bool {
	dockerfilePath := filepath.Join(repoPath, "Dockerfile")
	_, err := os.Stat(dockerfilePath)
	return err == nil
}

// DetermineBuildType returns the build type based on the build strategy.
// This is the ONLY place build type is determined automatically.
// **Feature: platform-enhancements, Property 1: Build Type Determination**
// **Validates: Requirements 4.1, 4.2, 4.7, 4.8**
//
// Rules:
// - If strategy is "dockerfile" → build_type is "oci"
// - For all other strategies → build_type is "pure-nix"
func DetermineBuildType(strategy models.BuildStrategy) models.BuildType {
	if strategy == models.BuildStrategyDockerfile {
		return models.BuildTypeOCI
	}
	return models.BuildTypePureNix
}

// DetermineBuildTypeFromLanguage returns the build type based on the selected language.
// This is used when a user selects a language (Go, Rust, Python, Node.js, Dockerfile)
// during service creation.
// **Validates: Requirements 4.1, 4.2, 4.7, 4.8**
//
// Rules:
// - If language is "dockerfile" → build_type is "oci", strategy is "dockerfile"
// - For Go → build_type is "pure-nix", strategy is "auto-go"
// - For Rust → build_type is "pure-nix", strategy is "auto-rust"
// - For Python → build_type is "pure-nix", strategy is "auto-python"
// - For Node.js → build_type is "pure-nix", strategy is "auto-node"
func DetermineBuildTypeFromLanguage(language string) (models.BuildStrategy, models.BuildType) {
	switch language {
	case "dockerfile", "Dockerfile":
		return models.BuildStrategyDockerfile, models.BuildTypeOCI
	case "go", "Go":
		return models.BuildStrategyAutoGo, models.BuildTypePureNix
	case "rust", "Rust":
		return models.BuildStrategyAutoRust, models.BuildTypePureNix
	case "python", "Python":
		return models.BuildStrategyAutoPython, models.BuildTypePureNix
	case "node", "nodejs", "Node.js", "node.js":
		return models.BuildStrategyAutoNode, models.BuildTypePureNix
	default:
		// Default to auto-detection with pure-nix
		return models.BuildStrategyAuto, models.BuildTypePureNix
	}
}

// IsOCIOnlyStrategy returns true if the strategy requires OCI build type.
// Currently, only the dockerfile strategy requires OCI.
// **Validates: Requirements 4.7, 4.8**
func IsOCIOnlyStrategy(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyDockerfile
}
