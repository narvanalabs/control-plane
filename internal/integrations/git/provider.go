// Package git provides a unified interface for interacting with multiple Git providers.
package git

import (
	"context"
	"time"
)

// ProviderType represents the type of Git provider.
type ProviderType string

const (
	ProviderGitHub    ProviderType = "github"
	ProviderGitLab    ProviderType = "gitlab"
	ProviderBitbucket ProviderType = "bitbucket"
	ProviderGitea     ProviderType = "gitea"
)

// ProviderConfig holds the configuration for a Git provider.
type ProviderConfig struct {
	// InstanceURL is the base URL for self-hosted instances (GitLab, Gitea).
	// For cloud providers (GitHub, Bitbucket), this can be empty.
	InstanceURL string `json:"instance_url,omitempty"`

	// ClientID is the OAuth application client ID.
	ClientID string `json:"client_id"`

	// ClientSecret is the OAuth application client secret.
	ClientSecret string `json:"client_secret"`

	// WebhookSecret is used to verify webhook payloads.
	WebhookSecret string `json:"webhook_secret,omitempty"`

	// PrivateKey is the private key for GitHub App authentication (GitHub only).
	PrivateKey string `json:"private_key,omitempty"`

	// AppID is the GitHub App ID (GitHub only).
	AppID int64 `json:"app_id,omitempty"`
}

// Repository represents a Git repository from any provider.
type Repository struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Description   string `json:"description"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
	SSHURL        string `json:"ssh_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Archived      bool   `json:"archived"`
}

// User represents a Git provider user.
type User struct {
	ID        string `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// Webhook represents a webhook configuration.
type Webhook struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// OAuthToken represents OAuth token information.
type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

// Provider defines the interface for Git provider operations.
// All Git providers (GitHub, GitLab, Bitbucket, Gitea) must implement this interface.
type Provider interface {
	// Name returns the provider type identifier.
	Name() ProviderType

	// Configure sets up the provider with the given configuration.
	Configure(ctx context.Context, config *ProviderConfig) error

	// GetAuthorizationURL returns the OAuth authorization URL for user authentication.
	GetAuthorizationURL(state, redirectURI string) string

	// ExchangeCode exchanges an OAuth authorization code for access tokens.
	ExchangeCode(ctx context.Context, code, redirectURI string) (*OAuthToken, error)

	// GetUser retrieves the authenticated user's information.
	GetUser(ctx context.Context, token string) (*User, error)

	// ListRepositories returns repositories the authenticated user has access to.
	ListRepositories(ctx context.Context, token string) ([]*Repository, error)

	// GetRepository retrieves a specific repository by owner and name.
	GetRepository(ctx context.Context, token, owner, repo string) (*Repository, error)

	// CreateWebhook creates a webhook for a repository.
	CreateWebhook(ctx context.Context, token, owner, repo, webhookURL, secret string, events []string) (*Webhook, error)

	// DeleteWebhook removes a webhook from a repository.
	DeleteWebhook(ctx context.Context, token, owner, repo, webhookID string) error

	// GetCloneURL returns the authenticated clone URL for a repository.
	GetCloneURL(ctx context.Context, token, owner, repo string) (string, error)
}

// ProviderRegistry holds registered Git providers.
var ProviderRegistry = map[ProviderType]func() Provider{
	ProviderGitHub:    NewGitHubProvider,
	ProviderGitLab:    NewGitLabProvider,
	ProviderBitbucket: NewBitbucketProvider,
	ProviderGitea:     NewGiteaProvider,
}

// GetProvider returns a new instance of the specified provider.
func GetProvider(providerType ProviderType) (Provider, bool) {
	factory, ok := ProviderRegistry[providerType]
	if !ok {
		return nil, false
	}
	return factory(), true
}
