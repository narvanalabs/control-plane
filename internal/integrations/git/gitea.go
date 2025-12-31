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

// GiteaProvider implements the Provider interface for Gitea.
type GiteaProvider struct {
	config  *ProviderConfig
	client  *http.Client
	baseURL string
}

// NewGiteaProvider creates a new Gitea provider instance.
func NewGiteaProvider() Provider {
	return &GiteaProvider{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the provider type identifier.
func (p *GiteaProvider) Name() ProviderType {
	return ProviderGitea
}

// Configure sets up the provider with the given configuration.
func (p *GiteaProvider) Configure(ctx context.Context, config *ProviderConfig) error {
	if config.InstanceURL == "" {
		return fmt.Errorf("instance_url is required for Gitea")
	}
	if config.ClientID == "" {
		return fmt.Errorf("client_id is required")
	}
	if config.ClientSecret == "" {
		return fmt.Errorf("client_secret is required")
	}

	p.config = config
	p.baseURL = strings.TrimSuffix(config.InstanceURL, "/")

	return nil
}

// GetAuthorizationURL returns the OAuth authorization URL for user authentication.
func (p *GiteaProvider) GetAuthorizationURL(state, redirectURI string) string {
	params := url.Values{}
	params.Set("client_id", p.config.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "read:user read:repository write:repository")
	params.Set("state", state)
	return p.baseURL + "/login/oauth/authorize?" + params.Encode()
}

// ExchangeCode exchanges an OAuth authorization code for access tokens.
func (p *GiteaProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (*OAuthToken, error) {
	tokenURL := p.baseURL + "/login/oauth/access_token"

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
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("gitea error: %s (%s)", result.Error, result.ErrorDesc)
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
func (p *GiteaProvider) GetUser(ctx context.Context, token string) (*User, error) {
	apiURL := p.baseURL + "/api/v1/user"

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user: status %d", resp.StatusCode)
	}

	var giteaUser struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		FullName  string `json:"full_name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&giteaUser); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &User{
		ID:        fmt.Sprintf("%d", giteaUser.ID),
		Login:     giteaUser.Login,
		Name:      giteaUser.FullName,
		Email:     giteaUser.Email,
		AvatarURL: giteaUser.AvatarURL,
	}, nil
}

// ListRepositories returns repositories the authenticated user has access to.
func (p *GiteaProvider) ListRepositories(ctx context.Context, token string) ([]*Repository, error) {
	apiURL := p.baseURL + "/api/v1/user/repos?limit=100"

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list repositories: status %d", resp.StatusCode)
	}

	var giteaRepos []struct {
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

	if err := json.NewDecoder(resp.Body).Decode(&giteaRepos); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	repos := make([]*Repository, len(giteaRepos))
	for i, r := range giteaRepos {
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
func (p *GiteaProvider) GetRepository(ctx context.Context, token, owner, repo string) (*Repository, error) {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s", p.baseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "token "+token)
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

	var giteaRepo struct {
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

	if err := json.NewDecoder(resp.Body).Decode(&giteaRepo); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &Repository{
		ID:            fmt.Sprintf("%d", giteaRepo.ID),
		Name:          giteaRepo.Name,
		FullName:      giteaRepo.FullName,
		Description:   giteaRepo.Description,
		HTMLURL:       giteaRepo.HTMLURL,
		CloneURL:      giteaRepo.CloneURL,
		SSHURL:        giteaRepo.SSHURL,
		DefaultBranch: giteaRepo.DefaultBranch,
		Private:       giteaRepo.Private,
		Archived:      giteaRepo.Archived,
	}, nil
}

// CreateWebhook creates a webhook for a repository.
func (p *GiteaProvider) CreateWebhook(ctx context.Context, token, owner, repo, webhookURL, secret string, events []string) (*Webhook, error) {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/hooks", p.baseURL, owner, repo)

	// Map generic events to Gitea-specific events
	giteaEvents := make([]string, 0)
	for _, e := range events {
		switch e {
		case "push":
			giteaEvents = append(giteaEvents, "push")
		case "pull_request":
			giteaEvents = append(giteaEvents, "pull_request")
		case "tag":
			giteaEvents = append(giteaEvents, "create") // Tag creation
		}
	}

	if len(giteaEvents) == 0 {
		giteaEvents = []string{"push"}
	}

	payload := map[string]interface{}{
		"type":   "gitea",
		"active": true,
		"events": giteaEvents,
		"config": map[string]interface{}{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       secret,
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

	req.Header.Set("Authorization", "token "+token)
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

	var giteaWebhook struct {
		ID        int64     `json:"id"`
		URL       string    `json:"url"`
		Events    []string  `json:"events"`
		Active    bool      `json:"active"`
		CreatedAt time.Time `json:"created_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&giteaWebhook); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &Webhook{
		ID:        fmt.Sprintf("%d", giteaWebhook.ID),
		URL:       webhookURL,
		Events:    events,
		Active:    giteaWebhook.Active,
		CreatedAt: giteaWebhook.CreatedAt,
	}, nil
}

// DeleteWebhook removes a webhook from a repository.
func (p *GiteaProvider) DeleteWebhook(ctx context.Context, token, owner, repo, webhookID string) error {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/hooks/%s", p.baseURL, owner, repo, webhookID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", apiURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "token "+token)
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
func (p *GiteaProvider) GetCloneURL(ctx context.Context, token, owner, repo string) (string, error) {
	repository, err := p.GetRepository(ctx, token, owner, repo)
	if err != nil {
		return "", err
	}

	// Return HTTPS clone URL with token embedded for authentication
	cloneURL := repository.CloneURL
	if token != "" {
		// Insert token into URL: https://token@gitea.example.com/owner/repo.git
		// Parse the URL to insert the token properly
		u, err := url.Parse(cloneURL)
		if err != nil {
			return "", fmt.Errorf("parsing clone URL: %w", err)
		}
		u.User = url.User(token)
		cloneURL = u.String()
	}

	return cloneURL, nil
}
