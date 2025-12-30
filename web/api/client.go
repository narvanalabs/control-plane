// Package api provides a client for communicating with the control-plane API.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is an API client for the control-plane.
type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

// NewClient creates a new API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WithToken returns a new client with the specified auth token.
func (c *Client) WithToken(token string) *Client {
	return &Client{
		baseURL:    c.baseURL,
		httpClient: c.httpClient,
		token:      token,
	}
}

// ============================================================================
// Data Types
// ============================================================================

// App represents an application from the API.
type App struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"owner_id"`
	Name      string    `json:"name"`
	Services  []Service `json:"services"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Service represents a service within an app.
type Service struct {
	Name       string `json:"name"`
	SourceType string `json:"source_type"`
	GitRepo    string `json:"git_repo,omitempty"`
	GitRef     string `json:"git_ref,omitempty"`
	FlakeURI   string `json:"flake_uri,omitempty"`
	ImageRef   string `json:"image_ref,omitempty"`
}

// Deployment represents a deployment from the API.
type Deployment struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	ServiceName string    `json:"service_name"`
	Version     int       `json:"version"`
	Status      string    `json:"status"`
	NodeID      string    `json:"node_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Node represents a compute node from the API.
type Node struct {
	ID            string         `json:"id"`
	Hostname      string         `json:"hostname"`
	Address       string         `json:"address"`
	Healthy       bool           `json:"healthy"`
	Resources     *NodeResources `json:"resources,omitempty"`
	LastHeartbeat time.Time      `json:"last_heartbeat"`
}

// NodeResources represents resource availability.
type NodeResources struct {
	CPUTotal        float64 `json:"cpu_total"`
	CPUAvailable    float64 `json:"cpu_available"`
	MemoryTotal     int64   `json:"memory_total"`
	MemoryAvailable int64   `json:"memory_available"`
}

// Build represents a build job from the API.
type Build struct {
	ID           string    `json:"id"`
	AppID        string    `json:"app_id"`
	DeploymentID string    `json:"deployment_id"`
	Status       string    `json:"status"`
	Strategy     string    `json:"build_strategy,omitempty"`
	Logs         string    `json:"logs,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Secret represents a secret/env var for an app.
type Secret struct {
	Key       string    `json:"key"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Log represents a log entry.
type Log struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deployment_id"`
	Source       string    `json:"source"`
	Level        string    `json:"level"`
	Message      string    `json:"message"`
	Timestamp    time.Time `json:"timestamp"`
}

// ============================================================================
// Dashboard Types
// ============================================================================

// DashboardStats holds aggregated stats for the dashboard.
type DashboardStats struct {
	TotalApps         int
	ActiveDeployments int
	HealthyNodes      int
	RunningBuilds     int
}

// RecentDeployment holds data for recent deployments display.
type RecentDeployment struct {
	AppName     string
	ServiceName string
	Status      string
	TimeAgo     string
}

// NodeHealth holds node health display data.
type NodeHealth struct {
	Name       string
	Address    string
	Healthy    bool
	CPUPercent int
	MemPercent int
}

// ============================================================================
// Auth Types
// ============================================================================

// AuthResponse is returned from login/register.
type AuthResponse struct {
	Token  string `json:"token"`
	UserID string `json:"user_id"`
}

// ============================================================================
// Request Types
// ============================================================================

// CreateAppRequest is the request body for creating an app.
type CreateAppRequest struct {
	Name string `json:"name"`
}

// BuildStrategy represents the method used to build an application.
type BuildStrategy string

const (
	BuildStrategyFlake      BuildStrategy = "flake"       // Use existing flake.nix
	BuildStrategyAutoGo     BuildStrategy = "auto-go"     // Generate flake for Go
	BuildStrategyAutoRust   BuildStrategy = "auto-rust"   // Generate flake for Rust
	BuildStrategyAutoNode   BuildStrategy = "auto-node"   // Generate flake for Node.js
	BuildStrategyAutoPython BuildStrategy = "auto-python" // Generate flake for Python
	BuildStrategyDockerfile BuildStrategy = "dockerfile"  // Build from Dockerfile
	BuildStrategyNixpacks   BuildStrategy = "nixpacks"    // Use Nixpacks
	BuildStrategyAuto       BuildStrategy = "auto"        // Auto-detect
)

