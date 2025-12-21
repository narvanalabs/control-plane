package models

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: control-plane, Property 24: Model JSON round-trip**
// For any valid Application, Deployment, or Node model, serializing to JSON
// and deserializing should produce an equivalent model.
// **Validates: Requirements 10.5, 10.6**

// genBuildType generates a random BuildType.
func genBuildType() gopter.Gen {
	return gen.OneConstOf(BuildTypeOCI, BuildTypePureNix)
}

// genResourceTier generates a random ResourceTier.
func genResourceTier() gopter.Gen {
	return gen.OneConstOf(
		ResourceTierNano,
		ResourceTierSmall,
		ResourceTierMedium,
		ResourceTierLarge,
		ResourceTierXLarge,
	)
}

// genDeploymentStatus generates a random DeploymentStatus.
func genDeploymentStatus() gopter.Gen {
	return gen.OneConstOf(
		DeploymentStatusPending,
		DeploymentStatusBuilding,
		DeploymentStatusBuilt,
		DeploymentStatusScheduled,
		DeploymentStatusStarting,
		DeploymentStatusRunning,
		DeploymentStatusStopping,
		DeploymentStatusStopped,
		DeploymentStatusFailed,
	)
}

// genTime generates a random time truncated to second precision for JSON compatibility.
func genTime() gopter.Gen {
	return gen.Int64Range(0, 2000000000).Map(func(secs int64) time.Time {
		return time.Unix(secs, 0).UTC()
	})
}


// genOptionalTime generates an optional time pointer.
func genOptionalTime() gopter.Gen {
	return gen.Bool().FlatMap(func(v interface{}) gopter.Gen {
		if v.(bool) {
			return genTime().Map(func(t time.Time) *time.Time {
				return &t
			})
		}
		return gen.Const((*time.Time)(nil))
	}, reflect.TypeOf((*time.Time)(nil)))
}

// genPortMapping generates a random PortMapping.
func genPortMapping() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 65535),
		gen.OneConstOf("tcp", "udp"),
	).Map(func(vals []interface{}) PortMapping {
		return PortMapping{
			ContainerPort: vals[0].(int),
			Protocol:      vals[1].(string),
		}
	})
}

// genHealthCheckConfig generates a random HealthCheckConfig (always non-nil).
func genHealthCheckConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString(),
		gen.IntRange(1, 65535),
		gen.IntRange(1, 300),
		gen.IntRange(1, 60),
		gen.IntRange(1, 10),
	).Map(func(vals []interface{}) HealthCheckConfig {
		return HealthCheckConfig{
			Path:            vals[0].(string),
			Port:            vals[1].(int),
			IntervalSeconds: vals[2].(int),
			TimeoutSeconds:  vals[3].(int),
			Retries:         vals[4].(int),
		}
	})
}

// genOptionalHealthCheckConfig generates an optional HealthCheckConfig pointer.
func genOptionalHealthCheckConfig() gopter.Gen {
	return gen.Bool().FlatMap(func(v interface{}) gopter.Gen {
		if v.(bool) {
			return genHealthCheckConfig().Map(func(hc HealthCheckConfig) *HealthCheckConfig {
				return &hc
			})
		}
		return gen.Const((*HealthCheckConfig)(nil))
	}, reflect.TypeOf((*HealthCheckConfig)(nil)))
}

// genSourceType generates a random SourceType.
func genSourceType() gopter.Gen {
	return gen.OneConstOf(SourceTypeGit, SourceTypeFlake, SourceTypeImage)
}

// genGitRepo generates a valid git repository URL.
func genGitRepo() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	).Map(func(vals []interface{}) string {
		return fmt.Sprintf("github.com/%s/%s", vals[0].(string), vals[1].(string))
	})
}

// genGitRef generates a valid git ref (branch, tag, or commit).
func genGitRef() gopter.Gen {
	return gen.OneConstOf("main", "master", "develop", "release/v1.0")
}

// genFlakeOutput generates a valid flake output path.
func genFlakeOutput() gopter.Gen {
	return gen.OneConstOf(
		"packages.x86_64-linux.default",
		"packages.x86_64-linux.api",
		"packages.aarch64-linux.default",
		"packages.x86_64-darwin.default",
	)
}

