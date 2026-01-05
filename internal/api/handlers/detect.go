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
	"path/filepath"
	"strings"
	"time"

	"github.com/narvanalabs/control-plane/internal/builder/detector"
	"github.com/narvanalabs/control-plane/internal/models"
)

// DetectHandler handles build strategy detection HTTP requests.
type DetectHandler struct {
	detector detector.Detector
	logger   *slog.Logger
}

// NewDetectHandler creates a new detect handler.
func NewDetectHandler(logger *slog.Logger) *DetectHandler {
	return &DetectHandler{
		detector: detector.NewDetector(),
		logger:   logger,
	}
}

// NewDetectHandlerWithDetector creates a new detect handler with a custom detector.
func NewDetectHandlerWithDetector(d detector.Detector, logger *slog.Logger) *DetectHandler {
	return &DetectHandler{
		detector: d,
		logger:   logger,
	}
}

// DetectRequest represents the request body for detecting build strategy.
type DetectRequest struct {
	GitURL string `json:"git_url"`
	GitRef string `json:"git_ref,omitempty"`
}

// DetectResponse represents the response for build strategy detection.
type DetectResponse struct {
	Strategy             models.BuildStrategy   `json:"strategy"`
	Framework            models.Framework       `json:"framework"`
	Version              string                 `json:"version,omitempty"`
	SuggestedConfig      map[string]interface{} `json:"suggested_config,omitempty"`
	RecommendedBuildType models.BuildType       `json:"recommended_build_type"`
	EntryPoints          []string               `json:"entry_points,omitempty"`
	Confidence           float64                `json:"confidence"`
	Warnings             []string               `json:"warnings,omitempty"`
}

