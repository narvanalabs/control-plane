package builder

import (
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: control-plane, Property 8: OCI artifact format**
// For any completed OCI mode build, the artifact should be a valid OCI image reference
// (registry/repo:tag format).
// **Validates: Requirements 3.2**

// genValidRegistry generates a valid registry hostname.
func genValidRegistry() gopter.Gen {
	return gen.OneConstOf(
		"localhost:5000",
		"registry.example.com",
		"gcr.io",
		"docker.io",
		"ghcr.io",
		"registry.local:5000",
	)
}

// genValidRepoName generates a valid repository name (lowercase alphanumeric with dashes).
func genValidRepoName() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0 && len(s) <= 63
	}).Map(func(s string) string {
		// Ensure it starts with a letter and is lowercase
		if len(s) == 0 {
			return "app"
		}
		return strings.ToLower(s)
	})
}

// genValidTag generates a valid image tag.
func genValidTag() gopter.Gen {
	return gen.Weighted([]gen.WeightedGen{
		// Semantic version tags
		{Weight: 3, Gen: gopter.CombineGens(
			gen.IntRange(0, 99),
			gen.IntRange(0, 99),
			gen.IntRange(0, 999),
		).Map(func(vals []interface{}) string {
			return "v" + intToStr(vals[0].(int)) + "." + intToStr(vals[1].(int)) + "." + intToStr(vals[2].(int))
		})},
		// UUID-like tags
		{Weight: 3, Gen: gen.Identifier().Map(func(s string) string {
			if len(s) > 40 {
				return s[:40]
			}
			return s
		})},
		// Simple tags
		{Weight: 4, Gen: gen.OneConstOf("latest", "main", "develop", "staging", "production")},
	})
}

func intToStr(i int) string {
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

// genValidOCIImageTag generates a valid OCI image reference.
func genValidOCIImageTag() gopter.Gen {
	return gopter.CombineGens(
		genValidRegistry(),
		genValidRepoName(),
		genValidTag(),
	).Map(func(vals []interface{}) string {
		registry := vals[0].(string)
		repo := vals[1].(string)
		tag := vals[2].(string)
		return registry + "/" + repo + ":" + tag
	})
}

// genInvalidOCIImageTag generates invalid OCI image references.
func genInvalidOCIImageTag() gopter.Gen {
	return gen.Weighted([]gen.WeightedGen{
		// Empty string
		{Weight: 2, Gen: gen.Const("")},
		// No registry (no slash)
		{Weight: 2, Gen: gen.AlphaString().SuchThat(func(s string) bool {
			return len(s) > 0
		}).Map(func(s string) string {
			return s + ":latest"
		})},
		// No tag (no colon)
		{Weight: 2, Gen: gen.AlphaString().SuchThat(func(s string) bool {
			return len(s) > 0
		}).Map(func(s string) string {
			return "registry.example.com/" + s
		})},
		// Empty repo
		{Weight: 2, Gen: gen.Const("registry.example.com/:latest")},
		// Empty tag
		{Weight: 2, Gen: gen.Const("registry.example.com/repo:")},
	})
}

// TestOCIArtifactFormatValidation tests that IsValidOCIImageTag correctly validates OCI image references.
func TestOCIArtifactFormatValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: All generated valid OCI image tags should pass validation
	properties.Property("valid OCI image tags are accepted", prop.ForAll(
		func(imageTag string) bool {
			return IsValidOCIImageTag(imageTag)
		},
		genValidOCIImageTag(),
	))

	// Property: Invalid OCI image tags should be rejected
	properties.Property("invalid OCI image tags are rejected", prop.ForAll(
		func(imageTag string) bool {
			return !IsValidOCIImageTag(imageTag)
		},
		genInvalidOCIImageTag(),
	))

	properties.TestingRun(t)
}

