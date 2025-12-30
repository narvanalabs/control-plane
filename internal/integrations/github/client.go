package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Client is a minimal GitHub API client.
type Client struct {
	hc *http.Client
}

// NewClient creates a new GitHub API client.
func NewClient() *Client {
	return &Client{
		hc: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ManifestConversion represents the result of a GitHub App manifest conversion.
type ManifestConversion struct {
	ID            int64  `json:"id"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	WebhookSecret string `json:"webhook_secret"`
	PEM           string `json:"pem"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
}

// ConvertManifest exchanges a manifest code for App credentials.
func (c *Client) ConvertManifest(ctx context.Context, code string) (*ManifestConversion, error) {
	targetURL := fmt.Sprintf("https://api.github.com/app-manifests/%s/conversions", code)
	
	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, nil)
	if err != nil {
		return nil, err
	}
	
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	
	var conversion ManifestConversion
	if err := json.NewDecoder(resp.Body).Decode(&conversion); err != nil {
		return nil, err
	}
	
	return &conversion, nil
}

// GenerateInstallationToken generates an access token for a specific installation.
func (c *Client) GenerateInstallationToken(ctx context.Context, appID int64, privateKeyPEM string, installationID int64) (string, error) {
	// 1. Create JWT
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyPEM))
	if err != nil {
		return "", fmt.Errorf("parsing private key: %w", err)
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(10 * time.Minute).Unix(),
		"iss": fmt.Sprintf("%d", appID),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("signing jwt: %w", err)
	}

	// 2. Exchange JWT for installation token
	apiURL := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+signedToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("failed to get installation token: status %d", resp.StatusCode)
	}

	var tokenResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	return tokenResp.Token, nil
}

// ListRepositories lists repositories for a specific installation.
func (c *Client) ListRepositories(ctx context.Context, token string) ([]map[string]interface{}, error) {
	apiURL := "https://api.github.com/installation/repositories"
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list repositories: status %d", resp.StatusCode)
	}

	var result struct {
		Repositories []map[string]interface{} `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Repositories, nil
}
// ExchangeCode exchanges an OAuth code for an access token.
func (c *Client) ExchangeCode(ctx context.Context, clientID, clientSecret, code string) (map[string]interface{}, error) {
	targetURL := "https://github.com/login/oauth/access_token"
	
	vals := url.Values{}
	vals.Set("client_id", clientID)
	vals.Set("client_secret", clientSecret)
	vals.Set("code", code)

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if errMsg, ok := result["error"]; ok {
		return nil, fmt.Errorf("github error: %v (%v)", errMsg, result["error_description"])
	}

	return result, nil
}

// GetUser fetches information about the authenticated user.
func (c *Client) GetUser(ctx context.Context, token string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user: status %d", resp.StatusCode)
	}

	var user map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return user, nil
}

// ListUserRepositories lists repositories for the authenticated user (OAuth flow).
func (c *Client) ListUserRepositories(ctx context.Context, token string) ([]map[string]interface{}, error) {
	apiURL := "https://api.github.com/user/repos?per_page=100&sort=updated"
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list user repositories: status %d", resp.StatusCode)
	}

	var repos []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, err
	}

	return repos, nil
}
