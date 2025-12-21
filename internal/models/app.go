package models

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// BuildType represents the deployment mode for an application.
type BuildType string

const (
	BuildTypeOCI     BuildType = "oci"
	BuildTypePureNix BuildType = "pure-nix"
)

// ResourceTier represents predefined resource allocation levels.
type ResourceTier string

const (
	ResourceTierNano   ResourceTier = "nano"   // 256MB RAM, 0.25 CPU
	ResourceTierSmall  ResourceTier = "small"  // 512MB RAM, 0.5 CPU
	ResourceTierMedium ResourceTier = "medium" // 1GB RAM, 1 CPU
	ResourceTierLarge  ResourceTier = "large"  // 2GB RAM, 2 CPU
	ResourceTierXLarge ResourceTier = "xlarge" // 4GB RAM, 4 CPU
)

// PortMapping defines a port mapping for a service.
type PortMapping struct {
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol,omitempty"` // tcp or udp, defaults to tcp
}

// HealthCheckConfig defines health check settings for a service.
type HealthCheckConfig struct {
	Path            string `json:"path,omitempty"`
	Port            int    `json:"port,omitempty"`
	IntervalSeconds int    `json:"interval_seconds,omitempty"`
	TimeoutSeconds  int    `json:"timeout_seconds,omitempty"`
	Retries         int    `json:"retries,omitempty"`
}

// SourceType represents the type of source for a service.
type SourceType string

const (
	SourceTypeGit   SourceType = "git"   // Git repo built via Nix flake
	SourceTypeFlake SourceType = "flake" // Direct flake URI (e.g., nixpkgs#redis)
	SourceTypeImage SourceType = "image" // Pre-built OCI image
)

// ValidationError represents a service configuration validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on field '%s': %s", e.Field, e.Message)
}

// ServiceConfig defines a single runnable component within an application.
type ServiceConfig struct {
	Name string `json:"name"`

	// Source configuration (exactly one must be set)
	SourceType SourceType `json:"source_type"`

	// Git source (SourceTypeGit)
	// Decomposed fields that build worker combines into flake URI
	GitRepo     string `json:"git_repo,omitempty"`     // e.g., "github.com/company/web-ui"
	GitRef      string `json:"git_ref,omitempty"`      // Branch/tag/commit (default: "main")
	FlakeOutput string `json:"flake_output,omitempty"` // Output path (default: "packages.${system}.default")

	// Flake source (SourceTypeFlake)
	// Complete flake URI used as-is by build worker
	FlakeURI string `json:"flake_uri,omitempty"` // e.g., "github:owner/repo#packages.x86_64-linux.api"

	// Image source (SourceTypeImage)
	// OCI image reference, build phase skipped
	Image string `json:"image,omitempty"` // e.g., "docker.io/postgres:16"

	// Runtime configuration
	ResourceTier ResourceTier       `json:"resource_tier"`
	Replicas     int                `json:"replicas"`
	Ports        []PortMapping      `json:"ports,omitempty"`
	HealthCheck  *HealthCheckConfig `json:"health_check,omitempty"`
	DependsOn    []string           `json:"depends_on,omitempty"`
	EnvVars      map[string]string  `json:"env_vars,omitempty"` // Service-level env vars (override app-level)
}

// GetCurrentSystem returns the Nix system string for the current platform.
func GetCurrentSystem() string {
	switch runtime.GOARCH {
	case "amd64":
		switch runtime.GOOS {
		case "linux":
			return "x86_64-linux"
		case "darwin":
			return "x86_64-darwin"
		}
	case "arm64":
		switch runtime.GOOS {
		case "linux":
			return "aarch64-linux"
		case "darwin":
			return "aarch64-darwin"
		}
	}
	return "x86_64-linux" // default
}

// Validate validates the service configuration.
func (s *ServiceConfig) Validate() error {
	if s.Name == "" {
		return &ValidationError{Field: "name", Message: "service name is required"}
	}

	// Count how many source types are set
	sourceCount := 0
	if s.GitRepo != "" {
		sourceCount++
		s.SourceType = SourceTypeGit
	}
	if s.FlakeURI != "" {
		sourceCount++
		s.SourceType = SourceTypeFlake
	}
	if s.Image != "" {
		sourceCount++
		s.SourceType = SourceTypeImage
	}

	if sourceCount == 0 {
		return &ValidationError{Field: "source", Message: "exactly one of git_repo, flake_uri, or image is required"}
	}
	if sourceCount > 1 {
		return &ValidationError{Field: "source", Message: "only one of git_repo, flake_uri, or image can be specified"}
	}

	// Validate and apply defaults based on source type
	switch s.SourceType {
	case SourceTypeGit:
		if err := validateGitRepo(s.GitRepo); err != nil {
			return &ValidationError{Field: "git_repo", Message: err.Error()}
		}
		if s.GitRef == "" {
			s.GitRef = "main"
		}
		if s.FlakeOutput == "" {
			s.FlakeOutput = fmt.Sprintf("packages.%s.default", GetCurrentSystem())
		}
	case SourceTypeFlake:
		if err := validateFlakeURI(s.FlakeURI); err != nil {
			return &ValidationError{Field: "flake_uri", Message: err.Error()}
		}
	case SourceTypeImage:
		if err := validateImageRef(s.Image); err != nil {
			return &ValidationError{Field: "image", Message: err.Error()}
		}
	}

	return nil
}

