// Package store provides database access interfaces and implementations.
package store

import (
	"context"

	"github.com/narvanalabs/control-plane/internal/models"
)

// OrgStore defines operations for organization management.
type OrgStore interface {
	// Create creates a new organization.
	Create(ctx context.Context, org *models.Organization) error
	// Get retrieves an organization by ID.
	Get(ctx context.Context, id string) (*models.Organization, error)
	// GetBySlug retrieves an organization by slug.
	GetBySlug(ctx context.Context, slug string) (*models.Organization, error)
	// List retrieves all organizations for a user.
	List(ctx context.Context, userID string) ([]*models.Organization, error)
	// Update updates an organization.
	Update(ctx context.Context, org *models.Organization) error
	// Delete deletes an organization (only if not the last one).
	Delete(ctx context.Context, id string) error
	// AddMember adds a user to an organization with a role.
	AddMember(ctx context.Context, orgID, userID string, role models.Role) error
	// RemoveMember removes a user from an organization.
	RemoveMember(ctx context.Context, orgID, userID string) error
	// GetDefault returns the default organization (first created).
	GetDefault(ctx context.Context) (*models.Organization, error)
	// Count returns the total number of organizations.
	Count(ctx context.Context) (int, error)
	// ListMembers retrieves all members of an organization.
	ListMembers(ctx context.Context, orgID string) ([]*models.OrgMembership, error)
}

// Store is the main interface for database operations.
type Store interface {
	// Orgs returns the OrgStore for organization operations.
	Orgs() OrgStore
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
	// Users returns the UserStore for user operations.
	Users() UserStore
	// GitHub returns the GitHubStore for GitHub App operations.
	GitHub() GitHubStore
	// GitHubAccounts returns the GitHubAccountStore for GitHub OAuth operations.
	GitHubAccounts() GitHubAccountStore
	// Settings returns the SettingsStore for global configuration.
	Settings() SettingsStore
	// Domains returns the DomainStore for custom domain operations.
	Domains() DomainStore
	// Invitations returns the InvitationStore for invitation operations.
	Invitations() InvitationStore

	// WithTx executes the given function within a database transaction.
	// If the function returns an error, the transaction is rolled back.
	// Otherwise, the transaction is committed.
	WithTx(ctx context.Context, fn func(Store) error) error

	// Close closes the database connection.
	Close() error
}

// Role represents a user's role in the system.
type Role string

const (
	// RoleOwner has full access and can invite users.
	RoleOwner Role = "owner"
	// RoleMember has standard access without admin functions.
	RoleMember Role = "member"
)

// User represents a user in the system.
type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Role      Role   `json:"role"`
	InvitedBy string `json:"invited_by,omitempty"`
	IsAdmin   bool   `json:"is_admin"` // Deprecated: use Role instead
	CreatedAt int64  `json:"created_at"`
}

// UserStore defines operations for user management.
type UserStore interface {
	// Create creates a new user with hashed password.
	Create(ctx context.Context, email, password string, isAdmin bool) (*User, error)
	// CreateWithRole creates a new user with hashed password and specified role.
	CreateWithRole(ctx context.Context, email, password string, role Role, invitedBy string) (*User, error)
	// GetByEmail retrieves a user by email.
	GetByEmail(ctx context.Context, email string) (*User, error)
	// GetByID retrieves a user by ID.
	GetByID(ctx context.Context, id string) (*User, error)
	// Authenticate verifies credentials and returns the user.
	Authenticate(ctx context.Context, email, password string) (*User, error)
	// Update updates an existing user's information.
	Update(ctx context.Context, user *User) error
	// List retrieves all users.
	List(ctx context.Context) ([]*User, error)
	// Delete removes a user by ID.
	Delete(ctx context.Context, id string) error
	// CountByRole returns the number of users with a specific role.
	CountByRole(ctx context.Context, role Role) (int, error)
	// GetFirstOwner returns the first user with owner role, if any.
	GetFirstOwner(ctx context.Context) (*User, error)
}

// GitHubStore defines operations for GitHub App management.
type GitHubStore interface {
	// GetConfig retrieves the GitHub App configuration.
	GetConfig(ctx context.Context) (*models.GitHubAppConfig, error)
	// SaveConfig saves the GitHub App configuration.
	SaveConfig(ctx context.Context, config *models.GitHubAppConfig) error
	// ResetConfig clears the GitHub App configuration.
	ResetConfig(ctx context.Context) error

	// CreateInstallation saves a new GitHub App installation.
	CreateInstallation(ctx context.Context, inst *models.GitHubInstallation) error
	// GetInstallation retrieves an installation by its ID.
	GetInstallation(ctx context.Context, id int64) (*models.GitHubInstallation, error)
	// ListInstallations retrieves all installations for a given user.
	ListInstallations(ctx context.Context, userID string) ([]*models.GitHubInstallation, error)
	// DeleteInstallation removes an installation.
	DeleteInstallation(ctx context.Context, id int64) error
}

