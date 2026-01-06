// Package handlers provides HTTP request handlers for the API.
package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// StatsHandler handles dashboard statistics endpoints.
type StatsHandler struct {
	store  store.Store
	logger *slog.Logger
}

// NewStatsHandler creates a new stats handler.
func NewStatsHandler(st store.Store, logger *slog.Logger) *StatsHandler {
	return &StatsHandler{
		store:  st,
		logger: logger,
	}
}

// DashboardStats represents the statistics returned for the dashboard.
// Requirements: 1.1
type DashboardStats struct {
	ActiveDeployments int               `json:"active_deployments"`
	TotalApps         int               `json:"total_apps"`
	TotalServices     int               `json:"total_services"`
	NodeHealth        NodeHealthSummary `json:"node_health"`
}

// NodeHealthSummary represents the health summary of all nodes.
type NodeHealthSummary struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
}

// GetDashboardStats handles GET /v1/dashboard/stats - returns dashboard statistics.
// Requirements: 1.1
func (h *StatsHandler) GetDashboardStats(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		h.logger.Error("no organization context found")
		WriteInternalError(w, "Organization context required")
		return
	}

	stats, err := h.calculateStats(r.Context(), orgID)
	if err != nil {
		h.logger.Error("failed to calculate dashboard stats", "error", err, "org_id", orgID)
		WriteInternalError(w, "Failed to calculate dashboard statistics")
		return
	}

	WriteJSON(w, http.StatusOK, stats)
}

// calculateStats computes dashboard statistics for the given organization.
// Requirements: 1.1, 1.2, 1.3
func (h *StatsHandler) calculateStats(ctx context.Context, orgID string) (*DashboardStats, error) {
	// Count active deployments (status = "running") - authoritative from models.DeploymentStatusRunning
	// Requirements: 1.2
	activeDeployments, err := h.store.Deployments().CountByStatusAndOrg(ctx, models.DeploymentStatusRunning, orgID)
	if err != nil {
		return nil, err
	}

	// Count total apps for org
	apps, err := h.store.Apps().ListByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	totalApps := len(apps)

	// Count total services across all apps in the org
	totalServices := 0
	for _, app := range apps {
		totalServices += len(app.Services)
	}

	// Calculate node health summary
	// Requirements: 1.3
	nodeHealth, err := h.calculateNodeHealth(ctx)
	if err != nil {
		return nil, err
	}

	return &DashboardStats{
		ActiveDeployments: activeDeployments,
		TotalApps:         totalApps,
		TotalServices:     totalServices,
		NodeHealth:        *nodeHealth,
	}, nil
}

// calculateNodeHealth computes the health summary of all nodes.
// Requirements: 1.3
func (h *StatsHandler) calculateNodeHealth(ctx context.Context) (*NodeHealthSummary, error) {
	nodes, err := h.store.Nodes().List(ctx)
	if err != nil {
		return nil, err
	}

	summary := &NodeHealthSummary{
		Total: len(nodes),
	}

	for _, node := range nodes {
		if node.Healthy {
			summary.Healthy++
		} else {
			summary.Unhealthy++
		}
	}

	return summary, nil
}
