package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/narvanalabs/control-plane/web/api"
	"github.com/narvanalabs/control-plane/web/pages"
)

var apiClient *api.Client

func main() {
	// Initialize API client
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	apiClient = api.NewClient(apiURL)

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
	// Get token from cookie (TODO: implement proper session handling)
	token := ""
	if cookie, err := r.Cookie("auth_token"); err == nil {
		token = cookie.Value
	}

	// Fetch dashboard data from API
	client := apiClient
	if token != "" {
		client = apiClient.WithToken(token)
	}

	stats, recentDeployments, nodeHealth, err := client.GetDashboardData(r.Context())
	if err != nil {
		// Log error but continue with empty data
		fmt.Printf("Error fetching dashboard data: %v\n", err)
		stats = &api.DashboardStats{}
	}

	data := pages.DashboardData{
		TotalApps:         stats.TotalApps,
		ActiveDeployments: stats.ActiveDeployments,
		HealthyNodes:      stats.HealthyNodes,
		RunningBuilds:     stats.RunningBuilds,
		RecentDeployments: recentDeployments,
		NodeHealth:        nodeHealth,
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