// BuildConfig contains strategy-specific configuration options.
type BuildConfig struct {
	BuildCommand string            `json:"build_command,omitempty"`
	StartCommand string            `json:"start_command,omitempty"`
	EntryPoint   string            `json:"entry_point,omitempty"`
	EnvVars      map[string]string `json:"environment_vars,omitempty"`
}

// CreateServiceRequest is the request body for creating a service.
type CreateServiceRequest struct {
	Name       string         `json:"name"`
	SourceType string         `json:"source_type"`
	GitRepo    string         `json:"git_repo,omitempty"`
	GitRef     string         `json:"git_ref,omitempty"`
	FlakeURI   string         `json:"flake_uri,omitempty"`
	ImageRef   string         `json:"image_ref,omitempty"`
	BuildStrategy BuildStrategy `json:"build_strategy,omitempty"`
	BuildConfig   *BuildConfig  `json:"build_config,omitempty"`
}

// CreateSecretRequest is the request body for creating a secret.
type CreateSecretRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// LoginRequest is the request body for login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterRequest is the request body for registration.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// GitHubConfigStatus represents the configuration state of the GitHub integration.
type GitHubConfigStatus struct {
	Configured bool   `json:"configured"`
	ConfigType string `json:"config_type,omitempty"` // "app" or "oauth"
	AppID      *int64 `json:"app_id,omitempty"`
	Slug       *string `json:"slug,omitempty"`
}

// GitHubInstallation represents an installation of the GitHub App.
type GitHubInstallation struct {
	ID           int64  `json:"id"`
	AccountLogin string `json:"account_login"`
	AccountType  string `json:"account_type"`
}

// GitHubRepository represents a repository from the GitHub API.
type GitHubRepository struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
}

// ============================================================================
// Auth Methods
// ============================================================================

// Login authenticates a user and returns a JWT token.
func (c *Client) Login(ctx context.Context, email, password string) (*AuthResponse, error) {
	req := LoginRequest{Email: email, Password: password}
	var resp AuthResponse
	err := c.post(ctx, "/auth/login", req, &resp)
	return &resp, err
}

// Register creates a new user account.
func (c *Client) Register(ctx context.Context, email, password string) (*AuthResponse, error) {
	req := RegisterRequest{Email: email, Password: password}
	var resp AuthResponse
	err := c.post(ctx, "/auth/register", req, &resp)
	return &resp, err
}

// ============================================================================
// App Methods
// ============================================================================

// ListApps fetches all apps for the current user.
func (c *Client) ListApps(ctx context.Context) ([]App, error) {
	var apps []App
	err := c.Get(ctx, "/v1/apps", &apps)
	if apps == nil {
		apps = []App{}
	}
	return apps, err
}

// GetApp fetches a single app by ID.
func (c *Client) GetApp(ctx context.Context, id string) (*App, error) {
	var app App
	err := c.Get(ctx, "/v1/apps/"+id, &app)
	return &app, err
}

// CreateApp creates a new application.
func (c *Client) CreateApp(ctx context.Context, name string) (*App, error) {
	req := CreateAppRequest{Name: name}
	var app App
	err := c.post(ctx, "/v1/apps", req, &app)
	return &app, err
}

// DeleteApp deletes an application.
func (c *Client) DeleteApp(ctx context.Context, id string) error {
	return c.delete(ctx, "/v1/apps/"+id)
}

// ============================================================================
// Service Methods
// ============================================================================

// CreateService adds a service to an app.
func (c *Client) CreateService(ctx context.Context, appID string, svc CreateServiceRequest) (*Service, error) {
	var service Service
	err := c.post(ctx, "/v1/apps/"+appID+"/services", svc, &service)
	return &service, err
}

// UpdateService updates an existing service.
func (c *Client) UpdateService(ctx context.Context, appID, serviceName string, svc CreateServiceRequest) (*Service, error) {
	var service Service
	err := c.patch(ctx, "/v1/apps/"+appID+"/services/"+serviceName, svc, &service)
	return &service, err
}

// DeleteService removes a service from an app.
func (c *Client) DeleteService(ctx context.Context, appID, serviceName string) error {
	return c.delete(ctx, "/v1/apps/"+appID+"/services/"+serviceName)
}

// ============================================================================
// Deployment Methods
// ============================================================================

// Deploy triggers a deployment for a service.
func (c *Client) Deploy(ctx context.Context, appID, serviceName string) (*Deployment, error) {
	var deployment Deployment
	err := c.post(ctx, "/v1/apps/"+appID+"/services/"+serviceName+"/deploy", nil, &deployment)
	return &deployment, err
}

