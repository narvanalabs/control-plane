package executor

import (
	"log/slog"

	"github.com/narvanalabs/control-plane/internal/models"
)

// MergeConfigs merges user-provided config with detected config.
// User config takes precedence on conflicts.
// **Validates: Requirements 3.1, 3.2, 3.3**
func MergeConfigs(userConfig, detectedConfig *models.BuildConfig, logger *slog.Logger) *models.BuildConfig {
	// If both are nil, return empty config
	if userConfig == nil && detectedConfig == nil {
		return &models.BuildConfig{}
	}

	// If user config is nil, return a copy of detected config
	if userConfig == nil {
		return copyBuildConfig(detectedConfig)
	}

	// If detected config is nil, return a copy of user config
	if detectedConfig == nil {
		return copyBuildConfig(userConfig)
	}

	// Merge configs - user values take precedence
	merged := &models.BuildConfig{}

	// Common options - user takes precedence
	merged.BuildCommand = mergeStringField(userConfig.BuildCommand, detectedConfig.BuildCommand, "build_command", logger)
	merged.StartCommand = mergeStringField(userConfig.StartCommand, detectedConfig.StartCommand, "start_command", logger)
	merged.EntryPoint = mergeStringField(userConfig.EntryPoint, detectedConfig.EntryPoint, "entry_point", logger)
	merged.BuildTimeout = mergeIntField(userConfig.BuildTimeout, detectedConfig.BuildTimeout, "build_timeout", logger)

	// Go-specific
	merged.GoVersion = mergeStringField(userConfig.GoVersion, detectedConfig.GoVersion, "go_version", logger)
	merged.CGOEnabled = mergeBoolPtrField(userConfig.CGOEnabled, detectedConfig.CGOEnabled, "cgo_enabled", logger)
	merged.BuildTags = mergeStringSliceField(userConfig.BuildTags, detectedConfig.BuildTags, "build_tags", logger)
	merged.Ldflags = mergeStringField(userConfig.Ldflags, detectedConfig.Ldflags, "ldflags", logger)

	// Pre/post build hooks
	merged.PreBuildCommands = mergeStringSliceField(userConfig.PreBuildCommands, detectedConfig.PreBuildCommands, "pre_build_commands", logger)
	merged.PostBuildCommands = mergeStringSliceField(userConfig.PostBuildCommands, detectedConfig.PostBuildCommands, "post_build_commands", logger)

	// Go workspace support
	merged.IsWorkspace = mergeBoolField(userConfig.IsWorkspace, detectedConfig.IsWorkspace, "is_workspace", logger)
	merged.WorkspaceModule = mergeStringField(userConfig.WorkspaceModule, detectedConfig.WorkspaceModule, "workspace_module", logger)

	// Node.js-specific
	merged.NodeVersion = mergeStringField(userConfig.NodeVersion, detectedConfig.NodeVersion, "node_version", logger)
	merged.PackageManager = mergeStringField(userConfig.PackageManager, detectedConfig.PackageManager, "package_manager", logger)

	// Rust-specific
	merged.RustEdition = mergeStringField(userConfig.RustEdition, detectedConfig.RustEdition, "rust_edition", logger)

	// Python-specific
	merged.PythonVersion = mergeStringField(userConfig.PythonVersion, detectedConfig.PythonVersion, "python_version", logger)

	// Framework-specific options
	merged.NextJSOptions = mergeNextJSOptions(userConfig.NextJSOptions, detectedConfig.NextJSOptions, logger)
	merged.DjangoOptions = mergeDjangoOptions(userConfig.DjangoOptions, detectedConfig.DjangoOptions, logger)
	merged.FastAPIOptions = mergeFastAPIOptions(userConfig.FastAPIOptions, detectedConfig.FastAPIOptions, logger)
	merged.DatabaseOptions = mergeDatabaseOptions(userConfig.DatabaseOptions, detectedConfig.DatabaseOptions, logger)

	// Advanced options
	merged.ExtraNixPackages = mergeStringSliceField(userConfig.ExtraNixPackages, detectedConfig.ExtraNixPackages, "extra_nix_packages", logger)
	merged.EnvironmentVars = mergeStringMapField(userConfig.EnvironmentVars, detectedConfig.EnvironmentVars, "environment_vars", logger)

	// Fallback behavior
	merged.AutoRetryAsOCI = mergeBoolField(userConfig.AutoRetryAsOCI, detectedConfig.AutoRetryAsOCI, "auto_retry_as_oci", logger)

	return merged
}


// mergeStringField merges a string field, with user value taking precedence.
// Logs a warning if user overrides a non-empty detected value.
func mergeStringField(userVal, detectedVal, fieldName string, logger *slog.Logger) string {
	if userVal != "" {
		if detectedVal != "" && userVal != detectedVal && logger != nil {
			logger.Warn("user config overrides detected value",
				"field", fieldName,
				"user_value", userVal,
				"detected_value", detectedVal,
			)
		}
		return userVal
	}
	return detectedVal
}