// genFlakeURI generates a valid flake URI.
func genFlakeURI() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	).Map(func(vals []interface{}) string {
		return fmt.Sprintf("github:%s/%s", vals[0].(string), vals[1].(string))
	})
}

// genImageRef generates a valid OCI image reference.
func genImageRef() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	).Map(func(vals []interface{}) string {
		return fmt.Sprintf("%s:%s", vals[0].(string), vals[1].(string))
	})
}

// genEnvVars generates a map of environment variables.
func genEnvVars() gopter.Gen {
	return gen.MapOf(gen.Identifier(), gen.AlphaString())
}

// genServiceConfig generates a random ServiceConfig.
func genServiceConfig() gopter.Gen {
	return genSourceType().FlatMap(func(v interface{}) gopter.Gen {
		sourceType := v.(SourceType)
		return gopter.CombineGens(
			gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
			genResourceTier(),
			gen.IntRange(1, 10),
			gen.SliceOfN(2, genPortMapping()),
			genOptionalHealthCheckConfig(),
			gen.SliceOfN(2, gen.Identifier()),
			genEnvVars(),
			genGitRepo(),
			genGitRef(),
			genFlakeOutput(),
			genFlakeURI(),
			genImageRef(),
		).Map(func(vals []interface{}) ServiceConfig {
			sc := ServiceConfig{
				Name:         vals[0].(string),
				SourceType:   sourceType,
				ResourceTier: vals[1].(ResourceTier),
				Replicas:     vals[2].(int),
				Ports:        vals[3].([]PortMapping),
				HealthCheck:  vals[4].(*HealthCheckConfig),
				DependsOn:    vals[5].([]string),
				EnvVars:      vals[6].(map[string]string),
			}
			// Set only the appropriate source field based on source type
			switch sourceType {
			case SourceTypeGit:
				sc.GitRepo = vals[7].(string)
				sc.GitRef = vals[8].(string)
				sc.FlakeOutput = vals[9].(string)
			case SourceTypeFlake:
				sc.FlakeURI = vals[10].(string)
			case SourceTypeImage:
				sc.Image = vals[11].(string)
			}
			return sc
		})
	}, reflect.TypeOf(ServiceConfig{}))
}


// genApp generates a random App.
func genApp() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),
		gen.Identifier(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
		gen.SliceOfN(3, genServiceConfig()),
		genTime(),
		genTime(),
		genOptionalTime(),
	).Map(func(vals []interface{}) App {
		return App{
			ID:          vals[0].(string),
			OwnerID:     vals[1].(string),
			Name:        vals[2].(string),
			Description: vals[3].(string),
			Services:    vals[4].([]ServiceConfig),
			CreatedAt:   vals[5].(time.Time),
			UpdatedAt:   vals[6].(time.Time),
			DeletedAt:   vals[7].(*time.Time),
		}
	})
}

// genRuntimeConfig generates a random RuntimeConfig.
func genRuntimeConfig() gopter.Gen {
	return gen.Bool().FlatMap(func(v interface{}) gopter.Gen {
		if v.(bool) {
			return gopter.CombineGens(
				genResourceTier(),
				gen.MapOf(gen.AlphaString(), gen.AlphaString()),
				gen.SliceOfN(2, genPortMapping()),
				genOptionalHealthCheckConfig(),
			).Map(func(vals []interface{}) *RuntimeConfig {
				return &RuntimeConfig{
					ResourceTier: vals[0].(ResourceTier),
					EnvVars:      vals[1].(map[string]string),
					Ports:        vals[2].([]PortMapping),
					HealthCheck:  vals[3].(*HealthCheckConfig),
				}
			})
		}
		return gen.Const((*RuntimeConfig)(nil))
	}, reflect.TypeOf((*RuntimeConfig)(nil)))
}