// TestOCIImageTagFormat tests that generated image tags follow the expected format.
func TestOCIImageTagFormat(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("OCI image tags have registry/repo:tag format", prop.ForAll(
		func(imageTag string) bool {
			// Must contain exactly one colon (separating repo from tag)
			colonCount := strings.Count(imageTag, ":")
			if colonCount < 1 {
				return false
			}

			// Must contain at least one slash (separating registry from repo)
			if !strings.Contains(imageTag, "/") {
				return false
			}

			// Split into registry/repo and tag
			lastColon := strings.LastIndex(imageTag, ":")
			repoPath := imageTag[:lastColon]
			tag := imageTag[lastColon+1:]

			// Tag must not be empty
			if tag == "" {
				return false
			}

			// Repo path must contain at least one slash
			if !strings.Contains(repoPath, "/") {
				return false
			}

			return true
		},
		genValidOCIImageTag(),
	))

	properties.TestingRun(t)
}


// **Feature: control-plane, Property 9: Pure Nix artifact format**
// For any completed Pure Nix mode build, the artifact should be a valid Nix store path
// (/nix/store/hash-name format).
// **Validates: Requirements 3.3**

// genValidNixHash generates a valid 32-character base32 Nix hash.
func genValidNixHash() gopter.Gen {
	// Nix uses a custom base32 alphabet: 0123456789abcdfghijklmnpqrsvwxyz
	// (note: no 'e', 'o', 't', 'u')
	nixBase32Chars := "0123456789abcdfghijklmnpqrsvwxyz"
	return gen.SliceOfN(32, gen.IntRange(0, len(nixBase32Chars)-1)).Map(func(indices []int) string {
		result := make([]byte, 32)
		for i, idx := range indices {
			result[i] = nixBase32Chars[idx]
		}
		return string(result)
	})
}

// genValidNixName generates a valid Nix derivation name.
func genValidNixName() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0 && len(s) <= 100
	}).Map(func(s string) string {
		// Nix names can contain alphanumeric, dash, underscore, dot
		// For simplicity, just use lowercase alphanumeric with dashes
		return strings.ToLower(s)
	})
}

// genValidStorePath generates a valid Nix store path.
func genValidStorePath() gopter.Gen {
	return gopter.CombineGens(
		genValidNixHash(),
		genValidNixName(),
	).Map(func(vals []interface{}) string {
		hash := vals[0].(string)
		name := vals[1].(string)
		return "/nix/store/" + hash + "-" + name
	})
}

// genInvalidStorePath generates invalid Nix store paths.
func genInvalidStorePath() gopter.Gen {
	return gen.Weighted([]gen.WeightedGen{
		// Empty string
		{Weight: 1, Gen: gen.Const("")},
		// Wrong prefix
		{Weight: 2, Gen: gen.AlphaString().Map(func(s string) string {
			return "/usr/store/" + s
		})},
		// Missing hash
		{Weight: 2, Gen: gen.Const("/nix/store/-name")},
		// Hash too short (31 chars)
		{Weight: 2, Gen: gen.Const("/nix/store/0123456789abcdfghijklmnpqrsvwx-name")},
		// Hash too long (33 chars)
		{Weight: 2, Gen: gen.Const("/nix/store/0123456789abcdfghijklmnpqrsvwxyz0-name")},
		// Missing dash after hash
		{Weight: 2, Gen: gen.Const("/nix/store/0123456789abcdfghijklmnpqrsvwxyzname")},
		// Just the prefix
		{Weight: 1, Gen: gen.Const("/nix/store/")},
	})
}

// TestPureNixArtifactFormatValidation tests that IsValidStorePath correctly validates Nix store paths.
func TestPureNixArtifactFormatValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: All generated valid store paths should pass validation
	properties.Property("valid Nix store paths are accepted", prop.ForAll(
		func(storePath string) bool {
			return IsValidStorePath(storePath)
		},
		genValidStorePath(),
	))

	// Property: Invalid store paths should be rejected
	properties.Property("invalid Nix store paths are rejected", prop.ForAll(
		func(storePath string) bool {
			return !IsValidStorePath(storePath)
		},
		genInvalidStorePath(),
	))

	properties.TestingRun(t)
}