// validateGitRepo validates a git repository URL.
func validateGitRepo(repo string) error {
	if repo == "" {
		return errors.New("git_repo cannot be empty")
	}
	// Accept: github.com/owner/repo, gitlab.com/owner/repo, https://..., git@...
	validPatterns := []string{
		`^github\.com/[\w.-]+/[\w.-]+`,
		`^gitlab\.com/[\w.-]+/[\w.-]+`,
		`^https?://[\w.-]+/[\w.-]+/[\w.-]+`,
		`^git@[\w.-]+:[\w.-]+/[\w.-]+`,
	}
	for _, pattern := range validPatterns {
		if matched, _ := regexp.MatchString(pattern, repo); matched {
			return nil
		}
	}
	return errors.New("invalid git repository URL format")
}

// validateFlakeURI validates a Nix flake URI.
func validateFlakeURI(uri string) error {
	if uri == "" {
		return errors.New("flake_uri cannot be empty")
	}
	// Accept: github:owner/repo, gitlab:owner/repo, nixpkgs#pkg, path:/local
	validPatterns := []string{
		`^github:[\w.-]+/[\w.-]+`,
		`^gitlab:[\w.-]+/[\w.-]+`,
		`^nixpkgs#[\w.-]+`,
		`^path:/`,
		`^git\+https?://`,
	}
	for _, pattern := range validPatterns {
		if matched, _ := regexp.MatchString(pattern, uri); matched {
			return nil
		}
	}
	return errors.New("invalid flake URI format")
}

// validateImageRef validates an OCI image reference.
func validateImageRef(ref string) error {
	if ref == "" {
		return errors.New("image cannot be empty")
	}
	// Accept: registry/image:tag, image:tag, image@sha256:...
	validPattern := `^[\w.-]+(\.[\w.-]+)*(:\d+)?(/[\w.-]+)*(:[a-zA-Z0-9._-]+)?(@sha256:[a-f0-9]+)?$`
	if matched, _ := regexp.MatchString(validPattern, ref); matched {
		return nil
	}
	return errors.New("invalid OCI image reference format")
}

// BuildFlakeURI constructs a flake URI for git sources.
// For git sources, this combines git_repo, git_ref, and flake_output.
// For flake sources, this returns the flake_uri directly.
func (s *ServiceConfig) BuildFlakeURI() string {
	switch s.SourceType {
	case SourceTypeGit:
		return buildFlakeURIFromGit(s.GitRepo, s.GitRef, s.FlakeOutput)
	case SourceTypeFlake:
		return s.FlakeURI
	default:
		return ""
	}
}

// buildFlakeURIFromGit constructs a Nix flake URI from git repo components.
func buildFlakeURIFromGit(gitRepo, gitRef, flakeOutput string) string {
	// Handle different git host formats
	repo := gitRepo

	// Remove protocol prefix if present
	repo = strings.TrimPrefix(repo, "https://")
	repo = strings.TrimPrefix(repo, "http://")

	// Convert to flake URI format based on host
	var flakeRef string
	switch {
	case strings.HasPrefix(repo, "github.com/"):
		// github.com/owner/repo -> github:owner/repo
		path := strings.TrimPrefix(repo, "github.com/")
		path = strings.TrimSuffix(path, ".git")
		flakeRef = fmt.Sprintf("github:%s", path)
	case strings.HasPrefix(repo, "gitlab.com/"):
		// gitlab.com/owner/repo -> gitlab:owner/repo
		path := strings.TrimPrefix(repo, "gitlab.com/")
		path = strings.TrimSuffix(path, ".git")
		flakeRef = fmt.Sprintf("gitlab:%s", path)
	default:
		// Generic git URL -> git+https://...
		flakeRef = fmt.Sprintf("git+https://%s", repo)
	}

	// Append git ref if not "main" or "master"
	if gitRef != "" && gitRef != "main" && gitRef != "master" {
		flakeRef = fmt.Sprintf("%s/%s", flakeRef, gitRef)
	}

	// Append flake output
	if flakeOutput != "" {
		flakeRef = fmt.Sprintf("%s#%s", flakeRef, flakeOutput)
	}

	return flakeRef
}


// App represents a user-defined deployable unit that may contain one or more services.
type App struct {
	ID          string          `json:"id"`
	OwnerID     string          `json:"owner_id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	BuildType   BuildType       `json:"build_type"`
	Services    []ServiceConfig `json:"services"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	DeletedAt   *time.Time      `json:"deleted_at,omitempty"`
}