// genDeployment generates a random Deployment.
func genDeployment() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),
		gen.Identifier(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(1, 1000),
		gen.AlphaString(),
		gen.AlphaString(),
		genBuildType(),
		gen.AlphaString(),
		genDeploymentStatus(),
		gen.Identifier(),
		genResourceTier(),
		genRuntimeConfig(),
		gen.SliceOfN(3, gen.AlphaString()), // DependsOn
		genTime(),
		genTime(),
		genOptionalTime(),
		genOptionalTime(),
	).Map(func(vals []interface{}) Deployment {
		return Deployment{
			ID:           vals[0].(string),
			AppID:        vals[1].(string),
			ServiceName:  vals[2].(string),
			Version:      vals[3].(int),
			GitRef:       vals[4].(string),
			GitCommit:    vals[5].(string),
			BuildType:    vals[6].(BuildType),
			Artifact:     vals[7].(string),
			Status:       vals[8].(DeploymentStatus),
			NodeID:       vals[9].(string),
			ResourceTier: vals[10].(ResourceTier),
			Config:       vals[11].(*RuntimeConfig),
			DependsOn:    vals[12].([]string),
			CreatedAt:    vals[13].(time.Time),
			UpdatedAt:    vals[14].(time.Time),
			StartedAt:    vals[15].(*time.Time),
			FinishedAt:   vals[16].(*time.Time),
		}
	})
}


// genNodeResources generates a random NodeResources.
func genNodeResources() gopter.Gen {
	return gopter.CombineGens(
		gen.Float64Range(1, 64),
		gen.Float64Range(0, 64),
		gen.Int64Range(1<<30, 256<<30),
		gen.Int64Range(0, 256<<30),
		gen.Int64Range(1<<30, 1<<40),
		gen.Int64Range(0, 1<<40),
	).Map(func(vals []interface{}) *NodeResources {
		return &NodeResources{
			CPUTotal:        vals[0].(float64),
			CPUAvailable:    vals[1].(float64),
			MemoryTotal:     vals[2].(int64),
			MemoryAvailable: vals[3].(int64),
			DiskTotal:       vals[4].(int64),
			DiskAvailable:   vals[5].(int64),
		}
	})
}

// genNode generates a random Node.
func genNode() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(1024, 65535),
		gen.Bool(),
		genNodeResources(),
		gen.SliceOfN(5, gen.AlphaString()),
		genTime(),
		genTime(),
	).Map(func(vals []interface{}) Node {
		return Node{
			ID:            vals[0].(string),
			Hostname:      vals[1].(string),
			Address:       vals[2].(string),
			GRPCPort:      vals[3].(int),
			Healthy:       vals[4].(bool),
			Resources:     vals[5].(*NodeResources),
			CachedPaths:   vals[6].([]string),
			LastHeartbeat: vals[7].(time.Time),
			RegisteredAt:  vals[8].(time.Time),
		}
	})
}


// jsonEqual compares two values by their JSON representation.
// This handles the case where empty slices/maps serialize the same as nil.
func jsonEqual(a, b interface{}) bool {
	jsonA, errA := json.Marshal(a)
	jsonB, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(jsonA) == string(jsonB)
}

// TestAppJSONRoundTrip tests that App serializes and deserializes correctly.
func TestAppJSONRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("App JSON round-trip preserves data", prop.ForAll(
		func(original App) bool {
			// Serialize to JSON
			data, err := json.Marshal(original)
			if err != nil {
				return false
			}

			// Deserialize from JSON
			var restored App
			if err := json.Unmarshal(data, &restored); err != nil {
				return false
			}

			// Compare via JSON (handles empty vs nil equivalence)
			return jsonEqual(original, restored)
		},
		genApp(),
	))

	properties.TestingRun(t)
}

// TestDeploymentJSONRoundTrip tests that Deployment serializes and deserializes correctly.
func TestDeploymentJSONRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Deployment JSON round-trip preserves data", prop.ForAll(
		func(original Deployment) bool {
			// Serialize to JSON
			data, err := json.Marshal(original)
			if err != nil {
				return false
			}

			// Deserialize from JSON
			var restored Deployment
			if err := json.Unmarshal(data, &restored); err != nil {
				return false
			}

			// Compare via JSON (handles empty vs nil equivalence)
			return jsonEqual(original, restored)
		},
		genDeployment(),
	))

	properties.TestingRun(t)
}


// TestNodeJSONRoundTrip tests that Node serializes and deserializes correctly.
func TestNodeJSONRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Node JSON round-trip preserves data", prop.ForAll(
		func(original Node) bool {
			// Serialize to JSON
			data, err := json.Marshal(original)
			if err != nil {
				return false
			}

			// Deserialize from JSON
			var restored Node
			if err := json.Unmarshal(data, &restored); err != nil {
				return false
			}

			// Compare via JSON (handles empty vs nil equivalence)
			return jsonEqual(original, restored)
		},
		genNode(),
	))

	properties.TestingRun(t)
}


