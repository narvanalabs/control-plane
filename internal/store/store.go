// Package store provides database access interfaces and implementations.
package store

import (
	"context"

	"github.com/narvanalabs/control-plane/internal/models"
)

// Store is the main interface for database operations.
type Store interface {
	// Apps returns the AppStore for application operations.
	Apps() AppStore
	// Deployments returns the DeploymentStore for deployment operations.
	Deployments() DeploymentStore
	// Nodes returns the NodeStore for node operations.
	Nodes() NodeStore
	// Builds returns the BuildStore for build job operations.
	Builds() BuildStore
	// Secrets returns the SecretStore for secret operations.
	Secrets() SecretStore
	// Logs returns the LogStore for log operations.
	Logs() LogStore

	// WithTx executes the given function within a database transaction.
	// If the function returns an error, the transaction is rolled back.
	// Otherwise, the transaction is committed.
	WithTx(ctx context.Context, fn func(Store) error) error

	// Close closes the database connection.
	Close() error
}

// AppStore defines operations for application management.
type AppStore interface {
	// Create creates a new application.
	Create(ctx context.Context, app *models.App) error
	// Get retrieves an application by ID.
	Get(ctx context.Context, id string) (*models.App, error)
	// GetByName retrieves an application by owner ID and name.
	GetByName(ctx context.Context, ownerID, name string) (*models.App, error)
	// List retrieves all applications for a given owner.
	List(ctx context.Context, ownerID string) ([]*models.App, error)
	// Update updates an existing application.
	Update(ctx context.Context, app *models.App) error
	// Delete soft-deletes an application by setting deleted_at.
	Delete(ctx context.Context, id string) error
}


// DeploymentStore defines operations for deployment management.
type DeploymentStore interface {
	// Create creates a new deployment.
	Create(ctx context.Context, deployment *models.Deployment) error
	// Get retrieves a deployment by ID.
	Get(ctx context.Context, id string) (*models.Deployment, error)
	// List retrieves all deployments for a given application, ordered by created_at DESC.
	List(ctx context.Context, appID string) ([]*models.Deployment, error)
	// ListByNode retrieves all deployments assigned to a given node.
	ListByNode(ctx context.Context, nodeID string) ([]*models.Deployment, error)
	// Update updates an existing deployment.
	Update(ctx context.Context, deployment *models.Deployment) error
	// GetLatestSuccessful retrieves the most recent successful deployment for an app.
	GetLatestSuccessful(ctx context.Context, appID string) (*models.Deployment, error)
}

// NodeStore defines operations for node management.
type NodeStore interface {
	// Register registers a new node or updates an existing one.
	Register(ctx context.Context, node *models.Node) error
	// Get retrieves a node by ID.
	Get(ctx context.Context, id string) (*models.Node, error)
	// List retrieves all registered nodes.
	List(ctx context.Context) ([]*models.Node, error)
	// UpdateHeartbeat updates a node's last heartbeat timestamp and resource metrics.
	UpdateHeartbeat(ctx context.Context, id string, resources *models.NodeResources) error
	// UpdateHealth updates a node's health status.
	UpdateHealth(ctx context.Context, id string, healthy bool) error
	// ListHealthy retrieves all healthy nodes.
	ListHealthy(ctx context.Context) ([]*models.Node, error)
	// ListWithClosure retrieves nodes that have a specific store path cached.
	ListWithClosure(ctx context.Context, storePath string) ([]*models.Node, error)
}

// BuildStore defines operations for build job management.
type BuildStore interface {
	// Create creates a new build job.
	Create(ctx context.Context, build *models.BuildJob) error
	// Get retrieves a build job by ID.
	Get(ctx context.Context, id string) (*models.BuildJob, error)
	// GetByDeployment retrieves a build job by deployment ID.
	GetByDeployment(ctx context.Context, deploymentID string) (*models.BuildJob, error)
	// Update updates an existing build job.
	Update(ctx context.Context, build *models.BuildJob) error
	// ListPending retrieves all pending build jobs.
	ListPending(ctx context.Context) ([]*models.BuildJob, error)
}

// SecretStore defines operations for secret management.
type SecretStore interface {
	// Set creates or updates a secret for an application.
	Set(ctx context.Context, appID, key string, encryptedValue []byte) error
	// Get retrieves a secret by app ID and key.
	Get(ctx context.Context, appID, key string) ([]byte, error)
	// List retrieves all secret keys for an application.
	List(ctx context.Context, appID string) ([]string, error)
	// Delete removes a secret.
	Delete(ctx context.Context, appID, key string) error
	// GetAll retrieves all secrets for an application as a map.
	GetAll(ctx context.Context, appID string) (map[string][]byte, error)
}

// LogStore defines operations for log management.
type LogStore interface {
	// Create creates a new log entry.
	Create(ctx context.Context, entry *models.LogEntry) error
	// List retrieves log entries for a deployment.
	List(ctx context.Context, deploymentID string, limit int) ([]*models.LogEntry, error)
	// ListBySource retrieves log entries filtered by source (build/runtime).
	ListBySource(ctx context.Context, deploymentID, source string, limit int) ([]*models.LogEntry, error)
	// DeleteOlderThan removes log entries older than the specified time.
	DeleteOlderThan(ctx context.Context, deploymentID string, before int64) error
}
