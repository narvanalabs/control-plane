// Package handlers provides HTTP request handlers for the API.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/builder/detector"
	"github.com/narvanalabs/control-plane/internal/builder/templates"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// PreviewHandler handles build preview HTTP requests.
type PreviewHandler struct {
	store          store.Store
	detector       detector.Detector
	templateEngine templates.TemplateEngine
	logger         *slog.Logger
}

// NewPreviewHandler creates a new preview handler.
func NewPreviewHandler(st store.Store, logger *slog.Logger) (*PreviewHandler, error) {
	tmplEngine, err := templates.NewTemplateEngine()
	if err != nil {
		return nil, fmt.Errorf("failed to create template engine: %w", err)
	}

	return &PreviewHandler{
		store:          st,
		detector:       detector.NewDetector(),
		templateEngine: tmplEngine,
		logger:         logger,
	}, nil
}

// NewPreviewHandlerWithDeps creates a new preview handler with custom dependencies.
func NewPreviewHandlerWithDeps(st store.Store, det detector.Detector, tmplEngine templates.TemplateEngine, logger *slog.Logger) *PreviewHandler {
	return &PreviewHandler{
		store:          st,
		detector:       det,
		templateEngine: tmplEngine,
		logger:         logger,
	}
}


// PreviewRequest represents the request body for build preview.
type PreviewRequest struct {
	// Optional: Override build strategy (if not specified, uses service's configured strategy)
	BuildStrategy *models.BuildStrategy `json:"build_strategy,omitempty"`
	// Optional: Override build config
	BuildConfig *models.BuildConfig `json:"build_config,omitempty"`
}

// PreviewResponse represents the response for build preview.
type PreviewResponse struct {
	// Generated flake.nix content (empty for flake/dockerfile/nixpacks strategies)
	GeneratedFlake string `json:"generated_flake,omitempty"`
	// The build strategy that will be used
	Strategy models.BuildStrategy `json:"strategy"`
	// The build type that will be used
	BuildType models.BuildType `json:"build_type"`
	// Estimated build time in seconds
	EstimatedBuildTime int `json:"estimated_build_time"`
	// Estimated resource usage
	EstimatedResources EstimatedResources `json:"estimated_resources"`
	// Detection result if auto-detection was performed
	Detection *DetectionSummary `json:"detection,omitempty"`
	// Warnings about the build configuration
	Warnings []string `json:"warnings,omitempty"`
	// Whether the flake syntax is valid (only for generated flakes)
	FlakeValid *bool `json:"flake_valid,omitempty"`
	// Validation error message if flake is invalid
	ValidationError string `json:"validation_error,omitempty"`
}

// EstimatedResources represents estimated resource usage for a build.
type EstimatedResources struct {
	// Estimated memory usage in MB
	MemoryMB int `json:"memory_mb"`
	// Estimated disk usage in MB
	DiskMB int `json:"disk_mb"`
	// Estimated CPU cores needed
	CPUCores float64 `json:"cpu_cores"`
}

// DetectionSummary provides a summary of detection results.
type DetectionSummary struct {
	Framework  models.Framework `json:"framework"`
	Version    string           `json:"version,omitempty"`
	EntryPoint string           `json:"entry_point,omitempty"`
	Confidence float64          `json:"confidence"`
}

// PreviewErrorResponse represents an error response for preview failures.
type PreviewErrorResponse struct {
	Error       string   `json:"error"`
	Code        string   `json:"code"`
	Suggestions []string `json:"suggestions,omitempty"`
}


