package cache

import (
	"context"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: build-detection-integration, Property 10: Cache hit returns same result**
// For any two builds with the same repository URL and commit SHA, the detection results must be identical (cache consistency).
// **Validates: Requirements 4.3**

// genRepoURL generates valid repository URLs.
func genRepoURL() gopter.Gen {
	return gen.OneConstOf(
		"https://github.com/user/repo.git",
		"git@github.com:user/repo.git",
		"https://gitlab.com/org/project.git",
		"https://bitbucket.org/team/repo.git",
		"https://github.com/narvanalabs/control-plane.git",
	)
}

// genCommitSHA generates valid commit SHAs.
func genCommitSHA() gopter.Gen {
	return gen.OneConstOf(
		"abc123def456789012345678901234567890abcd",
		"def456789012345678901234567890abcdef123",
		"123456789abcdef0123456789abcdef01234567",
		"fedcba9876543210fedcba9876543210fedcba98",
		"0123456789abcdef0123456789abcdef01234567",
	)
}

// genFramework generates valid frameworks.
func genFramework() gopter.Gen {
	return gen.OneConstOf(
		models.FrameworkGeneric,
		models.FrameworkNextJS,
		models.FrameworkExpress,
		models.FrameworkReact,
		models.FrameworkFastify,
		models.FrameworkDjango,
		models.FrameworkFastAPI,
		models.FrameworkFlask,
	)
}

// genDetectionBuildStrategy generates valid build strategies for detection.
func genDetectionBuildStrategy() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStrategyAutoGo,
		models.BuildStrategyAutoRust,
		models.BuildStrategyAutoNode,
		models.BuildStrategyAutoPython,
	)
}

// genDetectionBuildType generates valid build types for detection.
func genDetectionBuildType() gopter.Gen {
	return gen.OneConstOf(
		models.BuildTypePureNix,
		models.BuildTypeOCI,
	)
}

// genVersion generates valid version strings.
func genVersion() gopter.Gen {
	return gen.OneConstOf(
		"1.21",
		"1.22",
		"1.23",
		"18",
		"20",
		"22",
		"3.11",
		"3.12",
	)
}

// genEntryPoints generates valid entry point slices.
func genEntryPoints() gopter.Gen {
	return gen.OneConstOf(
		[]string{"cmd/main.go"},
		[]string{"main.go"},
		[]string{"cmd/server/main.go", "cmd/worker/main.go"},
		[]string{"src/index.ts"},
		[]string{"app.py"},
		[]string{},
	)
}

// genWarnings generates valid warning slices.
func genWarnings() gopter.Gen {
	return gen.OneConstOf(
		[]string{},
		[]string{"CGO detected"},
		[]string{"Multiple entry points found"},
		[]string{"Deprecated dependency detected", "Consider upgrading"},
	)
}

// genSuggestedConfig generates valid suggested config maps.
func genSuggestedConfig() gopter.Gen {
	return gen.OneConstOf(
		map[string]interface{}{"cgo_enabled": true},
		map[string]interface{}{"cgo_enabled": false},
		map[string]interface{}{"cgo_enabled": true, "go_version": "1.22"},
		map[string]interface{}{"node_version": "20"},
		map[string]interface{}{},
	)
}

// genConfidence generates valid confidence values.
func genConfidence() gopter.Gen {
	return gen.Float64Range(0.0, 1.0)
}

// genDetectionResult generates valid detection results.
func genDetectionResult() gopter.Gen {
	return gopter.CombineGens(
		genDetectionBuildStrategy(),
		genFramework(),
		genVersion(),
		genSuggestedConfig(),
		genDetectionBuildType(),
		genEntryPoints(),
		genConfidence(),
		genWarnings(),
	).Map(func(vals []interface{}) *models.DetectionResult {
		return &models.DetectionResult{
			Strategy:             vals[0].(models.BuildStrategy),
			Framework:            vals[1].(models.Framework),
			Version:              vals[2].(string),
			SuggestedConfig:      vals[3].(map[string]interface{}),
			RecommendedBuildType: vals[4].(models.BuildType),
			EntryPoints:          vals[5].([]string),
			Confidence:           vals[6].(float64),
			Warnings:             vals[7].([]string),
		}
	})
}

