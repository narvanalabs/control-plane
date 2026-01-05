package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
