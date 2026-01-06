package executor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/narvanalabs/control-plane/internal/builder/cache"
	"github.com/narvanalabs/control-plane/internal/builder/clone"
	"github.com/narvanalabs/control-plane/internal/builder/detector"
	"github.com/narvanalabs/control-plane/internal/builder/hash"
	"github.com/narvanalabs/control-plane/internal/builder/templates"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/validation"
)

// PreBuildResult contains the results of the pre-build phase.
// **Validates: Requirements 1.1, 1.2**
type PreBuildResult struct {
	// RepoPath is the path to the cloned repository
	RepoPath string

	// Detection contains the detection results
	Detection *models.DetectionResult

	// CommitSHA is the resolved commit SHA
	CommitSHA string

	// CloneDuration is how long the clone took
	CloneDuration time.Duration

	// DetectionDuration is how long detection took
	DetectionDuration time.Duration

	// CacheHit indicates whether the detection result came from cache
	// **Validates: Requirements 4.3**
	CacheHit bool
}

// AutoGoStrategyExecutor executes builds for Go applications by generating a flake.nix.
type AutoGoStrategyExecutor struct {
	detector        detector.Detector
	templateEngine  templates.TemplateEngine
	hashCalculator  hash.HashCalculator
	nixBuilder      NixBuilder
	ociBuilder      OCIBuilder
	logger          *slog.Logger
	detectionCache  cache.DetectionCache // **Validates: Requirements 4.3**
}

// NewAutoGoStrategyExecutor creates a new AutoGoStrategyExecutor.
func NewAutoGoStrategyExecutor(
	det detector.Detector,
	tmplEngine templates.TemplateEngine,
	hashCalc hash.HashCalculator,
	nixBuilder NixBuilder,
	ociBuilder OCIBuilder,
	logger *slog.Logger,
) *AutoGoStrategyExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &AutoGoStrategyExecutor{
		detector:       det,
		templateEngine: tmplEngine,
		hashCalculator: hashCalc,
		nixBuilder:     nixBuilder,
		ociBuilder:     ociBuilder,
		logger:         logger,
	}
}

// NewAutoGoStrategyExecutorWithCache creates a new AutoGoStrategyExecutor with detection caching.
// **Validates: Requirements 4.3**
func NewAutoGoStrategyExecutorWithCache(
	det detector.Detector,
	tmplEngine templates.TemplateEngine,
	hashCalc hash.HashCalculator,
	nixBuilder NixBuilder,
	ociBuilder OCIBuilder,
	logger *slog.Logger,
	detectionCache cache.DetectionCache,
) *AutoGoStrategyExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &AutoGoStrategyExecutor{
		detector:       det,
		templateEngine: tmplEngine,
		hashCalculator: hashCalc,
		nixBuilder:     nixBuilder,
		ociBuilder:     ociBuilder,
		logger:         logger,
		detectionCache: detectionCache,
	}
}

// SetDetectionCache sets the detection cache for the executor.
// **Validates: Requirements 4.3**
func (e *AutoGoStrategyExecutor) SetDetectionCache(cache cache.DetectionCache) {
	e.detectionCache = cache
}

// Supports returns true if this executor handles the given strategy.
func (e *AutoGoStrategyExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyAutoGo
}

// GenerateFlake generates a flake.nix for a Go application.
// **Validates: Requirements 16.2** - Sets CGO_ENABLED based on detection result
func (e *AutoGoStrategyExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	return e.GenerateFlakeWithContext(ctx, detection, config, validation.DefaultLdflagsBuildContext())
}

