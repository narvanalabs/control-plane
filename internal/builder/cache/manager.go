// Package cache provides build caching for faster rebuilds.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

// BuildCacheManager handles build caching for faster rebuilds.
type BuildCacheManager interface {
	// GetCacheKey generates a cache key for a build.
	GetCacheKey(ctx context.Context, job *models.BuildJob) (string, error)

	// CheckCache checks if a cached build exists.
	CheckCache(ctx context.Context, cacheKey string) (*CachedBuild, error)

	// StoreCache stores a build result in cache.
	StoreCache(ctx context.Context, cacheKey string, result *models.BuildResult) error

	// InvalidateCache invalidates cache for a service.
	InvalidateCache(ctx context.Context, serviceID string) error
}

// CachedBuild represents a cached build result.
type CachedBuild struct {
	CacheKey   string           `json:"cache_key"`
	Artifact   string           `json:"artifact"`
	BuildType  models.BuildType `json:"build_type"`
	CreatedAt  time.Time        `json:"created_at"`
	SourceHash string           `json:"source_hash"`
	DepsHash   string           `json:"deps_hash"`
	ConfigHash string           `json:"config_hash"`
}

// CacheKeyComponents contains the components used to generate a cache key.
type CacheKeyComponents struct {
	// SourceHash is the git commit SHA or source identifier.
	SourceHash string `json:"source_hash"`
	// DepsHash is the hash of dependency lock files (go.sum, package-lock.json, Cargo.lock).
	DepsHash string `json:"deps_hash"`
	// ConfigHash is the hash of the build configuration.
	ConfigHash string `json:"config_hash"`
	// Strategy is the build strategy used.
	Strategy models.BuildStrategy `json:"strategy"`
	// BuildType is the build output type.
	BuildType models.BuildType `json:"build_type"`
}

// Manager implements the BuildCacheManager interface.
type Manager struct {
	// storage holds cached builds keyed by cache key.
	storage map[string]*CachedBuild

	// serviceIndex maps service IDs to their cache keys for invalidation.
	serviceIndex map[string][]string

	// mu protects the storage and serviceIndex maps.
	mu sync.RWMutex

	// ttl is the time-to-live for cached builds.
	ttl time.Duration
}

// NewManager creates a new Manager with default settings.
func NewManager() *Manager {
	return &Manager{
		storage:      make(map[string]*CachedBuild),
		serviceIndex: make(map[string][]string),
		ttl:          24 * time.Hour, // Default 24 hour TTL
	}
}

// ManagerOption is a functional option for configuring Manager.
type ManagerOption func(*Manager)

// WithTTL sets the time-to-live for cached builds.
func WithTTL(ttl time.Duration) ManagerOption {
	return func(m *Manager) {
		m.ttl = ttl
	}
}

