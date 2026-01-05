// Package api provides a client for communicating with the control-plane API.
package api

import (
	"context"
	"sync"
	"time"
)

// ConfigCache provides thread-safe caching for platform configuration.
// This reduces API calls and improves performance for frequently accessed config values.
// **Validates: Requirements 4.1, 4.2, 4.3, 4.4**
type ConfigCache struct {
	mu          sync.RWMutex
	config      *PlatformConfig
	lastFetched time.Time
	ttl         time.Duration
}

// DefaultConfigCacheTTL is the default time-to-live for cached configuration.
const DefaultConfigCacheTTL = 5 * time.Minute

// NewConfigCache creates a new configuration cache with the specified TTL.
func NewConfigCache(ttl time.Duration) *ConfigCache {
	if ttl <= 0 {
		ttl = DefaultConfigCacheTTL
	}
	return &ConfigCache{
		ttl: ttl,
	}
}

// Get retrieves the cached configuration, fetching from the backend if expired or not cached.
func (cc *ConfigCache) Get(ctx context.Context, client *Client) (*PlatformConfig, error) {
	cc.mu.RLock()
	if cc.config != nil && time.Since(cc.lastFetched) < cc.ttl {
		config := cc.config
		cc.mu.RUnlock()
		return config, nil
	}
	cc.mu.RUnlock()

	// Need to fetch fresh config
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// Double-check after acquiring write lock
	if cc.config != nil && time.Since(cc.lastFetched) < cc.ttl {
		return cc.config, nil
	}

	config, err := client.GetConfig(ctx)
	if err != nil {
		// If we have stale config, return it rather than failing
		if cc.config != nil {
			return cc.config, nil
		}
		return nil, err
	}

	cc.config = config
	cc.lastFetched = time.Now()
	return config, nil
}

// Invalidate clears the cached configuration, forcing a refresh on next access.
func (cc *ConfigCache) Invalidate() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.config = nil
	cc.lastFetched = time.Time{}
}

// GetDefaultPort returns the default port for a given service type from the cached config.
// Falls back to 8080 if the type is not found or config is unavailable.
// **Validates: Requirements 4.1**
func (cc *ConfigCache) GetDefaultPort(ctx context.Context, client *Client, serviceType string) int {
	config, err := cc.Get(ctx, client)
	if err != nil || config == nil || config.DefaultPorts == nil {
		return 8080 // Fallback default
	}

	if port, ok := config.DefaultPorts[serviceType]; ok {
		return port
	}

	// Try "default" key
	if port, ok := config.DefaultPorts["default"]; ok {
		return port
	}

	return 8080
}

// GetDomain returns the platform domain from the cached config.
// Falls back to "localhost" if config is unavailable.
// **Validates: Requirements 4.2**
func (cc *ConfigCache) GetDomain(ctx context.Context, client *Client) string {
	config, err := cc.Get(ctx, client)
	if err != nil || config == nil || config.Domain == "" {
		return "localhost"
	}
	return config.Domain
}

// GetStatusMapping returns the status mapping for a given status from the cached config.
// Returns nil if the status is not found or config is unavailable.
// **Validates: Requirements 4.3**
func (cc *ConfigCache) GetStatusMapping(ctx context.Context, client *Client, status string) *StatusMapping {
	config, err := cc.Get(ctx, client)
	if err != nil || config == nil || config.StatusMappings == nil {
		return nil
	}

	if mapping, ok := config.StatusMappings[status]; ok {
		return &mapping
	}
	return nil
}

// GetSupportedDBTypes returns the list of supported database types from the cached config.
// Returns an empty slice if config is unavailable.
// **Validates: Requirements 4.4**
func (cc *ConfigCache) GetSupportedDBTypes(ctx context.Context, client *Client) []DatabaseTypeDef {
	config, err := cc.Get(ctx, client)
	if err != nil || config == nil {
		return []DatabaseTypeDef{}
	}
	return config.SupportedDBTypes
}

// globalConfigCache is the singleton config cache instance.
var globalConfigCache = NewConfigCache(DefaultConfigCacheTTL)

// GetGlobalConfigCache returns the global config cache instance.
func GetGlobalConfigCache() *ConfigCache {
	return globalConfigCache
}
