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

// **Feature: flexible-build-strategies, Property 16: Cache Key Consistency**
// For any BuildJob with identical source and dependencies, the BuildCacheManager
// SHALL generate the same cache key.
// **Validates: Requirements 16.1, 16.2, 16.3**

// genBuildStrategy generates valid build strategies.
func genBuildStrategy() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStrategyFlake,
		models.BuildStrategyAutoGo,
		models.BuildStrategyAutoRust,
		models.BuildStrategyAutoNode,
		models.BuildStrategyAutoPython,
		models.BuildStrategyDockerfile,
		models.BuildStrategyNixpacks,
		models.BuildStrategyAuto,
	)
}

// genBuildType generates valid build types.
func genBuildType() gopter.Gen {
	return gen.OneConstOf(
		models.BuildTypePureNix,
		models.BuildTypeOCI,
	)
}

// genGitRef generates valid git refs.
func genGitRef() gopter.Gen {
	return gen.OneConstOf(
		"abc123def456789",
		"main",
		"v1.0.0",
		"feature/test",
		"develop",
		"release-1.2.3",
		"hotfix/bug-123",
		"refs/heads/main",
	)
}

// genGitURL generates valid git URLs.
func genGitURL() gopter.Gen {
	return gen.OneConstOf(
		"https://github.com/user/repo.git",
		"git@github.com:user/repo.git",
		"https://gitlab.com/org/project.git",
		"https://bitbucket.org/team/repo.git",
	)
}

// genVendorHash generates valid vendor hashes.
func genVendorHash() gopter.Gen {
	return gen.OneConstOf(
		"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"sha256-BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=",
		"sha256-CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=",
		"sha256-DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD=",
		"",
	)
}

// genBuildConfig generates valid build configurations.
func genBuildConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("1.21", "1.22", "1.23", ""),
		gen.OneConstOf("18", "20", "22", ""),
		gen.OneConstOf("3.11", "3.12", ""),
		gen.Bool(),
	).Map(func(vals []interface{}) *models.BuildConfig {
		return &models.BuildConfig{
			GoVersion:     vals[0].(string),
			NodeVersion:   vals[1].(string),
			PythonVersion: vals[2].(string),
			CGOEnabled:    vals[3].(bool),
		}
	})
}

// genBuildJob generates valid build jobs.
func genBuildJob() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),
		gen.Identifier(),
		gen.Identifier(),
		genGitURL(),
		genGitRef(),
		genBuildStrategy(),
		genBuildType(),
		genVendorHash(),
		genBuildConfig(),
	).Map(func(vals []interface{}) *models.BuildJob {
		return &models.BuildJob{
			ID:            vals[0].(string),
			DeploymentID:  vals[1].(string),
			AppID:         vals[2].(string),
			GitURL:        vals[3].(string),
			GitRef:        vals[4].(string),
			BuildStrategy: vals[5].(models.BuildStrategy),
			BuildType:     vals[6].(models.BuildType),
			VendorHash:    vals[7].(string),
			BuildConfig:   vals[8].(*models.BuildConfig),
		}
	})
}

// genServiceID generates valid service IDs.
func genServiceID() gopter.Gen {
	return gen.OneConstOf(
		"service-001",
		"service-002",
		"app-abc/api",
		"app-def/worker",
		"my-service",
		"prod-api",
	)
}

// genArtifact generates valid artifact strings.
func genArtifact() gopter.Gen {
	return gen.OneConstOf(
		"/nix/store/abc123-myapp",
		"/nix/store/def456-myapp",
		"registry.example.com/myapp:v1.0.0",
		"registry.example.com/myapp:latest",
		"ghcr.io/user/repo:sha-abc123",
	)
}

// TestCacheKeyConsistency tests that identical build jobs produce identical cache keys.
func TestCacheKeyConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("identical build jobs produce identical cache keys", prop.ForAll(
		func(job *models.BuildJob) bool {
			m := NewManager()
			ctx := context.Background()

			// Generate cache key twice for the same job
			key1, err1 := m.GetCacheKey(ctx, job)
			key2, err2 := m.GetCacheKey(ctx, job)

			if err1 != nil || err2 != nil {
				return false
			}

			// Keys should be identical
			return key1 == key2
		},
		genBuildJob(),
	))

	properties.TestingRun(t)
}

