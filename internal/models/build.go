package models

import (
	"fmt"
	"time"
)

// BuildStatus represents the current state of a build job.
type BuildStatus string

const (
	BuildStatusQueued    BuildStatus = "queued"
	BuildStatusRunning   BuildStatus = "running"
	BuildStatusSucceeded BuildStatus = "succeeded"
	BuildStatusFailed    BuildStatus = "failed"
)

// ValidStatusTransitions defines the allowed state transitions for build jobs.
// The state machine is: queued → running → (succeeded | failed)
// with running → queued only allowed for retry operations.
var ValidStatusTransitions = map[BuildStatus][]BuildStatus{
	BuildStatusQueued:    {BuildStatusRunning},
	BuildStatusRunning:   {BuildStatusSucceeded, BuildStatusFailed, BuildStatusQueued}, // Queued only for retry
	BuildStatusSucceeded: {}, // Terminal state
	BuildStatusFailed:    {}, // Terminal state
}

// CanTransition checks if a state transition is valid.
// The isRetry parameter indicates if this is a retry operation, which allows
// the special case of running → queued transition.
func CanTransition(from, to BuildStatus, isRetry bool) bool {
	allowed := ValidStatusTransitions[from]
	for _, s := range allowed {
		if s == to {
			// Special case: running -> queued only allowed for retry
			if from == BuildStatusRunning && to == BuildStatusQueued && !isRetry {
				return false
			}
			return true
		}
	}
	return false
}

// IsTerminalState returns true if the status is a terminal state (succeeded or failed).
// Terminal states do not allow any further transitions.
func IsTerminalState(status BuildStatus) bool {
	return status == BuildStatusSucceeded || status == BuildStatusFailed
}

// BuildStrategy represents the method used to build an application.
type BuildStrategy string

const (
	BuildStrategyFlake      BuildStrategy = "flake"       // Use existing flake.nix
	BuildStrategyAutoGo     BuildStrategy = "auto-go"     // Generate flake for Go
	BuildStrategyAutoRust   BuildStrategy = "auto-rust"   // Generate flake for Rust
	BuildStrategyAutoNode   BuildStrategy = "auto-node"   // Generate flake for Node.js
	BuildStrategyAutoPython   BuildStrategy = "auto-python"   // Generate flake for Python
	BuildStrategyAutoDatabase BuildStrategy = "auto-database" // Generate flake for databases
	BuildStrategyDockerfile   BuildStrategy = "dockerfile"    // Build from Dockerfile
	BuildStrategyNixpacks     BuildStrategy = "nixpacks"      // Use Nixpacks
	BuildStrategyAuto         BuildStrategy = "auto"          // Auto-detect
)

// ValidBuildStrategies returns all valid build strategy options.
func ValidBuildStrategies() []BuildStrategy {
	return []BuildStrategy{
		BuildStrategyFlake,
		BuildStrategyAutoGo,
		BuildStrategyAutoRust,
		BuildStrategyAutoNode,
		BuildStrategyAutoPython,
		BuildStrategyAutoDatabase,
		BuildStrategyDockerfile,
		BuildStrategyNixpacks,
		BuildStrategyAuto,
	}
}

// IsValid checks if the build strategy is a valid option.
func (s BuildStrategy) IsValid() bool {
	for _, valid := range ValidBuildStrategies() {
		if s == valid {
			return true
		}
	}
	return false
}

// Framework represents a detected application framework.
type Framework string

const (
	FrameworkGeneric Framework = "generic"
	FrameworkNextJS  Framework = "nextjs"
	FrameworkExpress Framework = "express"
	FrameworkReact   Framework = "react"
	FrameworkFastify Framework = "fastify"
	FrameworkDjango  Framework = "django"
	FrameworkFastAPI Framework = "fastapi"
	FrameworkFlask   Framework = "flask"
)

// DetectionResult contains the results of analyzing a repository.
type DetectionResult struct {
	Strategy             BuildStrategy          `json:"strategy"`
	Framework            Framework              `json:"framework"`
	Version              string                 `json:"version"`
	SuggestedConfig      map[string]interface{} `json:"suggested_config,omitempty"`
	RecommendedBuildType BuildType              `json:"recommended_build_type"`
	EntryPoints          []string               `json:"entry_points,omitempty"`
	Confidence           float64                `json:"confidence"`
	Warnings             []string               `json:"warnings,omitempty"`
}