// NewManagerWithOptions creates a new Manager with custom options.
func NewManagerWithOptions(opts ...ManagerOption) *Manager {
	m := NewManager()
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// GetCacheKey generates a cache key for a build job.
// The cache key is a hash of:
// - Source hash (git commit SHA)
// - Dependencies hash (go.sum, package-lock.json, Cargo.lock)
// - Build config hash
// - Build strategy
// - Build type
func (m *Manager) GetCacheKey(ctx context.Context, job *models.BuildJob) (string, error) {
	if job == nil {
		return "", ErrNilBuildJob
	}

	components := CacheKeyComponents{
		SourceHash: m.getSourceHash(job),
		DepsHash:   m.getDepsHash(job),
		ConfigHash: m.getConfigHash(job),
		Strategy:   job.BuildStrategy,
		BuildType:  job.BuildType,
	}

	return m.hashComponents(components)
}

// getSourceHash extracts or generates a source hash from the build job.
func (m *Manager) getSourceHash(job *models.BuildJob) string {
	// Use GitRef as the source hash if available
	if job.GitRef != "" {
		return job.GitRef
	}
	// Fall back to hashing the git URL
	if job.GitURL != "" {
		return hashString(job.GitURL)
	}
	return ""
}

// getDepsHash extracts or generates a dependencies hash from the build job.
func (m *Manager) getDepsHash(job *models.BuildJob) string {
	// Use VendorHash if available (calculated from go.sum, package-lock.json, etc.)
	if job.VendorHash != "" {
		return job.VendorHash
	}
	return ""
}

// getConfigHash generates a hash of the build configuration.
func (m *Manager) getConfigHash(job *models.BuildJob) string {
	if job.BuildConfig == nil {
		return ""
	}

	// Serialize the build config to JSON and hash it
	configJSON, err := json.Marshal(job.BuildConfig)
	if err != nil {
		return ""
	}

	return hashString(string(configJSON))
}

// hashComponents generates a cache key from the components.
func (m *Manager) hashComponents(components CacheKeyComponents) (string, error) {
	// Serialize components to JSON
	data, err := json.Marshal(components)
	if err != nil {
		return "", fmt.Errorf("marshaling cache key components: %w", err)
	}

	// Generate SHA256 hash
	h := sha256.New()
	h.Write(data)
	hash := hex.EncodeToString(h.Sum(nil))

	return hash, nil
}

// CheckCache checks if a cached build exists for the given cache key.
func (m *Manager) CheckCache(ctx context.Context, cacheKey string) (*CachedBuild, error) {
	if cacheKey == "" {
		return nil, ErrEmptyCacheKey
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	cached, ok := m.storage[cacheKey]
	if !ok {
		return nil, ErrCacheNotFound
	}

	// Check if the cached build has expired
	if m.ttl > 0 && time.Since(cached.CreatedAt) > m.ttl {
		return nil, ErrCacheExpired
	}

	// Return a copy to prevent external modification
	return &CachedBuild{
		CacheKey:   cached.CacheKey,
		Artifact:   cached.Artifact,
		BuildType:  cached.BuildType,
		CreatedAt:  cached.CreatedAt,
		SourceHash: cached.SourceHash,
		DepsHash:   cached.DepsHash,
		ConfigHash: cached.ConfigHash,
	}, nil
}

// StoreCache stores a build result in the cache.
func (m *Manager) StoreCache(ctx context.Context, cacheKey string, result *models.BuildResult) error {
	if cacheKey == "" {
		return ErrEmptyCacheKey
	}

	if result == nil {
		return ErrNilBuildResult
	}

	if result.Artifact == "" {
		return ErrEmptyArtifact
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cached := &CachedBuild{
		CacheKey:  cacheKey,
		Artifact:  result.Artifact,
		CreatedAt: time.Now(),
	}

	// Set build type based on result
	if result.StorePath != "" {
		cached.BuildType = models.BuildTypePureNix
	} else if result.ImageTag != "" {
		cached.BuildType = models.BuildTypeOCI
	}

	m.storage[cacheKey] = cached

	return nil
}

// StoreCacheWithMetadata stores a build result with additional metadata.
func (m *Manager) StoreCacheWithMetadata(ctx context.Context, cacheKey string, result *models.BuildResult, job *models.BuildJob) error {
	if cacheKey == "" {
		return ErrEmptyCacheKey
	}

	if result == nil {
		return ErrNilBuildResult
	}

	if result.Artifact == "" {
		return ErrEmptyArtifact
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cached := &CachedBuild{
		CacheKey:   cacheKey,
		Artifact:   result.Artifact,
		CreatedAt:  time.Now(),
		SourceHash: m.getSourceHash(job),
		DepsHash:   m.getDepsHash(job),
		ConfigHash: m.getConfigHash(job),
	}

	// Set build type based on result
	if result.StorePath != "" {
		cached.BuildType = models.BuildTypePureNix
	} else if result.ImageTag != "" {
		cached.BuildType = models.BuildTypeOCI
	}

	m.storage[cacheKey] = cached

	// Update service index for invalidation
	if job != nil && job.ServiceName != "" {
		serviceKey := m.getServiceKey(job)
		m.serviceIndex[serviceKey] = append(m.serviceIndex[serviceKey], cacheKey)
	}

	return nil
}

// getServiceKey generates a key for the service index.
func (m *Manager) getServiceKey(job *models.BuildJob) string {
	if job.AppID != "" && job.ServiceName != "" {
		return fmt.Sprintf("%s/%s", job.AppID, job.ServiceName)
	}
	if job.ServiceName != "" {
		return job.ServiceName
	}
	return job.AppID
}

// InvalidateCache invalidates all cached builds for a service.
func (m *Manager) InvalidateCache(ctx context.Context, serviceID string) error {
	if serviceID == "" {
		return ErrEmptyServiceID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Get all cache keys for this service
	cacheKeys, ok := m.serviceIndex[serviceID]
	if !ok {
		// No cached builds for this service
		return nil
	}

	// Delete all cached builds for this service
	for _, key := range cacheKeys {
		delete(m.storage, key)
	}

	// Remove the service from the index
	delete(m.serviceIndex, serviceID)

	return nil
}

// InvalidateCacheKey invalidates a specific cache entry.
func (m *Manager) InvalidateCacheKey(ctx context.Context, cacheKey string) error {
	if cacheKey == "" {
		return ErrEmptyCacheKey
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.storage, cacheKey)

	return nil
}

// GetCacheStats returns statistics about the cache.
func (m *Manager) GetCacheStats(ctx context.Context) *CacheStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &CacheStats{
		TotalEntries:   len(m.storage),
		TotalServices:  len(m.serviceIndex),
		ExpiredEntries: 0,
	}

	// Count expired entries
	if m.ttl > 0 {
		now := time.Now()
		for _, cached := range m.storage {
			if now.Sub(cached.CreatedAt) > m.ttl {
				stats.ExpiredEntries++
			}
		}
	}

	return stats
}

// CacheStats contains statistics about the cache.
type CacheStats struct {
	TotalEntries   int `json:"total_entries"`
	TotalServices  int `json:"total_services"`
	ExpiredEntries int `json:"expired_entries"`
}

// CleanupExpired removes expired entries from the cache.
func (m *Manager) CleanupExpired(ctx context.Context) int {
	if m.ttl <= 0 {
		return 0
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	removed := 0

	for key, cached := range m.storage {
		if now.Sub(cached.CreatedAt) > m.ttl {
			delete(m.storage, key)
			removed++
		}
	}

	return removed
}

// ListCacheKeys returns all cache keys in the cache.
func (m *Manager) ListCacheKeys(ctx context.Context) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.storage))
	for key := range m.storage {
		keys = append(keys, key)
	}
	return keys
}

// hashString generates a SHA256 hash of a string.
func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// IsCacheHit checks if a cache key exists and is valid.
func (m *Manager) IsCacheHit(ctx context.Context, cacheKey string) bool {
	cached, err := m.CheckCache(ctx, cacheKey)
	return err == nil && cached != nil
}

// GetCachedArtifact returns the artifact for a cache key if it exists.
func (m *Manager) GetCachedArtifact(ctx context.Context, cacheKey string) (string, error) {
	cached, err := m.CheckCache(ctx, cacheKey)
	if err != nil {
		return "", err
	}
	return cached.Artifact, nil
}
