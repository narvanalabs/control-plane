package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/integrations/github"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// GitHubHandler handles GitHub-related endpoints.
type GitHubHandler struct {
	store        store.Store
	githubClient *github.Client
	logger       *slog.Logger
}

// NewGitHubHandler creates a new GitHub handler.
func NewGitHubHandler(st store.Store, logger *slog.Logger) *GitHubHandler {
	return &GitHubHandler{
		store:        st,
		githubClient: github.NewClient(),
		logger:       logger,
	}
}

// getBaseURLs determines the web and API base URLs from the request and environment.
func (h *GitHubHandler) getBaseURLs(r *http.Request) (string, string) {
	ctx := r.Context()

	// 1. Check database settings first (source of truth)
	dbDomain, _ := h.store.Settings().Get(ctx, "server_domain")

	// 2. Determine scheme
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	// 3. Determine API URL
	host := r.Host
	if forwardHost := r.Header.Get("X-Forwarded-Host"); forwardHost != "" {
		host = forwardHost
	}
	detectedAPI := fmt.Sprintf("%s://%s", scheme, host)

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = detectedAPI
	}

	// 4. Determine Web URL
	detectedWeb := detectedAPI
	if webOverride := r.URL.Query().Get("web_url"); webOverride != "" {
		detectedWeb = webOverride
	} else if origin := r.Header.Get("Origin"); origin != "" {
		detectedWeb = origin
	} else if referer := r.Header.Get("Referer"); referer != "" {
		if u, err := url.Parse(referer); err == nil {
			detectedWeb = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
		}
	}

	webURL := os.Getenv("WEB_URL")
	if webURL == "" {
		webURL = detectedWeb
	}

	// 5. Override if database setting exists and isn't just "localhost"
	if dbDomain != "" && dbDomain != "localhost" {
		// If it's an IP, use it directly. If it's a domain, use it directly.
		// We'll assume the ports are standard or same as detected
		h.logger.Debug("using server_domain from settings", "domain", dbDomain)

		// We need to be careful with ports. 
		// For now, if dbDomain doesn't have a port, we'll append standard ports if they exist in detected URLs
		apiUrlObj, _ := url.Parse(apiURL)
		webUrlObj, _ := url.Parse(webURL)

		apiHost := dbDomain
		if apiUrlObj != nil && apiUrlObj.Port() != "" {
			apiHost = dbDomain + ":" + apiUrlObj.Port()
		}

		webHost := dbDomain
		if webUrlObj != nil && webUrlObj.Port() != "" {
			webHost = dbDomain + ":" + webUrlObj.Port()
		}

		return fmt.Sprintf("%s://%s", scheme, webHost), fmt.Sprintf("%s://%s", scheme, apiHost)
	}

	return webURL, apiURL
}

