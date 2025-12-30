package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/narvanalabs/control-plane/web/api"
	"github.com/narvanalabs/control-plane/web/pages"
	"github.com/narvanalabs/control-plane/web/pages/apps"
	"github.com/narvanalabs/control-plane/web/pages/auth"
	"github.com/narvanalabs/control-plane/web/pages/git"
	"github.com/narvanalabs/control-plane/web/pages/nodes"
	settings_page "github.com/narvanalabs/control-plane/web/pages/settings"
)

// NOTE: Builds and deployments list handlers are currently omitted/stubbed 
// as the backend API client doesn't support listing them yet.

func main() {
	r := chi.NewRouter()

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
	r.Get("/settings/server", handleSettingsServer)
	r.Post("/settings/server", handleSettingsServerUpdate)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(requireAuth)

		r.Get("/", handleDashboard)
		r.Get("/git", handleGitPage)
		r.Route("/apps", func(r chi.Router) {
			r.Get("/", handleApps)
			r.Get("/new", handleCreateApp)
			r.Post("/", handleCreateAppSubmit)
			r.Get("/{appID}", handleAppDetail)
			r.Post("/{appID}/delete", handleDeleteApp)
			r.Post("/{appID}/services", handleCreateService)
		})
		r.Post("/apps/{appID}/services/{serviceName}/deploy", handleDeployService)
		r.Post("/apps/{appID}/secrets", handleCreateSecret)
		r.Post("/apps/{appID}/secrets/{key}/delete", handleDeleteSecret)
		r.Get("/nodes", handleNodes)
		// r.Get("/nodes/{nodeID}", handleNodeDetail) // Stubbed
		
		// SSE log stream proxy
		r.Get("/api/logs/stream", handleLogStream)

		// GitHub proxy routes
		r.Get("/api/github/config", handleGetGitHubConfig)
		r.Post("/api/github/config", handleSaveGitHubConfig)
		r.Delete("/api/github/config", handleResetGitHubConfig)
		r.Get("/api/github/repos", handleGetGitHubRepos)
		r.Get("/api/github/installations", handleGetGitHubInstallations)
		r.Get("/api/github/connect", handleGitHubConnect)
		r.Get("/api/github/setup", handleGitHubManifestStart)
	})

	fmt.Println("🚀 Web UI running on http://0.0.0.0:8090")
	if err := http.ListenAndServe(":8090", r); err != nil {
		fmt.Printf("❌ Web UI failed to start: %v\n", err)
		os.Exit(1)
	}
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
	apiURL := os.Getenv("INTERNAL_API_URL")
	if apiURL == "" {
		apiURL = os.Getenv("API_URL")
	}
	if apiURL == "" {
		apiURL = "http://127.0.0.1:8080"
	}
	// Use 127.0.0.1 for internal calls if localhost was specified
	if apiURL == "http://localhost:8080" {
		apiURL = "http://127.0.0.1:8080"
	}
	
	apiClient := api.NewClient(apiURL)
	token := getAuthToken(r)
	if token != "" {
		apiClient = apiClient.WithToken(token)
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

	client := getAPIClient(r)
	resp, err := client.Login(r.Context(), email, password)
	if err != nil {
		auth.Login(auth.LoginData{Error: "Invalid credentials"}).Render(r.Context(), w)
		return
	}

	setAuthCookie(w, resp.Token)
	http.Redirect(w, r, "/", http.StatusFound)
}

func handleRegisterPage(w http.ResponseWriter, r *http.Request) {
	auth.Register(auth.RegisterData{}).Render(r.Context(), w)
}

func handleRegisterSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		auth.Register(auth.RegisterData{Error: "Invalid form data"}).Render(r.Context(), w)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	client := getAPIClient(r)
	resp, err := client.Register(r.Context(), email, password)
	if err != nil {
		auth.Register(auth.RegisterData{Error: err.Error()}).Render(r.Context(), w)
		return
	}

	setAuthCookie(w, resp.Token)
	http.Redirect(w, r, "/", http.StatusFound)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	clearAuthCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

// ============================================================================
// Page Handlers
// ============================================================================

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	ctx := r.Context()

	stats, recent, nodesHealth, _ := client.GetDashboardData(ctx)

	pages.Dashboard(pages.DashboardData{
		TotalApps:         stats.TotalApps,
		ActiveDeployments: stats.ActiveDeployments,
		HealthyNodes:      stats.HealthyNodes,
		RunningBuilds:     stats.RunningBuilds,
		RecentDeployments: recent,
		NodeHealth:        nodesHealth,
	}).Render(ctx, w)
}