// TestDetectionCacheConsistency tests that cache hits return the same result.
// **Feature: build-detection-integration, Property 10: Cache hit returns same result**
// **Validates: Requirements 4.3**
func TestDetectionCacheConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("cache hit returns same detection result", prop.ForAll(
		func(repoURL, commitSHA string, detection *models.DetectionResult) bool {
			cache := NewInMemoryDetectionCache()
			ctx := context.Background()

			// Store the detection result
			if err := cache.Set(ctx, repoURL, commitSHA, detection); err != nil {
				return false
			}

			// Retrieve the result multiple times
			result1, found1 := cache.Get(ctx, repoURL, commitSHA)
			result2, found2 := cache.Get(ctx, repoURL, commitSHA)

			if !found1 || !found2 {
				return false
			}

			// Results should be identical
			return detectionsEqual(result1, result2)
		},
		genRepoURL(),
		genCommitSHA(),
		genDetectionResult(),
	))

	properties.TestingRun(t)
}

// TestDetectionCacheDeterminism tests that cache returns consistent results across instances.
func TestDetectionCacheDeterminism(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("same repo URL and commit SHA return same result from same cache", prop.ForAll(
		func(repoURL, commitSHA string, detection *models.DetectionResult) bool {
			cache := NewInMemoryDetectionCache()
			ctx := context.Background()

			// Store the detection result
			if err := cache.Set(ctx, repoURL, commitSHA, detection); err != nil {
				return false
			}

			// Retrieve multiple times and verify consistency
			for i := 0; i < 5; i++ {
				result, found := cache.Get(ctx, repoURL, commitSHA)
				if !found {
					return false
				}
				if !detectionsEqual(result, detection) {
					return false
				}
			}

			return true
		},
		genRepoURL(),
		genCommitSHA(),
		genDetectionResult(),
	))

	properties.TestingRun(t)
}

// TestDetectionCacheIsolation tests that different repo/commit combinations are isolated.
func TestDetectionCacheIsolation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("different commit SHAs return different cached results", prop.ForAll(
		func(repoURL string, commitSHA1, commitSHA2 string, detection1, detection2 *models.DetectionResult) bool {
			if commitSHA1 == commitSHA2 {
				return true // Skip if SHAs are the same
			}

			cache := NewInMemoryDetectionCache()
			ctx := context.Background()

			// Store two different detection results
			if err := cache.Set(ctx, repoURL, commitSHA1, detection1); err != nil {
				return false
			}
			if err := cache.Set(ctx, repoURL, commitSHA2, detection2); err != nil {
				return false
			}

			// Retrieve and verify they're isolated
			result1, found1 := cache.Get(ctx, repoURL, commitSHA1)
			result2, found2 := cache.Get(ctx, repoURL, commitSHA2)

			if !found1 || !found2 {
				return false
			}

			// Each result should match its original
			return detectionsEqual(result1, detection1) && detectionsEqual(result2, detection2)
		},
		genRepoURL(),
		genCommitSHA(),
		genCommitSHA(),
		genDetectionResult(),
		genDetectionResult(),
	))

	properties.TestingRun(t)
}

// TestDetectionCacheExpiration tests that expired entries are not returned.
func TestDetectionCacheExpiration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("expired detection cache entries are not returned", prop.ForAll(
		func(repoURL, commitSHA string, detection *models.DetectionResult) bool {
			// Create cache with very short TTL
			cache := NewInMemoryDetectionCache(WithDetectionTTL(1 * time.Nanosecond))
			ctx := context.Background()

			// Store the detection result
			if err := cache.Set(ctx, repoURL, commitSHA, detection); err != nil {
				return false
			}

			// Wait for expiration
			time.Sleep(10 * time.Millisecond)

			// Should not be found
			_, found := cache.Get(ctx, repoURL, commitSHA)
			return !found
		},
		genRepoURL(),
		genCommitSHA(),
		genDetectionResult(),
	))

	properties.TestingRun(t)
}

// TestDetectionCacheErrorConditions tests error handling.
func TestDetectionCacheErrorConditions(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	cache := NewInMemoryDetectionCache()
	ctx := context.Background()

	properties.Property("Set returns error for empty repo URL", prop.ForAll(
		func(commitSHA string, detection *models.DetectionResult) bool {
			err := cache.Set(ctx, "", commitSHA, detection)
			return err == ErrEmptyRepoURL
		},
		genCommitSHA(),
		genDetectionResult(),
	))

	properties.Property("Set returns error for empty commit SHA", prop.ForAll(
		func(repoURL string, detection *models.DetectionResult) bool {
			err := cache.Set(ctx, repoURL, "", detection)
			return err == ErrEmptyCommitSHA
		},
		genRepoURL(),
		genDetectionResult(),
	))

	properties.Property("Set returns error for nil detection result", prop.ForAll(
		func(repoURL, commitSHA string) bool {
			err := cache.Set(ctx, repoURL, commitSHA, nil)
			return err == ErrNilDetectionResult
		},
		genRepoURL(),
		genCommitSHA(),
	))

	properties.Property("Get returns false for empty repo URL", prop.ForAll(
		func(commitSHA string) bool {
			_, found := cache.Get(ctx, "", commitSHA)
			return !found
		},
		genCommitSHA(),
	))

	properties.Property("Get returns false for empty commit SHA", prop.ForAll(
		func(repoURL string) bool {
			_, found := cache.Get(ctx, repoURL, "")
			return !found
		},
		genRepoURL(),
	))

	properties.Property("Get returns false for non-existent entry", prop.ForAll(
		func(repoURL, commitSHA string) bool {
			newCache := NewInMemoryDetectionCache()
			_, found := newCache.Get(ctx, repoURL, commitSHA)
			return !found
		},
		genRepoURL(),
		genCommitSHA(),
	))

	properties.TestingRun(t)
}