// DetectErrorResponse represents an error response for detection failures.
type DetectErrorResponse struct {
	Error       string   `json:"error"`
	Code        string   `json:"code"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// Detect handles POST /v1/detect - detects build strategy for a repository.
func (h *DetectHandler) Detect(w http.ResponseWriter, r *http.Request) {
	var req DetectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Validate git URL
	if req.GitURL == "" {
		WriteBadRequest(w, "git_url is required")
		return
	}

	if !isValidGitURL(req.GitURL) {
		WriteBadRequest(w, "Invalid git_url format")
		return
	}

	// Default git ref to main
	gitRef := req.GitRef
	if gitRef == "" {
		gitRef = "main"
	}

	// Create temporary directory for cloning
	tempDir, err := os.MkdirTemp("", "detect-*")
	if err != nil {
		h.logger.Error("failed to create temp directory", "error", err)
		WriteInternalError(w, "Failed to create temporary directory")
		return
	}
	defer os.RemoveAll(tempDir)

	// Clone the repository
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	if err := h.cloneRepository(ctx, req.GitURL, gitRef, tempDir); err != nil {
		h.logger.Error("failed to clone repository", "error", err, "url", req.GitURL)
		h.writeDetectionError(w, "Failed to clone repository", "clone_failed", []string{
			"Verify the repository URL is correct",
			"Ensure the repository is publicly accessible or provide authentication",
			"Check that the specified branch/ref exists",
		})
		return
	}

	// Run detection
	result, err := h.detector.Detect(ctx, tempDir)
	if err != nil {
		h.logger.Info("detection failed", "error", err, "url", req.GitURL)
		h.handleDetectionError(w, err)
		return
	}

	// Build response
	response := DetectResponse{
		Strategy:             result.Strategy,
		Framework:            result.Framework,
		Version:              result.Version,
		SuggestedConfig:      result.SuggestedConfig,
		RecommendedBuildType: result.RecommendedBuildType,
		EntryPoints:          result.EntryPoints,
		Confidence:           result.Confidence,
		Warnings:             result.Warnings,
	}

	h.logger.Info("detection completed",
		"url", req.GitURL,
		"strategy", result.Strategy,
		"framework", result.Framework,
	)

	WriteJSON(w, http.StatusOK, response)
}

// cloneRepository clones a git repository to the specified directory.
func (h *DetectHandler) cloneRepository(ctx context.Context, gitURL, gitRef, destDir string) error {
	// Normalize git URL for cloning
	cloneURL := normalizeGitURL(gitURL)

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

// handleDetectionError handles detection errors and writes appropriate responses.
// **Validates: Requirements 19.6**
func (h *DetectHandler) handleDetectionError(w http.ResponseWriter, err error) {
	switch err {
	case detector.ErrNoLanguageDetected:
		h.writeDetectionError(w, "Could not detect application language", "no_language_detected", []string{
			"Try using 'dockerfile' strategy if you have a Dockerfile",
			"Try using 'nixpacks' strategy for automatic detection",
			"Ensure your repository contains standard project files (go.mod, package.json, Cargo.toml, etc.)",
		})
	case detector.ErrMultipleLanguages:
		h.writeDetectionError(w, "Multiple languages detected", "multiple_languages", []string{
			"Specify a build strategy explicitly (auto-go, auto-node, auto-rust, auto-python)",
			"Use 'dockerfile' or 'nixpacks' strategy for multi-language projects",
		})
	case detector.ErrUnsupportedLanguage:
		h.writeDetectionError(w, "Detected language is not supported", "unsupported_language", []string{
			"Try using 'dockerfile' strategy with a custom Dockerfile",
			"Try using 'nixpacks' strategy for automatic detection",
			"Use 'flake' strategy with a custom flake.nix",
		})
	case detector.ErrRepositoryAccessFailed:
		h.writeDetectionError(w, "Failed to access repository", "repository_access_failed", []string{
			"Verify the repository URL is correct",
			"Ensure the repository is publicly accessible",
		})
	case detector.ErrNoEntryPointsFound:
		// **Validates: Requirements 19.6** - Clear error for no entry point found
		h.writeDetectionError(w, "No entry points found in repository", "no_entry_points", []string{
			"For Go: Add a main.go file in the root, cmd/*, apps/*, or services/* directories",
			"For Node.js: Ensure package.json has a 'main' or 'bin' field, or add index.js/server.js",
			"For Python: Add main.py, app.py, or configure entry points in pyproject.toml",
			"For Rust: Add src/main.rs or configure binaries in Cargo.toml",
		})
	default:
		h.writeDetectionError(w, "Detection failed: "+err.Error(), "detection_failed", []string{
			"Try specifying a build strategy explicitly",
			"Check repository structure and project files",
		})
	}
}

// writeDetectionError writes a detection error response.
func (h *DetectHandler) writeDetectionError(w http.ResponseWriter, message, code string, suggestions []string) {
	response := DetectErrorResponse{
		Error:       message,
		Code:        code,
		Suggestions: suggestions,
	}
	WriteJSON(w, http.StatusUnprocessableEntity, response)
}

// isValidGitURL validates a git URL format.
func isValidGitURL(url string) bool {
	// Accept common git URL formats
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		return true
	}
	if strings.HasPrefix(url, "git@") {
		return true
	}
	// Accept shorthand format like github.com/owner/repo
	if strings.Contains(url, "/") && !strings.Contains(url, " ") {
		parts := strings.Split(url, "/")
		if len(parts) >= 2 {
			return true
		}
	}
	return false
}

// normalizeGitURL normalizes a git URL for cloning.
func normalizeGitURL(url string) string {
	// If already a full URL, return as-is
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "git@") {
		return url
	}
	// Convert shorthand to https URL
	return "https://" + url
}

// CloneAndDetect is a helper function that clones a repository and runs detection.
// This is useful for testing and can be called directly.
func (h *DetectHandler) CloneAndDetect(ctx context.Context, gitURL, gitRef string) (*models.DetectionResult, error) {
	// Create temporary directory for cloning
	tempDir, err := os.MkdirTemp("", "detect-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Clone the repository
	if err := h.cloneRepository(ctx, gitURL, gitRef, tempDir); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Run detection
	return h.detector.Detect(ctx, tempDir)
}

// DetectFromPath runs detection on a local path (useful for testing).
func (h *DetectHandler) DetectFromPath(ctx context.Context, repoPath string) (*models.DetectionResult, error) {
	// Verify path exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("path does not exist: %s", repoPath)
	}

	// Ensure it's a directory
	info, err := os.Stat(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", repoPath)
	}

	// Make path absolute
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	return h.detector.Detect(ctx, absPath)
}
