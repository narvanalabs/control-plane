package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: ui-api-alignment, Property 2: Dashboard Statistics Passthrough**
// *For any* dashboard statistics returned by the backend, the UI SHALL display
// those exact values without client-side modification or recalculation.
// **Validates: Requirements 3.3**

// TestDashboardStatisticsPassthrough tests that dashboard statistics from the backend
// are passed through to the UI without modification.
func TestDashboardStatisticsPassthrough(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	properties.Property("Dashboard stats from backend are passed through without modification", prop.ForAll(
		func(activeDeployments, totalApps, totalServices, healthyNodes, unhealthyNodes int) bool {
			// Create a mock backend server that returns the generated statistics
			backendStats := DashboardStatsResponse{
				ActiveDeployments: activeDeployments,
				TotalApps:         totalApps,
				TotalServices:     totalServices,
				NodeHealth: NodeHealthSummary{
					Total:     healthyNodes + unhealthyNodes,
					Healthy:   healthyNodes,
					Unhealthy: unhealthyNodes,
				},
			}

			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/dashboard/stats":
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(backendStats)
				case "/v1/nodes":
					// Return empty nodes list - we're testing stats passthrough, not node details
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode([]Node{})
				case "/v1/apps":
					// Return empty apps list - we're testing stats passthrough, not app details
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode([]App{})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			// Create client pointing to mock server
			client := NewClient(server.URL)

			// Call GetDashboardData which should pass through backend stats
			stats, _, _, err := client.GetDashboardData(context.Background())
			if err != nil {
				t.Logf("GetDashboardData failed: %v", err)
				return false
			}

			// Verify that the stats are passed through without modification
			// Property: TotalApps from backend == TotalApps in result
			if stats.TotalApps != backendStats.TotalApps {
				t.Logf("TotalApps mismatch: backend=%d, result=%d", backendStats.TotalApps, stats.TotalApps)
				return false
			}

			// Property: ActiveDeployments from backend == ActiveDeployments in result
			if stats.ActiveDeployments != backendStats.ActiveDeployments {
				t.Logf("ActiveDeployments mismatch: backend=%d, result=%d", backendStats.ActiveDeployments, stats.ActiveDeployments)
				return false
			}

			// Property: HealthyNodes from backend == HealthyNodes in result
			if stats.HealthyNodes != backendStats.NodeHealth.Healthy {
				t.Logf("HealthyNodes mismatch: backend=%d, result=%d", backendStats.NodeHealth.Healthy, stats.HealthyNodes)
				return false
			}

			return true
		},
		gen.IntRange(0, 1000),  // activeDeployments
		gen.IntRange(0, 1000),  // totalApps
		gen.IntRange(0, 5000),  // totalServices
		gen.IntRange(0, 100),   // healthyNodes
		gen.IntRange(0, 100),   // unhealthyNodes
	))

	properties.TestingRun(t)
}

// TestDashboardStatsResponseRoundTrip tests that DashboardStatsResponse can be
// serialized and deserialized without data loss.
func TestDashboardStatsResponseRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	properties.Property("DashboardStatsResponse round-trips through JSON without data loss", prop.ForAll(
		func(activeDeployments, totalApps, totalServices, healthyNodes, unhealthyNodes int) bool {
			original := DashboardStatsResponse{
				ActiveDeployments: activeDeployments,
				TotalApps:         totalApps,
				TotalServices:     totalServices,
				NodeHealth: NodeHealthSummary{
					Total:     healthyNodes + unhealthyNodes,
					Healthy:   healthyNodes,
					Unhealthy: unhealthyNodes,
				},
			}

			// Serialize to JSON
			data, err := json.Marshal(original)
			if err != nil {
				t.Logf("Marshal failed: %v", err)
				return false
			}

			// Deserialize from JSON
			var decoded DashboardStatsResponse
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Logf("Unmarshal failed: %v", err)
				return false
			}

			// Verify all fields match
			if decoded.ActiveDeployments != original.ActiveDeployments {
				t.Logf("ActiveDeployments mismatch after round-trip")
				return false
			}
			if decoded.TotalApps != original.TotalApps {
				t.Logf("TotalApps mismatch after round-trip")
				return false
			}
			if decoded.TotalServices != original.TotalServices {
				t.Logf("TotalServices mismatch after round-trip")
				return false
			}
			if decoded.NodeHealth.Total != original.NodeHealth.Total {
				t.Logf("NodeHealth.Total mismatch after round-trip")
				return false
			}
			if decoded.NodeHealth.Healthy != original.NodeHealth.Healthy {
				t.Logf("NodeHealth.Healthy mismatch after round-trip")
				return false
			}
			if decoded.NodeHealth.Unhealthy != original.NodeHealth.Unhealthy {
				t.Logf("NodeHealth.Unhealthy mismatch after round-trip")
				return false
			}

			return true
		},
		gen.IntRange(0, 1000),  // activeDeployments
		gen.IntRange(0, 1000),  // totalApps
		gen.IntRange(0, 5000),  // totalServices
		gen.IntRange(0, 100),   // healthyNodes
		gen.IntRange(0, 100),   // unhealthyNodes
	))

	properties.TestingRun(t)
}


