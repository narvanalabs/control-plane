package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/web/api"
	"github.com/narvanalabs/control-plane/web/pages"
	"github.com/narvanalabs/control-plane/web/pages/apps"
	"github.com/narvanalabs/control-plane/web/pages/auth"
	"github.com/narvanalabs/control-plane/web/pages/git"
	"github.com/narvanalabs/control-plane/web/pages/nodes"
	"github.com/narvanalabs/control-plane/web/pages/builds"
	"github.com/narvanalabs/control-plane/web/pages/deployments"
	settings_page "github.com/narvanalabs/control-plane/web/pages/settings"
)

func main() {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(sidebarStateMiddleware)

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
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Invitation acceptance routes (no auth required)
	r.Get("/invite/{token}", handleInviteAcceptPage)
	r.Post("/invite/accept", handleInviteAcceptSubmit)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(requireAuth)
		r.Use(userContextMiddleware)

		r.Get("/", handleDashboard)
		r.Get("/git", handleGitPage)
		r.Route("/apps", func(r chi.Router) {
			r.Get("/", handleApps)
			r.Post("/", handleCreateAppSubmit)
			r.Get("/{appID}", handleAppDetail)
			r.Post("/{appID}", handleUpdateApp)
			r.Post("/{appID}/delete", handleDeleteApp)
			r.Route("/{appID}/services", func(r chi.Router) {
				r.Post("/", handleCreateService)
				r.Get("/{serviceName}", handleServiceDetail)
				r.Post("/{serviceName}", handleUpdateService)
				r.Delete("/{serviceName}", handleDeleteService)
				r.Get("/{serviceName}/console/ws", handleServiceConsoleWS)
			})
		})
		r.Post("/apps/{appID}/services/{serviceName}/deploy", handleDeployService)
		r.Post("/apps/{appID}/services/{serviceName}/stop", handleStopService)
		r.Post("/apps/{appID}/services/{serviceName}/start", handleStartService)
		r.Post("/apps/{appID}/services/{serviceName}/reload", handleReloadService)
		r.Post("/apps/{appID}/services/{serviceName}/retry", handleRetryService)
		r.Post("/apps/{appID}/services/{serviceName}/delete", handleDeleteServicePost)
		r.Post("/apps/{appID}/secrets", handleCreateSecret)
		r.Post("/apps/{appID}/secrets/{key}/delete", handleDeleteSecret)
		r.Get("/nodes", handleNodes)
		
		r.Route("/builds", func(r chi.Router) {
			r.Get("/", handleBuildsList)
			r.Route("/{buildID}", func(r chi.Router) {
				r.Get("/", handleBuildsDetail)
				r.Post("/retry", handleBuildRetry)
			})
		})

		r.Route("/deployments", func(r chi.Router) {
			r.Get("/", handleDeploymentsList)
			r.Route("/{deploymentID}", func(r chi.Router) {
				r.Get("/", handleDeploymentsDetail)
				r.Post("/rollback", handleDeploymentRollback)
			})
		})

		// SSE log stream proxy
		r.Get("/api/logs/stream", handleLogStream)
		r.Get("/api/server/logs/stream", handleServerLogStream)
		r.Get("/api/server/logs/download", handleServerLogDownload)
		r.Post("/api/server/restart", handleServerRestart)
		r.Get("/api/server/console/ws", handleServerConsoleWS)
		r.Get("/api/server/stats", handleServerStats)
		r.Get("/api/server/stats/stream", handleServerStatsStream)

		// User profile proxy
		r.Get("/api/user/profile", handleUserProfile)
		r.Patch("/api/user/profile", handleUpdateUserProfile)

		// Detection API proxy
		// **Validates: Requirements 5.4, 5.5**
		r.Post("/api/detect", handleDetectProxy)

		// Server management pages
		r.Get("/settings/server/logs", handleSettingsServerLogs)
		r.Get("/settings/server/stats", handleSettingsServerStats)
		r.Get("/settings/profile", handleSettingsProfile)
		r.Post("/settings/profile", handleUpdateProfile)
		r.Get("/settings/ssh-keys", handleSettingsSSHKeys)
		r.Get("/settings/notifications", handleSettingsNotifications)
		
		// Users management routes (admin only)
		r.Get("/settings/users", handleSettingsUsers)
		r.Post("/settings/users/invite", handleInviteUser)
		r.Post("/settings/users/delete", handleDeleteUser)
		r.Post("/settings/users/revoke", handleRevokeInvitation)

		// GitHub proxy routes
		r.Get("/api/github/config", handleGetGitHubConfig)
		r.Post("/api/github/config", handleSaveGitHubConfig)
		r.Delete("/api/github/config", handleResetGitHubConfig)
		r.Get("/api/github/repos", handleGetGitHubRepos)
		r.Get("/api/github/installations", handleGetGitHubInstallations)
		r.Get("/api/github/connect", handleGitHubConnect)
		r.Get("/api/github/setup", handleGitHubManifestStart)
	})

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	
	apiURL := os.Getenv("INTERNAL_API_URL")
	if apiURL == "" {
		apiURL = os.Getenv("API_URL")
	}
	if apiURL == "" {
		apiURL = "http://127.0.0.1:8080"
	}
	
	logger.Info("starting web server", 
		"addr", "0.0.0.0:8090", 
		"internal_api_url", apiURL,
	)

	if err := http.ListenAndServe(":8090", r); err != nil {
		logger.Error("web server failed to start", "error", err)
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

func userContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := getAuthToken(r)
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		client := getAPIClient(r)
		user, err := client.GetUserProfile(r.Context())
		if err != nil {
			slog.Debug("failed to get user profile for context", "error", err)
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), "user", user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

const SidebarStateKey = "sidebar-collapsed"

func sidebarStateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collapsed := false
		if cookie, err := r.Cookie("sidebar-collapsed"); err == nil {
			collapsed = cookie.Value == "true"
		}
		ctx := context.WithValue(r.Context(), SidebarStateKey, collapsed)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getAuthToken(r *http.Request) string {
	if cookie, err := r.Cookie("auth_token"); err == nil {
		return cookie.Value
	}
	return ""
}

// deriveServiceStateFromDeployments derives the service state from the list of deployments.
// If there are no deployments, the service is in the "new" state.
func deriveServiceStateFromDeployments(deployments []api.Deployment) models.ServiceState {
	if len(deployments) == 0 {
		return models.ServiceStateNew
	}

	// Use the latest deployment (first in the list) to determine state
	latestDeployment := deployments[0]

	switch latestDeployment.Status {
	case "pending", "building", "built", "scheduled", "starting":
		return models.ServiceStateDeploying
	case "running":
		return models.ServiceStateRunning
	case "stopping", "stopped":
		return models.ServiceStateStopped
	case "failed":
		return models.ServiceStateFailed
	default:
		return models.ServiceStateNew
	}
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

func handleCreateAppSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	iconURL := r.FormValue("icon_url")

	client := getAPIClient(r)
	app, err := client.CreateApp(r.Context(), name, description, iconURL)
	if err != nil {
		appList, _ := client.ListApps(r.Context())
		apps.List(apps.ListData{
			Apps:  appList,
			Error: err.Error(),
		}).Render(r.Context(), w)
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

	// For the app overview, we only need the app (which includes services)
	// and maybe some summary data. Detailed deployments/logs are moved to service detail.
	secrets, _ := client.ListSecrets(ctx, appID)

	// Check GitHub connection
	githubStatus, _ := client.GetGitHubConfig(ctx)

	data := apps.DetailData{
		App:             *app,
		Secrets:         secrets,
		GitHubConnected: githubStatus.Configured,
		SuccessMsg:      r.URL.Query().Get("success"),
		ErrorMsg:        r.URL.Query().Get("error"),
		Token:           getAuthToken(r),
	}

	apps.Detail(data).Render(ctx, w)
}

func handleServiceDetail(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	serviceName := chi.URLParam(r, "serviceName")
	client := getAPIClient(r)
	ctx := r.Context()

	app, err := client.GetApp(ctx, appID)
	if err != nil {
		http.Error(w, "App not found", http.StatusNotFound)
		return
	}

	var service *api.Service
	for _, s := range app.Services {
		if s.Name == serviceName {
			service = &s
			break
		}
	}

	if service == nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	// Fetch deployments for this service
	allDeployments, _ := client.ListAppDeployments(ctx, appID)
	var deployments []api.Deployment
	for _, d := range allDeployments {
		if d.ServiceName == serviceName {
			deployments = append(deployments, d)
		}
	}

	// Fetch logs for this service
	var logs []api.Log
	var buildLogs string
	if len(deployments) > 0 {
		// Fetch service-level runtime logs
		logs, _ = client.GetServiceLogs(ctx, appID, serviceName)
		
		// Fetch build logs for the latest deployment
		latestDeployment := deployments[0]
		if build, _ := client.GetBuildByDeployment(ctx, latestDeployment.ID); build != nil {
			buildLogs = build.Logs
		}
	}

	// Derive service state from latest deployment
	serviceState := deriveServiceStateFromDeployments(deployments)

	data := apps.ServiceDetailData{
		App:          *app,
		Service:      *service,
		Deployments:  deployments,
		Logs:         logs,
		BuildLogs:    buildLogs,
		Token:        getAuthToken(r),
		SuccessMsg:   r.URL.Query().Get("success"),
		ErrorMsg:     r.URL.Query().Get("error"),
		ServiceState: serviceState,
	}

	apps.ServiceDetail(data).Render(ctx, w)
}

func handleUpdateService(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	serviceName := chi.URLParam(r, "serviceName")
	client := getAPIClient(r)
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/apps/%s/services/%s?error=Failed+to+parse+form", appID, serviceName), http.StatusSeeOther)
		return
	}

	replicas := 1
	fmt.Sscanf(r.FormValue("replicas"), "%d", &replicas)

	var dependsOn []string
	if val := r.FormValue("depends_on"); val != "" {
		parts := strings.Split(val, ",")
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				dependsOn = append(dependsOn, trimmed)
			}
		}
	}

	req := api.CreateServiceRequest{
		Name:          serviceName,
		Replicas:      replicas,
		BuildStrategy: api.BuildStrategy(r.FormValue("strategy")),
		DependsOn:     dependsOn,
	}

	_, err := client.UpdateService(ctx, appID, serviceName, req)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/apps/%s/services/%s?error=%s", appID, serviceName, url.QueryEscape(err.Error())), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/apps/%s/services/%s?success=Service+updated+successfully", appID, serviceName), http.StatusSeeOther)
}

