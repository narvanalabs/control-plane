package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/builder/detector"
	"github.com/narvanalabs/control-plane/internal/models"
)

// mockDetector implements detector.Detector for testing.
type mockDetector struct {
	result *models.DetectionResult
	err    error
}

func (m *mockDetector) Detect(ctx context.Context, repoPath string) (*models.DetectionResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func (m *mockDetector) DetectGo(ctx context.Context, repoPath string) (*models.DetectionResult, error) {
	return m.Detect(ctx, repoPath)
}

func (m *mockDetector) DetectNode(ctx context.Context, repoPath string) (*models.DetectionResult, error) {
	return m.Detect(ctx, repoPath)
}

func (m *mockDetector) DetectRust(ctx context.Context, repoPath string) (*models.DetectionResult, error) {
	return m.Detect(ctx, repoPath)
}

func (m *mockDetector) DetectPython(ctx context.Context, repoPath string) (*models.DetectionResult, error) {
	return m.Detect(ctx, repoPath)
}

func (m *mockDetector) HasFlake(ctx context.Context, repoPath string) bool {
	return false
}

func (m *mockDetector) HasDockerfile(ctx context.Context, repoPath string) bool {
	return false
}

// genBuildStrategy generates valid build strategies.
func genBuildStrategy() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStrategyFlake,
		models.BuildStrategyAutoGo,
		models.BuildStrategyAutoRust,
		models.BuildStrategyAutoNode,
		models.BuildStrategyAutoPython,
		models.BuildStrategyDockerfile,
		models.BuildStrategyNixpacks,
	)
}

// genFramework generates valid frameworks.
func genFramework() gopter.Gen {
	return gen.OneConstOf(
		models.FrameworkGeneric,
		models.FrameworkNextJS,
		models.FrameworkExpress,
		models.FrameworkReact,
		models.FrameworkFastify,
		models.FrameworkDjango,
		models.FrameworkFastAPI,
		models.FrameworkFlask,
	)
}

// genBuildType generates valid build types.
func genBuildType() gopter.Gen {
	return gen.OneConstOf(
		models.BuildTypePureNix,
		models.BuildTypeOCI,
	)
}

// genVersion generates valid version strings.
func genVersion() gopter.Gen {
	return gen.OneConstOf(
		"1.21",
		"1.22",
		"18.0.0",
		"20.0.0",
		"3.11",
		"3.12",
		"2021",
		"",
	)
}

// genConfidence generates valid confidence values.
func genConfidence() gopter.Gen {
	return gen.Float64Range(0.0, 1.0)
}

// genEntryPoints generates valid entry point lists.
func genEntryPoints() gopter.Gen {
	return gen.OneConstOf(
		[]string{},
		[]string{"cmd/api"},
		[]string{"cmd/api", "cmd/worker"},
		[]string{"cmd/api", "cmd/worker", "cmd/cli"},
	)
}

// genWarnings generates valid warning lists.
func genWarnings() gopter.Gen {
	return gen.OneConstOf(
		[]string{},
		[]string{"CGO detected"},
		[]string{"Multiple entry points found", "Consider specifying entry_point"},
	)
}

// genSuggestedConfig generates valid suggested config maps.
func genSuggestedConfig() gopter.Gen {
	return gen.OneConstOf(
		map[string]interface{}{},
		map[string]interface{}{"go_version": "1.21"},
		map[string]interface{}{"node_version": "20", "package_manager": "npm"},
		map[string]interface{}{"cgo_enabled": true},
	)
}

// DetectionResultInput represents input for generating detection results.
type DetectionResultInput struct {
	Strategy        models.BuildStrategy
	Framework       models.Framework
	Version         string
	BuildType       models.BuildType
	Confidence      float64
	EntryPoints     []string
	Warnings        []string
	SuggestedConfig map[string]interface{}
}