// mergeIntField merges an int field, with user value taking precedence.
// Logs a warning if user overrides a non-zero detected value.
func mergeIntField(userVal, detectedVal int, fieldName string, logger *slog.Logger) int {
	if userVal != 0 {
		if detectedVal != 0 && userVal != detectedVal && logger != nil {
			logger.Warn("user config overrides detected value",
				"field", fieldName,
				"user_value", userVal,
				"detected_value", detectedVal,
			)
		}
		return userVal
	}
	return detectedVal
}

// mergeBoolField merges a bool field, with user value taking precedence.
// Since bool has a zero value of false, we treat any true value as explicitly set.
func mergeBoolField(userVal, detectedVal bool, fieldName string, logger *slog.Logger) bool {
	if userVal {
		if detectedVal != userVal && logger != nil {
			logger.Warn("user config overrides detected value",
				"field", fieldName,
				"user_value", userVal,
				"detected_value", detectedVal,
			)
		}
		return userVal
	}
	return detectedVal
}

// mergeBoolPtrField merges a *bool field, with user value taking precedence.
// nil means "not set", so we can distinguish between explicit false and not set.
// **Validates: Requirements 3.1** - User CGOEnabled overrides detection
func mergeBoolPtrField(userVal, detectedVal *bool, fieldName string, logger *slog.Logger) *bool {
	if userVal != nil {
		if detectedVal != nil && *userVal != *detectedVal && logger != nil {
			logger.Warn("user config overrides detected value",
				"field", fieldName,
				"user_value", *userVal,
				"detected_value", *detectedVal,
			)
		}
		return userVal
	}
	return detectedVal
}

// mergeStringSliceField merges a string slice field, with user value taking precedence.
// If user provides a non-empty slice, it completely replaces the detected value.
func mergeStringSliceField(userVal, detectedVal []string, fieldName string, logger *slog.Logger) []string {
	if len(userVal) > 0 {
		if len(detectedVal) > 0 && logger != nil {
			logger.Warn("user config overrides detected value",
				"field", fieldName,
				"user_value", userVal,
				"detected_value", detectedVal,
			)
		}
		return userVal
	}
	return detectedVal
}

// mergeStringMapField merges a string map field, with user values taking precedence.
// User values override detected values for the same keys.
func mergeStringMapField(userVal, detectedVal map[string]string, fieldName string, logger *slog.Logger) map[string]string {
	if len(userVal) == 0 && len(detectedVal) == 0 {
		return nil
	}

	merged := make(map[string]string)

	// Copy detected values first
	for k, v := range detectedVal {
		merged[k] = v
	}

	// Override with user values
	for k, v := range userVal {
		if existingVal, exists := merged[k]; exists && existingVal != v && logger != nil {
			logger.Warn("user config overrides detected value",
				"field", fieldName,
				"key", k,
				"user_value", v,
				"detected_value", existingVal,
			)
		}
		merged[k] = v
	}

	return merged
}


// mergeNextJSOptions merges NextJS options, with user values taking precedence.
func mergeNextJSOptions(userVal, detectedVal *models.NextJSOptions, logger *slog.Logger) *models.NextJSOptions {
	if userVal == nil && detectedVal == nil {
		return nil
	}
	if userVal == nil {
		return detectedVal
	}
	if detectedVal == nil {
		return userVal
	}

	// Merge individual fields
	merged := &models.NextJSOptions{
		OutputMode:     mergeStringField(userVal.OutputMode, detectedVal.OutputMode, "nextjs_options.output_mode", logger),
		BasePath:       mergeStringField(userVal.BasePath, detectedVal.BasePath, "nextjs_options.base_path", logger),
		AssetPrefix:    mergeStringField(userVal.AssetPrefix, detectedVal.AssetPrefix, "nextjs_options.asset_prefix", logger),
		ImageOptimizer: mergeBoolField(userVal.ImageOptimizer, detectedVal.ImageOptimizer, "nextjs_options.image_optimizer", logger),
	}
	return merged
}

// mergeDjangoOptions merges Django options, with user values taking precedence.
func mergeDjangoOptions(userVal, detectedVal *models.DjangoOptions, logger *slog.Logger) *models.DjangoOptions {
	if userVal == nil && detectedVal == nil {
		return nil
	}
	if userVal == nil {
		return detectedVal
	}
	if detectedVal == nil {
		return userVal
	}

	merged := &models.DjangoOptions{
		SettingsModule: mergeStringField(userVal.SettingsModule, detectedVal.SettingsModule, "django_options.settings_module", logger),
		StaticRoot:     mergeStringField(userVal.StaticRoot, detectedVal.StaticRoot, "django_options.static_root", logger),
		CollectStatic:  mergeBoolField(userVal.CollectStatic, detectedVal.CollectStatic, "django_options.collect_static", logger),
		Migrations:     mergeBoolField(userVal.Migrations, detectedVal.Migrations, "django_options.migrations", logger),
	}
	return merged
}