// ManifestStart returns the URL to start the GitHub App manifest flow.
func (h *GitHubHandler) ManifestStart(w http.ResponseWriter, r *http.Request) {
	webURL, apiURL := h.getBaseURLs(r)

	// Generate a name if not provided: Narvana-{random}
	appName := r.URL.Query().Get("app_name")
	if appName == "" {
		suffix := uuid.New().String()[:8]
		appName = fmt.Sprintf("Narvana-%s", suffix)
	}

	manifest := map[string]interface{}{
		"name":        appName,
		"url":         webURL,
		"description": "Automated GitHub integration for Narvana Control Plane.",
		"redirect_url": apiURL + "/github/callback",
		"callback_urls": []string{
			apiURL + "/github/callback", // Needed for manifest redirect
			apiURL + "/github/oauth/callback",
		},
		"setup_url": apiURL + "/v1/github/post-install",
		"public":    false,
		"default_permissions": map[string]string{
			"contents":      "read",
			"metadata":      "read",
			"pull_requests": "read",
			"statuses":      "write",
			"checks":        "write",
		},
	}

	// 3. Handle Webhook URL (GitHub rejects 'localhost' in manifest flow)
	webhookURL := os.Getenv("GITHUB_WEBHOOK_URL")
	if webhookURL == "" {
		webhookURL = apiURL + "/v1/github/webhook"
	}

	// Only include hook_attributes and events if the URL appears publicly reachable
	// GitHub specifically blocks localhost/127.0.0.1 in the manifest setup flow.
	// If it's local, we MUST omit both to avoid "Hook url cannot be blank" error.
	isLocal := strings.Contains(webhookURL, "localhost") || strings.Contains(webhookURL, "127.0.0.1")
	if !isLocal {
		manifest["hook_attributes"] = map[string]interface{}{
			"url": webhookURL,
		}
		manifest["default_events"] = []string{
			"push",
			"pull_request",
			"check_run",
			"check_suite",
		}
	} else {
		h.logger.Info("Omitting hook_attributes and events from manifest because URL is local", "url", webhookURL)
	}

	manifestJSON, _ := json.Marshal(manifest)
	h.logger.Info("GitHub App Manifest generated (POST flow)", "name", appName)

	// Determine setup URL based on whether it's for an organization
	org := r.URL.Query().Get("org")
	var githubURL string
	if org != "" {
		githubURL = fmt.Sprintf("https://github.com/organizations/%s/settings/apps/new", url.PathEscape(org))
	} else {
		githubURL = "https://github.com/settings/apps/new"
	}

	// Return HTML form that auto-submits via POST
	// This is much more reliable for large manifests than GET params
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>Redirecting to GitHub...</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100vh; margin: 0; background-color: #0d1117; color: #c9d1d9; }
        .spinner { border: 4px solid rgba(255, 255, 255, 0.1); border-left-color: #58a6ff; border-radius: 50%%; width: 40px; height: 40px; animation: spin 1s linear infinite; margin-bottom: 20px; }
        @keyframes spin { to { transform: rotate(360deg); } }
    </style>
</head>
<body>
    <div class="spinner"></div>
    <p>Redirecting to GitHub to create your app...</p>
    <form id="manifest-form" method="POST" action="%s">
        <input type="hidden" name="manifest" value='%s'>
    </form>
    <script>
        document.getElementById('manifest-form').submit();
    </script>
</body>
</html>
    `, githubURL, string(manifestJSON))

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

// ManifestCallback handles the callback from GitHub App creation.
func (h *GitHubHandler) ManifestCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "code required"})
		return
	}

	ctx := r.Context()
	conversion, err := h.githubClient.ConvertManifest(ctx, code)
	if err != nil {
		h.logger.Error("failed to convert manifest", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to complete GitHub App setup"})
		return
	}

	// Save the config
	config := &models.GitHubAppConfig{
		ConfigType:    "app",
		AppID:         &conversion.ID,
		ClientID:      conversion.ClientID,
		ClientSecret:  conversion.ClientSecret,
		WebhookSecret: &conversion.WebhookSecret,
		PrivateKey:    &conversion.PEM,
		Slug:          &conversion.Slug,
	}

	if err := h.store.GitHub().SaveConfig(ctx, config); err != nil {
		h.logger.Error("failed to save GitHub config", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Seamlessly redirect to the installation page on GitHub
	installURL := fmt.Sprintf("https://github.com/settings/apps/%s/installations/new", *config.Slug)
	http.Redirect(w, r, installURL, http.StatusFound)
}

// OAuthStart redirects the user to GitHub's OAuth authorization page.
func (h *GitHubHandler) OAuthStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config, err := h.store.GitHub().GetConfig(ctx)
	if err != nil || config == nil || config.ConfigType != "oauth" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Standard OAuth not configured"})
		return
	}

	_, apiURL := h.getBaseURLs(r)

	params := url.Values{}
	params.Set("client_id", config.ClientID)
	params.Set("redirect_uri", apiURL+"/github/oauth/callback")
	params.Set("scope", "repo,user")
	params.Set("state", uuid.New().String()) // In a real app, store this in session/cookie

	authorizeURL := "https://github.com/login/oauth/authorize?" + params.Encode()
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// OAuthCallback handles the callback from GitHub's OAuth flow.
func (h *GitHubHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/git?error=missing_code", http.StatusFound)
		return
	}

	ctx := r.Context()
	config, err := h.store.GitHub().GetConfig(ctx)
	if err != nil || config == nil {
		h.logger.Error("failed to get GitHub config", "error", err)
		http.Redirect(w, r, "/git?error=not_configured", http.StatusFound)
		return
	}

	// Exchange code for token
	tokenData, err := h.githubClient.ExchangeCode(ctx, config.ClientID, config.ClientSecret, code)
	if err != nil {
		h.logger.Error("failed to exchange code", "error", err)
		http.Redirect(w, r, "/git?error=token_exchange_failed", http.StatusFound)
		return
	}

	accessToken := tokenData["access_token"].(string)

	// Fetch user info
	userData, err := h.githubClient.GetUser(ctx, accessToken)
	if err != nil {
		h.logger.Error("failed to fetch user info", "error", err)
		http.Redirect(w, r, "/git?error=user_info_failed", http.StatusFound)
		return
	}

	githubID := int64(userData["id"].(float64))
	login := userData["login"].(string)
	userID := middleware.GetUserID(ctx)

	// Save account
	account := &models.GitHubAccount{
		ID:          githubID,
		Login:       login,
		AccessToken: accessToken,
		UserID:      userID,
	}
	if name, ok := userData["name"].(string); ok {
		account.Name = name
	}
	if avatar, ok := userData["avatar_url"].(string); ok {
		account.AvatarURL = avatar
	}

	if err := h.store.GitHubAccounts().Create(ctx, account); err != nil {
		h.logger.Error("failed to save GitHub account", "error", err)
		http.Redirect(w, r, "/git?error=internal_error", http.StatusFound)
		return
	}

	webURL := os.Getenv("WEB_URL")
	if webURL == "" {
		webURL = "http://localhost:8090"
	}
	http.Redirect(w, r, webURL+"/git?success=GitHub+account+connected", http.StatusFound)
}

// SaveConfigManual saves the GitHub configuration manually (for OAuth).
func (h *GitHubHandler) SaveConfigManual(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConfigType   string `json:"config_type"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if req.ClientID == "" || req.ClientSecret == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "client_id and client_secret required"})
		return
	}

	ctx := r.Context()
	config := &models.GitHubAppConfig{
		ConfigType:   req.ConfigType,
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret,
	}

	if err := h.store.GitHub().SaveConfig(ctx, config); err != nil {
		h.logger.Error("failed to save GitHub config", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// AppInstall returns the URL to install the GitHub App.
func (h *GitHubHandler) AppInstall(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config, err := h.store.GitHub().GetConfig(ctx)
	if err != nil || config == nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "GitHub App not configured"})
		return
	}

	installURL := fmt.Sprintf("https://github.com/settings/apps/%s/installations/new", *config.Slug)
	WriteJSON(w, http.StatusOK, map[string]string{"url": installURL})
}

