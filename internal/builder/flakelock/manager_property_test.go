package flakelock

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: flexible-build-strategies, Property 12: Flake Lock Reproducibility**
// For any generated flake.nix, the Build_System SHALL generate a corresponding flake.lock,
// and rebuilds using the same flake.lock SHALL produce identical artifacts.
// **Validates: Requirements 15.1, 15.2**

// genBuildID generates valid build IDs.
func genBuildID() gopter.Gen {
	return gen.OneConstOf(
		"build-001",
		"build-002",
		"deploy-abc123",
		"deploy-def456",
		"job-12345",
		"job-67890",
		"test-build-1",
		"test-build-2",
		"prod-deploy-a",
		"prod-deploy-b",
	)
}

// genNixpkgsRev generates valid nixpkgs revision strings.
func genNixpkgsRev() gopter.Gen {
	return gen.OneConstOf(
		"abc123def456",
		"789xyz012abc",
		"fedcba987654",
		"111222333444",
		"aabbccddee11",
		"55667788990a",
	)
}

// genValidFlakeLock generates valid flake.lock JSON content.
func genValidFlakeLock() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 10),
		genNixpkgsRev(),
	).Map(func(vals []interface{}) string {
		version := vals[0].(int)
		rev := vals[1].(string)
		return generateFlakeLockJSON(version, rev)
	})
}

// generateFlakeLockJSON creates a valid flake.lock JSON string.
func generateFlakeLockJSON(version int, nixpkgsRev string) string {
	if nixpkgsRev == "" {
		nixpkgsRev = "abc123def456"
	}
	return `{
  "nodes": {
    "nixpkgs": {
      "locked": {
        "lastModified": 1700000000,
        "narHash": "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
        "owner": "NixOS",
        "repo": "nixpkgs",
        "rev": "` + nixpkgsRev + `",
        "type": "github"
      },
      "original": {
        "owner": "NixOS",
        "ref": "nixos-unstable",
        "repo": "nixpkgs",
        "type": "github"
      }
    },
    "root": {
      "inputs": {
        "nixpkgs": "nixpkgs"
      }
    }
  },
  "root": "root",
  "version": ` + itoa(version) + `
}`
}

// itoa converts int to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	result := ""
	for i > 0 {
		result = string(rune('0'+i%10)) + result
		i /= 10
	}
	return result
}

// TestFlakeLockStorageReproducibility tests that stored locks can be retrieved identically.
func TestFlakeLockStorageReproducibility(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Store then Retrieve returns identical content", prop.ForAll(
		func(buildID string, lockContent string) bool {
			m := NewManager()
			ctx := context.Background()

			// Store the lock
			err := m.Store(ctx, buildID, lockContent)
			if err != nil {
				return false
			}

			// Retrieve the lock
			retrieved, err := m.Retrieve(ctx, buildID)
			if err != nil {
				return false
			}

			// Content should be identical
			return retrieved == lockContent
		},
		genBuildID(),
		genValidFlakeLock(),
	))

	properties.TestingRun(t)
}

// TestFlakeLockDeterminism tests that the same lock content produces the same hash.
func TestFlakeLockDeterminism(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("same lock content produces same hash", prop.ForAll(
		func(lockContent string) bool {
			hash1 := calculateHash(lockContent)
			hash2 := calculateHash(lockContent)
			return hash1 == hash2
		},
		genValidFlakeLock(),
	))

	properties.TestingRun(t)
}

// TestFlakeLockValidation tests that valid locks are recognized.
func TestFlakeLockValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("isValidFlakeLock returns true for valid locks", prop.ForAll(
		func(lockContent string) bool {
			return isValidFlakeLock(lockContent)
		},
		genValidFlakeLock(),
	))

	properties.Property("isValidFlakeLock returns false for invalid content", prop.ForAll(
		func(invalidContent string) bool {
			return !isValidFlakeLock(invalidContent)
		},
		gen.OneConstOf(
			"",
			"not json",
			"{}",
			`{"version": 0}`,
			`{"nodes": {}}`,
			`{"version": "string", "nodes": {}}`,
		),
	))

	properties.TestingRun(t)
}