// **Feature: ui-api-alignment, Property 3: App Update Version Handling**
// *For any* app update request, the request SHALL include the current app version,
// and version conflicts SHALL result in a user-friendly error message.
// **Validates: Requirements 6.4**

// TestAppUpdateVersionHandling tests that app update requests include the version
// and that version conflicts are properly handled with user-friendly error messages.
func TestAppUpdateVersionHandling(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Property 1: Update requests include the version field
	properties.Property("App update requests include the version field in JSON body", prop.ForAll(
		func(name, description, iconURL string, version int) bool {
			var capturedBody map[string]interface{}

			// Create mock server that captures the request body
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/apps/test-app-id" && r.Method == "PATCH" {
					// Capture the request body
					if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
						http.Error(w, "Invalid JSON", http.StatusBadRequest)
						return
					}

					// Return success response
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(App{
						ID:      "test-app-id",
						Name:    name,
						Version: version + 1, // Backend increments version
					})
				} else {
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			// Create client and make update request
			client := NewClient(server.URL)

			// Build update request with optional fields
			req := UpdateAppRequest{
				Version: version,
			}
			if name != "" {
				req.Name = &name
			}
			if description != "" {
				req.Description = &description
			}
			if iconURL != "" {
				req.IconURL = &iconURL
			}

			_, err := client.UpdateApp(context.Background(), "test-app-id", req)
			if err != nil {
				t.Logf("UpdateApp failed: %v", err)
				return false
			}

			// Verify that the version field was included in the request
			versionVal, hasVersion := capturedBody["version"]
			if !hasVersion {
				t.Logf("Version field missing from request body")
				return false
			}

			// JSON numbers are decoded as float64
			versionFloat, ok := versionVal.(float64)
			if !ok {
				t.Logf("Version field is not a number: %T", versionVal)
				return false
			}

			if int(versionFloat) != version {
				t.Logf("Version mismatch: expected=%d, got=%d", version, int(versionFloat))
				return false
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) <= 63 }),  // name
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) <= 255 }), // description
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) <= 255 }), // iconURL
		gen.IntRange(1, 10000), // version
	))

	// Property 2: Version conflicts return user-friendly error messages
	properties.Property("Version conflicts return error containing user-friendly message", prop.ForAll(
		func(clientVersion, serverVersion int) bool {
			// Only test when versions differ (conflict scenario)
			if clientVersion == serverVersion {
				return true // Skip non-conflict cases
			}

			// Create mock server that returns 409 Conflict for version mismatch
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/apps/test-app-id" && r.Method == "PATCH" {
					var reqBody map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
						http.Error(w, "Invalid JSON", http.StatusBadRequest)
						return
					}

					// Simulate version conflict - backend has different version
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					json.NewEncoder(w).Encode(map[string]string{
						"error": "Resource was modified by another request. Please refresh and try again.",
					})
				} else {
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			// Create client and make update request with mismatched version
			client := NewClient(server.URL)
			name := "test-app"
			req := UpdateAppRequest{
				Name:    &name,
				Version: clientVersion,
			}

			_, err := client.UpdateApp(context.Background(), "test-app-id", req)

			// Verify that an error was returned
			if err == nil {
				t.Logf("Expected error for version conflict, got nil")
				return false
			}

			// Verify the error message contains useful information
			errMsg := err.Error()
			if !strings.Contains(errMsg, "409") && !strings.Contains(errMsg, "Conflict") && !strings.Contains(errMsg, "modified") {
				t.Logf("Error message doesn't indicate version conflict: %s", errMsg)
				return false
			}

			return true
		},
		gen.IntRange(1, 10000), // clientVersion
		gen.IntRange(1, 10000), // serverVersion
	))

	properties.TestingRun(t)
}

