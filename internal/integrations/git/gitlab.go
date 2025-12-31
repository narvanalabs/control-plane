package git

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	gitlabDefaultURL = "https://gitlab.com"
)

// GitLabProvider implements the Provider interface for GitLab.
type GitLabProvider struct {
	config  *ProviderConfig
	client  *http.Client
	baseURL string
}

// NewGitLabProvider creates a new GitLab provider instance.
func NewGitLabProvider() Provider {
	return &GitLabProvider{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: gitlabDefaultURL,
	}
}

// Name returns the provider type identifier.
func (p *GitLabProvider) Name() ProviderType {
	return ProviderGitLab
}

// Configure sets up the provider with the given configuration.
func (p *GitLabProvider) Configure(ctx context.Context, config *ProviderConfig) error {
	if config.ClientID == "" {
		return fmt.Errorf("client_id (application_id) is required")
	}
	if config.ClientSecret == "" {
		return fmt.Errorf("client_secret is required")
	}

	p.config = config

	// Use custom instance URL if provided, otherwise use gitlab.com
	if config.InstanceURL != "" {
		p.baseURL = strings.TrimSuffix(config.InstanceURL, "/")
	} else {
		p.baseURL = gitlabDefaultURL
	}

	return nil
}

// GetAuthorizationURL returns the OAuth authorization URL for user authentication.
func (p *GitLabProvider) GetAuthorizationURL(state, redirectURI string) string {
	params := url.Values{}
	params.Set("client_id", p.config.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "api read_user read_repository")
	params.Set("state", state)
	return p.baseURL + "/oauth/authorize?" + params.Encode()
}

// ExchangeCode exchanges an OAuth authorization code for access tokens.
func (p *GitLabProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (*OAuthToken, error) {
	tokenURL := p.baseURL + "/oauth/token"

	vals := url.Values{}
	vals.Set("client_id", p.config.ClientID)
	vals.Set("client_secret", p.config.ClientSecret)
	vals.Set("code", code)
	vals.Set("grant_type", "authorization_code")
	vals.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		CreatedAt    int64  `json:"created_at"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("gitlab error: %s (%s)", result.Error, result.ErrorDesc)
	}

	token := &OAuthToken{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    result.TokenType,
	}

	if result.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	}

	return token, nil
}

// GetUser retrieves the authenticated user's information.
func (p *GitLabProvider) GetUser(ctx context.Context, token string) (*User, error) {
	apiURL := p.baseURL + "/api/v4/user"

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user: status %d", resp.StatusCode)
	}

	var glUser struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&glUser); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &User{
		ID:        fmt.Sprintf("%d", glUser.ID),
		Login:     glUser.Username,
		Name:      glUser.Name,
		Email:     glUser.Email,
		AvatarURL: glUser.AvatarURL,
	}, nil
}

// ListRepositories returns repositories the authenticated user has access to.
func (p *GitLabProvider) ListRepositories(ctx context.Context, token string) ([]*Repository, error) {
	apiURL := p.baseURL + "/api/v4/projects?membership=true&per_page=100&order_by=updated_at"

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list repositories: status %d", resp.StatusCode)
	}

	var glProjects []struct {
		ID                int64  `json:"id"`
		Name              string `json:"name"`
		PathWithNamespace string `json:"path_with_namespace"`
		Description       string `json:"description"`
		WebURL            string `json:"web_url"`
		HTTPURLToRepo     string `json:"http_url_to_repo"`
		SSHURLToRepo      string `json:"ssh_url_to_repo"`
		DefaultBranch     string `json:"default_branch"`
		Visibility        string `json:"visibility"`
		Archived          bool   `json:"archived"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&glProjects); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	repos := make([]*Repository, len(glProjects))
	for i, p := range glProjects {
		repos[i] = &Repository{
			ID:            fmt.Sprintf("%d", p.ID),
			Name:          p.Name,
			FullName:      p.PathWithNamespace,
			Description:   p.Description,
			HTMLURL:       p.WebURL,
			CloneURL:      p.HTTPURLToRepo,
			SSHURL:        p.SSHURLToRepo,
			DefaultBranch: p.DefaultBranch,
			Private:       p.Visibility != "public",
			Archived:      p.Archived,
		}
	}

	return repos, nil
}