// GitHubAccountStore defines operations for GitHub OAuth account management.
type GitHubAccountStore interface {
	// Create saves a new GitHub account.
	Create(ctx context.Context, account *models.GitHubAccount) error
	// Get retrieves a GitHub account by its ID.
	Get(ctx context.Context, id int64) (*models.GitHubAccount, error)
	// GetByUserID retrieves a GitHub account by its associated user ID.
	GetByUserID(ctx context.Context, userID string) (*models.GitHubAccount, error)
	// Update updates an existing GitHub account.
	Update(ctx context.Context, account *models.GitHubAccount) error
	// Delete removes a GitHub account.
	Delete(ctx context.Context, id int64) error
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
	// ListByStatus retrieves all deployments with a given status.
	ListByStatus(ctx context.Context, status models.DeploymentStatus) ([]*models.Deployment, error)
	// Update updates an existing deployment.
	Update(ctx context.Context, deployment *models.Deployment) error
	// GetLatestSuccessful retrieves the most recent successful deployment for an app.
	// ListByUser retrieves all deployments for all apps owned by a given user.
	ListByUser(ctx context.Context, userID string) ([]*models.Deployment, error)
	// GetNextVersion returns the next version number for a service.
	// Returns 1 for the first deployment, or max(version) + 1 for subsequent deployments.
	// **Validates: Requirements 9.1, 9.2**
	GetNextVersion(ctx context.Context, appID, serviceName string) (int, error)
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
	// List retrieves all builds for a given application.
	List(ctx context.Context, appID string) ([]*models.BuildJob, error)
	// ListByUser retrieves all builds for a given user across all apps.
	ListByUser(ctx context.Context, userID string) ([]*models.BuildJob, error)
	// ListPending retrieves all pending build jobs.
	ListPending(ctx context.Context) ([]*models.BuildJob, error)
	// ListRunning retrieves all builds with status 'running'.
	// Used for startup recovery to identify interrupted builds.
	// **Validates: Requirements 15.1, 15.2**
	ListRunning(ctx context.Context) ([]*models.BuildJob, error)
	// ListQueued retrieves all builds with status 'queued'.
	// Used for startup recovery to resume pending builds.
	// **Validates: Requirements 15.1**
	ListQueued(ctx context.Context) ([]*models.BuildJob, error)
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

// SettingsStore defines operations for global system settings.
type SettingsStore interface {
	// Get retrieves a setting by key.
	Get(ctx context.Context, key string) (string, error)
	// Set sets a setting key-value pair.
	Set(ctx context.Context, key, value string) error
	// GetAll retrieves all global settings.
	GetAll(ctx context.Context) (map[string]string, error)
}

// DomainStore defines operations for custom domain management.
type DomainStore interface {
	// Create creates a new domain mapping.
	Create(ctx context.Context, domain *models.Domain) error
	// Get retrieves a domain by ID.
	Get(ctx context.Context, id string) (*models.Domain, error)
	// List retrieves all domains for a given application.
	List(ctx context.Context, appID string) ([]*models.Domain, error)
	// Delete removes a domain mapping.
	Delete(ctx context.Context, id string) error
	// GetByDomain retrieves a domain mapping by the domain name itself.
	GetByDomain(ctx context.Context, domain string) (*models.Domain, error)
}

// InvitationStore defines operations for invitation management.
type InvitationStore interface {
	// Create creates a new invitation.
	Create(ctx context.Context, invitation *models.Invitation) error
	// Get retrieves an invitation by ID.
	Get(ctx context.Context, id string) (*models.Invitation, error)
	// GetByToken retrieves an invitation by its token.
	GetByToken(ctx context.Context, token string) (*models.Invitation, error)
	// GetByEmail retrieves a pending invitation by email.
	GetByEmail(ctx context.Context, email string) (*models.Invitation, error)
	// List retrieves all invitations.
	List(ctx context.Context) ([]*models.Invitation, error)
	// Update updates an invitation.
	Update(ctx context.Context, invitation *models.Invitation) error
	// Delete removes an invitation.
	Delete(ctx context.Context, id string) error
}
