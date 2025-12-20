package models

import "time"

// BuildType represents the deployment mode for an application.
type BuildType string

const (
	BuildTypeOCI     BuildType = "oci"
	BuildTypePureNix BuildType = "pure-nix"
)

// ResourceTier represents predefined resource allocation levels.
type ResourceTier string

const (
	ResourceTierNano   ResourceTier = "nano"   // 256MB RAM, 0.25 CPU
	ResourceTierSmall  ResourceTier = "small"  // 512MB RAM, 0.5 CPU
	ResourceTierMedium ResourceTier = "medium" // 1GB RAM, 1 CPU
	ResourceTierLarge  ResourceTier = "large"  // 2GB RAM, 2 CPU
	ResourceTierXLarge ResourceTier = "xlarge" // 4GB RAM, 4 CPU
)

// PortMapping defines a port mapping for a service.
type PortMapping struct {
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol,omitempty"` // tcp or udp, defaults to tcp
}

// HealthCheckConfig defines health check settings for a service.
type HealthCheckConfig struct {
	Path            string `json:"path,omitempty"`
	Port            int    `json:"port,omitempty"`
	IntervalSeconds int    `json:"interval_seconds,omitempty"`
	TimeoutSeconds  int    `json:"timeout_seconds,omitempty"`
	Retries         int    `json:"retries,omitempty"`
}

// ServiceConfig defines a single runnable component within an application.
type ServiceConfig struct {
	Name         string             `json:"name"`
	FlakeOutput  string             `json:"flake_output"`
	ResourceTier ResourceTier       `json:"resource_tier"`
	Replicas     int                `json:"replicas"`
	Ports        []PortMapping      `json:"ports,omitempty"`
	HealthCheck  *HealthCheckConfig `json:"health_check,omitempty"`
	DependsOn    []string           `json:"depends_on,omitempty"`
}


// App represents a user-defined deployable unit that may contain one or more services.
type App struct {
	ID          string          `json:"id"`
	OwnerID     string          `json:"owner_id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	BuildType   BuildType       `json:"build_type"`
	Services    []ServiceConfig `json:"services"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	DeletedAt   *time.Time      `json:"deleted_at,omitempty"`
}
