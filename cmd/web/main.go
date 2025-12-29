package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/narvanalabs/control-plane/web/pages"
)

func main() {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Static assets
	fs := http.FileServer(http.Dir("web/assets"))
	r.Handle("/assets/*", http.StripPrefix("/assets/", fs))

	// Page routes
	r.Get("/", handleDashboard)
	r.Get("/apps", handleApps)
	r.Get("/apps/new", handleCreateApp)
	r.Get("/apps/{appID}", handleAppDetail)
	r.Get("/deployments", handleDeployments)
	r.Get("/deployments/{deploymentID}", handleDeploymentDetail)
	r.Get("/builds", handleBuilds)
	r.Get("/builds/{buildID}", handleBuildDetail)
	r.Get("/nodes", handleNodes)
	r.Get("/nodes/{nodeID}", handleNodeDetail)
	r.Get("/settings/general", handleSettingsGeneral)
	r.Get("/settings/api-keys", handleSettingsAPIKeys)

	fmt.Println("🚀 Web UI running on http://localhost:8090")
	http.ListenAndServe(":8090", r)
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch real data from API
	data := pages.DashboardData{
		TotalApps:         5,
		ActiveDeployments: 12,
		HealthyNodes:      3,
		RunningBuilds:     2,
	}
	pages.Dashboard(data).Render(r.Context(), w)
}

func handleApps(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement apps list page
	pages.Dashboard(pages.DashboardData{}).Render(r.Context(), w)
}

func handleCreateApp(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement create app page
	pages.Dashboard(pages.DashboardData{}).Render(r.Context(), w)
}

func handleAppDetail(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement app detail page
	pages.Dashboard(pages.DashboardData{}).Render(r.Context(), w)
}

func handleDeployments(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement deployments list page
	pages.Dashboard(pages.DashboardData{}).Render(r.Context(), w)
}

func handleDeploymentDetail(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement deployment detail page
	pages.Dashboard(pages.DashboardData{}).Render(r.Context(), w)
}

func handleBuilds(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement builds list page
	pages.Dashboard(pages.DashboardData{}).Render(r.Context(), w)
}

func handleBuildDetail(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement build detail page
	pages.Dashboard(pages.DashboardData{}).Render(r.Context(), w)
}

func handleNodes(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement nodes list page
	pages.Dashboard(pages.DashboardData{}).Render(r.Context(), w)
}

func handleNodeDetail(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement node detail page
	pages.Dashboard(pages.DashboardData{}).Render(r.Context(), w)
}

func handleSettingsGeneral(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement settings general page
	pages.Dashboard(pages.DashboardData{}).Render(r.Context(), w)
}

func handleSettingsAPIKeys(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement settings API keys page
	pages.Dashboard(pages.DashboardData{}).Render(r.Context(), w)
}