// Preview handles POST /v1/apps/{appID}/services/{serviceName}/preview - generates a build preview.
func (h *PreviewHandler) Preview(w http.ResponseWriter, r *http.Request) {
	// Get app ID and service name from URL
	appID := middleware.GetResolvedAppID(r.Context())
	if appID == "" {
		appID = chi.URLParam(r, "appID")
	}
	serviceName := chi.URLParam(r, "serviceName")

	if appID == "" {
		WriteBadRequest(w, "Application ID is required")
		return
	}
	if serviceName == "" {
		WriteBadRequest(w, "Service name is required")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

	// Parse optional request body
	var req PreviewRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteBadRequest(w, "Invalid request body")
			return
		}
	}

	// Get the app
	app, err := h.store.Apps().Get(r.Context(), appID)
	if err != nil {
		WriteNotFound(w, "Application not found")
		return
	}

	// Verify ownership
	if app.OwnerID != userID {
		WriteForbidden(w, "Access denied")
		return
	}

	// Find the service
	var service *models.ServiceConfig
	for i := range app.Services {
		if app.Services[i].Name == serviceName {
			service = &app.Services[i]
			break
		}
	}

	if service == nil {
		WriteNotFound(w, "Service not found")
		return
	}

	// Generate preview
	response, err := h.generatePreview(r.Context(), service, &req)
	if err != nil {
		h.logger.Error("failed to generate preview", "error", err, "service", serviceName)
		h.writePreviewError(w, err)
		return
	}

	h.logger.Info("preview generated",
		"app_id", appID,
		"service_name", serviceName,
		"strategy", response.Strategy,
	)

	WriteJSON(w, http.StatusOK, response)
}


// generatePreview generates a build preview for a service.
func (h *PreviewHandler) generatePreview(ctx context.Context, service *models.ServiceConfig, req *PreviewRequest) (*PreviewResponse, error) {
	// Determine the build strategy to use
	strategy := service.BuildStrategy
	if req.BuildStrategy != nil {
		strategy = *req.BuildStrategy
	}
	if strategy == "" {
		strategy = models.BuildStrategyFlake
	}

	// Merge build configs (request overrides service config)
	config := h.mergeConfigs(service.BuildConfig, req.BuildConfig)

	response := &PreviewResponse{
		Strategy: strategy,
		Warnings: []string{},
	}

	// Determine build type based on strategy
	response.BuildType = h.determineBuildType(strategy, config)

	// For strategies that don't generate flakes, return early with estimates
	switch strategy {
	case models.BuildStrategyFlake:
		response.Warnings = append(response.Warnings, "Using existing flake.nix from repository - no preview available")
		response.EstimatedBuildTime = h.estimateBuildTime(strategy, nil)
		response.EstimatedResources = h.estimateResources(strategy, nil)
		return response, nil

	case models.BuildStrategyDockerfile:
		response.Warnings = append(response.Warnings, "Using Dockerfile from repository - no flake preview available")
		response.EstimatedBuildTime = h.estimateBuildTime(strategy, nil)
		response.EstimatedResources = h.estimateResources(strategy, nil)
		return response, nil

	case models.BuildStrategyNixpacks:
		response.Warnings = append(response.Warnings, "Using Nixpacks for automatic build - no flake preview available")
		response.EstimatedBuildTime = h.estimateBuildTime(strategy, nil)
		response.EstimatedResources = h.estimateResources(strategy, nil)
		return response, nil
	}

	// For auto-* strategies, we need to detect and generate a flake
	detection, err := h.detectRepository(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("detection failed: %w", err)
	}

	// If strategy is "auto", use the detected strategy
	if strategy == models.BuildStrategyAuto {
		strategy = detection.Strategy
		response.Strategy = strategy
		response.BuildType = h.determineBuildType(strategy, config)
	}

	// Add detection summary
	response.Detection = &DetectionSummary{
		Framework:  detection.Framework,
		Version:    detection.Version,
		Confidence: detection.Confidence,
	}
	if len(detection.EntryPoints) > 0 {
		response.Detection.EntryPoint = detection.EntryPoints[0]
	}

	// Generate the flake
	flakeContent, err := h.generateFlake(ctx, strategy, detection, config)
	if err != nil {
		return nil, fmt.Errorf("flake generation failed: %w", err)
	}

	response.GeneratedFlake = flakeContent

	// Validate the generated flake syntax
	valid, validationErr := h.validateFlakeSyntax(flakeContent)
	response.FlakeValid = &valid
	if validationErr != "" {
		response.ValidationError = validationErr
		response.Warnings = append(response.Warnings, "Generated flake has syntax issues - review before building")
	}

	// Add estimates
	response.EstimatedBuildTime = h.estimateBuildTime(strategy, detection)
	response.EstimatedResources = h.estimateResources(strategy, detection)

	// Add warnings from detection
	response.Warnings = append(response.Warnings, detection.Warnings...)

	return response, nil
}