// NextJSOptions contains Next.js-specific build options.
type NextJSOptions struct {
	OutputMode     string `json:"output_mode,omitempty"`     // "standalone", "export", "default"
	BasePath       string `json:"base_path,omitempty"`       // Base path for deployment
	AssetPrefix    string `json:"asset_prefix,omitempty"`    // CDN prefix for assets
	ImageOptimizer bool   `json:"image_optimizer,omitempty"` // Enable image optimization
}

// DjangoOptions contains Django-specific build options.
type DjangoOptions struct {
	SettingsModule string `json:"settings_module,omitempty"` // e.g., "myapp.settings"
	StaticRoot     string `json:"static_root,omitempty"`     // Static files directory
	CollectStatic  bool   `json:"collect_static,omitempty"`  // Run collectstatic
	Migrations     bool   `json:"migrations,omitempty"`      // Run migrations on deploy
}

// FastAPIOptions contains FastAPI-specific build options.
type FastAPIOptions struct {
	AppModule string `json:"app_module,omitempty"` // e.g., "main:app"
	Workers   int    `json:"workers,omitempty"`    // Uvicorn workers
}

// DatabaseOptions contains database-specific build options.
type DatabaseOptions struct {
	Type    string `json:"type,omitempty"`
	Version string `json:"version,omitempty"`
}

// BuildConfig contains strategy-specific configuration options.
type BuildConfig struct {
	// Common options
	BuildCommand string `json:"build_command,omitempty"`
	StartCommand string `json:"start_command,omitempty"`
	EntryPoint   string `json:"entry_point,omitempty"`
	BuildTimeout int    `json:"build_timeout,omitempty"` // seconds, default 1800

	// Go-specific
	GoVersion  string `json:"go_version,omitempty"`
	CGOEnabled *bool  `json:"cgo_enabled,omitempty"` // Explicit CGO control (nil = auto-detect) **Validates: Requirements 16.5**

	// Go build tags (e.g., ["integration", "debug"]) **Validates: Requirements 17.1**
	BuildTags []string `json:"build_tags,omitempty"`

	// Custom ldflags for Go linker (e.g., "-X main.version=1.0.0") **Validates: Requirements 18.1**
	Ldflags string `json:"ldflags,omitempty"`

	// Pre/post build hooks **Validates: Requirements 21.1, 21.2**
	PreBuildCommands  []string `json:"pre_build_commands,omitempty"`  // Commands to run before build
	PostBuildCommands []string `json:"post_build_commands,omitempty"` // Commands to run after build

	// Node.js-specific
	NodeVersion    string `json:"node_version,omitempty"`
	PackageManager string `json:"package_manager,omitempty"` // npm, yarn, pnpm

	// Rust-specific
	RustEdition string `json:"rust_edition,omitempty"`

	// Python-specific
	PythonVersion string `json:"python_version,omitempty"`

	// Framework-specific options
	NextJSOptions   *NextJSOptions   `json:"nextjs_options,omitempty"`
	DjangoOptions   *DjangoOptions   `json:"django_options,omitempty"`
	FastAPIOptions  *FastAPIOptions  `json:"fastapi_options,omitempty"`
	DatabaseOptions *DatabaseOptions `json:"database_options,omitempty"`

	// Advanced options
	ExtraNixPackages []string          `json:"extra_nix_packages,omitempty"`
	EnvironmentVars  map[string]string `json:"environment_vars,omitempty"`

	// Fallback behavior
	AutoRetryAsOCI bool `json:"auto_retry_as_oci,omitempty"`
}