// mergeFastAPIOptions merges FastAPI options, with user values taking precedence.
func mergeFastAPIOptions(userVal, detectedVal *models.FastAPIOptions, logger *slog.Logger) *models.FastAPIOptions {
	if userVal == nil && detectedVal == nil {
		return nil
	}
	if userVal == nil {
		return detectedVal
	}
	if detectedVal == nil {
		return userVal
	}

	merged := &models.FastAPIOptions{
		AppModule: mergeStringField(userVal.AppModule, detectedVal.AppModule, "fastapi_options.app_module", logger),
		Workers:   mergeIntField(userVal.Workers, detectedVal.Workers, "fastapi_options.workers", logger),
	}
	return merged
}

// mergeDatabaseOptions merges Database options, with user values taking precedence.
func mergeDatabaseOptions(userVal, detectedVal *models.DatabaseOptions, logger *slog.Logger) *models.DatabaseOptions {
	if userVal == nil && detectedVal == nil {
		return nil
	}
	if userVal == nil {
		return detectedVal
	}
	if detectedVal == nil {
		return userVal
	}

	merged := &models.DatabaseOptions{
		Type:    mergeStringField(userVal.Type, detectedVal.Type, "database_options.type", logger),
		Version: mergeStringField(userVal.Version, detectedVal.Version, "database_options.version", logger),
	}
	return merged
}

// copyBuildConfig creates a shallow copy of a BuildConfig.
func copyBuildConfig(config *models.BuildConfig) *models.BuildConfig {
	if config == nil {
		return nil
	}

	copied := *config

	// Copy pointer fields
	if config.CGOEnabled != nil {
		cgo := *config.CGOEnabled
		copied.CGOEnabled = &cgo
	}

	// Copy slice fields
	if config.BuildTags != nil {
		copied.BuildTags = make([]string, len(config.BuildTags))
		copy(copied.BuildTags, config.BuildTags)
	}
	if config.PreBuildCommands != nil {
		copied.PreBuildCommands = make([]string, len(config.PreBuildCommands))
		copy(copied.PreBuildCommands, config.PreBuildCommands)
	}
	if config.PostBuildCommands != nil {
		copied.PostBuildCommands = make([]string, len(config.PostBuildCommands))
		copy(copied.PostBuildCommands, config.PostBuildCommands)
	}
	if config.ExtraNixPackages != nil {
		copied.ExtraNixPackages = make([]string, len(config.ExtraNixPackages))
		copy(copied.ExtraNixPackages, config.ExtraNixPackages)
	}

	// Copy map fields
	if config.EnvironmentVars != nil {
		copied.EnvironmentVars = make(map[string]string)
		for k, v := range config.EnvironmentVars {
			copied.EnvironmentVars[k] = v
		}
	}

	return &copied
}

// BuildConfigFromDetection creates a BuildConfig from a DetectionResult's SuggestedConfig.
// This is used to convert detection results into a format that can be merged with user config.
// **Validates: Requirements 3.2**
func BuildConfigFromDetection(detection *models.DetectionResult) *models.BuildConfig {
	if detection == nil || detection.SuggestedConfig == nil {
		return nil
	}

	config := &models.BuildConfig{}

	// Extract CGO setting
	if cgoEnabled, ok := detection.SuggestedConfig["cgo_enabled"].(bool); ok {
		config.CGOEnabled = &cgoEnabled
	}

	// Extract Go version
	if goVersion, ok := detection.SuggestedConfig["go_version"].(string); ok {
		config.GoVersion = goVersion
	}

	// Extract entry point
	if entryPoint, ok := detection.SuggestedConfig["entry_point"].(string); ok {
		config.EntryPoint = entryPoint
	}

	// Extract Node version
	if nodeVersion, ok := detection.SuggestedConfig["node_version"].(string); ok {
		config.NodeVersion = nodeVersion
	}

	// Extract package manager
	if packageManager, ok := detection.SuggestedConfig["package_manager"].(string); ok {
		config.PackageManager = packageManager
	}

	// Extract Python version
	if pythonVersion, ok := detection.SuggestedConfig["python_version"].(string); ok {
		config.PythonVersion = pythonVersion
	}

	// Extract Rust edition
	if rustEdition, ok := detection.SuggestedConfig["rust_edition"].(string); ok {
		config.RustEdition = rustEdition
	}

	// Extract workspace settings
	if isWorkspace, ok := detection.SuggestedConfig["is_workspace"].(bool); ok {
		config.IsWorkspace = isWorkspace
	}
	if workspaceModule, ok := detection.SuggestedConfig["workspace_module"].(string); ok {
		config.WorkspaceModule = workspaceModule
	}

	return config
}
