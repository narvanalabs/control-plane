package models

import "time"

// NodeResources represents the resource availability of a node.
type NodeResources struct {
	CPUTotal        float64 `json:"cpu_total"`
	CPUAvailable    float64 `json:"cpu_available"`
	MemoryTotal     int64   `json:"memory_total"`
	MemoryAvailable int64   `json:"memory_available"`
	DiskTotal       int64   `json:"disk_total"`
	DiskAvailable   int64   `json:"disk_available"`
}

// DiskStats represents disk usage statistics for a specific path.
// **Validates: Requirements 20.1**
type DiskStats struct {
	Path         string  `json:"path"`
	Total        int64   `json:"total"`
	Used         int64   `json:"used"`
	Available    int64   `json:"available"`
	UsagePercent float64 `json:"usage_percent"`
}

// NodeDiskMetrics contains disk usage metrics for specific paths on a node.
// **Validates: Requirements 20.1**
type NodeDiskMetrics struct {
	NixStore         *DiskStats `json:"nix_store,omitempty"`
	ContainerStorage *DiskStats `json:"container_storage,omitempty"`
}

// Node represents a compute instance running the Narvana node agent, Podman, and Caddy.
type Node struct {
	ID            string           `json:"id"`
	Hostname      string           `json:"hostname"`
	Address       string           `json:"address"`
	GRPCPort      int              `json:"grpc_port"`
	Healthy       bool             `json:"healthy"`
	Resources     *NodeResources   `json:"resources"`
	DiskMetrics   *NodeDiskMetrics `json:"disk_metrics,omitempty"`
	CachedPaths   []string         `json:"cached_paths,omitempty"`
	LastHeartbeat time.Time        `json:"last_heartbeat"`
	RegisteredAt  time.Time        `json:"registered_at"`
}