// BuildJob represents a build task in the queue.
type BuildJob struct {
	ID           string      `json:"id"`
	DeploymentID string      `json:"deployment_id"`
	AppID        string      `json:"app_id"`
	ServiceName  string      `json:"service_name,omitempty"`

	// Source information - explicit fields for clarity
	// SourceType indicates whether this is a git, flake, or database source
	// For git sources: GitURL contains the original git URL, FlakeURI contains the constructed flake URI
	// For flake sources: FlakeURI contains the direct flake URI, GitURL is empty
	// For database sources: Both GitURL and FlakeURI may be empty
	SourceType SourceType `json:"source_type,omitempty" db:"source_type"`
	GitURL     string     `json:"git_url"`
	GitRef     string     `json:"git_ref"`
	FlakeURI   string     `json:"flake_uri,omitempty" db:"flake_uri"` // Constructed or direct flake URI
	FlakeOutput string    `json:"flake_output"`

	BuildType    BuildType   `json:"build_type"`
	Status       BuildStatus `json:"status"`
	CreatedAt    time.Time   `json:"created_at"`
	StartedAt    *time.Time  `json:"started_at,omitempty"`
	FinishedAt   *time.Time  `json:"finished_at,omitempty"`

	// Build strategy fields
	BuildStrategy  BuildStrategy `json:"build_strategy,omitempty" db:"build_strategy"`
	BuildConfig    *BuildConfig  `json:"build_config,omitempty" db:"build_config"`
	GeneratedFlake string        `json:"generated_flake,omitempty" db:"generated_flake"`
	FlakeLock      string        `json:"flake_lock,omitempty" db:"flake_lock"`
	VendorHash     string        `json:"vendor_hash,omitempty" db:"vendor_hash"`

	// Resource limits and retry tracking
	TimeoutSeconds int  `json:"timeout_seconds,omitempty" db:"timeout_seconds"`
	RetryCount     int  `json:"retry_count,omitempty" db:"retry_count"`
	RetryAsOCI     bool `json:"retry_as_oci,omitempty" db:"retry_as_oci"`
}

// ValidateBuildJobSource validates that the BuildJob source fields are consistent.
// For git sources: GitURL must be non-empty, FlakeURI should contain the constructed flake URI
// For flake sources: FlakeURI must be non-empty, GitURL should be empty
// **Validates: Requirements 27.1, 27.2, 27.3**
func (b *BuildJob) ValidateBuildJobSource() error {
	switch b.SourceType {
	case SourceTypeGit:
		if b.GitURL == "" {
			return &ValidationError{
				Field:   "git_url",
				Message: "git_url is required for git source type",
			}
		}
		// FlakeURI should be set (constructed from git URL)
		if b.FlakeURI == "" {
			return &ValidationError{
				Field:   "flake_uri",
				Message: "flake_uri should be set for git source type (constructed from git URL)",
			}
		}
	case SourceTypeFlake:
		if b.FlakeURI == "" {
			return &ValidationError{
				Field:   "flake_uri",
				Message: "flake_uri is required for flake source type",
			}
		}
		// GitURL should be empty for direct flake sources
		if b.GitURL != "" {
			return &ValidationError{
				Field:   "git_url",
				Message: "git_url should be empty for flake source type",
			}
		}
	case SourceTypeDatabase:
		// Database sources may have empty GitURL and FlakeURI
		// No validation needed
	case "":
		// Empty source type is allowed for backward compatibility
		// but we should have at least GitURL or FlakeURI
		if b.GitURL == "" && b.FlakeURI == "" {
			return &ValidationError{
				Field:   "source_type",
				Message: "either source_type must be set, or git_url/flake_uri must be provided",
			}
		}
	default:
		return &ValidationError{
			Field:   "source_type",
			Message: fmt.Sprintf("invalid source type: %s", b.SourceType),
		}
	}
	return nil
}

// BuildResult represents the output of a completed build.
type BuildResult struct {
	Artifact  string `json:"artifact"`
	StorePath string `json:"store_path,omitempty"` // For pure-nix
	ImageTag  string `json:"image_tag,omitempty"`  // For OCI
	Logs      string `json:"logs"`
}

