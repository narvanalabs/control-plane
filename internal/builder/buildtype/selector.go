// Package buildtype provides build type selection logic for different build strategies.
package buildtype

import (
	"github.com/narvanalabs/control-plane/internal/models"
)

// BuildTypeRecommendation contains build type recommendation with reasoning.
type BuildTypeRecommendation struct {
	Recommended models.BuildType `json:"recommended"`
	Reason      string           `json:"reason"`
	Alternative models.BuildType `json:"alternative,omitempty"`
	AltReason   string           `json:"alt_reason,omitempty"`
}

// Selector determines the appropriate build type for a strategy.
type Selector interface {
	// SelectBuildType determines build type based on strategy and detection.
	// If userPreference is provided, it will be used unless the strategy forces a specific type.
	SelectBuildType(strategy models.BuildStrategy, detection *models.DetectionResult, userPreference *models.BuildType) models.BuildType

	// GetRecommendation returns the recommended build type with reasoning.
	GetRecommendation(strategy models.BuildStrategy, detection *models.DetectionResult) BuildTypeRecommendation
}

// DefaultSelector is the default implementation of Selector.
type DefaultSelector struct{}

// NewSelector creates a new DefaultSelector.
func NewSelector() *DefaultSelector {
	return &DefaultSelector{}
}

// SelectBuildType determines build type based on strategy and detection.
// Build type selection rules:
// - dockerfile strategy → ALWAYS oci
// - nixpacks strategy → ALWAYS oci
// - flake strategy → Based on flake outputs (user's flake determines type)
// - auto-go → PREFER pure-nix (smaller, faster), user can choose oci
// - auto-rust → PREFER pure-nix (smaller, faster), user can choose oci
// - auto-node → PREFER pure-nix for SSR apps, oci for complex native deps
// - auto-python → PREFER pure-nix, oci for complex native deps
func (s *DefaultSelector) SelectBuildType(strategy models.BuildStrategy, detection *models.DetectionResult, userPreference *models.BuildType) models.BuildType {
	// Dockerfile and Nixpacks always produce OCI - no user override allowed
	if strategy == models.BuildStrategyDockerfile || strategy == models.BuildStrategyNixpacks {
		return models.BuildTypeOCI
	}

	// For flake strategy, respect user preference or default to pure-nix
	if strategy == models.BuildStrategyFlake {
		if userPreference != nil {
			return *userPreference
		}
		return models.BuildTypePureNix
	}

	// For auto-* strategies, respect user preference if provided
	if userPreference != nil {
		return *userPreference
	}

	// Get recommendation based on strategy and detection
	recommendation := s.GetRecommendation(strategy, detection)
	return recommendation.Recommended
}

// GetRecommendation returns the recommended build type with reasoning.
func (s *DefaultSelector) GetRecommendation(strategy models.BuildStrategy, detection *models.DetectionResult) BuildTypeRecommendation {
	switch strategy {
	case models.BuildStrategyDockerfile:
		return BuildTypeRecommendation{
			Recommended: models.BuildTypeOCI,
			Reason:      "Dockerfile strategy always produces OCI containers",
		}

	case models.BuildStrategyNixpacks:
		return BuildTypeRecommendation{
			Recommended: models.BuildTypeOCI,
			Reason:      "Nixpacks strategy always produces OCI containers",
		}

	case models.BuildStrategyFlake:
		return BuildTypeRecommendation{
			Recommended: models.BuildTypePureNix,
			Reason:      "Flake strategy defaults to pure-nix for optimal performance",
			Alternative: models.BuildTypeOCI,
			AltReason:   "Use OCI if you need container-based deployment",
		}

	case models.BuildStrategyAutoGo:
		return s.getGoRecommendation(detection)

	case models.BuildStrategyAutoRust:
		return BuildTypeRecommendation{
			Recommended: models.BuildTypePureNix,
			Reason:      "Rust applications build efficiently as pure-nix closures with smaller size and faster startup",
			Alternative: models.BuildTypeOCI,
			AltReason:   "Use OCI if you have complex native dependencies or need container-based deployment",
		}

	case models.BuildStrategyAutoNode:
		return s.getNodeRecommendation(detection)

	case models.BuildStrategyAutoPython:
		return s.getPythonRecommendation(detection)

	case models.BuildStrategyAuto:
		// For auto strategy, use detection result's recommendation if available
		if detection != nil && detection.RecommendedBuildType != "" {
			return BuildTypeRecommendation{
				Recommended: detection.RecommendedBuildType,
				Reason:      "Based on auto-detection analysis",
				Alternative: s.getAlternativeBuildType(detection.RecommendedBuildType),
				AltReason:   "Alternative deployment option",
			}
		}
		return BuildTypeRecommendation{
			Recommended: models.BuildTypePureNix,
			Reason:      "Default recommendation for auto-detected applications",
			Alternative: models.BuildTypeOCI,
			AltReason:   "Use OCI for complex dependencies or container-based deployment",
		}

	default:
		return BuildTypeRecommendation{
			Recommended: models.BuildTypePureNix,
			Reason:      "Default build type for unknown strategies",
			Alternative: models.BuildTypeOCI,
			AltReason:   "Use OCI for container-based deployment",
		}
	}
}