// **Feature: service-git-repos, Property 1: Source Type Mutual Exclusivity**
// For any service creation request, if more than one of git_repo, flake_uri, or image
// is specified, the request SHALL be rejected with a validation error.
// **Validates: Requirements 1.5, 2.3**
func TestSourceTypeMutualExclusivity(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for service configs with multiple sources set
	genMultipleSourceConfig := func() gopter.Gen {
		return gopter.CombineGens(
			gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
			genGitRepo(),
			genFlakeURI(),
			genImageRef(),
			gen.IntRange(1, 3), // Which combination of sources to set (1=git+flake, 2=git+image, 3=flake+image)
		).Map(func(vals []interface{}) ServiceConfig {
			sc := ServiceConfig{
				Name:         vals[0].(string),
				ResourceTier: ResourceTierSmall,
				Replicas:     1,
			}
			combo := vals[4].(int)
			switch combo {
			case 1: // git + flake
				sc.GitRepo = vals[1].(string)
				sc.FlakeURI = vals[2].(string)
			case 2: // git + image
				sc.GitRepo = vals[1].(string)
				sc.Image = vals[3].(string)
			case 3: // flake + image
				sc.FlakeURI = vals[2].(string)
				sc.Image = vals[3].(string)
			}
			return sc
		})
	}

	properties.Property("Multiple source types should be rejected", prop.ForAll(
		func(sc ServiceConfig) bool {
			err := sc.Validate()
			if err == nil {
				return false // Should have failed
			}
			// Check that it's a validation error about source
			validationErr, ok := err.(*ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "source" &&
				validationErr.Message == "only one of git_repo, flake_uri, or image can be specified"
		},
		genMultipleSourceConfig(),
	))

	// Generator for service configs with no sources set
	genNoSourceConfig := func() gopter.Gen {
		return gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }).Map(func(name string) ServiceConfig {
			return ServiceConfig{
				Name:         name,
				ResourceTier: ResourceTierSmall,
				Replicas:     1,
			}
		})
	}

	properties.Property("No source type should be rejected", prop.ForAll(
		func(sc ServiceConfig) bool {
			err := sc.Validate()
			if err == nil {
				return false // Should have failed
			}
			// Check that it's a validation error about source
			validationErr, ok := err.(*ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "source" &&
				validationErr.Message == "exactly one of git_repo, flake_uri, or image is required"
		},
		genNoSourceConfig(),
	))

	// Generator for service configs with exactly one source set
	genSingleSourceConfig := func() gopter.Gen {
		return genServiceConfig()
	}

	properties.Property("Single source type should be accepted", prop.ForAll(
		func(sc ServiceConfig) bool {
			err := sc.Validate()
			return err == nil
		},
		genSingleSourceConfig(),
	))

	properties.TestingRun(t)
}


// **Feature: service-git-repos, Property 2: Service Source Round-Trip**
// For any valid service configuration with a source (git_repo, flake_uri, or image),
// serializing to JSON and deserializing should return the same source configuration.
// **Validates: Requirements 1.2, 1.3, 1.4, 2.6**
func TestServiceSourceRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Service source round-trip preserves data", prop.ForAll(
		func(original ServiceConfig) bool {
			// Validate first to ensure defaults are applied
			if err := original.Validate(); err != nil {
				return false
			}

			// Serialize to JSON
			data, err := json.Marshal(original)
			if err != nil {
				return false
			}

			// Deserialize from JSON
			var restored ServiceConfig
			if err := json.Unmarshal(data, &restored); err != nil {
				return false
			}

			// Check source type is preserved
			if original.SourceType != restored.SourceType {
				return false
			}

			// Check source-specific fields based on type
			switch original.SourceType {
			case SourceTypeGit:
				if original.GitRepo != restored.GitRepo {
					return false
				}
				if original.GitRef != restored.GitRef {
					return false
				}
				if original.FlakeOutput != restored.FlakeOutput {
					return false
				}
			case SourceTypeFlake:
				if original.FlakeURI != restored.FlakeURI {
					return false
				}
			case SourceTypeImage:
				if original.Image != restored.Image {
					return false
				}
			}

			return true
		},
		genServiceConfig(),
	))

	properties.TestingRun(t)
}