func handleDeleteService(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	serviceName := chi.URLParam(r, "serviceName")
	client := getAPIClient(r)
	ctx := r.Context()

	err := client.DeleteService(ctx, appID, serviceName)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/apps/%s/services/%s?error=%s", appID, serviceName, url.QueryEscape(err.Error())), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/apps/%s?success=Service+deleted+successfully", appID), http.StatusSeeOther)
}

func handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	client := getAPIClient(r)
	ctx := r.Context()

	err := client.DeleteApp(ctx, appID)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/apps/%s?error=%s", appID, url.QueryEscape(err.Error())), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/apps?success=App+deleted+successfully", http.StatusSeeOther)
}

func handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	client := getAPIClient(r)
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/apps/%s?error=Failed+to+parse+form", appID), http.StatusSeeOther)
		return
	}

	req := api.UpdateAppRequest{
		ResourceTier: r.FormValue("resource_tier"),
	}

	_, err := client.UpdateApp(ctx, appID, req)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/apps/%s?error=%s", appID, url.QueryEscape(err.Error())), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/apps/%s?success=App+updated+successfully", appID), http.StatusSeeOther)
}

func handleCreateService(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/apps/"+appID+"?error=Invalid+form", http.StatusFound)
		return
	}

	category := r.FormValue("category")
	sourceType := r.FormValue("source_type")
	if sourceType == "" {
		sourceType = category // Fallback to category if source_type not set
	}

	req := api.CreateServiceRequest{
		Name:       r.FormValue("name"),
		SourceType: sourceType,
		GitRepo:    r.FormValue("repo"),
		GitRef:     r.FormValue("git_ref"),
		FlakeURI:   r.FormValue("flake_uri"),
		ImageRef:   r.FormValue("image_ref"),
	}

	// Handle web service with language selection
	// **Validates: Requirements 5.2, 5.4, 5.5, 5.6**
	if category == "web-service" {
		language := r.FormValue("language")
		if language != "" {
			// Map language to build strategy
			strategyMap := map[string]api.BuildStrategy{
				"go":         api.BuildStrategyAutoGo,
				"rust":       api.BuildStrategyAutoRust,
				"python":     api.BuildStrategyAutoPython,
				"node":       api.BuildStrategyAutoNode,
				"dockerfile": api.BuildStrategyDockerfile,
			}
			if strategy, ok := strategyMap[language]; ok {
				req.BuildStrategy = strategy
			}
		}
		
		// Handle auto-detected fields
		entryPoint := r.FormValue("entry_point")
		buildCommand := r.FormValue("build_command")
		version := r.FormValue("version")
		
		if entryPoint != "" || buildCommand != "" {
			req.BuildConfig = &api.BuildConfig{
				EntryPoint:   entryPoint,
				BuildCommand: buildCommand,
			}
		}
		_ = version // Version is stored in build config if needed
	}

	// Handle static site
	// **Validates: Requirements 5.3**
	if category == "static-site" {
		framework := r.FormValue("static_framework")
		buildCommand := r.FormValue("build_command")
		outputDir := r.FormValue("output_dir")
		
		// Static sites use auto-node strategy by default
		req.BuildStrategy = api.BuildStrategyAutoNode
		
		if buildCommand != "" || outputDir != "" {
			req.BuildConfig = &api.BuildConfig{
				BuildCommand: buildCommand,
			}
		}
		_ = framework // Framework can be used for template selection
		_ = outputDir // Output dir for static file serving
	}

	// Handle database service
	// **Validates: Requirements 5.7, 5.8**
	if category == "database" || sourceType == "database" {
		dbType := r.FormValue("db_type")
		dbVersion := r.FormValue("db_version")
		
		// Default to PostgreSQL if not specified
		if dbType == "" {
			dbType = "postgres"
		}
		
		// Set default version if not provided
		if dbVersion == "" {
			defaultVersions := map[string]string{
				"postgres": "16",
				"mysql":    "8.0",
				"mariadb":  "11",
				"mongodb":  "7.0",
				"redis":    "7",
			}
			if v, ok := defaultVersions[dbType]; ok {
				dbVersion = v
			} else {
				dbVersion = "16" // Fallback to PostgreSQL default
			}
		}
		
		req.Database = &api.DatabaseConfig{
			Type:    dbType,
			Version: dbVersion,
		}
		req.SourceType = "database"
	}

	// Strategy mapping (fallback for explicit strategy selection)
	strategy := r.FormValue("strategy")
	if strategy != "" && req.BuildStrategy == "" {
		req.BuildStrategy = api.BuildStrategy(strategy)
	}

	client := getAPIClient(r)
	service, err := client.CreateService(r.Context(), appID, req)
	if err != nil {
		http.Redirect(w, r, "/apps/"+appID+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}

	// Redirect to the service detail page
	http.Redirect(w, r, fmt.Sprintf("/apps/%s/services/%s?success=Service+created", appID, service.Name), http.StatusFound)
}