// detectRepository detects the build strategy for a service's repository.
func (h *PreviewHandler) detectRepository(ctx context.Context, service *models.ServiceConfig) (*models.DetectionResult, error) {
	// Only git sources can be detected
	if service.GitRepo == "" {
		return nil, fmt.Errorf("service does not have a git repository configured")
	}

	// Create temporary directory for cloning
	tempDir, err := os.MkdirTemp("", "preview-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Clone the repository
	cloneCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if err := h.cloneRepository(cloneCtx, service.GitRepo, service.GitRef, tempDir); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Run detection
	return h.detector.Detect(ctx, tempDir)
}

// cloneRepository clones a git repository to the specified directory.
func (h *PreviewHandler) cloneRepository(ctx context.Context, gitURL, gitRef, destDir string) error {
	// Normalize git URL for cloning
	cloneURL := h.normalizeGitURL(gitURL)

	// Default to main if no ref specified
	if gitRef == "" {
		gitRef = "main"
	}

	// Clone with depth 1 for faster cloning
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", gitRef, cloneURL, destDir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try without branch specification (for default branch)
		cmd = exec.CommandContext(ctx, "git", "clone", "--depth", "1", cloneURL, destDir)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git clone failed: %s: %w", string(output), err)
		}
	}

	return nil
}

// normalizeGitURL normalizes a git URL for cloning.
func (h *PreviewHandler) normalizeGitURL(url string) string {
	// If already a full URL, return as-is
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "git@") {
		return url
	}
	// Convert shorthand to https URL
	return "https://" + url
}


// generateFlake generates a flake.nix for the given strategy and detection result.
func (h *PreviewHandler) generateFlake(ctx context.Context, strategy models.BuildStrategy, detection *models.DetectionResult, config models.BuildConfig) (string, error) {
	// Determine template name based on strategy
	templateName := templates.GetTemplateForStrategy(strategy, config)
	if templateName == "" {
		return "", fmt.Errorf("no template available for strategy: %s", strategy)
	}

	// Prepare template data
	data := templates.TemplateData{
		AppName:         h.getAppName(detection),
		Version:         detection.Version,
		Framework:       detection.Framework,
		EntryPoint:      config.EntryPoint,
		BuildCommand:    config.BuildCommand,
		StartCommand:    config.StartCommand,
		Config:          config,
		DetectionResult: detection,
	}

	// If entry point not specified in config, use first detected entry point
	if data.EntryPoint == "" && len(detection.EntryPoints) > 0 {
		data.EntryPoint = detection.EntryPoints[0]
	}

	// Render the template
	return h.templateEngine.Render(ctx, templateName, data)
}

// getAppName extracts the application name from detection result.
func (h *PreviewHandler) getAppName(detection *models.DetectionResult) string {
	if detection == nil {
		return "app"
	}
	// Use first entry point as app name if available
	if len(detection.EntryPoints) > 0 {
		parts := strings.Split(detection.EntryPoints[0], "/")
		return parts[len(parts)-1]
	}
	return "app"
}

// mergeConfigs merges two build configs, with override taking precedence.
func (h *PreviewHandler) mergeConfigs(base, override *models.BuildConfig) models.BuildConfig {
	if base == nil && override == nil {
		return models.BuildConfig{}
	}
	if base == nil {
		return *override
	}
	if override == nil {
		return *base
	}

	// Start with base config
	result := *base

	// Override with non-empty values from override
	if override.BuildCommand != "" {
		result.BuildCommand = override.BuildCommand
	}
	if override.StartCommand != "" {
		result.StartCommand = override.StartCommand
	}
	if override.EntryPoint != "" {
		result.EntryPoint = override.EntryPoint
	}
	if override.BuildTimeout > 0 {
		result.BuildTimeout = override.BuildTimeout
	}
	if override.GoVersion != "" {
		result.GoVersion = override.GoVersion
	}
	if override.CGOEnabled {
		result.CGOEnabled = override.CGOEnabled
	}
	if override.NodeVersion != "" {
		result.NodeVersion = override.NodeVersion
	}
	if override.PackageManager != "" {
		result.PackageManager = override.PackageManager
	}
	if override.RustEdition != "" {
		result.RustEdition = override.RustEdition
	}
	if override.PythonVersion != "" {
		result.PythonVersion = override.PythonVersion
	}
	if override.NextJSOptions != nil {
		result.NextJSOptions = override.NextJSOptions
	}
	if override.DjangoOptions != nil {
		result.DjangoOptions = override.DjangoOptions
	}
	if override.FastAPIOptions != nil {
		result.FastAPIOptions = override.FastAPIOptions
	}
	if len(override.ExtraNixPackages) > 0 {
		result.ExtraNixPackages = override.ExtraNixPackages
	}
	if len(override.EnvironmentVars) > 0 {
		result.EnvironmentVars = override.EnvironmentVars
	}
	if override.AutoRetryAsOCI {
		result.AutoRetryAsOCI = override.AutoRetryAsOCI
	}

	return result
}


