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

// Node represents a compute instance running the Narvana node agent, Podman, and Caddy.
type Node struct {
	ID            string         `json:"id"`
	Hostname      string         `json:"hostname"`
	Address       string         `json:"address"`
	GRPCPort      int            `json:"grpc_port"`
	Healthy       bool           `json:"healthy"`
	Resources     *NodeResources `json:"resources"`
	CachedPaths   []string       `json:"cached_paths,omitempty"`
	LastHeartbeat time.Time      `json:"last_heartbeat"`
	RegisteredAt  time.Time      `json:"registered_at"`
}
