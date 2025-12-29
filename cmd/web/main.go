package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/narvanalabs/control-plane/web/api"
	"github.com/narvanalabs/control-plane/web/pages"
	"github.com/narvanalabs/control-plane/web/pages/apps"
	"github.com/narvanalabs/control-plane/web/pages/builds"
	"github.com/narvanalabs/control-plane/web/pages/deployments"
	"github.com/narvanalabs/control-plane/web/pages/nodes"
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

func getAuthToken(r *http.Request) string {
	if cookie, err := r.Cookie("auth_token"); err == nil {
		return cookie.Value
	}
	return ""
}

func getAPIClient(r *http.Request) *api.Client {
	token := getAuthToken(r)
	if token != "" {
		return apiClient.WithToken(token)
	}
	return apiClient
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	stats, recentDeployments, nodeHealth, err := client.GetDashboardData(r.Context())
	if err != nil {
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
	client := getAPIClient(r)
	appList, err := client.ListApps(r.Context())
	if err != nil {
		fmt.Printf("Error fetching apps: %v\n", err)
		appList = []api.App{}
	}

	apps.List(apps.ListData{Apps: appList}).Render(r.Context(), w)
}

func handleCreateApp(w http.ResponseWriter, r *http.Request) {
	apps.Create(apps.CreateData{}).Render(r.Context(), w)
}

func handleAppDetail(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	client := getAPIClient(r)
	
	app, err := client.GetApp(r.Context(), appID)
	if err != nil {
		fmt.Printf("Error fetching app: %v\n", err)
		http.Redirect(w, r, "/apps", http.StatusFound)
		return
	}

	apps.Detail(apps.DetailData{
		App:         *app,
		Deployments: []api.Deployment{},
	}).Render(r.Context(), w)
}

func handleDeployments(w http.ResponseWriter, r *http.Request) {
	// TODO: Add ListDeployments to API client
	deployments.List(deployments.ListData{
		Deployments: []api.Deployment{},
		Apps:        map[string]string{},
	}).Render(r.Context(), w)
}

func handleDeploymentDetail(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement deployment detail page
	http.Redirect(w, r, "/deployments", http.StatusFound)
}

func handleBuilds(w http.ResponseWriter, r *http.Request) {
	// TODO: Add ListBuilds to API client
	builds.List(builds.ListData{
		Builds: []api.Build{},
		Apps:   map[string]string{},
	}).Render(r.Context(), w)
}

func handleBuildDetail(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement build detail page with logs
	http.Redirect(w, r, "/builds", http.StatusFound)
}

func handleNodes(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	nodeList, err := client.ListNodes(r.Context())
	if err != nil {
		fmt.Printf("Error fetching nodes: %v\n", err)
		nodeList = []api.Node{}
	}

	nodes.List(nodes.ListData{Nodes: nodeList}).Render(r.Context(), w)
}

func handleNodeDetail(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement node detail page
	http.Redirect(w, r, "/nodes", http.StatusFound)
}

func handleSettingsGeneral(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement settings general page
	pages.Dashboard(pages.DashboardData{}).Render(r.Context(), w)
}

func handleSettingsAPIKeys(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement settings API keys page
	pages.Dashboard(pages.DashboardData{}).Render(r.Context(), w)
}
