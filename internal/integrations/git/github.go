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
	githubAPIURL      = "https://api.github.com"
	githubAuthorizeURL = "https://github.com/login/oauth/authorize"
	githubTokenURL    = "https://github.com/login/oauth/access_token"
)

// GitHubProvider implements the Provider interface for GitHub.
type GitHubProvider struct {
	config *ProviderConfig
	client *http.Client
}

// NewGitHubProvider creates a new GitHub provider instance.
func NewGitHubProvider() Provider {
	return &GitHubProvider{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the provider type identifier.
func (p *GitHubProvider) Name() ProviderType {
	return ProviderGitHub
}

// Configure sets up the provider with the given configuration.
func (p *GitHubProvider) Configure(ctx context.Context, config *ProviderConfig) error {
	if config.ClientID == "" {
		return fmt.Errorf("client_id is required")
	}
	if config.ClientSecret == "" {
		return fmt.Errorf("client_secret is required")
	}
	p.config = config
	return nil
}

// GetAuthorizationURL returns the OAuth authorization URL for user authentication.
func (p *GitHubProvider) GetAuthorizationURL(state, redirectURI string) string {
	params := url.Values{}
	params.Set("client_id", p.config.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", "repo,user")
	params.Set("state", state)
	return githubAuthorizeURL + "?" + params.Encode()
}

// ExchangeCode exchanges an OAuth authorization code for access tokens.
func (p *GitHubProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (*OAuthToken, error) {
	vals := url.Values{}
	vals.Set("client_id", p.config.ClientID)
	vals.Set("client_secret", p.config.ClientSecret)
	vals.Set("code", code)
	vals.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", githubTokenURL, strings.NewReader(vals.Encode()))
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
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("github error: %s (%s)", result.Error, result.ErrorDesc)
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
func (p *GitHubProvider) GetUser(ctx context.Context, token string) (*User, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", githubAPIURL+"/user", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user: status %d", resp.StatusCode)
	}

	var ghUser struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &User{
		ID:        fmt.Sprintf("%d", ghUser.ID),
		Login:     ghUser.Login,
		Name:      ghUser.Name,
		Email:     ghUser.Email,
		AvatarURL: ghUser.AvatarURL,
	}, nil
}

// ListRepositories returns repositories the authenticated user has access to.
func (p *GitHubProvider) ListRepositories(ctx context.Context, token string) ([]*Repository, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", githubAPIURL+"/user/repos?per_page=100&sort=updated", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list repositories: status %d", resp.StatusCode)
	}

	var ghRepos []struct {
		ID            int64  `json:"id"`
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

	if err := json.NewDecoder(resp.Body).Decode(&ghRepos); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	repos := make([]*Repository, len(ghRepos))
	for i, r := range ghRepos {
		repos[i] = &Repository{
			ID:            fmt.Sprintf("%d", r.ID),
			Name:          r.Name,
			FullName:      r.FullName,
			Description:   r.Description,
			HTMLURL:       r.HTMLURL,
			CloneURL:      r.CloneURL,
			SSHURL:        r.SSHURL,
			DefaultBranch: r.DefaultBranch,
			Private:       r.Private,
			Archived:      r.Archived,
		}
	}

	return repos, nil
}

// GetRepository retrieves a specific repository by owner and name.
func (p *GitHubProvider) GetRepository(ctx context.Context, token, owner, repo string) (*Repository, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s", githubAPIURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

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

	var ghRepo struct {
		ID            int64  `json:"id"`
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

	if err := json.NewDecoder(resp.Body).Decode(&ghRepo); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &Repository{
		ID:            fmt.Sprintf("%d", ghRepo.ID),
		Name:          ghRepo.Name,
		FullName:      ghRepo.FullName,
		Description:   ghRepo.Description,
		HTMLURL:       ghRepo.HTMLURL,
		CloneURL:      ghRepo.CloneURL,
		SSHURL:        ghRepo.SSHURL,
		DefaultBranch: ghRepo.DefaultBranch,
		Private:       ghRepo.Private,
		Archived:      ghRepo.Archived,
	}, nil
}

// CreateWebhook creates a webhook for a repository.
func (p *GitHubProvider) CreateWebhook(ctx context.Context, token, owner, repo, webhookURL, secret string, events []string) (*Webhook, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/hooks", githubAPIURL, owner, repo)

	payload := map[string]interface{}{
		"name":   "web",
		"active": true,
		"events": events,
		"config": map[string]interface{}{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       secret,
			"insecure_ssl": "0",
		},
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
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create webhook: status %d", resp.StatusCode)
	}

	var ghWebhook struct {
		ID        int64     `json:"id"`
		URL       string    `json:"url"`
		Events    []string  `json:"events"`
		Active    bool      `json:"active"`
		CreatedAt time.Time `json:"created_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghWebhook); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &Webhook{
		ID:        fmt.Sprintf("%d", ghWebhook.ID),
		URL:       webhookURL,
		Events:    ghWebhook.Events,
		Active:    ghWebhook.Active,
		CreatedAt: ghWebhook.CreatedAt,
	}, nil
}

// DeleteWebhook removes a webhook from a repository.
func (p *GitHubProvider) DeleteWebhook(ctx context.Context, token, owner, repo, webhookID string) error {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/hooks/%s", githubAPIURL, owner, repo, webhookID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", apiURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

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
func (p *GitHubProvider) GetCloneURL(ctx context.Context, token, owner, repo string) (string, error) {
	repository, err := p.GetRepository(ctx, token, owner, repo)
	if err != nil {
		return "", err
	}

	// Return HTTPS clone URL with token embedded for authentication
	cloneURL := repository.CloneURL
	if token != "" {
		// Insert token into URL: https://token@github.com/owner/repo.git
		cloneURL = strings.Replace(cloneURL, "https://", fmt.Sprintf("https://%s@", token), 1)
	}

	return cloneURL, nil
}
