package models

import "time"

// GitHubAppConfig stores the credentials for a GitHub App or OAuth App.
type GitHubAppConfig struct {
	ID            string    `json:"id" db:"id"`
	ConfigType    string    `json:"config_type" db:"config_type"` // "app" or "oauth"
	AppID         *int64    `json:"app_id" db:"app_id"`           // Nullable
	ClientID      string    `json:"client_id" db:"client_id"`
	ClientSecret  string    `json:"client_secret" db:"client_secret"`
	WebhookSecret *string   `json:"webhook_secret,omitempty" db:"webhook_secret"` // Nullable
	PrivateKey    *string   `json:"-" db:"private_key"`                           // Nullable
	Slug          *string   `json:"slug" db:"slug"`                               // Nullable
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// GitHubInstallation represents an installation of the GitHub App.
type GitHubInstallation struct {
	ID              int64     `json:"id" db:"id"`
	AccountID       int64     `json:"account_id" db:"account_id"`
	AccountLogin    string    `json:"account_login" db:"account_login"`
	AccountType     string    `json:"account_type" db:"account_type"` // "User" or "Organization"
	AccessTokensURL string    `json:"access_tokens_url" db:"access_tokens_url"`
	RepositoriesURL string    `json:"repositories_url" db:"repositories_url"`
	HTMLURL         string    `json:"html_url" db:"html_url"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
	UserID          string    `json:"user_id" db:"user_id"`
}

// GitHubRepository represents a repository returned by the GitHub API.
type GitHubRepository struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	Private       bool   `json:"private"`
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
}

// GitHubAccount represents a user's GitHub account connected via OAuth.
type GitHubAccount struct {
	ID           int64     `json:"id" db:"id"`
	Login        string    `json:"login" db:"login"`
	Name         string    `json:"name" db:"name"`
	Email        string    `json:"email" db:"email"`
	AvatarURL    string    `json:"avatar_url" db:"avatar_url"`
	AccessToken  string    `json:"-" db:"access_token"`
	RefreshToken string    `json:"-" db:"refresh_token"`
	Expiry       time.Time `json:"-" db:"expiry"`
	TokenType    string    `json:"-" db:"token_type"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
	UserID       string    `json:"user_id" db:"user_id"`
}