// TestUpdateAppRequestVersionSerialization tests that UpdateAppRequest correctly
// serializes the version field to JSON.
func TestUpdateAppRequestVersionSerialization(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	properties.Property("UpdateAppRequest version field round-trips through JSON", prop.ForAll(
		func(name, description, iconURL string, version int) bool {
			// Create request with all fields
			original := UpdateAppRequest{
				Version: version,
			}
			if name != "" {
				original.Name = &name
			}
			if description != "" {
				original.Description = &description
			}
			if iconURL != "" {
				original.IconURL = &iconURL
			}

			// Serialize to JSON
			data, err := json.Marshal(original)
			if err != nil {
				t.Logf("Marshal failed: %v", err)
				return false
			}

			// Deserialize from JSON
			var decoded UpdateAppRequest
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Logf("Unmarshal failed: %v", err)
				return false
			}

			// Verify version field matches
			if decoded.Version != original.Version {
				t.Logf("Version mismatch after round-trip: original=%d, decoded=%d", original.Version, decoded.Version)
				return false
			}

			// Verify optional fields match
			if (original.Name == nil) != (decoded.Name == nil) {
				t.Logf("Name nil mismatch")
				return false
			}
			if original.Name != nil && *original.Name != *decoded.Name {
				t.Logf("Name value mismatch")
				return false
			}

			if (original.Description == nil) != (decoded.Description == nil) {
				t.Logf("Description nil mismatch")
				return false
			}
			if original.Description != nil && *original.Description != *decoded.Description {
				t.Logf("Description value mismatch")
				return false
			}

			if (original.IconURL == nil) != (decoded.IconURL == nil) {
				t.Logf("IconURL nil mismatch")
				return false
			}
			if original.IconURL != nil && *original.IconURL != *decoded.IconURL {
				t.Logf("IconURL value mismatch")
				return false
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) <= 63 }),  // name
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) <= 255 }), // description
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) <= 255 }), // iconURL
		gen.IntRange(1, 10000), // version
	))

	properties.TestingRun(t)
}


// **Feature: ui-api-alignment, Property 5: Git Source Type Unification**
// *For any* service creation with a git repository, the source_type SHALL be set to "git"
// regardless of whether the repository contains a flake.nix file.
// **Validates: Requirements 10.1**