// determineBuildType determines the build type based on strategy and config.
func (h *PreviewHandler) determineBuildType(strategy models.BuildStrategy, config models.BuildConfig) models.BuildType {
	// Dockerfile and Nixpacks always produce OCI
	switch strategy {
	case models.BuildStrategyDockerfile, models.BuildStrategyNixpacks:
		return models.BuildTypeOCI
	default:
		// Default to pure-nix for auto-* strategies
		return models.BuildTypePureNix
	}
}

// validateFlakeSyntax performs basic syntax validation on the generated flake.
func (h *PreviewHandler) validateFlakeSyntax(flakeContent string) (bool, string) {
	// Check for balanced braces
	braceCount := 0
	bracketCount := 0
	parenCount := 0

	for _, ch := range flakeContent {
		switch ch {
		case '{':
			braceCount++
		case '}':
			braceCount--
		case '[':
			bracketCount++
		case ']':
			bracketCount--
		case '(':
			parenCount++
		case ')':
			parenCount--
		}

		if braceCount < 0 || bracketCount < 0 || parenCount < 0 {
			return false, "unbalanced brackets"
		}
	}

	if braceCount != 0 {
		return false, fmt.Sprintf("unbalanced braces (count: %d)", braceCount)
	}
	if bracketCount != 0 {
		return false, fmt.Sprintf("unbalanced square brackets (count: %d)", bracketCount)
	}
	if parenCount != 0 {
		return false, fmt.Sprintf("unbalanced parentheses (count: %d)", parenCount)
	}

	// Check for required flake structure
	if !strings.Contains(flakeContent, "description") {
		return false, "missing description"
	}
	if !strings.Contains(flakeContent, "inputs") {
		return false, "missing inputs"
	}
	if !strings.Contains(flakeContent, "outputs") {
		return false, "missing outputs"
	}

	return true, ""
}


// estimateBuildTime estimates the build time in seconds based on strategy and detection.
func (h *PreviewHandler) estimateBuildTime(strategy models.BuildStrategy, detection *models.DetectionResult) int {
	// Base estimates in seconds
	baseEstimates := map[models.BuildStrategy]int{
		models.BuildStrategyFlake:      300,  // 5 minutes
		models.BuildStrategyAutoGo:     180,  // 3 minutes
		models.BuildStrategyAutoRust:   600,  // 10 minutes (Rust is slow)
		models.BuildStrategyAutoNode:   240,  // 4 minutes
		models.BuildStrategyAutoPython: 120,  // 2 minutes
		models.BuildStrategyDockerfile: 300,  // 5 minutes
		models.BuildStrategyNixpacks:   360,  // 6 minutes
		models.BuildStrategyAuto:       300,  // 5 minutes default
	}

	estimate := baseEstimates[strategy]
	if estimate == 0 {
		estimate = 300 // Default 5 minutes
	}

	// Adjust based on framework if detected
	if detection != nil {
		switch detection.Framework {
		case models.FrameworkNextJS:
			estimate += 120 // Next.js builds take longer
		case models.FrameworkDjango:
			estimate += 60 // Django with collectstatic
		}
	}

	return estimate
}