// ListAppDeployments fetches all deployments for an app.
func (c *Client) ListAppDeployments(ctx context.Context, appID string) ([]Deployment, error) {
	var deployments []Deployment
	err := c.Get(ctx, "/v1/apps/"+appID+"/deployments", &deployments)
	if deployments == nil {
		deployments = []Deployment{}
	}
	return deployments, err
}

// GetDeployment fetches a single deployment by ID.
func (c *Client) GetDeployment(ctx context.Context, id string) (*Deployment, error) {
	var deployment Deployment
	err := c.Get(ctx, "/v1/deployments/"+id, &deployment)
	return &deployment, err
}

// RollbackDeployment rolls back a deployment.
func (c *Client) RollbackDeployment(ctx context.Context, id string) (*Deployment, error) {
	var deployment Deployment
	err := c.post(ctx, "/v1/deployments/"+id+"/rollback", nil, &deployment)
	return &deployment, err
}

// ============================================================================
// Secret Methods
// ============================================================================

// ListSecrets fetches all secrets for an app.
func (c *Client) ListSecrets(ctx context.Context, appID string) ([]Secret, error) {
	var secrets []Secret
	err := c.Get(ctx, "/v1/apps/"+appID+"/secrets", &secrets)
	if secrets == nil {
		secrets = []Secret{}
	}
	return secrets, err
}

// CreateSecret adds a secret to an app.
func (c *Client) CreateSecret(ctx context.Context, appID, key, value string) error {
	req := CreateSecretRequest{Key: key, Value: value}
	return c.post(ctx, "/v1/apps/"+appID+"/secrets", req, nil)
}

// DeleteSecret removes a secret from an app.
func (c *Client) DeleteSecret(ctx context.Context, appID, key string) error {
	return c.delete(ctx, "/v1/apps/"+appID+"/secrets/"+key)
}

// ============================================================================
// Log Methods
// ============================================================================

// GetAppLogs fetches logs for an app.
func (c *Client) GetAppLogs(ctx context.Context, appID string) ([]Log, error) {
	var logs []Log
	err := c.Get(ctx, "/v1/apps/"+appID+"/logs", &logs)
	if logs == nil {
		logs = []Log{}
	}
	return logs, err
}

// ============================================================================
// Node Methods
// ============================================================================

// ListNodes fetches all nodes.
func (c *Client) ListNodes(ctx context.Context) ([]Node, error) {
	var nodes []Node
	err := c.Get(ctx, "/v1/nodes", &nodes)
	if nodes == nil {
		nodes = []Node{}
	}
	return nodes, err
}

// ============================================================================
// GitHub Methods
// ============================================================================

// GetGitHubConfig checks if the GitHub App is configured.
func (c *Client) GetGitHubConfig(ctx context.Context) (*GitHubConfigStatus, error) {
	var status GitHubConfigStatus
	err := c.Get(ctx, "/v1/github/config", &status)
	return &status, err
}

// GetGitHubSetupURL returns the path to start GitHub App creation.
// Note: This endpoint now returns HTML for a POST manifest flow, so we don't fetch it as JSON.
func (c *Client) GetGitHubSetupURL(ctx context.Context, org string) (string, error) {
	path := "/v1/github/setup"
	if org != "" {
		path += "?org=" + url.QueryEscape(org)
	}
	return path, nil
}

// GetGitHubOAuthURL gets the URL to start standard GitHub OAuth authorization.
func (c *Client) GetGitHubOAuthURL(ctx context.Context) (string, error) {
	var resp struct {
		URL string `json:"url"`
	}
	err := c.Get(ctx, "/v1/github/oauth/start", &resp)
	return resp.URL, err
}

// SaveGitHubConfig saves the GitHub configuration (for manual OAuth).
func (c *Client) SaveGitHubConfig(ctx context.Context, configType, clientID, clientSecret string) error {
	req := map[string]string{
		"config_type":   configType,
		"client_id":     clientID,
		"client_secret": clientSecret,
	}
	return c.post(ctx, "/v1/github/config", req, nil)
}

// GetGitHubInstallURL gets the URL to install the GitHub App.
func (c *Client) GetGitHubInstallURL(ctx context.Context) (string, error) {
	var resp struct {
		URL string `json:"url"`
	}
	err := c.Get(ctx, "/v1/github/install", &resp)
	return resp.URL, err
}

