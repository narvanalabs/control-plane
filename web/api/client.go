// Package api provides a client for communicating with the control-plane API.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

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

	// TODO: Fetch deployments and builds when we have those endpoints working

	return stats, recentDeployments, nodeHealth, nil
}

// ListApps fetches all apps for the current user.
func (c *Client) ListApps(ctx context.Context) ([]App, error) {
	var apps []App
	err := c.get(ctx, "/v1/apps", &apps)
	return apps, err
}

// GetApp fetches a single app by ID.
func (c *Client) GetApp(ctx context.Context, id string) (*App, error) {
	var app App
	err := c.get(ctx, "/v1/apps/"+id, &app)
	return &app, err
}

// ListNodes fetches all nodes.
func (c *Client) ListNodes(ctx context.Context) ([]Node, error) {
	var nodes []Node
	err := c.get(ctx, "/v1/nodes", &nodes)
	return nodes, err
}

// get performs a GET request and unmarshals the response.
func (c *Client) get(ctx context.Context, path string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}