// GenerateFlakeWithContext generates a flake.nix for a Go application with build context for ldflags substitution.
// **Validates: Requirements 16.2** - Sets CGO_ENABLED based on detection result
// **Validates: Requirements 18.3** - Performs ldflags variable substitution
func (e *AutoGoStrategyExecutor) GenerateFlakeWithContext(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig, buildCtx validation.LdflagsBuildContext) (string, error) {
	// Determine CGO setting:
	// 1. If explicitly set in config, use that (user override)
	// 2. Otherwise, use detection result from SuggestedConfig
	// **Validates: Requirements 16.5** - User config overrides auto-detection
	cgoEnabled := false
	if config.CGOEnabled != nil {
		// User explicitly set CGO - honor their choice
		cgoEnabled = *config.CGOEnabled
	} else if detection != nil && detection.SuggestedConfig != nil {
		// Use auto-detected CGO setting
		if detected, ok := detection.SuggestedConfig["cgo_enabled"].(bool); ok {
			cgoEnabled = detected
		}
	}

	// Determine template name based on CGO
	templateName := "go.nix"
	if cgoEnabled {
		templateName = "go-cgo.nix"
	}

	// Perform ldflags variable substitution if ldflags contains variables
	// **Validates: Requirements 18.3**
	processedConfig := config
	if config.Ldflags != "" && validation.HasLdflagsVariables(config.Ldflags) {
		processedConfig.Ldflags = validation.SubstituteLdflagsVariables(config.Ldflags, buildCtx)
		e.logger.Debug("substituted ldflags variables",
			"original", config.Ldflags,
			"substituted", processedConfig.Ldflags,
		)
	}

	// Prepare template data
	data := templates.TemplateData{
		AppName:         getAppName(detection),
		Version:         detection.Version,
		Framework:       detection.Framework,
		EntryPoint:      processedConfig.EntryPoint,
		Config:          processedConfig,
		DetectionResult: detection,
	}

	// If entry point not specified in config, use first detected entry point
	if data.EntryPoint == "" && len(detection.EntryPoints) > 0 {
		data.EntryPoint = detection.EntryPoints[0]
	}

	// Render the template
	flakeContent, err := e.templateEngine.Render(ctx, templateName, data)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplateRenderFailed, err)
	}

	return flakeContent, nil
}

// Execute runs the build for a Go application (without external log streaming).
func (e *AutoGoStrategyExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	// Use a no-op callback that just collects logs
	return e.ExecuteWithLogs(ctx, job, nil)
}