func handleNodes(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	ctx := r.Context()
	nodeList, _ := client.ListNodes(ctx)
	nodes.List(nodes.ListData{Nodes: nodeList}).Render(ctx, w)
}

func handleLogStream(w http.ResponseWriter, r *http.Request) {
	apiURL := os.Getenv("INTERNAL_API_URL")
	if apiURL == "" {
		apiURL = os.Getenv("API_URL")
	}
	if apiURL == "" {
		apiURL = "http://127.0.0.1:8080"
	}
	if apiURL == "http://localhost:8080" {
		apiURL = "http://127.0.0.1:8080"
	}
	
	u, _ := url.Parse(apiURL)
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.FlushInterval = -1 // Disable buffering for SSE
	
	// Add auth token if present
	token := getAuthToken(r)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	
	// Rewrite path: /api/logs/stream?app_id=XYZ -> /v1/apps/XYZ/logs/stream
	appID := r.URL.Query().Get("app_id")
	serviceName := r.URL.Query().Get("service_name")
	if appID != "" {
		r.URL.Path = fmt.Sprintf("/v1/apps/%s/logs/stream", appID)
		if serviceName != "" {
			// Ensure service_name is in the query string for the backend
			q := r.URL.Query()
			q.Set("service_name", serviceName)
			r.URL.RawQuery = q.Encode()
		}
	} else {
		// Fallback: just strip /api/ and prefix /v1/
		r.URL.Path = "/v1" + r.URL.Path[4:]
	}
	
	slog.Info("proxying log stream", "path", r.URL.Path, "app_id", appID)
	proxy.ServeHTTP(w, r)
}