// genDetectionResultInput generates valid detection result inputs.
func genDetectionResultInput() gopter.Gen {
	return gopter.CombineGens(
		genBuildStrategy(),
		genFramework(),
		genVersion(),
		genBuildType(),
		genConfidence(),
		genEntryPoints(),
		genWarnings(),
		genSuggestedConfig(),
	).Map(func(vals []interface{}) DetectionResultInput {
		return DetectionResultInput{
			Strategy:        vals[0].(models.BuildStrategy),
			Framework:       vals[1].(models.Framework),
			Version:         vals[2].(string),
			BuildType:       vals[3].(models.BuildType),
			Confidence:      vals[4].(float64),
			EntryPoints:     vals[5].([]string),
			Warnings:        vals[6].([]string),
			SuggestedConfig: vals[7].(map[string]interface{}),
		}
	})
}

// **Feature: flexible-build-strategies, Property 13: Detection API Response Completeness**
// *For any* successful detection, the API response SHALL include strategy, framework,
// version, suggested_config, and recommended_build_type fields.
// **Validates: Requirements 9.3**
func TestDetectionAPIResponseCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("successful detection response includes all required fields", prop.ForAll(
		func(input DetectionResultInput) bool {
			// Create a mock detector that returns the generated result
			mockDet := &mockDetector{
				result: &models.DetectionResult{
					Strategy:             input.Strategy,
					Framework:            input.Framework,
					Version:              input.Version,
					RecommendedBuildType: input.BuildType,
					Confidence:           input.Confidence,
					EntryPoints:          input.EntryPoints,
					Warnings:             input.Warnings,
					SuggestedConfig:      input.SuggestedConfig,
				},
			}

			// Create handler with mock detector
			handler := NewDetectHandlerWithDetector(mockDet, logger)

			// Create a temporary directory to simulate a cloned repo
			tempDir := t.TempDir()

			// Test DetectFromPath directly (avoids git clone)
			result, err := handler.DetectFromPath(context.Background(), tempDir)
			if err != nil {
				return false
			}

			// Verify all required fields are present in the result
			// Strategy must be set
			if result.Strategy == "" {
				return false
			}

			// Framework must be set
			if result.Framework == "" {
				return false
			}

			// RecommendedBuildType must be set
			if result.RecommendedBuildType == "" {
				return false
			}

			// Confidence must be in valid range
			if result.Confidence < 0 || result.Confidence > 1 {
				return false
			}

			// Verify the values match what we set
			if result.Strategy != input.Strategy {
				return false
			}
			if result.Framework != input.Framework {
				return false
			}
			if result.Version != input.Version {
				return false
			}
			if result.RecommendedBuildType != input.BuildType {
				return false
			}

			return true
		},
		genDetectionResultInput(),
	))

	properties.TestingRun(t)
}

// TestDetectionAPIResponseJSONCompleteness tests that the JSON response includes all required fields.
func TestDetectionAPIResponseJSONCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("JSON response includes all required fields", prop.ForAll(
		func(input DetectionResultInput) bool {
			// Create a mock detector
			mockDet := &mockDetector{
				result: &models.DetectionResult{
					Strategy:             input.Strategy,
					Framework:            input.Framework,
					Version:              input.Version,
					RecommendedBuildType: input.BuildType,
					Confidence:           input.Confidence,
					EntryPoints:          input.EntryPoints,
					Warnings:             input.Warnings,
					SuggestedConfig:      input.SuggestedConfig,
				},
			}

			// Create handler with mock detector
			handler := &testableDetectHandler{
				DetectHandler: NewDetectHandlerWithDetector(mockDet, logger),
				tempDir:       t.TempDir(),
			}

			// Create router
			r := chi.NewRouter()
			r.Post("/v1/detect", handler.DetectWithMockClone)

			// Create request
			reqBody := DetectRequest{
				GitURL: "github.com/example/repo",
				GitRef: "main",
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest("POST", "/v1/detect", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user")
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				return false
			}

			// Parse response
			var response DetectResponse
			if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
				return false
			}

			// Verify required fields are present
			if response.Strategy == "" {
				return false
			}
			if response.Framework == "" {
				return false
			}
			if response.RecommendedBuildType == "" {
				return false
			}

			// Verify values match input
			if response.Strategy != input.Strategy {
				return false
			}
			if response.Framework != input.Framework {
				return false
			}
			if response.Version != input.Version {
				return false
			}
			if response.RecommendedBuildType != input.BuildType {
				return false
			}

			return true
		},
		genDetectionResultInput(),
	))

	properties.TestingRun(t)
}