// ExecuteWithLogs runs the build for a Go application with real-time log streaming.
// **Validates: Requirements 1.1, 1.2, 4.2**
func (e *AutoGoStrategyExecutor) ExecuteWithLogs(ctx context.Context, job *models.BuildJob, externalCallback LogCallback) (*BuildResult, error) {
	e.logger.Info("executing auto-go strategy",
		"job_id", job.ID,
		"build_type", job.BuildType,
		"has_generated_flake_before", job.GeneratedFlake != "",
	)

	var logs string
	logCallback := func(line string) {
		logs += line + "\n"
		if externalCallback != nil {
			externalCallback(line)
		}
	}

	// Track the pre-build result for repo reuse
	var preBuildResult *PreBuildResult

	// If we don't have a generated flake yet, we need to generate one
	if job.GeneratedFlake == "" {
		logCallback("=== Pre-build phase: Clone and Detect ===")

		// **Validates: Requirements 1.1** - Clone repository before generating the flake
		// **Validates: Requirements 1.2** - Run the full detection pipeline including CGO detection
		var err error
		preBuildResult, err = e.PreBuild(ctx, job)
		if err != nil {
			return &BuildResult{Logs: logs}, fmt.Errorf("%w: %v", ErrDetectionFailed, err)
		}

		// Store detection result in job for persistence
		// **Validates: Requirements 2.1**
		job.DetectionResult = preBuildResult.Detection
		now := time.Now()
		job.DetectedAt = &now

		// Log cache hit or clone/detection timing
		// **Validates: Requirements 4.3**
		if preBuildResult.CacheHit {
			logCallback("Detection cache hit - using cached result")
			logCallback(fmt.Sprintf("Commit SHA: %s", preBuildResult.CommitSHA))
		} else {
			logCallback(fmt.Sprintf("Repository cloned in %v", preBuildResult.CloneDuration))
			logCallback(fmt.Sprintf("Commit SHA: %s", preBuildResult.CommitSHA))
			logCallback(fmt.Sprintf("Detection completed in %v", preBuildResult.DetectionDuration))
		}

		logCallback(fmt.Sprintf("Detected Go version: %s", preBuildResult.Detection.Version))
		if len(preBuildResult.Detection.EntryPoints) > 0 {
			logCallback(fmt.Sprintf("Entry points: %v", preBuildResult.Detection.EntryPoints))
		}
		if cgoEnabled, ok := preBuildResult.Detection.SuggestedConfig["cgo_enabled"].(bool); ok && cgoEnabled {
			logCallback("CGO detected: enabled")
		}

		// Generate the flake with build context for ldflags substitution
		// **Validates: Requirements 18.3**
		logCallback("=== Generating flake.nix ===")

		// Get user config from job and detected config from detection result
		// **Validates: Requirements 3.1, 3.2** - Merge user config with detected config
		userConfig := e.getConfigFromJob(job)
		detectedConfig := BuildConfigFromDetection(preBuildResult.Detection)
		config := MergeConfigs(&userConfig, detectedConfig, e.logger)

		// Log if user config overrides detection
		if userConfig.CGOEnabled != nil && detectedConfig != nil && detectedConfig.CGOEnabled != nil {
			if *userConfig.CGOEnabled != *detectedConfig.CGOEnabled {
				logCallback(fmt.Sprintf("User CGO setting (%v) overrides detected setting (%v)",
					*userConfig.CGOEnabled, *detectedConfig.CGOEnabled))
			}
		}

		// Create build context for ldflags variable substitution
		buildCtx := e.createBuildContext(job)

		// Use the commit SHA from pre-build if available
		if preBuildResult.CommitSHA != "" {
			buildCtx.Commit = preBuildResult.CommitSHA
		}

		flakeContent, err := e.GenerateFlakeWithContext(ctx, preBuildResult.Detection, *config, buildCtx)
		if err != nil {
			// Clean up cloned repo on failure
			if preBuildResult != nil && preBuildResult.RepoPath != "" {
				os.RemoveAll(filepath.Dir(preBuildResult.RepoPath))
			}
			return &BuildResult{Logs: logs}, err
		}

		job.GeneratedFlake = flakeContent
		e.logger.Info("generated flake for job",
			"job_id", job.ID,
			"flake_length", len(flakeContent),
			"has_generated_flake_after", job.GeneratedFlake != "",
		)
		logCallback("Generated flake.nix successfully")
	}

	// Calculate vendor hash if not already set
	if job.VendorHash == "" {
		logCallback("=== Calculating vendor hash ===")
		// Note: In a real implementation, we'd need the repo path
		// For now, we'll use a placeholder approach
		logCallback("Vendor hash calculation would happen here")
	}

	// Execute the build based on build type
	// **Validates: Requirements 4.2** - Reuse cloned repo path for build container
	// Pass the pre-cloned repo path to the job so the build container can reuse it
	if preBuildResult != nil && preBuildResult.RepoPath != "" && !preBuildResult.CacheHit {
		job.PreClonedRepoPath = preBuildResult.RepoPath
		logCallback(fmt.Sprintf("Reusing pre-cloned repository at %s", preBuildResult.RepoPath))
	}

	var buildResult *BuildResult
	var buildErr error

	switch job.BuildType {
	case models.BuildTypePureNix:
		buildResult, buildErr = e.buildPureNix(ctx, job, logCallback, &logs)
	case models.BuildTypeOCI:
		buildResult, buildErr = e.buildOCI(ctx, job, logCallback, &logs)
	default:
		buildErr = fmt.Errorf("%w: unknown build type %s", ErrBuildFailed, job.BuildType)
	}

	// Clean up cloned repo after build completes
	if preBuildResult != nil && preBuildResult.RepoPath != "" {
		os.RemoveAll(filepath.Dir(preBuildResult.RepoPath))
	}

	// Clear the pre-cloned repo path after build (it's been cleaned up)
	job.PreClonedRepoPath = ""

	if buildErr != nil {
		return &BuildResult{Logs: logs}, buildErr
	}

	return buildResult, nil
}

