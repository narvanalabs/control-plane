package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/narvanalabs/control-plane/web/api"
	"github.com/narvanalabs/control-plane/web/pages"
	"github.com/narvanalabs/control-plane/web/pages/apps"
	"github.com/narvanalabs/control-plane/web/pages/auth"
	"github.com/narvanalabs/control-plane/web/pages/builds"
	"github.com/narvanalabs/control-plane/web/pages/deployments"
	"github.com/narvanalabs/control-plane/web/pages/nodes"
	"github.com/narvanalabs/control-plane/web/pages/settings"
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

	// Auth routes (no auth required)
	r.Get("/login", handleLoginPage)
	r.Post("/login", handleLoginSubmit)
	r.Get("/register", handleRegisterPage)
	r.Post("/register", handleRegisterSubmit)
	r.Get("/logout", handleLogout)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(requireAuth)

		r.Get("/", handleDashboard)
		r.Get("/apps", handleApps)
		r.Get("/apps/new", handleCreateApp)
		r.Post("/apps", handleCreateAppSubmit)
		r.Get("/apps/{appID}", handleAppDetail)
		r.Post("/apps/{appID}/delete", handleDeleteApp)
		r.Post("/apps/{appID}/services", handleCreateService)
		r.Post("/apps/{appID}/services/{serviceName}/deploy", handleDeployService)
		r.Post("/apps/{appID}/secrets", handleCreateSecret)
		r.Post("/apps/{appID}/secrets/{key}/delete", handleDeleteSecret)
		r.Get("/deployments", handleDeployments)
		r.Get("/deployments/{deploymentID}", handleDeploymentDetail)
		r.Post("/deployments/{deploymentID}/rollback", handleRollbackDeployment)
		r.Get("/builds", handleBuilds)
		r.Get("/builds/{buildID}", handleBuildDetail)
		r.Get("/nodes", handleNodes)
		r.Get("/nodes/{nodeID}", handleNodeDetail)
		r.Get("/settings/general", handleSettingsGeneral)
		r.Post("/settings/general", handleSettingsGeneralSubmit)
		r.Get("/settings/api-keys", handleSettingsAPIKeys)
		r.Post("/settings/api-keys", handleCreateAPIKey)
		r.Post("/settings/api-keys/{keyID}/delete", handleDeleteAPIKey)
		
		// SSE log stream proxy
		r.Get("/api/logs/stream", handleLogStream)
	})

	fmt.Println("🚀 Web UI running on http://localhost:8090")
	http.ListenAndServe(":8090", r)
}

// ============================================================================
// Auth Middleware
// ============================================================================

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := getAuthToken(r)
		if token == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
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

func setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 7, // 7 days
	})
}

func clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// ============================================================================
// Auth Handlers
// ============================================================================

func handleLoginPage(w http.ResponseWriter, r *http.Request) {
	// If already logged in, redirect to dashboard
	if getAuthToken(r) != "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	auth.Login(auth.LoginData{}).Render(r.Context(), w)
}

func handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		auth.Login(auth.LoginData{Error: "Invalid form data"}).Render(r.Context(), w)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	resp, err := apiClient.Login(r.Context(), email, password)
	if err != nil {
		auth.Login(auth.LoginData{Error: "Invalid email or password"}).Render(r.Context(), w)
		return
	}

	setAuthCookie(w, resp.Token)
	http.Redirect(w, r, "/", http.StatusFound)
}

func handleRegisterPage(w http.ResponseWriter, r *http.Request) {
	// If already logged in, redirect to dashboard
	if getAuthToken(r) != "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	auth.Register(auth.RegisterData{}).Render(r.Context(), w)
}

func handleRegisterSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		auth.Register(auth.RegisterData{Error: "Invalid form data"}).Render(r.Context(), w)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	if password != confirmPassword {
		auth.Register(auth.RegisterData{Error: "Passwords do not match"}).Render(r.Context(), w)
		return
	}

	if len(password) < 8 {
		auth.Register(auth.RegisterData{Error: "Password must be at least 8 characters"}).Render(r.Context(), w)
		return
	}

	_, err := apiClient.Register(r.Context(), email, password)
	if err != nil {
		auth.Register(auth.RegisterData{Error: "Registration failed. Email may already be in use."}).Render(r.Context(), w)
		return
	}

	auth.Register(auth.RegisterData{Success: true}).Render(r.Context(), w)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	clearAuthCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

// ============================================================================
// Dashboard Handler
// ============================================================================

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

// ============================================================================
// Apps Handlers
// ============================================================================

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

func handleCreateAppSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		apps.Create(apps.CreateData{Error: "Invalid form data"}).Render(r.Context(), w)
		return
	}

	name := r.FormValue("name")
	client := getAPIClient(r)

	// Create the app
	app, err := client.CreateApp(r.Context(), name)
	if err != nil {
		apps.Create(apps.CreateData{Error: "Failed to create app: " + err.Error()}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/apps/"+app.ID+"?success=App+created", http.StatusFound)
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

	deploymentList, _ := client.ListAppDeployments(r.Context(), appID)
	secretList, _ := client.ListSecrets(r.Context(), appID)
	logList, _ := client.GetAppLogs(r.Context(), appID)

	apps.Detail(apps.DetailData{
		App:         *app,
		Deployments: deploymentList,
		Secrets:     secretList,
		Logs:        logList,
		SuccessMsg:  r.URL.Query().Get("success"),
		ErrorMsg:    r.URL.Query().Get("error"),
	}).Render(r.Context(), w)
}

func handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	client := getAPIClient(r)

	if err := client.DeleteApp(r.Context(), appID); err != nil {
		fmt.Printf("Error deleting app: %v\n", err)
		http.Redirect(w, r, "/apps?error=Failed+to+delete+app", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/apps?success=App+deleted", http.StatusFound)
}

func handleCreateService(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/apps/"+appID+"?error=Invalid+form+data", http.StatusFound)
		return
	}

	svc := api.CreateServiceRequest{
		Name:       r.FormValue("name"),
		SourceType: r.FormValue("type"),
	}

	switch svc.SourceType {
	case "git":
		svc.GitRepo = r.FormValue("repo")
	case "flake":
		svc.FlakeURI = r.FormValue("repo")
	case "image":
		svc.ImageRef = r.FormValue("repo")
	}

	client := getAPIClient(r)
	_, err := client.CreateService(r.Context(), appID, svc)
	if err != nil {
		fmt.Printf("Error creating service: %v\n", err)
		http.Redirect(w, r, "/apps/"+appID+"?error=Failed+to+add+service", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/apps/"+appID+"?success=Service+added", http.StatusFound)
}

func handleDeployService(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	serviceName := chi.URLParam(r, "serviceName")
	client := getAPIClient(r)

	_, err := client.Deploy(r.Context(), appID, serviceName)
	if err != nil {
		fmt.Printf("Error deploying service: %v\n", err)
		http.Redirect(w, r, "/apps/"+appID+"?error=Failed+to+deploy+service", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/apps/"+appID+"?success=Deployment+started", http.StatusFound)
}

func handleCreateSecret(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/apps/"+appID, http.StatusFound)
		return
	}

	key := r.FormValue("key")
	value := r.FormValue("value")
	client := getAPIClient(r)

	if err := client.CreateSecret(r.Context(), appID, key, value); err != nil {
		fmt.Printf("Error creating secret: %v\n", err)
		http.Redirect(w, r, "/apps/"+appID+"?error=Failed+to+create+secret", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/apps/"+appID+"?success=Secret+created", http.StatusFound)
}

func handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	key := chi.URLParam(r, "key")
	client := getAPIClient(r)

	if err := client.DeleteSecret(r.Context(), appID, key); err != nil {
		fmt.Printf("Error deleting secret: %v\n", err)
		http.Redirect(w, r, "/apps/"+appID+"?error=Failed+to+delete+secret", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/apps/"+appID+"?success=Secret+deleted", http.StatusFound)
}


// ============================================================================
// Deployments Handlers
// ============================================================================

func handleDeployments(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)

	// Fetch all apps and their deployments
	appList, _ := client.ListApps(r.Context())
	var allDeployments []api.Deployment
	appNames := make(map[string]string)

	for _, app := range appList {
		appNames[app.ID] = app.Name
		deps, _ := client.ListAppDeployments(r.Context(), app.ID)
		allDeployments = append(allDeployments, deps...)
	}

	deployments.List(deployments.ListData{
		Deployments: allDeployments,
		Apps:        appNames,
	}).Render(r.Context(), w)
}

func handleDeploymentDetail(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "deploymentID")
	client := getAPIClient(r)

	deployment, err := client.GetDeployment(r.Context(), deploymentID)
	if err != nil {
		fmt.Printf("Error fetching deployment: %v\n", err)
		http.Redirect(w, r, "/deployments", http.StatusFound)
		return
	}

	app, _ := client.GetApp(r.Context(), deployment.AppID)
	appName := "Unknown App"
	if app != nil {
		appName = app.Name
	}
	
	// Try to get logs
	logs_list, _ := client.GetAppLogs(r.Context(), deployment.AppID)

	deployments.Detail(deployments.DetailData{
		Deployment: *deployment,
		AppName:    appName,
		Logs:       logs_list,
		SuccessMsg: r.URL.Query().Get("success"),
		ErrorMsg:   r.URL.Query().Get("error"),
	}).Render(r.Context(), w)
}

func handleRollbackDeployment(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "deploymentID")
	client := getAPIClient(r)

	d, err := client.RollbackDeployment(r.Context(), deploymentID)
	if err != nil {
		fmt.Printf("Error rolling back deployment: %v\n", err)
		http.Redirect(w, r, "/deployments/"+deploymentID+"?error=Rollback+failed", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/deployments/"+d.ID+"?success=Rollback+initiated", http.StatusFound)
}

// ============================================================================
// Builds Handlers
// ============================================================================

func handleBuilds(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch builds from API when endpoint available
	builds.List(builds.ListData{
		Builds: []api.Build{},
		Apps:   map[string]string{},
	}).Render(r.Context(), w)
}

func handleBuildDetail(w http.ResponseWriter, r *http.Request) {
	buildID := chi.URLParam(r, "buildID")
	// TODO: Fetch actual build data when API endpoint available
	// For now, show a placeholder
	builds.Detail(builds.DetailData{
		Build: api.Build{
			ID:       buildID,
			Status:   "queued",
			Strategy: "auto",
		},
		AppName: "Unknown",
	}).Render(r.Context(), w)
}

// ============================================================================
// Nodes Handlers
// ============================================================================

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
	nodeID := chi.URLParam(r, "nodeID")
	client := getAPIClient(r)

	// Find the node in the list
	nodeList, err := client.ListNodes(r.Context())
	if err != nil {
		http.Redirect(w, r, "/nodes", http.StatusFound)
		return
	}

	var node *api.Node
	for _, n := range nodeList {
		if n.ID == nodeID {
			node = &n
			break
		}
	}

	if node == nil {
		http.Redirect(w, r, "/nodes", http.StatusFound)
		return
	}

	// Get deployments on this node
	var nodeDeployments []api.Deployment
	appList, _ := client.ListApps(r.Context())
	for _, app := range appList {
		deps, _ := client.ListAppDeployments(r.Context(), app.ID)
		for _, d := range deps {
			if d.NodeID == nodeID {
				nodeDeployments = append(nodeDeployments, d)
			}
		}
	}

	nodes.Detail(nodes.DetailData{
		Node:        *node,
		Deployments: nodeDeployments,
	}).Render(r.Context(), w)
}

// ============================================================================
// Settings Handlers
// ============================================================================

func handleSettingsGeneral(w http.ResponseWriter, r *http.Request) {
	// TODO: Get user data from API
	settings.General(settings.GeneralData{
		UserEmail:  "admin@narvana.io",
		UserName:   "Admin",
		SuccessMsg: r.URL.Query().Get("success"),
		ErrorMsg:   r.URL.Query().Get("error"),
	}).Render(r.Context(), w)
}

func handleSettingsGeneralSubmit(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement user profile update in API
	// For now, just simulate success
	http.Redirect(w, r, "/settings/general?success=Profile+updated", http.StatusFound)
}

func handleSettingsAPIKeys(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch API keys from API
	settings.APIKeys(settings.APIKeysData{
		Keys:       []settings.APIKey{},
		SuccessMsg: r.URL.Query().Get("success"),
		ErrorMsg:   r.URL.Query().Get("error"),
	}).Render(r.Context(), w)
}

func handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement API key creation in API
	http.Redirect(w, r, "/settings/api-keys?success=API+key+created", http.StatusFound)
}

func handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement API key deletion in API
	http.Redirect(w, r, "/settings/api-keys?success=API+key+revoked", http.StatusFound)
}

// ============================================================================
// Log Stream Handler (SSE Proxy)
// ============================================================================

func handleLogStream(w http.ResponseWriter, r *http.Request) {
	appID := r.URL.Query().Get("app_id")
	if appID == "" {
		http.Error(w, "app_id required", http.StatusBadRequest)
		return
	}

	token := getAuthToken(r)
	
	// Build the API URL
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	streamURL := fmt.Sprintf("%s/v1/apps/%s/logs/stream", apiURL, appID)

	// Create request to API
	req, err := http.NewRequestWithContext(r.Context(), "GET", streamURL, nil)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}
	
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "text/event-stream")

	// Execute request
	client := &http.Client{Timeout: 0} // No timeout for SSE
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Failed to connect to log stream", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Flush headers
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Stream the response
	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if err != nil {
			break
		}
	}
}

// Suppress unused import warning
var _ = time.Now