// **Feature: service-git-repos, Property 13: Flake URI Construction Correctness**
// For any valid git_repo, git_ref, and flake_output, BuildFlakeURI() SHALL produce
// a valid Nix flake URI format.
// **Validates: Requirements 1.6, 11.7**
func TestFlakeURIConstruction(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for git source configs
	genGitSourceConfig := func() gopter.Gen {
		return gopter.CombineGens(
			gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
			genGitRepo(),
			genGitRef(),
			genFlakeOutput(),
		).Map(func(vals []interface{}) ServiceConfig {
			return ServiceConfig{
				Name:         vals[0].(string),
				GitRepo:      vals[1].(string),
				GitRef:       vals[2].(string),
				FlakeOutput:  vals[3].(string),
				ResourceTier: ResourceTierSmall,
				Replicas:     1,
			}
		})
	}

	properties.Property("BuildFlakeURI produces valid format for git sources", prop.ForAll(
		func(sc ServiceConfig) bool {
			// Validate to set source type
			if err := sc.Validate(); err != nil {
				return false
			}

			uri := sc.BuildFlakeURI()

			// URI should not be empty for git sources
			if uri == "" {
				return false
			}

			// URI should start with a valid flake prefix
			validPrefixes := []string{"github:", "gitlab:", "git+https://"}
			hasValidPrefix := false
			for _, prefix := range validPrefixes {
				if len(uri) >= len(prefix) && uri[:len(prefix)] == prefix {
					hasValidPrefix = true
					break
				}
			}
			if !hasValidPrefix {
				return false
			}

			// URI should contain the flake output after #
			if sc.FlakeOutput != "" {
				if !containsString(uri, "#"+sc.FlakeOutput) {
					return false
				}
			}

			return true
		},
		genGitSourceConfig(),
	))

	// Test that flake sources return their URI directly
	genFlakeSourceConfig := func() gopter.Gen {
		return gopter.CombineGens(
			gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
			genFlakeURI(),
		).Map(func(vals []interface{}) ServiceConfig {
			return ServiceConfig{
				Name:         vals[0].(string),
				FlakeURI:     vals[1].(string),
				ResourceTier: ResourceTierSmall,
				Replicas:     1,
			}
		})
	}

	properties.Property("BuildFlakeURI returns flake_uri directly for flake sources", prop.ForAll(
		func(sc ServiceConfig) bool {
			// Validate to set source type
			if err := sc.Validate(); err != nil {
				return false
			}

			uri := sc.BuildFlakeURI()
			return uri == sc.FlakeURI
		},
		genFlakeSourceConfig(),
	))

	// Test that image sources return empty string
	genImageSourceConfig := func() gopter.Gen {
		return gopter.CombineGens(
			gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
			genImageRef(),
		).Map(func(vals []interface{}) ServiceConfig {
			return ServiceConfig{
				Name:         vals[0].(string),
				Image:        vals[1].(string),
				ResourceTier: ResourceTierSmall,
				Replicas:     1,
			}
		})
	}

	properties.Property("BuildFlakeURI returns empty for image sources", prop.ForAll(
		func(sc ServiceConfig) bool {
			// Validate to set source type
			if err := sc.Validate(); err != nil {
				return false
			}

			uri := sc.BuildFlakeURI()
			return uri == ""
		},
		genImageSourceConfig(),
	))

	properties.TestingRun(t)
}

// containsString checks if a string contains a substring.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}


