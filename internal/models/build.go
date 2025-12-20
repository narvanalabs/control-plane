package models

import "time"

// BuildStatus represents the current state of a build job.
type BuildStatus string

const (
	BuildStatusQueued    BuildStatus = "queued"
	BuildStatusRunning   BuildStatus = "running"
	BuildStatusSucceeded BuildStatus = "succeeded"
	BuildStatusFailed    BuildStatus = "failed"
)

// BuildJob represents a build task in the queue.
type BuildJob struct {
	ID           string      `json:"id"`
	DeploymentID string      `json:"deployment_id"`
	AppID        string      `json:"app_id"`
	GitURL       string      `json:"git_url"`
	GitRef       string      `json:"git_ref"`
	FlakeOutput  string      `json:"flake_output"`
	BuildType    BuildType   `json:"build_type"`
	Status       BuildStatus `json:"status"`
	CreatedAt    time.Time   `json:"created_at"`
	StartedAt    *time.Time  `json:"started_at,omitempty"`
	FinishedAt   *time.Time  `json:"finished_at,omitempty"`
}

// BuildResult represents the output of a completed build.
type BuildResult struct {
	Artifact  string `json:"artifact"`
	StorePath string `json:"store_path,omitempty"` // For pure-nix
	ImageTag  string `json:"image_tag,omitempty"`  // For OCI
	Logs      string `json:"logs"`
}