// createBuildContext creates a build context for ldflags variable substitution.
// **Validates: Requirements 18.3**
func (e *AutoGoStrategyExecutor) createBuildContext(job *models.BuildJob) validation.LdflagsBuildContext {
	ctx := validation.LdflagsBuildContext{
		Version:   "0.0.0-dev",
		Commit:    "unknown",
		BuildTime: time.Now(),
	}

	// Try to extract version from job if available
	if job.BuildConfig != nil && job.BuildConfig.EnvironmentVars != nil {
		if version, ok := job.BuildConfig.EnvironmentVars["VERSION"]; ok && version != "" {
			ctx.Version = version
		}
	}

	// Try to extract commit from git ref
	if job.GitRef != "" {
		// Use the git ref as commit (could be a branch, tag, or commit hash)
		ctx.Commit = job.GitRef
	}

	return ctx
}

// PreBuild performs the pre-build phase: clone and detect.
// It returns the cloned repo path and detection results.
// **Validates: Requirements 1.1, 1.2, 4.3**
func (e *AutoGoStrategyExecutor) PreBuild(ctx context.Context, job *models.BuildJob) (*PreBuildResult, error) {
	result := &PreBuildResult{}

	// First, try to get a cached detection result if we have a commit SHA
	// **Validates: Requirements 4.3** - Cache detection results by commit SHA
	if e.detectionCache != nil && job.GitRef != "" {
		cachedResult, found := e.detectionCache.Get(ctx, job.GitURL, job.GitRef)
		if found {
			if e.logger != nil {
				e.logger.Info("detection cache hit, skipping clone for detection",
					"job_id", job.ID,
					"git_url", job.GitURL,
					"git_ref", job.GitRef,
				)
			}
			result.Detection = cachedResult
			result.CommitSHA = job.GitRef
			result.CacheHit = true
			// Note: RepoPath will be empty for cache hits - the build phase will still clone
			// This is intentional: we skip clone only for detection, not for the actual build
			return result, nil
		}
	}

	// Create a temporary directory for the cloned repository
	tempDir, err := os.MkdirTemp("", "prebuild-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	repoPath := filepath.Join(tempDir, "repo")

	// Clone the repository
	// **Validates: Requirements 1.1** - Clone repository before generating the flake
	if e.logger != nil {
		e.logger.Info("cloning repository for pre-build detection",
			"job_id", job.ID,
			"git_url", job.GitURL,
			"git_ref", job.GitRef,
		)
	}

	cloneStart := time.Now()
	cloneResult, err := clone.Repository(ctx, job.GitURL, job.GitRef, repoPath)
	result.CloneDuration = time.Since(cloneStart)

	if err != nil {
		// Clean up temp directory on clone failure
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("%w: %v", ErrDetectionFailed, err)
	}

	result.RepoPath = cloneResult.RepoPath
	result.CommitSHA = cloneResult.CommitSHA

	if e.logger != nil {
		e.logger.Info("repository cloned successfully",
			"job_id", job.ID,
			"commit_sha", result.CommitSHA,
			"clone_duration", result.CloneDuration,
		)
	}

	// Run detection on the cloned repository
	// **Validates: Requirements 1.2** - Run the full detection pipeline including CGO detection
	if e.logger != nil {
		e.logger.Info("running detection on cloned repository",
			"job_id", job.ID,
			"repo_path", result.RepoPath,
		)
	}

	detectionStart := time.Now()
	detection, err := e.detector.DetectGo(ctx, result.RepoPath)
	result.DetectionDuration = time.Since(detectionStart)

	if err != nil {
		// Clean up temp directory on detection failure
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("%w: %v", ErrDetectionFailed, err)
	}

	// If detection returned nil (not a Go project), return an error
	if detection == nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("%w: repository is not a Go project", ErrDetectionFailed)
	}

	result.Detection = detection

	// Store detection result in cache for future builds
	// **Validates: Requirements 4.3** - Cache detection results by commit SHA
	if e.detectionCache != nil && result.CommitSHA != "" {
		if err := e.detectionCache.Set(ctx, job.GitURL, result.CommitSHA, detection); err != nil {
			// Log warning but don't fail the build
			if e.logger != nil {
				e.logger.Warn("failed to cache detection result",
					"job_id", job.ID,
					"error", err,
				)
			}
		} else if e.logger != nil {
			e.logger.Info("cached detection result",
				"job_id", job.ID,
				"git_url", job.GitURL,
				"commit_sha", result.CommitSHA,
			)
		}
	}

	if e.logger != nil {
		e.logger.Info("detection completed successfully",
			"job_id", job.ID,
			"strategy", detection.Strategy,
			"framework", detection.Framework,
			"version", detection.Version,
			"cgo_enabled", detection.SuggestedConfig["cgo_enabled"],
			"entry_points", detection.EntryPoints,
			"detection_duration", result.DetectionDuration,
		)
	}

	return result, nil
}