// EnforceBuildType ensures the correct build type for a strategy.
// Some strategies (dockerfile, nixpacks) require OCI build type.
// Returns the enforced build type and whether it was changed from the requested type.
// **Validates: Requirements 10.2, 11.2, 18.1, 18.2**
func EnforceBuildType(strategy BuildStrategy, requestedType BuildType) (BuildType, bool) {
	switch strategy {
	case BuildStrategyDockerfile, BuildStrategyNixpacks:
		// Dockerfile and Nixpacks strategies always require OCI build type
		// **Validates: Requirements 10.2, 18.1** (dockerfile)
		// **Validates: Requirements 11.2, 18.2** (nixpacks)
		return BuildTypeOCI, requestedType != BuildTypeOCI
	case BuildStrategyFlake:
		// Flake strategy uses the user's choice
		// **Validates: Requirements 18.3**
		return requestedType, false
	default:
		// Auto-* strategies: user's choice or default to pure-nix
		// **Validates: Requirements 18.4**
		if requestedType == "" {
			return BuildTypePureNix, false
		}
		return requestedType, false
	}
}

// ArtifactType represents the type of build artifact produced.
type ArtifactType string

const (
	// ArtifactTypeStorePath represents a Nix store path artifact (for pure-nix builds).
	ArtifactTypeStorePath ArtifactType = "store_path"
	// ArtifactTypeImageTag represents an OCI image tag artifact (for OCI builds).
	ArtifactTypeImageTag ArtifactType = "image_tag"
	// ArtifactTypeUnknown represents an unknown or invalid artifact type.
	ArtifactTypeUnknown ArtifactType = "unknown"
)

// ValidateArtifact validates that an artifact is appropriate for the given build type.
// For pure-nix builds, the artifact should be a Nix store path.
// For OCI builds, the artifact should be an image tag.
// **Validates: Requirements 13.1, 13.2**
func ValidateArtifact(buildType BuildType, artifact string) (ArtifactType, bool) {
	if artifact == "" {
		return ArtifactTypeUnknown, false
	}

	switch buildType {
	case BuildTypePureNix:
		// Pure-nix builds should produce Nix store paths
		// **Validates: Requirements 13.1**
		if IsNixStorePath(artifact) {
			return ArtifactTypeStorePath, true
		}
		return ArtifactTypeUnknown, false

	case BuildTypeOCI:
		// OCI builds should produce image tags
		// **Validates: Requirements 13.2**
		if IsOCIImageTag(artifact) {
			return ArtifactTypeImageTag, true
		}
		return ArtifactTypeUnknown, false

	default:
		return ArtifactTypeUnknown, false
	}
}

// IsNixStorePath checks if a string is a valid Nix store path.
// Nix store paths follow the format: /nix/store/<hash>-<name>
// **Validates: Requirements 13.1**
func IsNixStorePath(path string) bool {
	if path == "" {
		return false
	}
	// Nix store paths start with /nix/store/
	if len(path) < 44 { // /nix/store/ (11) + hash (32) + - (1) = 44 minimum
		return false
	}
	return len(path) >= 11 && path[:11] == "/nix/store/"
}

// IsOCIImageTag checks if a string is a valid OCI image tag.
// OCI image tags follow formats like:
// - registry.example.com/image:tag
// - registry.example.com/namespace/image:tag
// - image:tag
// - image@sha256:digest
// **Validates: Requirements 13.2**
func IsOCIImageTag(tag string) bool {
	if tag == "" {
		return false
	}
	// Basic validation: should contain either : or @ for tag/digest
	// and should not start with /nix/store/ (which would be a store path)
	if len(tag) >= 11 && tag[:11] == "/nix/store/" {
		return false
	}
	// Must contain a colon (for tag) or @ (for digest)
	hasTag := false
	hasDigest := false
	for _, c := range tag {
		if c == ':' {
			hasTag = true
		}
		if c == '@' {
			hasDigest = true
		}
	}
	return hasTag || hasDigest
}

// GetExpectedArtifactType returns the expected artifact type for a build type.
// **Validates: Requirements 13.1, 13.2**
func GetExpectedArtifactType(buildType BuildType) ArtifactType {
	switch buildType {
	case BuildTypePureNix:
		return ArtifactTypeStorePath
	case BuildTypeOCI:
		return ArtifactTypeImageTag
	default:
		return ArtifactTypeUnknown
	}
}