func handleServerLogStream(w http.ResponseWriter, r *http.Request) {
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
	
	// Rewrite path: /api/server/logs/stream -> /v1/server/logs/stream
	r.URL.Path = "/v1/server/logs/stream"
	
	proxy.ServeHTTP(w, r)
}
func handleServerLogDownload(w http.ResponseWriter, r *http.Request) {
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
	
	// Rewrite path: /api/server/logs/download -> /v1/server/logs/download
	r.URL.Path = "/v1/server/logs/download"
	
	proxy.ServeHTTP(w, r)
}

func handleServerRestart(w http.ResponseWriter, r *http.Request) {
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
	
	// Rewrite path: /api/server/restart -> /v1/server/restart
	r.URL.Path = "/v1/server/restart"
	
	proxy.ServeHTTP(w, r)
}

func handleServerStats(w http.ResponseWriter, r *http.Request) {
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
	
	// Rewrite path: /api/server/stats -> /v1/server/stats
	r.URL.Path = "/v1/server/stats"
	
	proxy.ServeHTTP(w, r)
}

func handleServerStatsStream(w http.ResponseWriter, r *http.Request) {
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
	
	// Rewrite path: /api/server/stats/stream -> /v1/server/stats/stream
	r.URL.Path = "/v1/server/stats/stream"
	
	proxy.ServeHTTP(w, r)
}

func handleUserProfile(w http.ResponseWriter, r *http.Request) {
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	u, _ := url.Parse(apiURL)
	proxy := httputil.NewSingleHostReverseProxy(u)
	
	token := getAuthToken(r)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	
	r.URL.Path = "/v1/user/profile"
	proxy.ServeHTTP(w, r)
}

func handleUpdateUserProfile(w http.ResponseWriter, r *http.Request) {
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	u, _ := url.Parse(apiURL)
	proxy := httputil.NewSingleHostReverseProxy(u)
	
	token := getAuthToken(r)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	
	r.URL.Path = "/v1/user/profile"
	proxy.ServeHTTP(w, r)
}

