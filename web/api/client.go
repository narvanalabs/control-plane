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
	"sync"
	"time"

	"github.com/narvanalabs/control-plane/internal/store"
)

// Client is an API client for the control-plane.
type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
	orgID      string // Organization ID for X-Org-ID header
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
		orgID:      c.orgID,
	}
}

// WithOrg returns a new client with the specified organization ID.
// The organization ID will be included in the X-Org-ID header for all requests.
// **Validates: Requirements 13.1**
func (c *Client) WithOrg(orgID string) *Client {
	return &Client{
		baseURL:    c.baseURL,
		httpClient: c.httpClient,
		token:      c.token,
		orgID:      orgID,
	}
}

// ============================================================================
// Data Types
// ============================================================================

// App represents an application from the API.
type App struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	OwnerID     string    `json:"owner_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IconURL     string    `json:"icon_url"`
	Services    []Service `json:"services"`
	Version     int       `json:"version"` // For optimistic locking
	Domains     []string  `json:"domains"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Service represents a service within an app.
type Service struct {
	Name       string `json:"name"`
	SourceType string `json:"source_type"`
	GitRepo    string `json:"git_repo,omitempty"`
	GitRef     string `json:"git_ref,omitempty"`
	FlakeURI   string `json:"flake_uri,omitempty"`
	Database   *DatabaseConfig `json:"database,omitempty"`

	// Build & Runtime
	BuildStrategy BuildStrategy     `json:"build_strategy,omitempty"`
	BuildConfig   *BuildConfig      `json:"build_config,omitempty"`
	Resources     *ResourceSpec     `json:"resources,omitempty"` // Direct CPU/memory specification
	Replicas      int               `json:"replicas"`
	Port          int               `json:"port,omitempty"`          // Container port the app listens on
	EnvVars       map[string]string `json:"env_vars,omitempty"`
	DependsOn     []string          `json:"depends_on,omitempty"`
}

// ResourceSpec represents direct resource allocation.
type ResourceSpec struct {
	CPU    string `json:"cpu"`    // e.g., "0.5", "1", "2"
	Memory string `json:"memory"` // e.g., "256Mi", "1Gi"
}

// DatabaseConfig represents a database configuration.
type DatabaseConfig struct {
	Type    string `json:"type"`
	Version string `json:"version"`
}

// Deployment represents a deployment from the API.
type Deployment struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	ServiceName string    `json:"service_name"`
	Version     int       `json:"version"`
	GitRef      string    `json:"git_ref"`
	GitCommit   string    `json:"git_commit,omitempty"`
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

