package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// AtticClient provides methods for interacting with an Attic binary cache.
type AtticClient struct {
	endpoint   string
	cacheName  string
	signingKey string
	httpClient *http.Client
	logger     *slog.Logger
}

// AtticConfig holds configuration for the Attic client.
type AtticConfig struct {
	Endpoint   string // Attic server endpoint (e.g., "https://cache.example.com")
	CacheName  string // Name of the cache to push to
	SigningKey string // Path to the signing key or the key itself
	Timeout    time.Duration
}

// DefaultAtticConfig returns an AtticConfig with sensible defaults.
func DefaultAtticConfig() *AtticConfig {
	return &AtticConfig{
		Endpoint:  "http://localhost:8080",
		CacheName: "default",
		Timeout:   5 * time.Minute,
	}
}

// NewAtticClient creates a new Attic client.
func NewAtticClient(cfg *AtticConfig, logger *slog.Logger) *AtticClient {
	if logger == nil {
		logger = slog.Default()
	}

	return &AtticClient{
		endpoint:   cfg.Endpoint,
		cacheName:  cfg.CacheName,
		signingKey: cfg.SigningKey,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: logger,
	}
}


// PushResult holds the result of pushing a closure to Attic.
type PushResult struct {
	StorePath string        // The store path that was pushed
	CacheURL  string        // URL where the closure can be fetched
	Duration  time.Duration // Time taken to push
}

// Push pushes a Nix store path (closure) to the Attic cache.
// It uses the attic CLI tool to push with cryptographic signatures.
func (c *AtticClient) Push(ctx context.Context, storePath string) (*PushResult, error) {
	if !IsValidStorePath(storePath) {
		return nil, fmt.Errorf("invalid store path: %s", storePath)
	}

	c.logger.Info("pushing closure to Attic",
		"store_path", storePath,
		"cache", c.cacheName,
	)

	start := time.Now()

	// Build the attic push command
	args := []string{"push", c.cacheName, storePath}

	// Add signing key if configured
	if c.signingKey != "" {
		args = append(args, "--signing-key", c.signingKey)
	}

	cmd := exec.CommandContext(ctx, "attic", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("attic push failed: %w\nStderr: %s", err, stderr.String())
	}

	duration := time.Since(start)

	result := &PushResult{
		StorePath: storePath,
		CacheURL:  fmt.Sprintf("%s/%s", c.endpoint, c.cacheName),
		Duration:  duration,
	}

	c.logger.Info("closure pushed to Attic",
		"store_path", storePath,
		"duration", duration,
	)

	return result, nil
}

// PushWithDependencies pushes a store path and all its dependencies to Attic.
func (c *AtticClient) PushWithDependencies(ctx context.Context, storePath string) (*PushResult, error) {
	if !IsValidStorePath(storePath) {
		return nil, fmt.Errorf("invalid store path: %s", storePath)
	}

	c.logger.Info("pushing closure with dependencies to Attic",
		"store_path", storePath,
		"cache", c.cacheName,
	)

	start := time.Now()

	// Use nix-store to get the closure and pipe to attic
	// First, get all paths in the closure
	closureCmd := exec.CommandContext(ctx, "nix-store", "-qR", storePath)
	closureOutput, err := closureCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting closure: %w", err)
	}

	paths := strings.Split(strings.TrimSpace(string(closureOutput)), "\n")
	c.logger.Debug("pushing closure paths", "count", len(paths))

	// Push all paths
	args := append([]string{"push", c.cacheName}, paths...)
	if c.signingKey != "" {
		args = append(args, "--signing-key", c.signingKey)
	}

	cmd := exec.CommandContext(ctx, "attic", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("attic push failed: %w\nStderr: %s", err, stderr.String())
	}

	duration := time.Since(start)

	result := &PushResult{
		StorePath: storePath,
		CacheURL:  fmt.Sprintf("%s/%s", c.endpoint, c.cacheName),
		Duration:  duration,
	}

	c.logger.Info("closure with dependencies pushed to Attic",
		"store_path", storePath,
		"paths_count", len(paths),
		"duration", duration,
	)

	return result, nil
}

// Verify checks if a store path exists in the Attic cache.
func (c *AtticClient) Verify(ctx context.Context, storePath string) (bool, error) {
	if !IsValidStorePath(storePath) {
		return false, fmt.Errorf("invalid store path: %s", storePath)
	}

	// Extract the hash from the store path
	// Format: /nix/store/<hash>-<name>
	hash := extractStoreHash(storePath)
	if hash == "" {
		return false, fmt.Errorf("could not extract hash from store path: %s", storePath)
	}

	// Query the Attic API to check if the path exists
	url := fmt.Sprintf("%s/api/v1/cache/%s/narinfo/%s", c.endpoint, c.cacheName, hash)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("checking cache: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// GetCacheInfo retrieves information about the Attic cache.
func (c *AtticClient) GetCacheInfo(ctx context.Context) (*CacheInfo, error) {
	url := fmt.Sprintf("%s/api/v1/cache/%s", c.endpoint, c.cacheName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting cache info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cache info request failed: %s - %s", resp.Status, string(body))
	}

	var info CacheInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding cache info: %w", err)
	}

	return &info, nil
}

// CacheInfo holds information about an Attic cache.
type CacheInfo struct {
	Name           string `json:"name"`
	IsPublic       bool   `json:"is_public"`
	StoreDir       string `json:"store_dir"`
	Priority       int    `json:"priority"`
	UpstreamCaches []string `json:"upstream_caches,omitempty"`
}

// extractStoreHash extracts the hash portion from a Nix store path.
func extractStoreHash(storePath string) string {
	// Format: /nix/store/<hash>-<name>
	prefix := "/nix/store/"
	if !strings.HasPrefix(storePath, prefix) {
		return ""
	}

	remainder := strings.TrimPrefix(storePath, prefix)
	dashIdx := strings.Index(remainder, "-")
	if dashIdx == -1 || dashIdx != 32 {
		return ""
	}

	return remainder[:dashIdx]
}

// SignClosure signs a store path with the configured signing key.
// This is typically done automatically by Push, but can be called separately.
func (c *AtticClient) SignClosure(ctx context.Context, storePath string) error {
	if c.signingKey == "" {
		return fmt.Errorf("no signing key configured")
	}

	if !IsValidStorePath(storePath) {
		return fmt.Errorf("invalid store path: %s", storePath)
	}

	c.logger.Debug("signing closure", "store_path", storePath)

	cmd := exec.CommandContext(ctx, "nix", "store", "sign",
		"--key-file", c.signingKey,
		storePath,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("signing closure: %w\nStderr: %s", err, stderr.String())
	}

	return nil
}