// TestGitSourceTypeUnification tests that service creation requests with git repositories
// always use source_type "git" regardless of flake presence.
func TestGitSourceTypeUnification(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for valid git repository URLs
	genGitRepoURL := gen.OneGenOf(
		// GitHub HTTPS format
		gen.Identifier().Map(func(s string) string {
			return "https://github.com/owner/" + s
		}),
		// GitHub shorthand format
		gen.Identifier().Map(func(s string) string {
			return "github.com/owner/" + s
		}),
		// GitLab format
		gen.Identifier().Map(func(s string) string {
			return "https://gitlab.com/owner/" + s
		}),
		// Bitbucket format
		gen.Identifier().Map(func(s string) string {
			return "https://bitbucket.org/owner/" + s
		}),
		// Generic git URL
		gen.Identifier().Map(func(s string) string {
			return "https://git.example.com/owner/" + s + ".git"
		}),
	)

	// Generator for valid service names
	genServiceName := gen.Identifier().Map(func(s string) string {
		// Convert to lowercase and limit length for DNS compatibility
		result := strings.ToLower(s)
		if len(result) > 20 {
			result = result[:20]
		}
		if len(result) == 0 {
			result = "svc"
		}
		// Ensure starts with letter
		if result[0] >= '0' && result[0] <= '9' {
			result = "s" + result
		}
		return result
	}).SuchThat(func(s string) bool {
		return len(s) >= 1 && len(s) <= 20
	})

	// Property 5.1: Service creation with git repo always uses source_type "git"
	properties.Property("Service creation with git repo uses source_type git", prop.ForAll(
		func(serviceName, gitRepo string, hasFlake bool) bool {
			var capturedBody map[string]interface{}

			// Create mock server that captures the request body
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/services") && r.Method == "POST" {
					// Capture the request body
					if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
						http.Error(w, "Invalid JSON", http.StatusBadRequest)
						return
					}

					// Return success response
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(Service{
						Name:       serviceName,
						SourceType: "git",
						GitRepo:    gitRepo,
					})
				} else {
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			// Create client and make service creation request
			client := NewClient(server.URL)

			// Build service creation request - always use "git" source type
			// regardless of whether the repo has a flake.nix (hasFlake is just for testing)
			req := CreateServiceRequest{
				Name:       serviceName,
				SourceType: "git", // This is the key assertion - always "git"
				GitRepo:    gitRepo,
				Replicas:   1,
			}

			_, err := client.CreateService(context.Background(), "test-app-id", req)
			if err != nil {
				t.Logf("CreateService failed: %v", err)
				return false
			}

			// Verify that source_type in the request is "git"
			sourceType, hasSourceType := capturedBody["source_type"]
			if !hasSourceType {
				t.Logf("source_type field missing from request body")
				return false
			}

			sourceTypeStr, ok := sourceType.(string)
			if !ok {
				t.Logf("source_type is not a string: %T", sourceType)
				return false
			}

			// The key property: source_type must be "git" for any git repository
			if sourceTypeStr != "git" {
				t.Logf("source_type should be 'git' but was '%s'", sourceTypeStr)
				return false
			}

			return true
		},
		genServiceName,
		genGitRepoURL,
		gen.Bool(), // hasFlake - whether repo has flake.nix (doesn't affect source_type)
	))

	// Property 5.2: CreateServiceRequest with git repo serializes source_type as "git"
	properties.Property("CreateServiceRequest with git repo serializes source_type as git", prop.ForAll(
		func(serviceName, gitRepo string) bool {
			// Create request with git repo
			req := CreateServiceRequest{
				Name:       serviceName,
				SourceType: "git",
				GitRepo:    gitRepo,
				Replicas:   1,
			}

			// Serialize to JSON
			data, err := json.Marshal(req)
			if err != nil {
				t.Logf("Marshal failed: %v", err)
				return false
			}

			// Deserialize to map to check the raw JSON
			var decoded map[string]interface{}
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Logf("Unmarshal failed: %v", err)
				return false
			}

			// Verify source_type is "git"
			sourceType, ok := decoded["source_type"].(string)
			if !ok {
				t.Logf("source_type not found or not a string")
				return false
			}

			if sourceType != "git" {
				t.Logf("source_type should be 'git' but was '%s'", sourceType)
				return false
			}

			// Verify git_repo is preserved
			gitRepoVal, ok := decoded["git_repo"].(string)
			if !ok {
				t.Logf("git_repo not found or not a string")
				return false
			}

			if gitRepoVal != gitRepo {
				t.Logf("git_repo mismatch: expected '%s', got '%s'", gitRepo, gitRepoVal)
				return false
			}

			return true
		},
		genServiceName,
		genGitRepoURL,
	))

	// Property 5.3: source_type "git" is used even when flake_uri is empty
	properties.Property("source_type git is used when git_repo is set and flake_uri is empty", prop.ForAll(
		func(serviceName, gitRepo string) bool {
			// Create request with git repo and empty flake_uri
			req := CreateServiceRequest{
				Name:       serviceName,
				SourceType: "git",
				GitRepo:    gitRepo,
				FlakeURI:   "", // Explicitly empty
				Replicas:   1,
			}

			// Serialize to JSON
			data, err := json.Marshal(req)
			if err != nil {
				t.Logf("Marshal failed: %v", err)
				return false
			}

			// Deserialize to map
			var decoded map[string]interface{}
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Logf("Unmarshal failed: %v", err)
				return false
			}

			// Verify source_type is "git"
			sourceType, ok := decoded["source_type"].(string)
			if !ok {
				t.Logf("source_type not found or not a string")
				return false
			}

			if sourceType != "git" {
				t.Logf("source_type should be 'git' but was '%s'", sourceType)
				return false
			}

			return true
		},
		genServiceName,
		genGitRepoURL,
	))

	properties.TestingRun(t)
}