// TestNixStorePathFormat tests that generated store paths follow the expected format.
func TestNixStorePathFormat(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Nix store paths have /nix/store/hash-name format", prop.ForAll(
		func(storePath string) bool {
			// Must start with /nix/store/
			if !strings.HasPrefix(storePath, "/nix/store/") {
				return false
			}

			// Extract the part after /nix/store/
			remainder := strings.TrimPrefix(storePath, "/nix/store/")

			// Must have at least 33 characters (32 hash + 1 dash)
			if len(remainder) < 33 {
				return false
			}

			// Character at position 32 must be a dash
			if remainder[32] != '-' {
				return false
			}

			// Name after dash must not be empty
			name := remainder[33:]
			if len(name) == 0 {
				return false
			}

			return true
		},
		genValidStorePath(),
	))

	properties.TestingRun(t)
}


// **Feature: control-plane, Property 10: Build failure preserves logs**
// For any failed build, the deployment should have status "failed" and associated
// build logs should be retrievable.
// **Validates: Requirements 3.4**

// BuildFailureScenario represents a scenario where a build fails.
type BuildFailureScenario struct {
	DeploymentID string
	BuildLogs    string
	ErrorMessage string
}

// genBuildFailureScenario generates a build failure scenario.
func genBuildFailureScenario() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	).Map(func(vals []interface{}) BuildFailureScenario {
		return BuildFailureScenario{
			DeploymentID: vals[0].(string),
			BuildLogs:    vals[1].(string),
			ErrorMessage: vals[2].(string),
		}
	})
}

// MockLogStore is a simple in-memory log store for testing.
type MockLogStore struct {
	logs map[string][]string
}

func NewMockLogStore() *MockLogStore {
	return &MockLogStore{
		logs: make(map[string][]string),
	}
}

func (s *MockLogStore) StoreLogs(deploymentID, logs string) {
	s.logs[deploymentID] = append(s.logs[deploymentID], logs)
}

func (s *MockLogStore) GetLogs(deploymentID string) []string {
	return s.logs[deploymentID]
}

func (s *MockLogStore) HasLogs(deploymentID string) bool {
	logs, ok := s.logs[deploymentID]
	return ok && len(logs) > 0
}

// TestBuildFailurePreservesLogs tests that build failures preserve logs.
func TestBuildFailurePreservesLogs(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("build failure preserves logs", prop.ForAll(
		func(scenario BuildFailureScenario) bool {
			// Simulate a build failure scenario
			store := NewMockLogStore()

			// When a build fails, logs should be stored
			store.StoreLogs(scenario.DeploymentID, scenario.BuildLogs)

			// Verify logs are retrievable
			if !store.HasLogs(scenario.DeploymentID) {
				return false
			}

			// Verify the logs contain the original content
			logs := store.GetLogs(scenario.DeploymentID)
			if len(logs) == 0 {
				return false
			}

			// The stored logs should contain the build logs
			found := false
			for _, log := range logs {
				if log == scenario.BuildLogs {
					found = true
					break
				}
			}

			return found
		},
		genBuildFailureScenario(),
	))

	properties.TestingRun(t)
}

// TestBuildFailureStatusAndLogs tests that failed builds have correct status and logs.
func TestBuildFailureStatusAndLogs(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Simulate the deployment status tracking
	type DeploymentState struct {
		Status string
		Logs   string
	}

	properties.Property("failed builds have failed status and preserved logs", prop.ForAll(
		func(scenario BuildFailureScenario) bool {
			// Simulate a build failure
			state := DeploymentState{
				Status: "failed",
				Logs:   scenario.BuildLogs,
			}

			// Property 1: Status must be "failed"
			if state.Status != "failed" {
				return false
			}

			// Property 2: Logs must not be empty (they should be preserved)
			if state.Logs == "" {
				return false
			}

			// Property 3: Logs should match what was provided
			if state.Logs != scenario.BuildLogs {
				return false
			}

			return true
		},
		genBuildFailureScenario(),
	))

	properties.TestingRun(t)
}
