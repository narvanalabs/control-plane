package executor

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/narvanalabs/control-plane/internal/builder/detector"
	"github.com/narvanalabs/control-plane/internal/builder/templates"
	"github.com/narvanalabs/control-plane/internal/models"
)

// AutoDatabaseStrategyExecutor executes builds for database services by generating a flake.nix.
type AutoDatabaseStrategyExecutor struct {
	detector       detector.Detector
	templateEngine templates.TemplateEngine
	nixBuilder     NixBuilder
	ociBuilder     OCIBuilder
	logger         *slog.Logger
}

// NewAutoDatabaseStrategyExecutor creates a new AutoDatabaseStrategyExecutor.
func NewAutoDatabaseStrategyExecutor(
	det detector.Detector,
	tmplEngine templates.TemplateEngine,
	nixBuilder NixBuilder,
	ociBuilder OCIBuilder,
	logger *slog.Logger,
) *AutoDatabaseStrategyExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &AutoDatabaseStrategyExecutor{
		detector:       det,
		templateEngine: tmplEngine,
		nixBuilder:     nixBuilder,
		ociBuilder:     ociBuilder,
		logger:         logger,
	}
}

// Supports returns true if this executor handles the given strategy.
func (e *AutoDatabaseStrategyExecutor) Supports(strategy models.BuildStrategy) bool {
	return strategy == models.BuildStrategyAutoDatabase
}

// GenerateFlake generates a flake.nix for a database service.
func (e *AutoDatabaseStrategyExecutor) GenerateFlake(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	// Prepare template data
	data := templates.TemplateData{
		AppName:         getAppName(detection),
		System:          models.GetCurrentSystem(),
		Config:          config,
		DetectionResult: detection,
	}

	// Priority 1: Use config from build job
	if config.DatabaseOptions != nil {
		data.DatabaseType = config.DatabaseOptions.Type
		data.DatabaseVersion = config.DatabaseOptions.Version
	}

	// Priority 2: Use detection result if config is missing
	if data.DatabaseType == "" && detection.SuggestedConfig != nil {
		if dbType, ok := detection.SuggestedConfig["database_type"].(string); ok {
			data.DatabaseType = dbType
		}
		if dbVersion, ok := detection.SuggestedConfig["database_version"].(string); ok {
			data.DatabaseVersion = dbVersion
		}
	}

	// Render the template
	flakeContent, err := e.templateEngine.Render(ctx, "database.nix", data)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplateRenderFailed, err)
	}

	return flakeContent, nil
}

// Execute runs the build for a database service.
func (e *AutoDatabaseStrategyExecutor) Execute(ctx context.Context, job *models.BuildJob) (*BuildResult, error) {
	return e.ExecuteWithLogs(ctx, job, nil)
}

// ExecuteWithLogs runs the build for a database service with real-time log streaming.
func (e *AutoDatabaseStrategyExecutor) ExecuteWithLogs(ctx context.Context, job *models.BuildJob, externalCallback LogCallback) (*BuildResult, error) {
	e.logger.Info("executing auto-database strategy",
		"job_id", job.ID,
		"build_type", job.BuildType,
	)

	var logs string
	logCallback := func(line string) {
		logs += line + "\n"
		if externalCallback != nil {
			externalCallback(line)
		}
	}

	// Generate the flake if missing
	if job.GeneratedFlake == "" {
		logCallback("=== Preparing database build ===")
		
		// Use config from job if available
		dbType := "postgres" // Default to PostgreSQL
		dbVersion := "15"    // Default PostgreSQL version
		if job.BuildConfig != nil && job.BuildConfig.DatabaseOptions != nil {
			if job.BuildConfig.DatabaseOptions.Type != "" {
				dbType = job.BuildConfig.DatabaseOptions.Type
			}
			// Set default version if not provided
			if job.BuildConfig.DatabaseOptions.Version != "" {
				dbVersion = job.BuildConfig.DatabaseOptions.Version
			}
		}

		detection := &models.DetectionResult{
			Strategy: models.BuildStrategyAutoDatabase,
			SuggestedConfig: map[string]interface{}{
				"database_type":    dbType,
				"database_version": dbVersion,
			},
		}
		
		// If entry points or other info is needed, we could add it here
		
		logCallback("=== Generating flake.nix ===")
		config := models.BuildConfig{}
		if job.BuildConfig != nil {
			config = *job.BuildConfig
		}
		
		flakeContent, err := e.GenerateFlake(ctx, detection, config)
		if err != nil {
			return &BuildResult{Logs: logs}, err
		}
		
		job.GeneratedFlake = flakeContent
		logCallback("Generated flake.nix successfully")
	}

	// Execute build
	switch job.BuildType {
	case models.BuildTypePureNix:
		return e.buildPureNix(ctx, job, logCallback, &logs)
	case models.BuildTypeOCI:
		return e.buildOCI(ctx, job, logCallback, &logs)
	default:
		return nil, fmt.Errorf("%w: unknown build type %s", ErrBuildFailed, job.BuildType)
	}
}

func (e *AutoDatabaseStrategyExecutor) buildPureNix(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
	result, err := e.nixBuilder.BuildWithLogCallback(ctx, job, logCallback)
	if err != nil {
		return &BuildResult{Logs: *logs}, fmt.Errorf("%w: %v", ErrBuildFailed, err)
	}
	return &BuildResult{
		Artifact:  result.StorePath,
		StorePath: result.StorePath,
		Logs:      *logs,
	}, nil
}

func (e *AutoDatabaseStrategyExecutor) buildOCI(ctx context.Context, job *models.BuildJob, logCallback func(string), logs *string) (*BuildResult, error) {
	result, err := e.ociBuilder.BuildWithLogCallback(ctx, job, logCallback)
	if err != nil {
		return &BuildResult{Logs: *logs}, fmt.Errorf("%w: %v", ErrBuildFailed, err)
	}
	return &BuildResult{
		Artifact:  result.ImageTag,
		ImageTag:  result.ImageTag,
		StorePath: result.StorePath,
		Logs:      *logs,
	}, nil
}

// Note: getAppName is shared in executor package if I defined it in auto_go.go but I didn't see it exported.
// I'll redefine it or check if it's available.
// In auto_go.go it was: func getAppName(detection *models.DetectionResult) string
// It's in the same package, so I can use it.