// estimateResources estimates resource usage based on strategy and detection.
func (h *PreviewHandler) estimateResources(strategy models.BuildStrategy, detection *models.DetectionResult) EstimatedResources {
	// Base estimates
	resources := EstimatedResources{
		MemoryMB: 2048, // 2GB default
		DiskMB:   5120, // 5GB default
		CPUCores: 2.0,
	}

	// Adjust based on strategy
	switch strategy {
	case models.BuildStrategyAutoRust:
		resources.MemoryMB = 4096 // Rust needs more memory
		resources.DiskMB = 10240  // Rust builds are large
		resources.CPUCores = 4.0
	case models.BuildStrategyAutoNode:
		resources.MemoryMB = 3072 // Node.js can be memory hungry
		resources.DiskMB = 8192  // node_modules can be large
	case models.BuildStrategyDockerfile, models.BuildStrategyNixpacks:
		resources.MemoryMB = 4096 // Container builds need more resources
		resources.DiskMB = 10240
	}

	// Adjust based on framework
	if detection != nil {
		switch detection.Framework {
		case models.FrameworkNextJS:
			resources.MemoryMB += 1024 // Next.js builds are memory intensive
			resources.DiskMB += 2048
		}
	}

	return resources
}


// writePreviewError writes a preview error response.
func (h *PreviewHandler) writePreviewError(w http.ResponseWriter, err error) {
	errStr := err.Error()
	code := "preview_failed"
	suggestions := []string{}

	// Determine error type and provide helpful suggestions
	switch {
	case strings.Contains(errStr, "detection failed"):
		code = "detection_failed"
		suggestions = []string{
			"Ensure the repository contains standard project files (go.mod, package.json, Cargo.toml, etc.)",
			"Try specifying a build strategy explicitly",
			"Use 'dockerfile' or 'nixpacks' strategy for unsupported languages",
		}
	case strings.Contains(errStr, "clone"):
		code = "clone_failed"
		suggestions = []string{
			"Verify the repository URL is correct",
			"Ensure the repository is publicly accessible",
			"Check that the specified branch/ref exists",
		}
	case strings.Contains(errStr, "template"):
		code = "template_failed"
		suggestions = []string{
			"The build strategy may not be supported for this project type",
			"Try using a different build strategy",
			"Check the build configuration for errors",
		}
	case strings.Contains(errStr, "git repository"):
		code = "no_git_repo"
		suggestions = []string{
			"Preview is only available for services with a git repository",
			"Configure a git_repo for this service to enable preview",
		}
	default:
		suggestions = []string{
			"Try specifying a build strategy explicitly",
			"Check the service configuration",
		}
	}

	response := PreviewErrorResponse{
		Error:       errStr,
		Code:        code,
		Suggestions: suggestions,
	}
	WriteJSON(w, http.StatusUnprocessableEntity, response)
}

// PreviewFromDetection generates a preview directly from a detection result.
// This is useful for testing and can be called directly.
func (h *PreviewHandler) PreviewFromDetection(ctx context.Context, detection *models.DetectionResult, config models.BuildConfig) (*PreviewResponse, error) {
	strategy := detection.Strategy
	if strategy == "" || strategy == models.BuildStrategyAuto {
		return nil, fmt.Errorf("detection result must have a specific strategy")
	}

	response := &PreviewResponse{
		Strategy: strategy,
		BuildType: h.determineBuildType(strategy, config),
		Warnings: []string{},
	}

	// Add detection summary
	response.Detection = &DetectionSummary{
		Framework:  detection.Framework,
		Version:    detection.Version,
		Confidence: detection.Confidence,
	}
	if len(detection.EntryPoints) > 0 {
		response.Detection.EntryPoint = detection.EntryPoints[0]
	}

	// Generate the flake
	flakeContent, err := h.generateFlake(ctx, strategy, detection, config)
	if err != nil {
		return nil, fmt.Errorf("flake generation failed: %w", err)
	}

	response.GeneratedFlake = flakeContent

	// Validate the generated flake syntax
	valid, validationErr := h.validateFlakeSyntax(flakeContent)
	response.FlakeValid = &valid
	if validationErr != "" {
		response.ValidationError = validationErr
	}

	// Add estimates
	response.EstimatedBuildTime = h.estimateBuildTime(strategy, detection)
	response.EstimatedResources = h.estimateResources(strategy, detection)

	// Add warnings from detection
	response.Warnings = append(response.Warnings, detection.Warnings...)

	return response, nil
}