// GetRepository retrieves a specific repository by owner and name.
func (p *GitLabProvider) GetRepository(ctx context.Context, token, owner, repo string) (*Repository, error) {
	// GitLab uses URL-encoded project path
	projectPath := url.PathEscape(owner + "/" + repo)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s", p.baseURL, projectPath)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("repository not found: %s/%s", owner, repo)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get repository: status %d", resp.StatusCode)
	}

	var glProject struct {
		ID                int64  `json:"id"`
		Name              string `json:"name"`
		PathWithNamespace string `json:"path_with_namespace"`
		Description       string `json:"description"`
		WebURL            string `json:"web_url"`
		HTTPURLToRepo     string `json:"http_url_to_repo"`
		SSHURLToRepo      string `json:"ssh_url_to_repo"`
		DefaultBranch     string `json:"default_branch"`
		Visibility        string `json:"visibility"`
		Archived          bool   `json:"archived"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&glProject); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &Repository{
		ID:            fmt.Sprintf("%d", glProject.ID),
		Name:          glProject.Name,
		FullName:      glProject.PathWithNamespace,
		Description:   glProject.Description,
		HTMLURL:       glProject.WebURL,
		CloneURL:      glProject.HTTPURLToRepo,
		SSHURL:        glProject.SSHURLToRepo,
		DefaultBranch: glProject.DefaultBranch,
		Private:       glProject.Visibility != "public",
		Archived:      glProject.Archived,
	}, nil
}

// CreateWebhook creates a webhook for a repository.
func (p *GitLabProvider) CreateWebhook(ctx context.Context, token, owner, repo, webhookURL, secret string, events []string) (*Webhook, error) {
	projectPath := url.PathEscape(owner + "/" + repo)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/hooks", p.baseURL, projectPath)

	// Map generic events to GitLab-specific events
	payload := map[string]interface{}{
		"url":                   webhookURL,
		"token":                 secret,
		"push_events":           contains(events, "push"),
		"merge_requests_events": contains(events, "pull_request") || contains(events, "merge_request"),
		"tag_push_events":       contains(events, "tag") || contains(events, "push"),
		"enable_ssl_verification": true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create webhook: status %d", resp.StatusCode)
	}

	var glWebhook struct {
		ID        int64     `json:"id"`
		URL       string    `json:"url"`
		CreatedAt time.Time `json:"created_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&glWebhook); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &Webhook{
		ID:        fmt.Sprintf("%d", glWebhook.ID),
		URL:       webhookURL,
		Events:    events,
		Active:    true,
		CreatedAt: glWebhook.CreatedAt,
	}, nil
}

// DeleteWebhook removes a webhook from a repository.
func (p *GitLabProvider) DeleteWebhook(ctx context.Context, token, owner, repo, webhookID string) error {
	projectPath := url.PathEscape(owner + "/" + repo)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/hooks/%s", p.baseURL, projectPath, webhookID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", apiURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete webhook: status %d", resp.StatusCode)
	}

	return nil
}

// GetCloneURL returns the authenticated clone URL for a repository.
func (p *GitLabProvider) GetCloneURL(ctx context.Context, token, owner, repo string) (string, error) {
	repository, err := p.GetRepository(ctx, token, owner, repo)
	if err != nil {
		return "", err
	}

	// Return HTTPS clone URL with oauth2 token embedded for authentication
	cloneURL := repository.CloneURL
	if token != "" {
		// Insert oauth2 token into URL: https://oauth2:token@gitlab.com/owner/repo.git
		cloneURL = strings.Replace(cloneURL, "https://", fmt.Sprintf("https://oauth2:%s@", token), 1)
	}

	return cloneURL, nil
}

// contains checks if a string slice contains a specific value.
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