// detectFromJob performs Go detection based on job information.
// Deprecated: Use PreBuild instead for proper clone-then-detect workflow.
func (e *AutoGoStrategyExecutor) detectFromJob(ctx context.Context, job *models.BuildJob) (*models.DetectionResult, error) {
	// In a real implementation, we'd clone the repo and detect
	// For now, return a basic detection result
	return &models.DetectionResult{
		Strategy:             models.BuildStrategyAutoGo,
		Framework:            models.FrameworkGeneric,
		Version:              "1.21",
		Confidence:           0.9,
		RecommendedBuildType: models.BuildTypePureNix,
	}, nil
}

// getConfigFromJob extracts build config from the job.
func (e *AutoGoStrategyExecutor) getConfigFromJob(job *models.BuildJob) models.BuildConfig {
	if job.BuildConfig != nil {
		return *job.BuildConfig
	}
	return models.BuildConfig{}
}

// buildPureNix executes a pure-nix build.
func (e *AutoGoStrategyExecutor) buildPureNix(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
	result, err := e.nixBuilder.BuildWithLogCallback(ctx, job, logCallback)
	if err != nil {
		return &BuildResult{
			Logs: *logs,
		}, fmt.Errorf("%w: %v", ErrBuildFailed, err)
	}

	return &BuildResult{
		Artifact:  result.StorePath,
		StorePath: result.StorePath,
		Logs:      *logs,
	}, nil
}

// buildOCI executes an OCI build.
func (e *AutoGoStrategyExecutor) buildOCI(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
	result, err := e.ociBuilder.BuildWithLogCallback(ctx, job, logCallback)
	if err != nil {
		return &BuildResult{
			Logs: *logs,
		}, fmt.Errorf("%w: %v", ErrBuildFailed, err)
	}

	return &BuildResult{
		Artifact:  result.ImageTag,
		ImageTag:  result.ImageTag,
		StorePath: result.StorePath,
		Logs:      *logs,
	}, nil
}

// getAppName extracts the application name from detection result.
func getAppName(detection *models.DetectionResult) string {
	if detection == nil {
		return "app"
	}
	// Use first entry point as app name if available
	if len(detection.EntryPoints) > 0 {
		return filepath.Base(detection.EntryPoints[0])
	}
	return "app"
}

// ValidateGoModExists checks if a go.mod exists in the given repository path.
func ValidateGoModExists(repoPath string) error {
	goModPath := filepath.Join(repoPath, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		return fmt.Errorf("go.mod not found in %s", repoPath)
	}
	return nil
}