// TestCacheKeyDeterminism tests that cache key generation is deterministic.
func TestCacheKeyDeterminism(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("cache key generation is deterministic across manager instances", prop.ForAll(
		func(job *models.BuildJob) bool {
			ctx := context.Background()

			// Create two separate manager instances
			m1 := NewManager()
			m2 := NewManager()

			// Generate cache keys from both managers
			key1, err1 := m1.GetCacheKey(ctx, job)
			key2, err2 := m2.GetCacheKey(ctx, job)

			if err1 != nil || err2 != nil {
				return false
			}

			// Keys should be identical
			return key1 == key2
		},
		genBuildJob(),
	))

	properties.TestingRun(t)
}

// TestCacheKeyDifferentiation tests that different build jobs produce different cache keys.
func TestCacheKeyDifferentiation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("different git refs produce different cache keys", prop.ForAll(
		func(job *models.BuildJob, ref1, ref2 string) bool {
			if ref1 == ref2 {
				return true // Skip if refs are the same
			}

			m := NewManager()
			ctx := context.Background()

			// Create two jobs with different git refs
			job1 := *job
			job1.GitRef = ref1

			job2 := *job
			job2.GitRef = ref2

			key1, err1 := m.GetCacheKey(ctx, &job1)
			key2, err2 := m.GetCacheKey(ctx, &job2)

			if err1 != nil || err2 != nil {
				return false
			}

			// Keys should be different
			return key1 != key2
		},
		genBuildJob(),
		genGitRef(),
		genGitRef(),
	))

	properties.Property("different vendor hashes produce different cache keys", prop.ForAll(
		func(job *models.BuildJob) bool {
			m := NewManager()
			ctx := context.Background()

			// Create two jobs with different vendor hashes
			job1 := *job
			job1.VendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

			job2 := *job
			job2.VendorHash = "sha256-BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="

			key1, err1 := m.GetCacheKey(ctx, &job1)
			key2, err2 := m.GetCacheKey(ctx, &job2)

			if err1 != nil || err2 != nil {
				return false
			}

			// Keys should be different
			return key1 != key2
		},
		genBuildJob(),
	))

	properties.Property("different build strategies produce different cache keys", prop.ForAll(
		func(job *models.BuildJob) bool {
			m := NewManager()
			ctx := context.Background()

			// Create two jobs with different strategies
			job1 := *job
			job1.BuildStrategy = models.BuildStrategyAutoGo

			job2 := *job
			job2.BuildStrategy = models.BuildStrategyAutoNode

			key1, err1 := m.GetCacheKey(ctx, &job1)
			key2, err2 := m.GetCacheKey(ctx, &job2)

			if err1 != nil || err2 != nil {
				return false
			}

			// Keys should be different
			return key1 != key2
		},
		genBuildJob(),
	))

	properties.TestingRun(t)
}

// TestCacheStoreRetrieve tests that stored cache entries can be retrieved.
func TestCacheStoreRetrieve(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("stored cache entries can be retrieved", prop.ForAll(
		func(job *models.BuildJob, artifact string) bool {
			m := NewManager()
			ctx := context.Background()

			// Generate cache key
			cacheKey, err := m.GetCacheKey(ctx, job)
			if err != nil {
				return false
			}

			// Create build result
			result := &models.BuildResult{
				Artifact:  artifact,
				StorePath: artifact,
			}

			// Store the cache entry
			if err := m.StoreCache(ctx, cacheKey, result); err != nil {
				return false
			}

			// Retrieve the cache entry
			cached, err := m.CheckCache(ctx, cacheKey)
			if err != nil {
				return false
			}

			// Verify the artifact matches
			return cached.Artifact == artifact
		},
		genBuildJob(),
		genArtifact(),
	))

	properties.TestingRun(t)
}

// TestCacheInvalidation tests that cache invalidation works correctly.
func TestCacheInvalidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("invalidated cache entries are not found", prop.ForAll(
		func(serviceID string, artifact string) bool {
			m := NewManager()
			ctx := context.Background()

			// Create a job with the service ID
			job := &models.BuildJob{
				ID:            "test-job",
				DeploymentID:  "test-deploy",
				AppID:         "test-app",
				ServiceName:   serviceID,
				GitURL:        "https://github.com/test/repo.git",
				GitRef:        "main",
				BuildStrategy: models.BuildStrategyAutoGo,
				BuildType:     models.BuildTypePureNix,
			}

			// Generate cache key
			cacheKey, err := m.GetCacheKey(ctx, job)
			if err != nil {
				return false
			}

			// Create build result
			result := &models.BuildResult{
				Artifact:  artifact,
				StorePath: artifact,
			}

			// Store with metadata (which updates service index)
			if err := m.StoreCacheWithMetadata(ctx, cacheKey, result, job); err != nil {
				return false
			}

			// Verify it exists
			if _, err := m.CheckCache(ctx, cacheKey); err != nil {
				return false
			}

			// Invalidate the cache for the service
			serviceKey := m.getServiceKey(job)
			if err := m.InvalidateCache(ctx, serviceKey); err != nil {
				return false
			}

			// Verify it's gone
			_, err = m.CheckCache(ctx, cacheKey)
			return err == ErrCacheNotFound
		},
		genServiceID(),
		genArtifact(),
	))

	properties.TestingRun(t)
}