// TestShouldRegenerateLogic tests the regeneration decision logic.
func TestShouldRegenerateLogic(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("ShouldRegenerate returns true for non-existent build ID", prop.ForAll(
		func(buildID string) bool {
			m := NewManager()
			ctx := context.Background()
			return m.ShouldRegenerate(ctx, buildID, "somehash")
		},
		genBuildID(),
	))

	properties.Property("ShouldRegenerate returns false when lock exists with same hash", prop.ForAll(
		func(buildID string, lockContent string) bool {
			m := NewManager()
			ctx := context.Background()

			// Store a lock
			if err := m.Store(ctx, buildID, lockContent); err != nil {
				return false
			}

			// Get the stored lock to find its hash
			stored, err := m.GetStoredLock(ctx, buildID)
			if err != nil {
				return false
			}

			// Should not regenerate when hash matches
			return !m.ShouldRegenerate(ctx, buildID, stored.SourceHash)
		},
		genBuildID(),
		genValidFlakeLock(),
	))

	properties.Property("ShouldRegenerate returns true when source hash differs", prop.ForAll(
		func(buildID string, lockContent string) bool {
			m := NewManager()
			ctx := context.Background()

			// Store a lock
			if err := m.Store(ctx, buildID, lockContent); err != nil {
				return false
			}

			// Should regenerate when hash differs
			return m.ShouldRegenerate(ctx, buildID, "different-hash-value")
		},
		genBuildID(),
		genValidFlakeLock(),
	))

	properties.TestingRun(t)
}

// TestFlakeLockParsing tests the ParseFlakeLock function.
func TestFlakeLockParsing(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("ParseFlakeLock succeeds for valid locks", prop.ForAll(
		func(lockContent string) bool {
			lock, err := ParseFlakeLock(lockContent)
			if err != nil {
				return false
			}
			return lock.Version > 0 && lock.Nodes != nil
		},
		genValidFlakeLock(),
	))

	properties.Property("ParseFlakeLock fails for invalid content", prop.ForAll(
		func(invalidContent string) bool {
			_, err := ParseFlakeLock(invalidContent)
			return err != nil
		},
		gen.OneConstOf(
			"",
			"not json",
			"{}",
			`{"version": 0}`,
		),
	))

	properties.TestingRun(t)
}

// TestCompareFlakeLocks tests the comparison function.
func TestCompareFlakeLocks(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("CompareFlakeLocks returns true for identical locks", prop.ForAll(
		func(lockContent string) bool {
			return CompareFlakeLocks(lockContent, lockContent)
		},
		genValidFlakeLock(),
	))

	properties.Property("CompareFlakeLocks returns false for different versions", prop.ForAll(
		func(rev string) bool {
			lock1 := generateFlakeLockJSON(1, rev)
			lock2 := generateFlakeLockJSON(2, rev)
			return !CompareFlakeLocks(lock1, lock2)
		},
		genNixpkgsRev(),
	))

	properties.TestingRun(t)
}

// TestStoredLockMetadata tests that metadata is correctly maintained.
func TestStoredLockMetadata(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("GetStoredLock returns correct metadata", prop.ForAll(
		func(buildID string, lockContent string) bool {
			m := NewManager()
			ctx := context.Background()

			beforeStore := time.Now()

			// Store the lock
			if err := m.Store(ctx, buildID, lockContent); err != nil {
				return false
			}

			afterStore := time.Now()

			// Get the stored lock
			stored, err := m.GetStoredLock(ctx, buildID)
			if err != nil {
				return false
			}

			// Verify metadata
			if stored.Content != lockContent {
				return false
			}
			if stored.SourceHash == "" {
				return false
			}
			if stored.CreatedAt.Before(beforeStore) || stored.CreatedAt.After(afterStore) {
				return false
			}
			if stored.UpdatedAt.Before(beforeStore) || stored.UpdatedAt.After(afterStore) {
				return false
			}

			return true
		},
		genBuildID(),
		genValidFlakeLock(),
	))

	properties.TestingRun(t)
}

// TestDeleteOperation tests the Delete function.
func TestDeleteOperation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Delete removes stored lock", prop.ForAll(
		func(buildID string, lockContent string) bool {
			m := NewManager()
			ctx := context.Background()

			// Store the lock
			if err := m.Store(ctx, buildID, lockContent); err != nil {
				return false
			}

			// Verify it exists
			if _, err := m.Retrieve(ctx, buildID); err != nil {
				return false
			}

			// Delete it
			if err := m.Delete(ctx, buildID); err != nil {
				return false
			}

			// Verify it's gone
			_, err := m.Retrieve(ctx, buildID)
			return err == ErrLockNotFound
		},
		genBuildID(),
		genValidFlakeLock(),
	))

	properties.Property("Delete returns error for non-existent build ID", prop.ForAll(
		func(buildID string) bool {
			m := NewManager()
			ctx := context.Background()
			err := m.Delete(ctx, buildID)
			return err == ErrLockNotFound
		},
		genBuildID(),
	))

	properties.TestingRun(t)
}