// PostInstallation handles the redirect after a user installs the GitHub App.
func (h *GitHubHandler) PostInstallation(w http.ResponseWriter, r *http.Request) {
	installationIDStr := r.URL.Query().Get("installation_id")
	if installationIDStr == "" {
		http.Redirect(w, r, "/git?error=missing_installation_id", http.StatusFound)
		return
	}

	installationID, _ := strconv.ParseInt(installationIDStr, 10, 64)
	userID := middleware.GetUserID(r.Context())

	// Save the installation
	// Note: In a real app we'd fetch installation details from GitHub first
	inst := &models.GitHubInstallation{
		ID:           installationID,
		UserID:       userID,
		AccountLogin: "Searching...", // Will be updated on first repo list/webhook
	}

	if err := h.store.GitHub().CreateInstallation(r.Context(), inst); err != nil {
		h.logger.Error("failed to create installation", "error", err)
	}

	webURL, _ := h.getBaseURLs(r)

	http.Redirect(w, r, webURL+"/git?success=GitHub+App+installed", http.StatusFound)
}

// ListRepos lists repositories for the authenticated user from both App installations and OAuth accounts.
func (h *GitHubHandler) ListRepos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)

	var allRepos []map[string]interface{}

	// 1. Collect repos from App installations
	installations, err := h.store.GitHub().ListInstallations(ctx, userID)
	if err == nil && len(installations) > 0 {
		config, err := h.store.GitHub().GetConfig(ctx)
		if err == nil && config != nil && config.ConfigType == "app" {
			for _, inst := range installations {
				token, err := h.githubClient.GenerateInstallationToken(ctx, *config.AppID, *config.PrivateKey, inst.ID)
				if err != nil {
					h.logger.Error("failed to generate installation token", "installation_id", inst.ID, "error", err)
					continue
				}

				repos, err := h.githubClient.ListRepositories(ctx, token)
				if err != nil {
					h.logger.Error("failed to list repositories", "installation_id", inst.ID, "error", err)
					continue
				}
				allRepos = append(allRepos, repos...)
			}
		}
	}

	// 2. Collect repos from standard OAuth accounts
	account, err := h.store.GitHubAccounts().GetByUserID(ctx, userID)
	if err == nil && account != nil {
		repos, err := h.githubClient.ListUserRepositories(ctx, account.AccessToken)
		if err != nil {
			h.logger.Error("failed to list user repositories", "account_id", account.ID, "error", err)
		} else {
			allRepos = append(allRepos, repos...)
		}
	}

	// 3. Deduplicate repos by ID
	seen := make(map[int64]bool)
	uniqueRepos := []map[string]interface{}{}
	for _, repo := range allRepos {
		id := int64(repo["id"].(float64))
		if !seen[id] {
			seen[id] = true
			uniqueRepos = append(uniqueRepos, repo)
		}
	}

	WriteJSON(w, http.StatusOK, uniqueRepos)
}

// ListInstallations lists all GitHub App installations for the authenticated user.
func (h *GitHubHandler) ListInstallations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)

	installations, err := h.store.GitHub().ListInstallations(ctx, userID)
	if err != nil {
		h.logger.Error("failed to list installations", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	WriteJSON(w, http.StatusOK, installations)
}

// GetConfig returns the current GitHub App configuration status.
func (h *GitHubHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config, err := h.store.GitHub().GetConfig(ctx)
	if err != nil {
		h.logger.Error("failed to get GitHub config", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if config == nil {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"configured": false,
		})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"configured":  true,
		"config_type": config.ConfigType,
		"app_id":      config.AppID,
		"slug":        config.Slug,
	})
}

// ResetConfig clears the GitHub configuration and all installations/accounts.
func (h *GitHubHandler) ResetConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.store.GitHub().ResetConfig(ctx); err != nil {
		h.logger.Error("failed to reset GitHub config", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "success"})
}