// Domain represents a custom domain mapping.
type Domain struct {
	ID         string    `json:"id"`
	AppID      string    `json:"app_id"`
	Service    string    `json:"service"`
	Domain     string    `json:"domain"`
	IsWildcard bool      `json:"is_wildcard"`
	Verified   bool      `json:"verified"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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

// DashboardStatsResponse holds pre-calculated dashboard statistics from the backend.
type DashboardStatsResponse struct {
	ActiveDeployments int               `json:"active_deployments"`
	TotalApps         int               `json:"total_apps"`
	TotalServices     int               `json:"total_services"`
	NodeHealth        NodeHealthSummary `json:"node_health"`
}

// NodeHealthSummary holds node health statistics.
type NodeHealthSummary struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
}

// GetDashboardStats fetches pre-calculated dashboard statistics from the backend.
func (c *Client) GetDashboardStats(ctx context.Context) (*DashboardStatsResponse, error) {
	var stats DashboardStatsResponse
	err := c.Get(ctx, "/v1/dashboard/stats", &stats)
	return &stats, err
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
// Platform Configuration Types
// ============================================================================

// PlatformConfig holds platform-wide configuration from the backend.
// **Validates: Requirements 4.1, 4.2, 4.3, 4.4**
type PlatformConfig struct {
	Domain            string                   `json:"domain"`
	DefaultPorts      map[string]int           `json:"default_ports"`
	StatusMappings    map[string]StatusMapping `json:"status_mappings"`
	DefaultResources  ResourceSpec             `json:"default_resources"`
	SupportedDBTypes  []DatabaseTypeDef        `json:"supported_db_types"`
	MaxServicesPerApp int                      `json:"max_services_per_app"`
}

// StatusMapping defines how a status should be displayed in the UI.
// **Validates: Requirements 4.3**
type StatusMapping struct {
	Label string `json:"label"`
	Color string `json:"color"`
	Icon  string `json:"icon,omitempty"`
}

// DatabaseTypeDef defines a supported database type with its versions.
// **Validates: Requirements 4.4**
type DatabaseTypeDef struct {
	Type           string   `json:"type"`
	Versions       []string `json:"versions"`
	DefaultVersion string   `json:"default_version"`
}

// GetConfig fetches platform configuration from the backend.
func (c *Client) GetConfig(ctx context.Context) (*PlatformConfig, error) {
	var config PlatformConfig
	err := c.Get(ctx, "/v1/config", &config)
	return &config, err
}

// GetUserProfile retrieves the current user's profile.
func (c *Client) GetUserProfile(ctx context.Context) (*store.User, error) {
	var user store.User
	if err := c.Get(ctx, "/v1/user/profile", &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateUserProfile updates the current user's profile.
func (c *Client) UpdateUserProfile(ctx context.Context, name, avatarURL string) (*store.User, error) {
	req := map[string]string{
		"name":       name,
		"avatar_url": avatarURL,
	}
	var user store.User
	if err := c.patch(ctx, "/v1/user/profile", req, &user); err != nil {
		return nil, err
	}
	return &user, nil
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
	Name        string `json:"name"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
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
	Name          string            `json:"name"`
	SourceType    string            `json:"source_type"`
	GitRepo       string            `json:"git_repo,omitempty"`
	GitRef        string            `json:"git_ref,omitempty"`
	FlakeURI      string            `json:"flake_uri,omitempty"`
	BuildStrategy BuildStrategy     `json:"build_strategy,omitempty"`
	BuildConfig   *BuildConfig      `json:"build_config,omitempty"`
	Database      *DatabaseConfig   `json:"database,omitempty"`
	Resources     *ResourceSpec     `json:"resources,omitempty"` // Direct CPU/memory specification
	Replicas      int               `json:"replicas"`
	EnvVars       map[string]string `json:"env_vars,omitempty"`
	DependsOn     []string          `json:"depends_on,omitempty"`
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

// CanRegister checks if public registration is allowed (no owner exists yet).
func (c *Client) CanRegister(ctx context.Context) (bool, error) {
	var resp struct {
		CanRegister bool `json:"can_register"`
	}
	err := c.Get(ctx, "/auth/can-register", &resp)
	return resp.CanRegister, err
}

// ============================================================================
// Organization Methods
// ============================================================================

// Organization represents an organization from the API.
type Organization struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
}

// ListOrgs fetches all organizations for the current user.
func (c *Client) ListOrgs(ctx context.Context) ([]Organization, error) {
	var orgs []Organization
	err := c.Get(ctx, "/v1/orgs", &orgs)
	if orgs == nil {
		orgs = []Organization{}
	}
	return orgs, err
}

// GetOrg fetches an organization by ID.
func (c *Client) GetOrg(ctx context.Context, orgID string) (*Organization, error) {
	var org Organization
	err := c.Get(ctx, "/v1/orgs/"+orgID, &org)
	return &org, err
}

// GetOrgBySlug fetches an organization by slug.
func (c *Client) GetOrgBySlug(ctx context.Context, slug string) (*Organization, error) {
	var org Organization
	err := c.Get(ctx, "/v1/orgs/slug/"+slug, &org)
	return &org, err
}

// CreateOrgRequest is the request body for creating an organization.
type CreateOrgRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug,omitempty"`
	Description string `json:"description,omitempty"`
	IconURL     string `json:"icon_url,omitempty"`
}