// TestCacheExpiration tests that expired cache entries are not returned.
func TestCacheExpiration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("expired cache entries return ErrCacheExpired", prop.ForAll(
		func(job *models.BuildJob, artifact string) bool {
			// Create manager with very short TTL
			m := NewManagerWithOptions(WithTTL(1 * time.Nanosecond))
			ctx := context.Background()

			// Generate cache key
			cacheKey, err := m.GetCacheKey(ctx, job)
			if err != nil {
				return false
			}

			// Create build result
			result := &models.BuildResult{
				Artifact:  artifact,
				StorePath: artifact,
			}

			// Store the cache entry
			if err := m.StoreCache(ctx, cacheKey, result); err != nil {
				return false
			}

			// Wait for expiration
			time.Sleep(10 * time.Millisecond)

			// Try to retrieve - should be expired
			_, err = m.CheckCache(ctx, cacheKey)
			return err == ErrCacheExpired
		},
		genBuildJob(),
		genArtifact(),
	))

	properties.TestingRun(t)
}

// TestCacheHitDetection tests the IsCacheHit helper function.
func TestCacheHitDetection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("IsCacheHit returns true for existing entries", prop.ForAll(
		func(job *models.BuildJob, artifact string) bool {
			m := NewManager()
			ctx := context.Background()

			// Generate cache key
			cacheKey, err := m.GetCacheKey(ctx, job)
			if err != nil {
				return false
			}

			// Initially should be a miss
			if m.IsCacheHit(ctx, cacheKey) {
				return false
			}

			// Store the cache entry
			result := &models.BuildResult{
				Artifact:  artifact,
				StorePath: artifact,
			}
			if err := m.StoreCache(ctx, cacheKey, result); err != nil {
				return false
			}

			// Now should be a hit
			return m.IsCacheHit(ctx, cacheKey)
		},
		genBuildJob(),
		genArtifact(),
	))

	properties.TestingRun(t)
}

