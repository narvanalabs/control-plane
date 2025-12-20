package models

import "time"

// LogEntry represents a single log entry from a build or runtime.
type LogEntry struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deployment_id"`
	Source       string    `json:"source"` // "build" or "runtime"
	Level        string    `json:"level"`
	Message      string    `json:"message"`
	Timestamp    time.Time `json:"timestamp"`
}