// ResetGitHubConfig clears the GitHub configuration and all associated data.
func (c *Client) ResetGitHubConfig(ctx context.Context) error {
	return c.delete(ctx, "/v1/github/config")
}

// ListGitHubInstallations lists all GitHub App installations.
func (c *Client) ListGitHubInstallations(ctx context.Context) ([]GitHubInstallation, error) {
	var resp []GitHubInstallation
	err := c.Get(ctx, "/v1/github/installations", &resp)
	return resp, err
}

// ListGitHubRepos lists repositories across all user installations.
func (c *Client) ListGitHubRepos(ctx context.Context) ([]GitHubRepository, error) {
	var repos []GitHubRepository
	err := c.Get(ctx, "/v1/github/repos", &repos)
	if repos == nil {
		repos = []GitHubRepository{}
	}
	return repos, err
}

// ============================================================================
// Settings Methods
// ============================================================================

// GetSettings fetches global settings.
func (c *Client) GetSettings(ctx context.Context) (map[string]string, error) {
	var settings map[string]string
	err := c.Get(ctx, "/v1/settings", &settings)
	return settings, err
}

// UpdateSettings updates global settings.
func (c *Client) UpdateSettings(ctx context.Context, settings map[string]string) error {
	return c.patch(ctx, "/v1/settings", settings, nil)
}

// ============================================================================
// Dashboard Methods
// ============================================================================

// GetDashboardData fetches all dashboard data.
func (c *Client) GetDashboardData(ctx context.Context) (*DashboardStats, []RecentDeployment, []NodeHealth, error) {
	stats := &DashboardStats{}
	var recentDeployments []RecentDeployment
	var nodeHealth []NodeHealth

	// Fetch apps
	apps, err := c.ListApps(ctx)
	if err == nil {
		stats.TotalApps = len(apps)
	}

	// Fetch nodes
	nodes, err := c.ListNodes(ctx)
	if err == nil {
		for _, node := range nodes {
			if node.Healthy {
				stats.HealthyNodes++
			}
			nh := NodeHealth{
				Name:    node.Hostname,
				Address: node.Address,
				Healthy: node.Healthy,
			}
			if node.Resources != nil && node.Resources.CPUTotal > 0 {
				nh.CPUPercent = int(((node.Resources.CPUTotal - node.Resources.CPUAvailable) / node.Resources.CPUTotal) * 100)
			}
			if node.Resources != nil && node.Resources.MemoryTotal > 0 {
				nh.MemPercent = int(((float64(node.Resources.MemoryTotal) - float64(node.Resources.MemoryAvailable)) / float64(node.Resources.MemoryTotal)) * 100)
			}
			nodeHealth = append(nodeHealth, nh)
		}
	}

	// Fetch recent deployments from all apps
	for _, app := range apps {
		deployments, err := c.ListAppDeployments(ctx, app.ID)
		if err == nil {
			for _, d := range deployments {
				if d.Status == "running" {
					stats.ActiveDeployments++
				}
				// Add to recent (limit to first 5)
				if len(recentDeployments) < 5 {
					recentDeployments = append(recentDeployments, RecentDeployment{
						AppName:     app.Name,
						ServiceName: d.ServiceName,
						Status:      d.Status,
						TimeAgo:     formatTimeAgo(d.CreatedAt),
					})
				}
			}
		}
	}

	return stats, recentDeployments, nodeHealth, nil
}

func formatTimeAgo(t time.Time) string {
	diff := time.Since(t)
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	default:
		return t.Format("Jan 2")
	}
}

// ============================================================================
// HTTP Helpers
// ============================================================================

// Get performs a GET request and unmarshals the response.
func (c *Client) Get(ctx context.Context, path string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	return c.doRequest(req, result)
}

// GetRaw performs a GET request and returns the raw response body as a byte slice.
func (c *Client) GetRaw(ctx context.Context, path string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	return body, resp.Header.Get("Content-Type"), nil
}

// post performs a POST request and unmarshals the response.
func (c *Client) post(ctx context.Context, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.doRequest(req, result)
}

// patch performs a PATCH request and unmarshals the response.
func (c *Client) patch(ctx context.Context, path string, body interface{}, result interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", c.baseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.doRequest(req, result)
}

// delete performs a DELETE request.
func (c *Client) delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	return c.doRequest(req, nil)
}

// doRequest executes the HTTP request and handles the response.
func (c *Client) doRequest(req *http.Request, result interface{}) error {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}

	return nil
}
