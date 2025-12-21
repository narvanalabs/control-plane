// Package cache provides build caching for faster rebuilds.
package cache

import "errors"

// Cache errors.
var (
	// ErrNilBuildJob is returned when a nil build job is provided.
	ErrNilBuildJob = errors.New("build job is nil")

	// ErrEmptyCacheKey is returned when an empty cache key is provided.
	ErrEmptyCacheKey = errors.New("cache key is empty")

	// ErrCacheNotFound is returned when a cache entry is not found.
	ErrCacheNotFound = errors.New("cache entry not found")

	// ErrCacheExpired is returned when a cache entry has expired.
	ErrCacheExpired = errors.New("cache entry has expired")

	// ErrNilBuildResult is returned when a nil build result is provided.
	ErrNilBuildResult = errors.New("build result is nil")

	// ErrEmptyArtifact is returned when an empty artifact is provided.
	ErrEmptyArtifact = errors.New("artifact is empty")

	// ErrEmptyServiceID is returned when an empty service ID is provided.
	ErrEmptyServiceID = errors.New("service ID is empty")
)