// getGoRecommendation returns the recommendation for Go applications.
func (s *DefaultSelector) getGoRecommendation(detection *models.DetectionResult) BuildTypeRecommendation {
	// Check if CGO is enabled - might need OCI for complex native deps
	if detection != nil && detection.SuggestedConfig != nil {
		if cgoEnabled, ok := detection.SuggestedConfig["cgo_enabled"].(bool); ok && cgoEnabled {
			return BuildTypeRecommendation{
				Recommended: models.BuildTypePureNix,
				Reason:      "Go with CGO can still build as pure-nix, but may need additional native dependencies",
				Alternative: models.BuildTypeOCI,
				AltReason:   "Use OCI if CGO dependencies are complex or not available in nixpkgs",
			}
		}
	}

	return BuildTypeRecommendation{
		Recommended: models.BuildTypePureNix,
		Reason:      "Go applications build efficiently as pure-nix closures with smaller size and faster startup",
		Alternative: models.BuildTypeOCI,
		AltReason:   "Use OCI if you need container-based deployment",
	}
}

// getNodeRecommendation returns the recommendation for Node.js applications.
func (s *DefaultSelector) getNodeRecommendation(detection *models.DetectionResult) BuildTypeRecommendation {
	if detection != nil {
		switch detection.Framework {
		case models.FrameworkNextJS:
			return BuildTypeRecommendation{
				Recommended: models.BuildTypePureNix,
				Reason:      "Next.js applications work well as pure-nix with standalone output mode",
				Alternative: models.BuildTypeOCI,
				AltReason:   "Use OCI if you have complex native npm dependencies",
			}
		case models.FrameworkExpress, models.FrameworkFastify:
			return BuildTypeRecommendation{
				Recommended: models.BuildTypePureNix,
				Reason:      "Express/Fastify applications build efficiently as pure-nix closures",
				Alternative: models.BuildTypeOCI,
				AltReason:   "Use OCI if you have native npm dependencies that are difficult to build with Nix",
			}
		}
	}

	return BuildTypeRecommendation{
		Recommended: models.BuildTypePureNix,
		Reason:      "Node.js applications generally work well as pure-nix closures",
		Alternative: models.BuildTypeOCI,
		AltReason:   "Use OCI if you have complex native npm dependencies",
	}
}

// getPythonRecommendation returns the recommendation for Python applications.
func (s *DefaultSelector) getPythonRecommendation(detection *models.DetectionResult) BuildTypeRecommendation {
	if detection != nil {
		switch detection.Framework {
		case models.FrameworkDjango:
			return BuildTypeRecommendation{
				Recommended: models.BuildTypePureNix,
				Reason:      "Django applications build well as pure-nix closures",
				Alternative: models.BuildTypeOCI,
				AltReason:   "Use OCI if you have complex native Python dependencies (e.g., ML libraries)",
			}
		case models.FrameworkFastAPI:
			return BuildTypeRecommendation{
				Recommended: models.BuildTypePureNix,
				Reason:      "FastAPI applications are lightweight and work well as pure-nix closures",
				Alternative: models.BuildTypeOCI,
				AltReason:   "Use OCI if you have complex native Python dependencies",
			}
		case models.FrameworkFlask:
			return BuildTypeRecommendation{
				Recommended: models.BuildTypePureNix,
				Reason:      "Flask applications are lightweight and work well as pure-nix closures",
				Alternative: models.BuildTypeOCI,
				AltReason:   "Use OCI if you have complex native Python dependencies",
			}
		}
	}

	return BuildTypeRecommendation{
		Recommended: models.BuildTypePureNix,
		Reason:      "Python applications generally work well as pure-nix closures",
		Alternative: models.BuildTypeOCI,
		AltReason:   "Use OCI if you have complex native dependencies (e.g., ML libraries with CUDA)",
	}
}

// getAlternativeBuildType returns the alternative build type.
func (s *DefaultSelector) getAlternativeBuildType(recommended models.BuildType) models.BuildType {
	if recommended == models.BuildTypeOCI {
		return models.BuildTypePureNix
	}
	return models.BuildTypeOCI
}

// IsOCIEnforced returns true if the strategy always produces OCI containers.
func IsOCIEnforced(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyDockerfile || strategy == models.BuildStrategyNixpacks
}