// CreateOrg creates a new organization.
func (c *Client) CreateOrg(ctx context.Context, req CreateOrgRequest) (*Organization, error) {
	var org Organization
	err := c.post(ctx, "/v1/orgs", req, &org)
	return &org, err
}

// UpdateOrgRequest is the request body for updating an organization.
type UpdateOrgRequest struct {
	Name        string `json:"name,omitempty"`
	Slug        string `json:"slug,omitempty"`
	Description string `json:"description,omitempty"`
	IconURL     string `json:"icon_url,omitempty"`
}

// UpdateOrg updates an organization.
func (c *Client) UpdateOrg(ctx context.Context, orgID string, req UpdateOrgRequest) (*Organization, error) {
	var org Organization
	err := c.patch(ctx, "/v1/orgs/"+orgID, req, &org)
	return &org, err
}

// DeleteOrg deletes an organization.
func (c *Client) DeleteOrg(ctx context.Context, orgID string) error {
	return c.delete(ctx, "/v1/orgs/"+orgID)
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
func (c *Client) CreateApp(ctx context.Context, name, description, iconURL string) (*App, error) {
	req := CreateAppRequest{
		Name:        name,
		Description: description,
		IconURL:     iconURL,
	}
	var app App
	err := c.post(ctx, "/v1/apps", req, &app)
	return &app, err
}

// UpdateApp updates an existing application.
func (c *Client) UpdateApp(ctx context.Context, id string, req UpdateAppRequest) (*App, error) {
	var app App
	err := c.patch(ctx, "/v1/apps/"+id, req, &app)
	return &app, err
}

// DeleteApp removes an application.
func (c *Client) DeleteApp(ctx context.Context, id string) error {
	return c.delete(ctx, "/v1/apps/"+id)
}

// UpdateAppRequest is the request body for updating an app.
type UpdateAppRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	IconURL     *string `json:"icon_url,omitempty"`
	Version     int     `json:"version"` // Required for optimistic locking
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

// StopService stops a running service.
func (c *Client) StopService(ctx context.Context, appID, serviceName string) error {
	return c.post(ctx, "/v1/apps/"+appID+"/services/"+serviceName+"/stop", nil, nil)
}

// StartService starts a stopped service.
func (c *Client) StartService(ctx context.Context, appID, serviceName string) error {
	return c.post(ctx, "/v1/apps/"+appID+"/services/"+serviceName+"/start", nil, nil)
}

// ReloadService restarts a service without rebuilding.
func (c *Client) ReloadService(ctx context.Context, appID, serviceName string) error {
	return c.post(ctx, "/v1/apps/"+appID+"/services/"+serviceName+"/reload", nil, nil)
}

// RetryService retries a failed deployment.
func (c *Client) RetryService(ctx context.Context, appID, serviceName string) error {
	return c.post(ctx, "/v1/apps/"+appID+"/services/"+serviceName+"/retry", nil, nil)
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
// ListDeployments retrieves all deployments for the user.
func (c *Client) ListDeployments(ctx context.Context) ([]Deployment, error) {
	var deployments []Deployment
	err := c.Get(ctx, "/v1/deployments", &deployments)
	return deployments, err
}

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

// GetServiceLogs fetches logs for a specific service within an app.
func (c *Client) GetServiceLogs(ctx context.Context, appID, serviceName string) ([]Log, error) {
	var resp struct {
		Logs []Log `json:"logs"`
	}
	err := c.Get(ctx, "/v1/apps/"+appID+"/logs?service_name="+serviceName, &resp)
	if resp.Logs == nil {
		resp.Logs = []Log{}
	}
	return resp.Logs, err
}

// GetBuildByDeployment fetches a build by its deployment ID.
func (c *Client) GetBuildByDeployment(ctx context.Context, deploymentID string) (*Build, error) {
	var builds []Build
	// List builds and find the one matching the deployment ID
	if err := c.Get(ctx, "/v1/builds", &builds); err != nil {
		return nil, err
	}
	for _, b := range builds {
		if b.DeploymentID == deploymentID {
			return &b, nil
		}
	}
	return nil, nil
}

// ============================================================================
// Node Methods
// ============================================================================

// ListNodes retrieves all nodes.
func (c *Client) ListNodes(ctx context.Context) ([]Node, error) {
	var nodes []Node
	err := c.Get(ctx, "/v1/nodes", &nodes)
	return nodes, err
}

// GetNode retrieves a specific node by ID.
func (c *Client) GetNode(ctx context.Context, id string) (*Node, error) {
	var node Node
	err := c.Get(ctx, "/v1/nodes/"+id, &node)
	return &node, err
}

// ListBuilds retrieves all builds for the user from the API.
func (c *Client) ListBuilds(ctx context.Context) ([]Build, error) {
	var builds []Build
	if err := c.Get(ctx, "/v1/builds", &builds); err != nil {
		return nil, err
	}
	return builds, nil
}

// GetBuild retrieves a specific build by ID.
func (c *Client) GetBuild(ctx context.Context, id string) (*Build, error) {
	var build Build
	if err := c.Get(ctx, "/v1/builds/"+id, &build); err != nil {
		return nil, err
	}
	return &build, nil
}

// RetryBuild retries a failed build.
func (c *Client) RetryBuild(ctx context.Context, id string) error {
	return c.post(ctx, "/v1/builds/"+id+"/retry", nil, nil)
}

// ============================================================================
// Domain Methods
// ============================================================================

// ListAllDomains retrieves all domains across all applications.
func (c *Client) ListAllDomains(ctx context.Context) ([]Domain, error) {
	var domains []Domain
	err := c.Get(ctx, "/v1/domains", &domains)
	if domains == nil {
		domains = []Domain{}
	}
	return domains, err
}

// CreateDomain creates a new domain mapping.
func (c *Client) CreateDomain(ctx context.Context, appID, service, domain string, isWildcard bool) (*Domain, error) {
	req := map[string]interface{}{
		"app_id":      appID,
		"service":     service,
		"domain":      domain,
		"is_wildcard": isWildcard,
	}
	var d Domain
	err := c.post(ctx, "/v1/domains", req, &d)
	return &d, err
}

// DeleteDomain removes a domain mapping.
func (c *Client) DeleteDomain(ctx context.Context, id string) error {
	return c.delete(ctx, "/v1/domains/"+id)
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
// User Management Methods
// ============================================================================

// UserInfo represents a user in the system.
type UserInfo struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Role      string `json:"role"`
	InvitedBy string `json:"invited_by,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// ListUsers fetches all users (admin only).
func (c *Client) ListUsers(ctx context.Context) ([]UserInfo, error) {
	var users []UserInfo
	err := c.Get(ctx, "/v1/users", &users)
	if users == nil {
		users = []UserInfo{}
	}
	return users, err
}

// DeleteUser removes a user (admin only).
func (c *Client) DeleteUser(ctx context.Context, userID string) error {
	return c.delete(ctx, "/v1/users/"+userID)
}

// ============================================================================
// Invitation Methods
// ============================================================================

// Invitation represents an invitation to join the platform.
type Invitation struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	Token      string `json:"token,omitempty"`
	InvitedBy  string `json:"invited_by"`
	Role       string `json:"role"`
	Status     string `json:"status"`
	ExpiresAt  string `json:"expires_at"`
	AcceptedAt string `json:"accepted_at,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// CreateInvitation creates a new invitation (admin only).
func (c *Client) CreateInvitation(ctx context.Context, email, role string) (*Invitation, error) {
	req := map[string]string{
		"email": email,
		"role":  role,
	}
	var invitation Invitation
	err := c.post(ctx, "/v1/invitations", req, &invitation)
	return &invitation, err
}

// ListInvitations fetches all invitations (admin only).
func (c *Client) ListInvitations(ctx context.Context) ([]Invitation, error) {
	var invitations []Invitation
	err := c.Get(ctx, "/v1/invitations", &invitations)
	if invitations == nil {
		invitations = []Invitation{}
	}
	return invitations, err
}

// RevokeInvitation revokes an invitation (admin only).
func (c *Client) RevokeInvitation(ctx context.Context, invitationID string) error {
	return c.delete(ctx, "/v1/invitations/"+invitationID)
}

// GetInvitationByToken fetches invitation details by token (public).
func (c *Client) GetInvitationByToken(ctx context.Context, token string) (*Invitation, error) {
	var invitation Invitation
	err := c.Get(ctx, "/auth/invite/"+token, &invitation)
	return &invitation, err
}

// AcceptInvitation accepts an invitation and creates a user (public).
func (c *Client) AcceptInvitation(ctx context.Context, token, password string) (*AuthResponse, error) {
	req := map[string]string{
		"token":    token,
		"password": password,
	}
	var resp AuthResponse
	err := c.post(ctx, "/auth/invite/accept", req, &resp)
	return &resp, err
}

// ============================================================================
// Dashboard Methods
// ============================================================================

// GetDashboardData fetches dashboard data from the backend statistics endpoint.
// This uses the backend as the source of truth for statistics instead of calculating client-side.
// **Validates: Requirements 3.1, 3.2, 3.3**
func (c *Client) GetDashboardData(ctx context.Context) (*DashboardStats, []RecentDeployment, []NodeHealth, error) {
	stats := &DashboardStats{}
	var recentDeployments []RecentDeployment
	var nodeHealth []NodeHealth
	var mu sync.Mutex
	var wg sync.WaitGroup
	var statsErr error

	// Fetch backend statistics and node details in parallel
	wg.Add(2)

	// 1. Fetch pre-calculated statistics from backend (source of truth)
	// **Validates: Requirements 3.1, 3.2**
	go func() {
		defer wg.Done()
		backendStats, err := c.GetDashboardStats(ctx)
		if err != nil {
			mu.Lock()
			statsErr = err
			mu.Unlock()
			return
		}
		mu.Lock()
		stats.TotalApps = backendStats.TotalApps
		stats.ActiveDeployments = backendStats.ActiveDeployments
		stats.HealthyNodes = backendStats.NodeHealth.Healthy
		mu.Unlock()
	}()

	// 2. Fetch node details for the node health display (need full node info for CPU/memory)
	go func() {
		defer wg.Done()
		nodes, err := c.ListNodes(ctx)
		if err != nil {
			return
		}
		mu.Lock()
		for _, node := range nodes {
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
		mu.Unlock()
	}()

	wg.Wait()

	// Return error if stats fetch failed
	if statsErr != nil {
		return nil, nil, nil, statsErr
	}

	// 3. Fetch recent deployments for display
	// We still need to fetch deployments for the "recent deployments" list
	apps, err := c.ListApps(ctx)
	if err == nil && len(apps) > 0 {
		wg.Add(len(apps))
		for _, app := range apps {
			go func(a App) {
				defer wg.Done()
				deployments, err := c.ListAppDeployments(ctx, a.ID)
				if err == nil {
					mu.Lock()
					defer mu.Unlock()
					for _, d := range deployments {
						// Add to recent (limit to first 10, then sorted/sliced later if needed)
						if len(recentDeployments) < 10 {
							recentDeployments = append(recentDeployments, RecentDeployment{
								AppName:     a.Name,
								ServiceName: d.ServiceName,
								Status:      d.Status,
								TimeAgo:     formatTimeAgo(d.CreatedAt),
							})
						}
					}
				}
			}(app)
		}
		wg.Wait()
	}

	// Limit to final 5 recent deployments if needed
	if len(recentDeployments) > 5 {
		recentDeployments = recentDeployments[:5]
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
	if c.orgID != "" {
		req.Header.Set("X-Org-ID", c.orgID)
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
	if c.orgID != "" {
		req.Header.Set("X-Org-ID", c.orgID)
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