// handleDetectProxy proxies detection requests to the API server.
// **Validates: Requirements 5.4, 5.5**
func handleDetectProxy(w http.ResponseWriter, r *http.Request) {
	apiURL := os.Getenv("INTERNAL_API_URL")
	if apiURL == "" {
		apiURL = os.Getenv("API_URL")
	}
	if apiURL == "" {
		apiURL = "http://127.0.0.1:8080"
	}
	if apiURL == "http://localhost:8080" {
		apiURL = "http://127.0.0.1:8080"
	}
	
	u, _ := url.Parse(apiURL)
	proxy := httputil.NewSingleHostReverseProxy(u)
	
	token := getAuthToken(r)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	
	// Rewrite path: /api/detect -> /v1/detect
	r.URL.Path = "/v1/detect"
	
	slog.Info("proxying detection request", "path", r.URL.Path)
	proxy.ServeHTTP(w, r)
}

func handleServerConsoleWS(w http.ResponseWriter, r *http.Request) {
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	
	u, _ := url.Parse(apiURL)
	target := "ws://" + u.Host + "/v1/server/console/ws"
	if u.Scheme == "https" {
		target = "wss://" + u.Host + "/v1/server/console/ws"
	}

	// Upgrade client connection
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("failed to upgrade client websocket", "error", err)
		return
	}
	defer clientConn.Close()

	// Connect to backend
	header := http.Header{}
	token := getAuthToken(r)
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}

	backendConn, resp, err := websocket.DefaultDialer.Dial(target, header)
	if err != nil {
		slog.Error("failed to dial backend websocket", "error", err, "resp_code", resp.StatusCode)
		return
	}
	defer backendConn.Close()

	// Bridge connections
	errChan := make(chan error, 2)

	go func() {
		for {
			mt, msg, err := clientConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := backendConn.WriteMessage(mt, msg); err != nil {
				errChan <- err
				return
			}
		}
	}()

	go func() {
		for {
			mt, msg, err := backendConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := clientConn.WriteMessage(mt, msg); err != nil {
				errChan <- err
				return
			}
		}
	}()

	<-errChan
}

func handleServiceConsoleWS(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	serviceName := chi.URLParam(r, "serviceName")

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	
	u, _ := url.Parse(apiURL)
	target := fmt.Sprintf("ws://%s/v1/apps/%s/services/%s/terminal/ws", u.Host, appID, serviceName)
	if u.Scheme == "https" {
		target = fmt.Sprintf("wss://%s/v1/apps/%s/services/%s/terminal/ws", u.Host, appID, serviceName)
	}

	// Upgrade client connection
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("failed to upgrade client websocket", "error", err)
		return
	}
	defer clientConn.Close()

	// Connect to backend
	header := http.Header{}
	token := getAuthToken(r)
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}

	backendConn, resp, err := websocket.DefaultDialer.Dial(target, header)
	if err != nil {
		slog.Error("failed to dial backend websocket", "error", err, "app_id", appID, "service", serviceName)
		if resp != nil {
			slog.Error("backend response code", "resp_code", resp.StatusCode)
		}
		return
	}
	defer backendConn.Close()

	// Bridge connections
	errChan := make(chan error, 2)

	go func() {
		for {
			mt, msg, err := clientConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := backendConn.WriteMessage(mt, msg); err != nil {
				errChan <- err
				return
			}
		}
	}()

	go func() {
		for {
			mt, msg, err := backendConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := clientConn.WriteMessage(mt, msg); err != nil {
				errChan <- err
				return
			}
		}
	}()

	<-errChan
}