// TestListBuildIDs tests the ListBuildIDs function.
func TestListBuildIDs(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// genUniqueBuildIDs generates a list of unique build IDs.
	genUniqueBuildIDs := func() gopter.Gen {
		return gen.SliceOfN(5, gen.IntRange(0, 9)).Map(func(indices []int) []string {
			allIDs := []string{
				"build-001", "build-002", "deploy-abc123", "deploy-def456",
				"job-12345", "job-67890", "test-build-1", "test-build-2",
				"prod-deploy-a", "prod-deploy-b",
			}
			seen := make(map[string]bool)
			result := make([]string, 0)
			for _, idx := range indices {
				id := allIDs[idx]
				if !seen[id] {
					seen[id] = true
					result = append(result, id)
				}
			}
			return result
		})
	}

	properties.Property("ListBuildIDs returns all stored build IDs", prop.ForAll(
		func(buildIDs []string) bool {
			m := NewManager()
			ctx := context.Background()

			// Store locks for each ID
			lockContent := generateFlakeLockJSON(7, "testrev")
			for _, id := range buildIDs {
				if err := m.Store(ctx, id, lockContent); err != nil {
					return false
				}
			}

			// List all IDs
			listedIDs := m.ListBuildIDs(ctx)

			// Verify count matches
			if len(listedIDs) != len(buildIDs) {
				return false
			}

			// Verify all IDs are present
			listedSet := make(map[string]bool)
			for _, id := range listedIDs {
				listedSet[id] = true
			}

			for _, id := range buildIDs {
				if !listedSet[id] {
					return false
				}
			}

			return true
		},
		genUniqueBuildIDs(),
	))

	properties.TestingRun(t)
}

// TestGetInputRevision tests extracting input revisions from lock files.
func TestGetInputRevision(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("GetInputRevision extracts nixpkgs revision", prop.ForAll(
		func(rev string) bool {
			lockContent := generateFlakeLockJSON(7, rev)
			extractedRev, err := GetInputRevision(lockContent, "nixpkgs")
			if err != nil {
				return false
			}
			return extractedRev == rev
		},
		genNixpkgsRev(),
	))

	properties.Property("GetInputRevision returns error for non-existent input", prop.ForAll(
		func(lockContent string) bool {
			_, err := GetInputRevision(lockContent, "nonexistent-input")
			return err != nil
		},
		genValidFlakeLock(),
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

	properties.Property("Store returns error for empty build ID", prop.ForAll(
		func(lockContent string) bool {
			err := m.Store(ctx, "", lockContent)
			return err == ErrEmptyBuildID
		},
		genValidFlakeLock(),
	))

	properties.Property("Store returns error for empty lock content", prop.ForAll(
		func(buildID string) bool {
			err := m.Store(ctx, buildID, "")
			return err == ErrEmptyLockContent
		},
		genBuildID(),
	))

	properties.Property("Store returns error for invalid lock content", prop.ForAll(
		func(buildID string) bool {
			err := m.Store(ctx, buildID, "invalid json")
			return err == ErrInvalidLockFormat
		},
		genBuildID(),
	))

	properties.Property("Retrieve returns error for empty build ID", prop.ForAll(
		func(_ int) bool {
			_, err := m.Retrieve(ctx, "")
			return err == ErrEmptyBuildID
		},
		gen.Int(),
	))

	properties.Property("Retrieve returns error for non-existent build ID", prop.ForAll(
		func(buildID string) bool {
			newM := NewManager() // Use fresh manager to ensure ID doesn't exist
			_, err := newM.Retrieve(ctx, buildID)
			return err == ErrLockNotFound
		},
		genBuildID(),
	))

	properties.TestingRun(t)
}

// TestManagerOptions tests the functional options.
func TestManagerOptions(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("WithTimeout sets correct timeout", prop.ForAll(
		func(seconds int) bool {
			if seconds < 0 {
				seconds = 0
			}
			timeout := time.Duration(seconds) * time.Second
			m := NewManagerWithOptions(WithTimeout(timeout))
			return m.Timeout == timeout
		},
		gen.IntRange(0, 600),
	))

	properties.TestingRun(t)
}

// Ensure reflect is used (for gopter.CombineGens)
var _ = reflect.TypeOf(nil)