// TestDetectionCacheCleanup tests the cleanup functionality.
func TestDetectionCacheCleanup(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("CleanupExpired removes expired entries", prop.ForAll(
		func(repoURL, commitSHA string, detection *models.DetectionResult) bool {
			// Create cache with very short TTL
			cache := NewInMemoryDetectionCache(WithDetectionTTL(1 * time.Nanosecond))
			ctx := context.Background()

			// Store the detection result
			if err := cache.Set(ctx, repoURL, commitSHA, detection); err != nil {
				return false
			}

			// Wait for expiration
			time.Sleep(10 * time.Millisecond)

			// Cleanup should remove the entry
			removed := cache.CleanupExpired(ctx)
			if removed != 1 {
				return false
			}

			// Entry should be gone
			_, found := cache.Get(ctx, repoURL, commitSHA)
			return !found
		},
		genRepoURL(),
		genCommitSHA(),
		genDetectionResult(),
	))

	properties.TestingRun(t)
}

// TestDetectionCacheClear tests the clear functionality.
func TestDetectionCacheClear(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Clear removes all entries", prop.ForAll(
		func(repoURL, commitSHA string, detection *models.DetectionResult) bool {
			cache := NewInMemoryDetectionCache()
			ctx := context.Background()

			// Store the detection result
			if err := cache.Set(ctx, repoURL, commitSHA, detection); err != nil {
				return false
			}

			// Verify it exists
			if _, found := cache.Get(ctx, repoURL, commitSHA); !found {
				return false
			}

			// Clear the cache
			cache.Clear(ctx)

			// Verify it's gone
			_, found := cache.Get(ctx, repoURL, commitSHA)
			return !found && cache.Size() == 0
		},
		genRepoURL(),
		genCommitSHA(),
		genDetectionResult(),
	))

	properties.TestingRun(t)
}

// TestDetectionCacheSize tests the size tracking.
func TestDetectionCacheSize(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Size returns correct count", prop.ForAll(
		func(count int) bool {
			if count < 0 || count > 10 {
				count = 5
			}

			cache := NewInMemoryDetectionCache()
			ctx := context.Background()

			// Store multiple entries
			for i := 0; i < count; i++ {
				repoURL := "https://github.com/test/repo" + itoa(i) + ".git"
				commitSHA := "abc123def456789012345678901234567890abc" + itoa(i)
				detection := &models.DetectionResult{
					Strategy:   models.BuildStrategyAutoGo,
					Framework:  models.FrameworkGeneric,
					Version:    "1.21",
					Confidence: 0.9,
				}

				if err := cache.Set(ctx, repoURL, commitSHA, detection); err != nil {
					return false
				}
			}

			return cache.Size() == count
		},
		gen.IntRange(0, 10),
	))

	properties.TestingRun(t)
}

// detectionsEqual compares two detection results for equality.
func detectionsEqual(a, b *models.DetectionResult) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if a.Strategy != b.Strategy {
		return false
	}
	if a.Framework != b.Framework {
		return false
	}
	if a.Version != b.Version {
		return false
	}
	if a.RecommendedBuildType != b.RecommendedBuildType {
		return false
	}
	if a.Confidence != b.Confidence {
		return false
	}

	// Compare entry points
	if len(a.EntryPoints) != len(b.EntryPoints) {
		return false
	}
	for i, ep := range a.EntryPoints {
		if ep != b.EntryPoints[i] {
			return false
		}
	}

	// Compare warnings
	if len(a.Warnings) != len(b.Warnings) {
		return false
	}
	for i, w := range a.Warnings {
		if w != b.Warnings[i] {
			return false
		}
	}

	// Compare suggested config
	if len(a.SuggestedConfig) != len(b.SuggestedConfig) {
		return false
	}
	for k, v := range a.SuggestedConfig {
		if bv, ok := b.SuggestedConfig[k]; !ok || v != bv {
			return false
		}
	}

	return true
}