// **Feature: service-git-repos, Property 14: Git Ref Variants**
// For any service with git source, git_ref SHALL accept branches, tags, and commit SHAs,
// and each SHALL produce a buildable flake URI.
// **Validates: Requirements 5.3, 11.2**
func TestGitRefVariants(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for branch refs
	genBranchRef := func() gopter.Gen {
		return gen.OneConstOf("main", "master", "develop", "feature/test", "release/v1.0")
	}

	// Generator for tag refs
	genTagRef := func() gopter.Gen {
		return gen.OneConstOf("v1.0.0", "v2.0.0-beta", "release-1.0")
	}

	// Generator for commit SHA refs (40 hex chars)
	genCommitRef := func() gopter.Gen {
		return gen.Const("abc123def456789012345678901234567890abcd")
	}

	// Generator for all ref types
	genAnyRef := func() gopter.Gen {
		return gen.OneGenOf(genBranchRef(), genTagRef(), genCommitRef())
	}

	// Generator for git source configs with various refs
	genGitSourceWithRef := func() gopter.Gen {
		return gopter.CombineGens(
			gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
			genGitRepo(),
			genAnyRef(),
			genFlakeOutput(),
		).Map(func(vals []interface{}) ServiceConfig {
			return ServiceConfig{
				Name:         vals[0].(string),
				GitRepo:      vals[1].(string),
				GitRef:       vals[2].(string),
				FlakeOutput:  vals[3].(string),
				ResourceTier: ResourceTierSmall,
				Replicas:     1,
			}
		})
	}

	properties.Property("Git refs (branches, tags, commits) produce valid flake URIs", prop.ForAll(
		func(sc ServiceConfig) bool {
			// Validate to set source type
			if err := sc.Validate(); err != nil {
				return false
			}

			uri := sc.BuildFlakeURI()

			// URI should not be empty
			if uri == "" {
				return false
			}

			// URI should be a valid flake format
			validPrefixes := []string{"github:", "gitlab:", "git+https://"}
			hasValidPrefix := false
			for _, prefix := range validPrefixes {
				if len(uri) >= len(prefix) && uri[:len(prefix)] == prefix {
					hasValidPrefix = true
					break
				}
			}

			return hasValidPrefix
		},
		genGitSourceWithRef(),
	))

	// Test that non-main/master refs are included in the URI
	properties.Property("Non-default refs are included in flake URI", prop.ForAll(
		func(sc ServiceConfig) bool {
			// Validate to set source type
			if err := sc.Validate(); err != nil {
				return false
			}

			uri := sc.BuildFlakeURI()

			// If ref is not main or master, it should appear in the URI
			if sc.GitRef != "main" && sc.GitRef != "master" {
				return findSubstring(uri, sc.GitRef)
			}

			// For main/master, the ref may or may not be in the URI (it's the default)
			return true
		},
		genGitSourceWithRef(),
	))

	properties.TestingRun(t)
}


// **Feature: service-git-repos, Property 15: Flake Output System Consistency**
// For any service with default flake_output, the system in the output path
// SHALL match the build node's architecture.
// **Validates: Requirements 1.6, 11.8**
func TestFlakeOutputSystemConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for git source configs without explicit flake_output
	genGitSourceNoOutput := func() gopter.Gen {
		return gopter.CombineGens(
			gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
			genGitRepo(),
			genGitRef(),
		).Map(func(vals []interface{}) ServiceConfig {
			return ServiceConfig{
				Name:         vals[0].(string),
				GitRepo:      vals[1].(string),
				GitRef:       vals[2].(string),
				FlakeOutput:  "", // Empty to trigger default
				ResourceTier: ResourceTierSmall,
				Replicas:     1,
			}
		})
	}

	properties.Property("Default flake_output contains current system", prop.ForAll(
		func(sc ServiceConfig) bool {
			// Validate to apply defaults
			if err := sc.Validate(); err != nil {
				return false
			}

			// Get the current system
			currentSystem := GetCurrentSystem()

			// The default flake output should contain the current system
			expectedDefault := fmt.Sprintf("packages.%s.default", currentSystem)
			return sc.FlakeOutput == expectedDefault
		},
		genGitSourceNoOutput(),
	))

	// Test that GetCurrentSystem returns a valid system string
	properties.Property("GetCurrentSystem returns valid system string", prop.ForAll(
		func(_ int) bool {
			system := GetCurrentSystem()

			// Should be one of the known systems
			validSystems := []string{
				"x86_64-linux",
				"x86_64-darwin",
				"aarch64-linux",
				"aarch64-darwin",
			}

			for _, valid := range validSystems {
				if system == valid {
					return true
				}
			}
			return false
		},
		gen.IntRange(0, 100), // Dummy generator to run the test
	))

	properties.TestingRun(t)
}


