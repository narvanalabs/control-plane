// Package cache provides build caching for faster rebuilds.
package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

// DetectionCache caches detection results by commit SHA.
// **Validates: Requirements 4.3**
type DetectionCache interface {
	// Get retrieves cached detection results for a commit.
	Get(ctx context.Context, repoURL, commitSHA string) (*models.DetectionResult, bool)

	// Set stores detection results for a commit.
	Set(ctx context.Context, repoURL, commitSHA string, result *models.DetectionResult) error
}

// CachedDetection represents a cached detection result.
type CachedDetection struct {
	RepoURL   string                  `json:"repo_url"`
	CommitSHA string                  `json:"commit_sha"`
	Result    *models.DetectionResult `json:"result"`
	CreatedAt time.Time               `json:"created_at"`
}

// InMemoryDetectionCache implements DetectionCache with an in-memory store.
// It is thread-safe and supports TTL-based expiration.
// **Validates: Requirements 4.3**
type InMemoryDetectionCache struct {
	// storage holds cached detections keyed by "repoURL:commitSHA"
	storage map[string]*CachedDetection

	// mu protects the storage map
	mu sync.RWMutex

	// ttl is the time-to-live for cached detections
	ttl time.Duration
}

// DetectionCacheOption is a functional option for configuring InMemoryDetectionCache.
type DetectionCacheOption func(*InMemoryDetectionCache)

// WithDetectionTTL sets the time-to-live for cached detections.
func WithDetectionTTL(ttl time.Duration) DetectionCacheOption {
	return func(c *InMemoryDetectionCache) {
		c.ttl = ttl
	}
}

// NewInMemoryDetectionCache creates a new InMemoryDetectionCache with default settings.
// Default TTL is 24 hours.
func NewInMemoryDetectionCache(opts ...DetectionCacheOption) *InMemoryDetectionCache {
	c := &InMemoryDetectionCache{
		storage: make(map[string]*CachedDetection),
		ttl:     24 * time.Hour, // Default 24 hour TTL
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// cacheKey generates a cache key from repo URL and commit SHA.
func cacheKey(repoURL, commitSHA string) string {
	return fmt.Sprintf("%s:%s", repoURL, commitSHA)
}

// Get retrieves cached detection results for a commit.
// Returns the cached result and true if found and not expired, nil and false otherwise.
// **Validates: Requirements 4.3**
func (c *InMemoryDetectionCache) Get(ctx context.Context, repoURL, commitSHA string) (*models.DetectionResult, bool) {
	if repoURL == "" || commitSHA == "" {
		return nil, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	key := cacheKey(repoURL, commitSHA)
	cached, ok := c.storage[key]
	if !ok {
		return nil, false
	}

	// Check if the cached entry has expired
	if c.ttl > 0 && time.Since(cached.CreatedAt) > c.ttl {
		return nil, false
	}

	// Return a deep copy to prevent external modification
	return copyDetectionResult(cached.Result), true
}

// Set stores detection results for a commit.
// **Validates: Requirements 4.3**
func (c *InMemoryDetectionCache) Set(ctx context.Context, repoURL, commitSHA string, result *models.DetectionResult) error {
	if repoURL == "" {
		return ErrEmptyRepoURL
	}
	if commitSHA == "" {
		return ErrEmptyCommitSHA
	}
	if result == nil {
		return ErrNilDetectionResult
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey(repoURL, commitSHA)
	c.storage[key] = &CachedDetection{
		RepoURL:   repoURL,
		CommitSHA: commitSHA,
		Result:    copyDetectionResult(result),
		CreatedAt: time.Now(),
	}

	return nil
}

// Delete removes a cached detection result.
func (c *InMemoryDetectionCache) Delete(ctx context.Context, repoURL, commitSHA string) error {
	if repoURL == "" || commitSHA == "" {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey(repoURL, commitSHA)
	delete(c.storage, key)

	return nil
}

// Clear removes all cached detection results.
func (c *InMemoryDetectionCache) Clear(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.storage = make(map[string]*CachedDetection)
}

// Size returns the number of cached entries.
func (c *InMemoryDetectionCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.storage)
}

// CleanupExpired removes expired entries from the cache.
// Returns the number of entries removed.
func (c *InMemoryDetectionCache) CleanupExpired(ctx context.Context) int {
	if c.ttl <= 0 {
		return 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removed := 0

	for key, cached := range c.storage {
		if now.Sub(cached.CreatedAt) > c.ttl {
			delete(c.storage, key)
			removed++
		}
	}

	return removed
}

// copyDetectionResult creates a deep copy of a DetectionResult.
func copyDetectionResult(result *models.DetectionResult) *models.DetectionResult {
	if result == nil {
		return nil
	}

	copy := &models.DetectionResult{
		Strategy:             result.Strategy,
		Framework:            result.Framework,
		Version:              result.Version,
		RecommendedBuildType: result.RecommendedBuildType,
		Confidence:           result.Confidence,
	}

	// Copy SuggestedConfig map
	if result.SuggestedConfig != nil {
		copy.SuggestedConfig = make(map[string]interface{}, len(result.SuggestedConfig))
		for k, v := range result.SuggestedConfig {
			copy.SuggestedConfig[k] = v
		}
	}

	// Copy EntryPoints slice
	if result.EntryPoints != nil {
		copy.EntryPoints = make([]string, len(result.EntryPoints))
		for i, ep := range result.EntryPoints {
			copy.EntryPoints[i] = ep
		}
	}

	// Copy Warnings slice
	if result.Warnings != nil {
		copy.Warnings = make([]string, len(result.Warnings))
		for i, w := range result.Warnings {
			copy.Warnings[i] = w
		}
	}

	return copy
}
