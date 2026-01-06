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
	bitbucketAPIURL       = "https://api.bitbucket.org/2.0"
	bitbucketAuthorizeURL = "https://bitbucket.org/site/oauth2/authorize"
	bitbucketTokenURL     = "https://bitbucket.org/site/oauth2/access_token"
)

// BitbucketProvider implements the Provider interface for Bitbucket.
type BitbucketProvider struct {
	config *ProviderConfig
	client *http.Client
}

// NewBitbucketProvider creates a new Bitbucket provider instance.
func NewBitbucketProvider() Provider {
	return &BitbucketProvider{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the provider type identifier.
func (p *BitbucketProvider) Name() ProviderType {
	return ProviderBitbucket
}

// Configure sets up the provider with the given configuration.
func (p *BitbucketProvider) Configure(ctx context.Context, config *ProviderConfig) error {
	if config.ClientID == "" {
		return fmt.Errorf("client_id (consumer key) is required")
	}
	if config.ClientSecret == "" {
		return fmt.Errorf("client_secret (consumer secret) is required")
	}
	p.config = config
	return nil
}

// GetAuthorizationURL returns the OAuth authorization URL for user authentication.
func (p *BitbucketProvider) GetAuthorizationURL(state, redirectURI string) string {
	params := url.Values{}
	params.Set("client_id", p.config.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("state", state)
	return bitbucketAuthorizeURL + "?" + params.Encode()
}

// ExchangeCode exchanges an OAuth authorization code for access tokens.
func (p *BitbucketProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (*OAuthToken, error) {
	vals := url.Values{}
	vals.Set("grant_type", "authorization_code")
	vals.Set("code", code)
	vals.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", bitbucketTokenURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Bitbucket uses Basic Auth for token exchange
	req.SetBasicAuth(p.config.ClientID, p.config.ClientSecret)
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
		Scopes       string `json:"scopes"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("bitbucket error: %s (%s)", result.Error, result.ErrorDesc)
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
func (p *BitbucketProvider) GetUser(ctx context.Context, token string) (*User, error) {
	apiURL := bitbucketAPIURL + "/user"

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

	var bbUser struct {
		UUID        string `json:"uuid"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Links       struct {
			Avatar struct {
				Href string `json:"href"`
			} `json:"avatar"`
		} `json:"links"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&bbUser); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Fetch email separately as it's not included in the user endpoint
	email, _ := p.getUserEmail(ctx, token)

	return &User{
		ID:        bbUser.UUID,
		Login:     bbUser.Username,
		Name:      bbUser.DisplayName,
		Email:     email,
		AvatarURL: bbUser.Links.Avatar.Href,
	}, nil
}

// getUserEmail fetches the primary email for the authenticated user.
func (p *BitbucketProvider) getUserEmail(ctx context.Context, token string) (string, error) {
	apiURL := bitbucketAPIURL + "/user/emails"

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get emails: status %d", resp.StatusCode)
	}

	var result struct {
		Values []struct {
			Email     string `json:"email"`
			IsPrimary bool   `json:"is_primary"`
		} `json:"values"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	for _, e := range result.Values {
		if e.IsPrimary {
			return e.Email, nil
		}
	}

	if len(result.Values) > 0 {
		return result.Values[0].Email, nil
	}

	return "", nil
}

// ListRepositories returns repositories the authenticated user has access to.
func (p *BitbucketProvider) ListRepositories(ctx context.Context, token string) ([]*Repository, error) {
	// First get the user to get their username
	user, err := p.GetUser(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}

	apiURL := fmt.Sprintf("%s/repositories/%s?pagelen=100&sort=-updated_on", bitbucketAPIURL, user.Login)

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

	var result struct {
		Values []struct {
			UUID        string `json:"uuid"`
			Name        string `json:"name"`
			FullName    string `json:"full_name"`
			Description string `json:"description"`
			IsPrivate   bool   `json:"is_private"`
			Links       struct {
				HTML  struct{ Href string } `json:"html"`
				Clone []struct {
					Name string `json:"name"`
					Href string `json:"href"`
				} `json:"clone"`
			} `json:"links"`
			Mainbranch struct {
				Name string `json:"name"`
			} `json:"mainbranch"`
		} `json:"values"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	repos := make([]*Repository, len(result.Values))
	for i, r := range result.Values {
		var cloneURL, sshURL string
		for _, link := range r.Links.Clone {
			if link.Name == "https" {
				cloneURL = link.Href
			} else if link.Name == "ssh" {
				sshURL = link.Href
			}
		}

		repos[i] = &Repository{
			ID:            r.UUID,
			Name:          r.Name,
			FullName:      r.FullName,
			Description:   r.Description,
			HTMLURL:       r.Links.HTML.Href,
			CloneURL:      cloneURL,
			SSHURL:        sshURL,
			DefaultBranch: r.Mainbranch.Name,
			Private:       r.IsPrivate,
			Archived:      false, // Bitbucket doesn't have archived concept in the same way
		}
	}

	return repos, nil
}

// GetRepository retrieves a specific repository by owner and name.
func (p *BitbucketProvider) GetRepository(ctx context.Context, token, owner, repo string) (*Repository, error) {
	apiURL := fmt.Sprintf("%s/repositories/%s/%s", bitbucketAPIURL, owner, repo)

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

	var bbRepo struct {
		UUID        string `json:"uuid"`
		Name        string `json:"name"`
		FullName    string `json:"full_name"`
		Description string `json:"description"`
		IsPrivate   bool   `json:"is_private"`
		Links       struct {
			HTML  struct{ Href string } `json:"html"`
			Clone []struct {
				Name string `json:"name"`
				Href string `json:"href"`
			} `json:"clone"`
		} `json:"links"`
		Mainbranch struct {
			Name string `json:"name"`
		} `json:"mainbranch"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&bbRepo); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var cloneURL, sshURL string
	for _, link := range bbRepo.Links.Clone {
		if link.Name == "https" {
			cloneURL = link.Href
		} else if link.Name == "ssh" {
			sshURL = link.Href
		}
	}

	return &Repository{
		ID:            bbRepo.UUID,
		Name:          bbRepo.Name,
		FullName:      bbRepo.FullName,
		Description:   bbRepo.Description,
		HTMLURL:       bbRepo.Links.HTML.Href,
		CloneURL:      cloneURL,
		SSHURL:        sshURL,
		DefaultBranch: bbRepo.Mainbranch.Name,
		Private:       bbRepo.IsPrivate,
		Archived:      false,
	}, nil
}

// CreateWebhook creates a webhook for a repository.
func (p *BitbucketProvider) CreateWebhook(ctx context.Context, token, owner, repo, webhookURL, secret string, events []string) (*Webhook, error) {
	apiURL := fmt.Sprintf("%s/repositories/%s/%s/hooks", bitbucketAPIURL, owner, repo)

	// Map generic events to Bitbucket-specific events
	bbEvents := make([]string, 0)
	for _, e := range events {
		switch e {
		case "push":
			bbEvents = append(bbEvents, "repo:push")
		case "pull_request":
			bbEvents = append(bbEvents, "pullrequest:created", "pullrequest:updated", "pullrequest:fulfilled")
		case "tag":
			bbEvents = append(bbEvents, "repo:push") // Tags are included in push events
		}
	}

	if len(bbEvents) == 0 {
		bbEvents = []string{"repo:push"}
	}

	payload := map[string]interface{}{
		"description": "Narvana deployment webhook",
		"url":         webhookURL,
		"active":      true,
		"events":      bbEvents,
		"secret":      secret,
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

	var bbWebhook struct {
		UUID      string    `json:"uuid"`
		URL       string    `json:"url"`
		Events    []string  `json:"events"`
		Active    bool      `json:"active"`
		CreatedAt time.Time `json:"created_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&bbWebhook); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &Webhook{
		ID:        bbWebhook.UUID,
		URL:       webhookURL,
		Events:    events,
		Active:    bbWebhook.Active,
		CreatedAt: bbWebhook.CreatedAt,
	}, nil
}

// DeleteWebhook removes a webhook from a repository.
func (p *BitbucketProvider) DeleteWebhook(ctx context.Context, token, owner, repo, webhookID string) error {
	apiURL := fmt.Sprintf("%s/repositories/%s/%s/hooks/%s", bitbucketAPIURL, owner, repo, webhookID)

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
func (p *BitbucketProvider) GetCloneURL(ctx context.Context, token, owner, repo string) (string, error) {
	repository, err := p.GetRepository(ctx, token, owner, repo)
	if err != nil {
		return "", err
	}

	// Return HTTPS clone URL with x-token-auth for authentication
	cloneURL := repository.CloneURL
	if token != "" {
		// Insert token into URL: https://x-token-auth:token@bitbucket.org/owner/repo.git
		cloneURL = strings.Replace(cloneURL, "https://", fmt.Sprintf("https://x-token-auth:%s@", token), 1)
	}

	return cloneURL, nil
}