func handleDeployService(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	serviceName := chi.URLParam(r, "serviceName")
	client := getAPIClient(r)
	if _, err := client.Deploy(r.Context(), appID, serviceName); err != nil {
		http.Redirect(w, r, "/apps/"+appID+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/apps/"+appID+"/services/"+serviceName+"?success=Deployment+initiated", http.StatusFound)
}

func handleStopService(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	serviceName := chi.URLParam(r, "serviceName")
	client := getAPIClient(r)
	if err := client.StopService(r.Context(), appID, serviceName); err != nil {
		http.Redirect(w, r, "/apps/"+appID+"/services/"+serviceName+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/apps/"+appID+"/services/"+serviceName+"?success=Service+stopped", http.StatusFound)
}

func handleStartService(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	serviceName := chi.URLParam(r, "serviceName")
	client := getAPIClient(r)
	if err := client.StartService(r.Context(), appID, serviceName); err != nil {
		http.Redirect(w, r, "/apps/"+appID+"/services/"+serviceName+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/apps/"+appID+"/services/"+serviceName+"?success=Service+started", http.StatusFound)
}

func handleReloadService(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	serviceName := chi.URLParam(r, "serviceName")
	client := getAPIClient(r)
	if err := client.ReloadService(r.Context(), appID, serviceName); err != nil {
		http.Redirect(w, r, "/apps/"+appID+"/services/"+serviceName+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/apps/"+appID+"/services/"+serviceName+"?success=Service+reloading", http.StatusFound)
}

func handleRetryService(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	serviceName := chi.URLParam(r, "serviceName")
	client := getAPIClient(r)
	if err := client.RetryService(r.Context(), appID, serviceName); err != nil {
		http.Redirect(w, r, "/apps/"+appID+"/services/"+serviceName+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/apps/"+appID+"/services/"+serviceName+"?success=Retry+initiated", http.StatusFound)
}

func handleDeleteServicePost(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	serviceName := chi.URLParam(r, "serviceName")
	client := getAPIClient(r)
	if err := client.DeleteService(r.Context(), appID, serviceName); err != nil {
		http.Redirect(w, r, "/apps/"+appID+"/services/"+serviceName+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/apps/"+appID+"?success=Service+deleted", http.StatusFound)
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
	http.Redirect(w, r, "/settings/profile", http.StatusMovedPermanently)
}

func handleSettingsProfile(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	user, err := client.GetUserProfile(r.Context())
	
	successMsg := r.URL.Query().Get("success")
	errorMsg := r.URL.Query().Get("error")

	if err != nil {
		slog.Error("failed to get user profile", "error", err)
		settings_page.Profile(settings_page.ProfileData{
			ErrorMsg: "Failed to load profile",
		}).Render(r.Context(), w)
		return
	}

	data := settings_page.ProfileData{
		UserEmail:  user.Email,
		UserName:   user.Name,
		AvatarURL:  user.AvatarURL,
		SuccessMsg: successMsg,
		ErrorMsg:   errorMsg,
	}
	settings_page.Profile(data).Render(r.Context(), w)
}

func handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.FormValue("name")
	avatarURL := r.FormValue("avatar_url") // Hidden or from upload

	client := getAPIClient(r)
	_, err := client.UpdateUserProfile(r.Context(), name, avatarURL)
	if err != nil {
		slog.Error("failed to update user profile", "error", err)
		http.Redirect(w, r, "/settings/profile?error=Failed to update profile", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/settings/profile?success=Profile updated successfully", http.StatusSeeOther)
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

func handleSettingsServerLogs(w http.ResponseWriter, r *http.Request) {
	settings_page.ServerLogs(settings_page.ServerLogsData{}).Render(r.Context(), w)
}

func handleSettingsServerStats(w http.ResponseWriter, r *http.Request) {
	settings_page.ServerStats(settings_page.ServerStatsData{}).Render(r.Context(), w)
}

func handleSettingsSSHKeys(w http.ResponseWriter, r *http.Request) {
	// Mock data for SSH keys since backend is not implemented yet
	data := settings_page.SSHKeysData{
		Keys: []settings_page.SSHKey{
			{
				ID:          "key_1",
				Name:        "Personal MacBook",
				Fingerprint: "SHA256:m0...xX...Yy...Zz",
				Type:        "ed25519",
				CreatedAt:   time.Now().Add(-720 * time.Hour), // 30 days ago
			},
			{
				ID:          "key_2",
				Name:        "Work Workstation",
				Fingerprint: "SHA256:ab...cd...ef...gh",
				Type:        "ssh-rsa",
				CreatedAt:   time.Now().Add(-168 * time.Hour), // 7 days ago
			},
		},
	}
	settings_page.SSHKeys(data).Render(r.Context(), w)
}

func handleSettingsNotifications(w http.ResponseWriter, r *http.Request) {
	// Mock data for notification providers
	data := settings_page.NotificationsData{
		Providers: []settings_page.Provider{
			{
				ID:          "p1",
				Type:        settings_page.ProviderSlack,
				Name:        "Slack",
				Description: "Send notifications to a Slack channel using incoming webhooks.",
				Enabled:     true,
				Configured:  true,
				Config: map[string]string{
					"webhook_url": "",
				},
			},
			{
				ID:          "p2",
				Type:        settings_page.ProviderDiscord,
				Name:        "Discord",
				Description: "Post updates to a Discord server via webhook integration.",
				Enabled:     false,
				Configured:  true,
				Config: map[string]string{
					"webhook_url": "",
				},
			},
			{
				ID:          "p3",
				Type:        settings_page.ProviderTelegram,
				Name:        "Telegram",
				Description: "Receive instant alerts via a Telegram bot.",
				Enabled:     false,
				Configured:  false,
				Config:      make(map[string]string),
			},
			{
				ID:          "p4",
				Type:        settings_page.ProviderEmail,
				Name:        "Email (SMTP)",
				Description: "Send system alerts to your inbox using a custom SMTP server.",
				Enabled:     false,
				Configured:  false,
				Config:      make(map[string]string),
			},
			{
				ID:          "p5",
				Type:        settings_page.ProviderGotify,
				Name:        "Gotify",
				Description: "Self-hosted notification server. Receive alerts on your own infrastructure.",
				Enabled:     false,
				Configured:  false,
				Config:      make(map[string]string),
			},
			{
				ID:          "p6",
				Type:        settings_page.ProviderCustom,
				Name:        "Custom Webhook",
				Description: "Integrate with any third-party service by sending a custom HTTP POST request.",
				Enabled:     false,
				Configured:  false,
				Config:      make(map[string]string),
			},
		},
	}
	settings_page.Notifications(data).Render(r.Context(), w)
}

// ============================================================================
// Users Management Handlers
// ============================================================================

func handleSettingsUsers(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	
	successMsg := r.URL.Query().Get("success")
	errorMsg := r.URL.Query().Get("error")
	
	// Get current user
	currentUser, err := client.GetUserProfile(r.Context())
	if err != nil {
		slog.Error("failed to get current user", "error", err)
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	
	// Check if user is owner
	if currentUser.Role != "owner" {
		http.Redirect(w, r, "/?error=Access denied", http.StatusFound)
		return
	}
	
	// Get all users
	users, err := client.ListUsers(r.Context())
	if err != nil {
		slog.Error("failed to list users", "error", err)
		users = []api.UserInfo{}
		errorMsg = "Failed to load users"
	}
	
	// Get all invitations
	invitations, err := client.ListInvitations(r.Context())
	if err != nil {
		slog.Error("failed to list invitations", "error", err)
		invitations = []api.Invitation{}
	}
	
	// Filter to only show pending invitations
	pendingInvitations := []api.Invitation{}
	for _, inv := range invitations {
		if inv.Status == "pending" {
			pendingInvitations = append(pendingInvitations, inv)
		}
	}
	
	currentUserInfo := &api.UserInfo{
		ID:    currentUser.ID,
		Email: currentUser.Email,
		Name:  currentUser.Name,
		Role:  string(currentUser.Role),
	}
	
	data := settings_page.UsersData{
		Users:       users,
		Invitations: pendingInvitations,
		CurrentUser: currentUserInfo,
		SuccessMsg:  successMsg,
		ErrorMsg:    errorMsg,
	}
	settings_page.Users(data).Render(r.Context(), w)
}

func handleInviteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	email := r.FormValue("email")
	role := r.FormValue("role")
	
	if email == "" {
		http.Redirect(w, r, "/settings/users?error=Email is required", http.StatusSeeOther)
		return
	}
	
	if role == "" {
		role = "member"
	}
	
	client := getAPIClient(r)
	_, err := client.CreateInvitation(r.Context(), email, role)
	if err != nil {
		slog.Error("failed to create invitation", "error", err)
		http.Redirect(w, r, "/settings/users?error=Failed to send invitation", http.StatusSeeOther)
		return
	}
	
	http.Redirect(w, r, "/settings/users?success=Invitation sent to "+email, http.StatusSeeOther)
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	userID := r.FormValue("user_id")
	if userID == "" {
		http.Redirect(w, r, "/settings/users?error=User ID is required", http.StatusSeeOther)
		return
	}
	
	client := getAPIClient(r)
	err := client.DeleteUser(r.Context(), userID)
	if err != nil {
		slog.Error("failed to delete user", "error", err, "user_id", userID)
		http.Redirect(w, r, "/settings/users?error=Failed to remove user", http.StatusSeeOther)
		return
	}
	
	http.Redirect(w, r, "/settings/users?success=User removed successfully", http.StatusSeeOther)
}

func handleRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	invitationID := r.FormValue("invitation_id")
	if invitationID == "" {
		http.Redirect(w, r, "/settings/users?error=Invitation ID is required", http.StatusSeeOther)
		return
	}
	
	client := getAPIClient(r)
	err := client.RevokeInvitation(r.Context(), invitationID)
	if err != nil {
		slog.Error("failed to revoke invitation", "error", err, "invitation_id", invitationID)
		http.Redirect(w, r, "/settings/users?error=Failed to revoke invitation", http.StatusSeeOther)
		return
	}
	
	http.Redirect(w, r, "/settings/users?success=Invitation revoked", http.StatusSeeOther)
}

// ============================================================================
// Builds Handlers
// ============================================================================

func handleBuildsList(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	
	buildJobs, err := client.ListBuilds(r.Context())
	if err != nil {
		slog.Error("failed to list builds", "error", err)
		// Show empty list on error for now
		buildJobs = []api.Build{}
	}

	apps, err := client.ListApps(r.Context())
	appMap := make(map[string]string)
	if err == nil {
		for _, a := range apps {
			appMap[a.ID] = a.Name
		}
	}

	builds.List(builds.ListData{
		Builds: buildJobs,
		Apps:   appMap,
	}).Render(r.Context(), w)
}

func handleBuildsDetail(w http.ResponseWriter, r *http.Request) {
	buildID := chi.URLParam(r, "buildID")
	client := getAPIClient(r)

	buildJob, err := client.GetBuild(r.Context(), buildID)
	if err != nil {
		slog.Error("failed to get build detail", "error", err, "build_id", buildID)
		http.NotFound(w, r)
		return
	}

	app, err := client.GetApp(r.Context(), buildJob.AppID)
	appName := "Unknown App"
	if err == nil {
		appName = app.Name
	}

	builds.Detail(builds.DetailData{
		Build:   *buildJob,
		AppName: appName,
	}).Render(r.Context(), w)
}

func handleBuildRetry(w http.ResponseWriter, r *http.Request) {
	buildID := chi.URLParam(r, "buildID")
	client := getAPIClient(r)

	if err := client.RetryBuild(r.Context(), buildID); err != nil {
		slog.Error("failed to retry build", "error", err, "build_id", buildID)
	}

	http.Redirect(w, r, "/builds/"+buildID, http.StatusSeeOther)
}

// ============================================================================
// Deployments Handlers
// ============================================================================

func handleDeploymentsList(w http.ResponseWriter, r *http.Request) {
	client := getAPIClient(r)
	
	deployList, err := client.ListDeployments(r.Context())
	if err != nil {
		slog.Error("failed to list deployments", "error", err)
		deployList = []api.Deployment{}
	}

	apps, err := client.ListApps(r.Context())
	appMap := make(map[string]string)
	if err == nil {
		for _, a := range apps {
			appMap[a.ID] = a.Name
		}
	}

	deployments.List(deployments.ListData{
		Deployments: deployList,
		Apps:        appMap,
	}).Render(r.Context(), w)
}

func handleDeploymentsDetail(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "deploymentID")
	client := getAPIClient(r)

	deployment, err := client.GetDeployment(r.Context(), deploymentID)
	if err != nil {
		slog.Error("failed to get deployment detail", "error", err, "deployment_id", deploymentID)
		http.NotFound(w, r)
		return
	}

	app, err := client.GetApp(r.Context(), deployment.AppID)
	appName := "Unknown App"
	if err == nil {
		appName = app.Name
	}

	var node *api.Node
	if deployment.NodeID != "" {
		node, _ = client.GetNode(r.Context(), deployment.NodeID)
	}

	deployments.Detail(deployments.DetailData{
		Deployment: *deployment,
		AppName:    appName,
		Node:       node,
	}).Render(r.Context(), w)
}

func handleDeploymentRollback(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "deploymentID")
	// Rollback implementation in client/API would go here
	// For now just redirect back
	http.Redirect(w, r, "/deployments/"+deploymentID, http.StatusSeeOther)
}

// ============================================================================
// Invitation Acceptance Handlers
// ============================================================================

func handleInviteAcceptPage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	client := getAPIClient(r)
	invitation, err := client.GetInvitationByToken(r.Context(), token)
	if err != nil {
		slog.Error("failed to get invitation", "error", err, "token", token)
		auth.AcceptInvite(auth.AcceptInviteData{
			Token:   token,
			IsValid: false,
			Error:   "Failed to load invitation details",
		}).Render(r.Context(), w)
		return
	}

	// Check if invitation is valid
	isValid := invitation.Status == "pending"
	isExpired := invitation.Status == "expired"

	auth.AcceptInvite(auth.AcceptInviteData{
		Token:     token,
		Email:     invitation.Email,
		IsValid:   isValid,
		IsExpired: isExpired,
	}).Render(r.Context(), w)
}

func handleInviteAcceptSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		auth.AcceptInvite(auth.AcceptInviteData{
			Error: "Invalid form data",
		}).Render(r.Context(), w)
		return
	}

	token := r.FormValue("token")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	if token == "" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if password == "" {
		auth.AcceptInvite(auth.AcceptInviteData{
			Token:   token,
			IsValid: true,
			Error:   "Password is required",
		}).Render(r.Context(), w)
		return
	}

	if len(password) < 8 {
		auth.AcceptInvite(auth.AcceptInviteData{
			Token:   token,
			IsValid: true,
			Error:   "Password must be at least 8 characters",
		}).Render(r.Context(), w)
		return
	}

	if password != confirmPassword {
		auth.AcceptInvite(auth.AcceptInviteData{
			Token:   token,
			IsValid: true,
			Error:   "Passwords do not match",
		}).Render(r.Context(), w)
		return
	}

	client := getAPIClient(r)
	resp, err := client.AcceptInvitation(r.Context(), token, password)
	if err != nil {
		slog.Error("failed to accept invitation", "error", err)
		
		// Get invitation details to show proper error
		invitation, _ := client.GetInvitationByToken(r.Context(), token)
		email := ""
		isValid := true
		isExpired := false
		if invitation != nil {
			email = invitation.Email
			isValid = invitation.Status == "pending"
			isExpired = invitation.Status == "expired"
		}
		
		auth.AcceptInvite(auth.AcceptInviteData{
			Token:     token,
			Email:     email,
			IsValid:   isValid,
			IsExpired: isExpired,
			Error:     "Failed to create account. The invitation may have expired or already been used.",
		}).Render(r.Context(), w)
		return
	}

	// Set auth cookie and redirect to dashboard
	setAuthCookie(w, resp.Token)
	http.Redirect(w, r, "/", http.StatusFound)
}