// **Feature: flexible-build-strategies, Property 3: Build Strategy Determinism**
// For any service configuration with a specified build_strategy, the Build_System
// SHALL always use that exact strategy regardless of repository contents.
// **Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.5**
func TestBuildStrategyDeterminism(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for valid build strategies
	genBuildStrategy := func() gopter.Gen {
		return gen.OneConstOf(
			BuildStrategyFlake,
			BuildStrategyAutoGo,
			BuildStrategyAutoRust,
			BuildStrategyAutoNode,
			BuildStrategyAutoPython,
			BuildStrategyDockerfile,
			BuildStrategyNixpacks,
			BuildStrategyAuto,
		)
	}

	// Property: All valid strategies should be recognized as valid
	properties.Property("Valid build strategies are recognized", prop.ForAll(
		func(strategy BuildStrategy) bool {
			return strategy.IsValid()
		},
		genBuildStrategy(),
	))

	// Property: ValidBuildStrategies returns all expected strategies
	properties.Property("ValidBuildStrategies returns complete list", prop.ForAll(
		func(_ int) bool {
			strategies := ValidBuildStrategies()

			// Should have exactly 8 strategies
			if len(strategies) != 8 {
				return false
			}

			// All expected strategies should be present
			expected := map[BuildStrategy]bool{
				BuildStrategyFlake:      false,
				BuildStrategyAutoGo:     false,
				BuildStrategyAutoRust:   false,
				BuildStrategyAutoNode:   false,
				BuildStrategyAutoPython: false,
				BuildStrategyDockerfile: false,
				BuildStrategyNixpacks:   false,
				BuildStrategyAuto:       false,
			}

			for _, s := range strategies {
				if _, ok := expected[s]; !ok {
					return false // Unexpected strategy
				}
				expected[s] = true
			}

			// All expected strategies should have been found
			for _, found := range expected {
				if !found {
					return false
				}
			}

			return true
		},
		gen.IntRange(0, 100), // Dummy generator
	))

	// Property: Invalid strategies should be rejected
	genInvalidStrategy := func() gopter.Gen {
		return gen.AlphaString().SuchThat(func(s string) bool {
			// Exclude valid strategies
			strategy := BuildStrategy(s)
			return !strategy.IsValid()
		}).Map(func(s string) BuildStrategy {
			return BuildStrategy(s)
		})
	}

	properties.Property("Invalid build strategies are rejected", prop.ForAll(
		func(strategy BuildStrategy) bool {
			return !strategy.IsValid()
		},
		genInvalidStrategy(),
	))

	// Property: Build strategy in ServiceConfig is preserved through JSON round-trip
	genServiceConfigWithStrategy := func() gopter.Gen {
		return gopter.CombineGens(
			genServiceConfig(),
			genBuildStrategy(),
		).Map(func(vals []interface{}) ServiceConfig {
			sc := vals[0].(ServiceConfig)
			sc.BuildStrategy = vals[1].(BuildStrategy)
			return sc
		})
	}

	properties.Property("Build strategy preserved in ServiceConfig JSON round-trip", prop.ForAll(
		func(original ServiceConfig) bool {
			// Validate first
			if err := original.Validate(); err != nil {
				return false
			}

			// Serialize to JSON
			data, err := json.Marshal(original)
			if err != nil {
				return false
			}

			// Deserialize from JSON
			var restored ServiceConfig
			if err := json.Unmarshal(data, &restored); err != nil {
				return false
			}

			// Build strategy should be preserved
			return original.BuildStrategy == restored.BuildStrategy
		},
		genServiceConfigWithStrategy(),
	))

	properties.TestingRun(t)
}

// genBuildConfig generates a random BuildConfig.
func genBuildConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString(),                                    // BuildCommand
		gen.AlphaString(),                                    // StartCommand
		gen.AlphaString(),                                    // EntryPoint
		gen.IntRange(60, 3600),                               // BuildTimeout
		gen.OneConstOf("1.21", "1.22", "1.23"),               // GoVersion
		gen.Bool(),                                           // CGOEnabled
		gen.OneConstOf("18", "20", "22"),                     // NodeVersion
		gen.OneConstOf("npm", "yarn", "pnpm"),                // PackageManager
		gen.OneConstOf("2018", "2021", "2024"),               // RustEdition
		gen.OneConstOf("3.10", "3.11", "3.12"),               // PythonVersion
		gen.Bool(),                                           // AutoRetryAsOCI
	).Map(func(vals []interface{}) *BuildConfig {
		return &BuildConfig{
			BuildCommand:   vals[0].(string),
			StartCommand:   vals[1].(string),
			EntryPoint:     vals[2].(string),
			BuildTimeout:   vals[3].(int),
			GoVersion:      vals[4].(string),
			CGOEnabled:     vals[5].(bool),
			NodeVersion:    vals[6].(string),
			PackageManager: vals[7].(string),
			RustEdition:    vals[8].(string),
			PythonVersion:  vals[9].(string),
			AutoRetryAsOCI: vals[10].(bool),
		}
	})
}