func handleApps(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	appList, err := client.ListApps(r.Context())
	if err != nil {
		apps.List(apps.ListData{}).Render(r.Context(), w)
		return
	}
	apps.List(apps.ListData{Apps: appList}).Render(r.Context(), w)
}

func handleCreateApp(w http.ResponseWriter, r *http.Request) {
	apps.Create(apps.CreateData{}).Render(r.Context(), w)
}

func handleCreateAppSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	client := getAPIClient(r)
	app, err := client.CreateApp(r.Context(), name)
	if err != nil {
		apps.Create(apps.CreateData{Error: err.Error()}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/apps/"+app.ID, http.StatusFound)
}

func handleAppDetail(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	client := getAPIClient(r)
	ctx := r.Context()

	app, err := client.GetApp(ctx, appID)
	if err != nil {
		http.Error(w, "App not found", http.StatusNotFound)
		return
	}

	deployments, _ := client.ListAppDeployments(ctx, appID)
	secrets, _ := client.ListSecrets(ctx, appID)
	logs, _ := client.GetAppLogs(ctx, appID)

	// Check GitHub connection
	githubStatus, _ := client.GetGitHubConfig(ctx)

	data := apps.DetailData{
		App:             *app,
		Deployments:     deployments,
		Secrets:         secrets,
		Logs:            logs,
		GitHubConnected: githubStatus.Configured,
		SuccessMsg:      r.URL.Query().Get("success"),
		ErrorMsg:        r.URL.Query().Get("error"),
	}

	apps.Detail(data).Render(ctx, w)
}

func handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	client := getAPIClient(r)
	if err := client.DeleteApp(r.Context(), appID); err != nil {
		http.Redirect(w, r, "/apps/"+appID+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/apps?success=App+deleted", http.StatusFound)
}

func handleCreateService(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/apps/"+appID+"?error=Invalid+form", http.StatusFound)
		return
	}

	req := api.CreateServiceRequest{
		Name:       r.FormValue("name"),
		SourceType: r.FormValue("source_type"),
		GitRepo:    r.FormValue("repo"),
		GitRef:     r.FormValue("git_ref"),
		FlakeURI:   r.FormValue("flake_uri"),
		ImageRef:   r.FormValue("image_ref"),
	}

	// Strategy mapping
	strategy := r.FormValue("strategy")
	if strategy != "" {
		req.BuildStrategy = api.BuildStrategy(strategy)
	}

	client := getAPIClient(r)
	_, err := client.CreateService(r.Context(), appID, req)
	if err != nil {
		http.Redirect(w, r, "/apps/"+appID+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}

	http.Redirect(w, r, "/apps/"+appID+"?success=Service+created", http.StatusFound)
}

func handleNodes(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	ctx := r.Context()
	nodeList, _ := client.ListNodes(ctx)
	nodes.List(nodes.ListData{Nodes: nodeList}).Render(ctx, w)
}

func handleLogStream(w http.ResponseWriter, r *http.Request) {
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	u, _ := url.Parse(apiURL)
	proxy := httputil.NewSingleHostReverseProxy(u)
	
	// Add auth token if present
	token := getAuthToken(r)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	
	proxy.ServeHTTP(w, r)
}

func handleDeployService(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	serviceName := chi.URLParam(r, "serviceName")
	client := getAPIClient(r)
	if _, err := client.Deploy(r.Context(), appID, serviceName); err != nil {
		http.Redirect(w, r, "/apps/"+appID+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/apps/"+appID+"?success=Deployment+initiated", http.StatusFound)
}

func handleCreateSecret(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	key := r.FormValue("key")
	value := r.FormValue("value")
	client := getAPIClient(r)
	if err := client.CreateSecret(r.Context(), appID, key, value); err != nil {
		http.Redirect(w, r, "/apps/"+appID+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/apps/"+appID+"?success=Secret+created", http.StatusFound)
}

func handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	key := chi.URLParam(r, "key")
	client := getAPIClient(r)
	if err := client.DeleteSecret(r.Context(), appID, key); err != nil {
		http.Redirect(w, r, "/apps/"+appID+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/apps/"+appID+"?success=Secret+deleted", http.StatusFound)
}

// Git Handlers

func handleGitPage(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	ctx := r.Context()

	status, err := client.GetGitHubConfig(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	installations := []api.GitHubInstallation{}
	if status.Configured {
		installations, err = client.ListGitHubInstallations(ctx)
		if err != nil {
			// Don't fail the whole page if installations can't be fetched
			fmt.Printf("Error fetching installations: %v\n", err)
		}
	}

	successMsg := r.URL.Query().Get("success")
	errorMsg := r.URL.Query().Get("error")

	data := git.IndexData{
		Status:        *status,
		Installations: installations,
		SuccessMsg:    successMsg,
		ErrorMsg:      errorMsg,
	}

	git.Index(data).Render(ctx, w)
}

func handleGetGitHubConfig(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	status, err := client.GetGitHubConfig(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(status)
}

func handleSaveGitHubConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConfigType   string `json:"config_type"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	client := getAPIClient(r)
	if err := client.SaveGitHubConfig(r.Context(), req.ConfigType, req.ClientID, req.ClientSecret); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func handleResetGitHubConfig(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	if err := client.ResetGitHubConfig(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func handleGetGitHubRepos(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	repos, err := client.ListGitHubRepos(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(repos)
}

func handleGetGitHubInstallations(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	installations, err := client.ListGitHubInstallations(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(installations)
}

func handleGitHubConnect(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	ctx := r.Context()
	org := r.URL.Query().Get("org")

	// 1. Check if App is configured
	status, err := client.GetGitHubConfig(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var redirectURL string
	if !status.Configured {
		json.NewEncoder(w).Encode(map[string]string{"error": "GitHub not configured", "configured": "false"})
		return
	}

	if status.ConfigType == "app" {
		redirectURL, err = client.GetGitHubInstallURL(ctx)
	} else if status.ConfigType == "oauth" {
		redirectURL, err = client.GetGitHubOAuthURL(ctx)
	} else {
		// Fallback for manifest flow if no type set yet (unlikely)
		// We use our local redirect to trigger the POST manifest flow
		redirectURL = "/api/github/setup"
		if org != "" {
			redirectURL += "?org=" + url.QueryEscape(org)
		}
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"url": redirectURL})
}

// Global Manifest Start handler (pre-config)
func handleGitHubManifestStart(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	org := r.URL.Query().Get("org")
	appName := r.URL.Query().Get("app_name")
	
	// We need to fetch the setup URL specifically for manifest start
	// Determine our own base URL to pass to the API so it can pre-fill correctly
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	webURL := fmt.Sprintf("%s://%s", scheme, r.Host)

	path := "/v1/github/setup"
	params := url.Values{}
	params.Set("web_url", webURL)
	if org != "" {
		params.Set("org", org)
	}
	if appName != "" {
		params.Set("app_name", appName)
	}
	path += "?" + params.Encode()
	
	html, contentType, err := client.GetRaw(r.Context(), path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	w.Write(html)
}

// Settings Handlers

func handleSettingsGeneral(w http.ResponseWriter, r *http.Request) {
	// Mock user data for now
	data := settings_page.GeneralData{
		UserEmail: "admin@narvana.io",
		UserName:  "Admin",
	}
	settings_page.General(data).Render(r.Context(), w)
}

func handleSettingsGeneralUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	data := settings_page.GeneralData{
		UserEmail:  "admin@narvana.io",
		UserName:   name,
		SuccessMsg: "Profile updated successfully",
	}
	settings_page.General(data).Render(r.Context(), w)
}

func handleSettingsServer(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	settings, err := client.GetSettings(r.Context())
	if err != nil {
		// If settings don't exist yet or API fails, show empty
		settings = make(map[string]string)
	}

	data := settings_page.ServerData{
		Domain:   settings["server_domain"],
		PublicIP: settings["public_ip"],
	}
	settings_page.Server(data).Render(r.Context(), w)
}

func handleSettingsServerUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	domain := r.FormValue("domain")
	publicIP := r.FormValue("public_ip")

	client := getAPIClient(r)
	err := client.UpdateSettings(r.Context(), map[string]string{
		"server_domain": domain,
		"public_ip":     publicIP,
	})

	data := settings_page.ServerData{
		Domain:   domain,
		PublicIP: publicIP,
	}

	if err != nil {
		data.ErrorMsg = "Failed to update settings: " + err.Error()
	} else {
		data.SuccessMsg = "Settings updated successfully"
	}

	settings_page.Server(data).Render(r.Context(), w)
}

func handleSettingsAPIKeys(w http.ResponseWriter, r *http.Request) {
	data := settings_page.APIKeysData{}
	settings_page.APIKeys(data).Render(r.Context(), w)
}

// Suppress unused import warning
var _ = time.Now
