package models

import "time"

// DeploymentStatus represents the current state of a deployment.
type DeploymentStatus string

const (
	DeploymentStatusPending   DeploymentStatus = "pending"
	DeploymentStatusBuilding  DeploymentStatus = "building"
	DeploymentStatusBuilt     DeploymentStatus = "built"
	DeploymentStatusScheduled DeploymentStatus = "scheduled"
	DeploymentStatusStarting  DeploymentStatus = "starting"
	DeploymentStatusRunning   DeploymentStatus = "running"
	DeploymentStatusStopping  DeploymentStatus = "stopping"
	DeploymentStatusStopped   DeploymentStatus = "stopped"
	DeploymentStatusFailed    DeploymentStatus = "failed"
)

// RuntimeConfig holds runtime configuration for a deployment.
type RuntimeConfig struct {
	ResourceTier ResourceTier      `json:"resource_tier"`
	EnvVars      map[string]string `json:"env_vars,omitempty"`
	Ports        []PortMapping     `json:"ports,omitempty"`
	HealthCheck  *HealthCheckConfig `json:"health_check,omitempty"`
}

// Deployment represents an instance of an application version running on one or more nodes.
type Deployment struct {
	ID           string           `json:"id"`
	AppID        string           `json:"app_id"`
	ServiceName  string           `json:"service_name"`
	Version      int              `json:"version"`
	GitRef       string           `json:"git_ref"`
	GitCommit    string           `json:"git_commit,omitempty"`
	BuildType    BuildType        `json:"build_type"`
	Artifact     string           `json:"artifact,omitempty"`
	Status       DeploymentStatus `json:"status"`
	NodeID       string           `json:"node_id,omitempty"`
	ResourceTier ResourceTier     `json:"resource_tier"`
	Config       *RuntimeConfig   `json:"config,omitempty"`
	DependsOn    []string         `json:"depends_on,omitempty"` // Service names this deployment depends on
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	StartedAt    *time.Time       `json:"started_at,omitempty"`
	FinishedAt   *time.Time       `json:"finished_at,omitempty"`
}