// **Feature: flexible-build-strategies, Property 3: Build Strategy Determinism (BuildJob)**
// For any BuildJob with a specified build_strategy, the strategy SHALL be preserved
// through serialization and deserialization.
// **Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.5**
func TestBuildJobStrategyDeterminism(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for valid build strategies
	genBuildStrategy := func() gopter.Gen {
		return gen.OneConstOf(
			BuildStrategyFlake,
			BuildStrategyAutoGo,
			BuildStrategyAutoRust,
			BuildStrategyAutoNode,
			BuildStrategyAutoPython,
			BuildStrategyDockerfile,
			BuildStrategyNixpacks,
			BuildStrategyAuto,
		)
	}

	// Generator for BuildJob with strategy
	genBuildJob := func() gopter.Gen {
		return gopter.CombineGens(
			gen.Identifier(),                                     // ID
			gen.Identifier(),                                     // DeploymentID
			gen.Identifier(),                                     // AppID
			gen.AlphaString(),                                    // ServiceName
			gen.AlphaString(),                                    // GitURL
			gen.AlphaString(),                                    // GitRef
			gen.AlphaString(),                                    // FlakeOutput
			genBuildType(),                                       // BuildType
			gen.OneConstOf(BuildStatusQueued, BuildStatusRunning, BuildStatusSucceeded, BuildStatusFailed),
			genTime(),                                            // CreatedAt
			genBuildStrategy(),                                   // BuildStrategy
			gen.AlphaString(),                                    // GeneratedFlake
			gen.AlphaString(),                                    // FlakeLock
			gen.AlphaString(),                                    // VendorHash
			gen.IntRange(60, 3600),                               // TimeoutSeconds
			gen.IntRange(0, 5),                                   // RetryCount
			gen.Bool(),                                           // RetryAsOCI
		).Map(func(vals []interface{}) BuildJob {
			return BuildJob{
				ID:             vals[0].(string),
				DeploymentID:   vals[1].(string),
				AppID:          vals[2].(string),
				ServiceName:    vals[3].(string),
				GitURL:         vals[4].(string),
				GitRef:         vals[5].(string),
				FlakeOutput:    vals[6].(string),
				BuildType:      vals[7].(BuildType),
				Status:         vals[8].(BuildStatus),
				CreatedAt:      vals[9].(time.Time),
				BuildStrategy:  vals[10].(BuildStrategy),
				GeneratedFlake: vals[11].(string),
				FlakeLock:      vals[12].(string),
				VendorHash:     vals[13].(string),
				TimeoutSeconds: vals[14].(int),
				RetryCount:     vals[15].(int),
				RetryAsOCI:     vals[16].(bool),
			}
		})
	}

	properties.Property("BuildJob strategy preserved through JSON round-trip", prop.ForAll(
		func(original BuildJob) bool {
			// Serialize to JSON
			data, err := json.Marshal(original)
			if err != nil {
				return false
			}

			// Deserialize from JSON
			var restored BuildJob
			if err := json.Unmarshal(data, &restored); err != nil {
				return false
			}

			// Build strategy should be preserved
			if original.BuildStrategy != restored.BuildStrategy {
				return false
			}

			// Other new fields should also be preserved
			if original.GeneratedFlake != restored.GeneratedFlake {
				return false
			}
			if original.FlakeLock != restored.FlakeLock {
				return false
			}
			if original.VendorHash != restored.VendorHash {
				return false
			}
			if original.TimeoutSeconds != restored.TimeoutSeconds {
				return false
			}
			if original.RetryCount != restored.RetryCount {
				return false
			}
			if original.RetryAsOCI != restored.RetryAsOCI {
				return false
			}

			return true
		},
		genBuildJob(),
	))

	properties.TestingRun(t)
}