// testableDetectHandler wraps DetectHandler for testing without actual git clone.
type testableDetectHandler struct {
	*DetectHandler
	tempDir string
}

// DetectWithMockClone handles detection without actual git clone.
func (h *testableDetectHandler) DetectWithMockClone(w http.ResponseWriter, r *http.Request) {
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

	// Run detection on temp directory (mock clone)
	result, err := h.detector.Detect(r.Context(), h.tempDir)
	if err != nil {
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

	WriteJSON(w, http.StatusOK, response)
}

// TestDetectionErrorResponseFormat tests that error responses have proper format.
func TestDetectionErrorResponseFormat(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	// Generator for detection errors
	genDetectionError := gen.OneConstOf(
		detector.ErrNoLanguageDetected,
		detector.ErrMultipleLanguages,
		detector.ErrUnsupportedLanguage,
		detector.ErrRepositoryAccessFailed,
	)

	properties.Property("error responses include error, code, and suggestions", prop.ForAll(
		func(detErr error) bool {
			// Create a mock detector that returns an error
			mockDet := &mockDetector{
				err: detErr,
			}

			// Create handler with mock detector
			handler := &testableDetectHandler{
				DetectHandler: NewDetectHandlerWithDetector(mockDet, logger),
				tempDir:       t.TempDir(),
			}

			// Create router
			r := chi.NewRouter()
			r.Post("/v1/detect", handler.DetectWithMockClone)

			// Create request
			reqBody := DetectRequest{
				GitURL: "github.com/example/repo",
				GitRef: "main",
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest("POST", "/v1/detect", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			// Should return 422 Unprocessable Entity for detection errors
			if rr.Code != http.StatusUnprocessableEntity {
				return false
			}

			// Parse error response
			var errResponse DetectErrorResponse
			if err := json.NewDecoder(rr.Body).Decode(&errResponse); err != nil {
				return false
			}

			// Verify error response has required fields
			if errResponse.Error == "" {
				return false
			}
			if errResponse.Code == "" {
				return false
			}
			if len(errResponse.Suggestions) == 0 {
				return false
			}

			return true
		},
		genDetectionError,
	))

	properties.TestingRun(t)
}

// TestDetectionWithRealDetector tests detection with real detector on generated repositories.
func TestDetectionWithRealDetector(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	// Generator for Go module content
	genGoModContent := gopter.CombineGens(
		gen.OneConstOf("github.com/example/app", "github.com/test/service"),
		gen.OneConstOf("1.21", "1.22"),
	).Map(func(vals []interface{}) struct {
		Module  string
		Version string
	} {
		return struct {
			Module  string
			Version string
		}{
			Module:  vals[0].(string),
			Version: vals[1].(string),
		}
	})

	properties.Property("Go repositories are detected with complete response", prop.ForAll(
		func(content struct {
			Module  string
			Version string
		}) bool {
			// Create a temporary directory with Go project
			dir := t.TempDir()

			// Create go.mod
			goModContent := "module " + content.Module + "\n\ngo " + content.Version + "\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644); err != nil {
				return false
			}

			// Create main.go
			mainContent := `package main

func main() {
	println("hello")
}
`
			if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainContent), 0644); err != nil {
				return false
			}

			// Create handler with real detector
			handler := NewDetectHandler(logger)

			// Run detection
			result, err := handler.DetectFromPath(context.Background(), dir)
			if err != nil {
				return false
			}

			// Verify required fields
			if result.Strategy == "" {
				return false
			}
			if result.Framework == "" {
				return false
			}
			if result.RecommendedBuildType == "" {
				return false
			}

			// Verify Go-specific detection
			if result.Strategy != models.BuildStrategyAutoGo {
				return false
			}
			if result.Version != content.Version {
				return false
			}

			return true
		},
		genGoModContent,
	))

	properties.TestingRun(t)
}