// TestErrorConditions tests various error conditions.
func TestErrorConditions(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	m := NewManager()
	ctx := context.Background()

	properties.Property("GetCacheKey returns error for nil job", prop.ForAll(
		func(_ int) bool {
			_, err := m.GetCacheKey(ctx, nil)
			return err == ErrNilBuildJob
		},
		gen.Int(),
	))

	properties.Property("CheckCache returns error for empty cache key", prop.ForAll(
		func(_ int) bool {
			_, err := m.CheckCache(ctx, "")
			return err == ErrEmptyCacheKey
		},
		gen.Int(),
	))

	properties.Property("CheckCache returns error for non-existent key", prop.ForAll(
		func(key string) bool {
			if key == "" {
				return true // Skip empty keys
			}
			newM := NewManager()
			_, err := newM.CheckCache(ctx, key)
			return err == ErrCacheNotFound
		},
		gen.Identifier(),
	))

	properties.Property("StoreCache returns error for empty cache key", prop.ForAll(
		func(artifact string) bool {
			result := &models.BuildResult{Artifact: artifact, StorePath: artifact}
			err := m.StoreCache(ctx, "", result)
			return err == ErrEmptyCacheKey
		},
		genArtifact(),
	))

	properties.Property("StoreCache returns error for nil result", prop.ForAll(
		func(key string) bool {
			if key == "" {
				return true
			}
			err := m.StoreCache(ctx, key, nil)
			return err == ErrNilBuildResult
		},
		gen.Identifier(),
	))

	properties.Property("StoreCache returns error for empty artifact", prop.ForAll(
		func(key string) bool {
			if key == "" {
				return true
			}
			result := &models.BuildResult{Artifact: ""}
			err := m.StoreCache(ctx, key, result)
			return err == ErrEmptyArtifact
		},
		gen.Identifier(),
	))

	properties.Property("InvalidateCache returns error for empty service ID", prop.ForAll(
		func(_ int) bool {
			err := m.InvalidateCache(ctx, "")
			return err == ErrEmptyServiceID
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}

// TestCacheStats tests the GetCacheStats function.
func TestCacheStats(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("GetCacheStats returns correct entry count", prop.ForAll(
		func(count int) bool {
			if count < 0 || count > 10 {
				count = 5
			}

			m := NewManager()
			ctx := context.Background()

			// Store multiple entries
			for i := 0; i < count; i++ {
				job := &models.BuildJob{
					ID:            "job-" + itoa(i),
					DeploymentID:  "deploy-" + itoa(i),
					GitURL:        "https://github.com/test/repo.git",
					GitRef:        "ref-" + itoa(i),
					BuildStrategy: models.BuildStrategyAutoGo,
					BuildType:     models.BuildTypePureNix,
				}

				cacheKey, err := m.GetCacheKey(ctx, job)
				if err != nil {
					return false
				}

				result := &models.BuildResult{
					Artifact:  "/nix/store/test-" + itoa(i),
					StorePath: "/nix/store/test-" + itoa(i),
				}

				if err := m.StoreCache(ctx, cacheKey, result); err != nil {
					return false
				}
			}

			stats := m.GetCacheStats(ctx)
			return stats.TotalEntries == count
		},
		gen.IntRange(0, 10),
	))

	properties.TestingRun(t)
}

// TestCleanupExpired tests the CleanupExpired function.
func TestCleanupExpired(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("CleanupExpired removes expired entries", prop.ForAll(
		func(job *models.BuildJob, artifact string) bool {
			// Create manager with very short TTL
			m := NewManagerWithOptions(WithTTL(1 * time.Nanosecond))
			ctx := context.Background()

			// Generate cache key
			cacheKey, err := m.GetCacheKey(ctx, job)
			if err != nil {
				return false
			}

			// Store the cache entry
			result := &models.BuildResult{
				Artifact:  artifact,
				StorePath: artifact,
			}
			if err := m.StoreCache(ctx, cacheKey, result); err != nil {
				return false
			}

			// Wait for expiration
			time.Sleep(10 * time.Millisecond)

			// Cleanup should remove the entry
			removed := m.CleanupExpired(ctx)
			if removed != 1 {
				return false
			}

			// Entry should be gone
			_, err = m.CheckCache(ctx, cacheKey)
			return err == ErrCacheNotFound
		},
		genBuildJob(),
		genArtifact(),
	))

	properties.TestingRun(t)
}

// TestListCacheKeys tests the ListCacheKeys function.
func TestListCacheKeys(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("ListCacheKeys returns all stored keys", prop.ForAll(
		func(count int) bool {
			if count < 0 || count > 10 {
				count = 5
			}

			m := NewManager()
			ctx := context.Background()

			storedKeys := make(map[string]bool)

			// Store multiple entries
			for i := 0; i < count; i++ {
				job := &models.BuildJob{
					ID:            "job-" + itoa(i),
					DeploymentID:  "deploy-" + itoa(i),
					GitURL:        "https://github.com/test/repo.git",
					GitRef:        "ref-" + itoa(i),
					BuildStrategy: models.BuildStrategyAutoGo,
					BuildType:     models.BuildTypePureNix,
				}

				cacheKey, err := m.GetCacheKey(ctx, job)
				if err != nil {
					return false
				}

				result := &models.BuildResult{
					Artifact:  "/nix/store/test-" + itoa(i),
					StorePath: "/nix/store/test-" + itoa(i),
				}

				if err := m.StoreCache(ctx, cacheKey, result); err != nil {
					return false
				}

				storedKeys[cacheKey] = true
			}

			// List all keys
			listedKeys := m.ListCacheKeys(ctx)

			// Verify count matches
			if len(listedKeys) != count {
				return false
			}

			// Verify all keys are present
			for _, key := range listedKeys {
				if !storedKeys[key] {
					return false
				}
			}

			return true
		},
		gen.IntRange(0, 10),
	))

	properties.TestingRun(t)
}

// TestManagerOptions tests the functional options.
func TestManagerOptions(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("WithTTL sets correct TTL", prop.ForAll(
		func(hours int) bool {
			if hours < 0 {
				hours = 0
			}
			if hours > 168 { // Max 1 week
				hours = 168
			}
			ttl := time.Duration(hours) * time.Hour
			m := NewManagerWithOptions(WithTTL(ttl))
			return m.ttl == ttl
		},
		gen.IntRange(0, 168),
	))

	properties.TestingRun(t)
}

// itoa converts int to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	negative := false
	if i < 0 {
		negative = true
		i = -i
	}
	result := ""
	for i > 0 {
		result = string(rune('0'+i%10)) + result
		i /= 10
	}
	if negative {
		result = "-" + result
	}
	return result
}
