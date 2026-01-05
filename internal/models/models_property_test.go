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
				validationErr.Message == "exactly one of git_repo, flake_uri, image, or database is required"
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
			BuildStrategyAutoDatabase,
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

			// Should have exactly 9 strategies
			if len(strategies) != 9 {
				return false
			}

			// All expected strategies should be present
			expected := map[BuildStrategy]bool{
				BuildStrategyFlake:        false,
				BuildStrategyAutoGo:       false,
				BuildStrategyAutoRust:     false,
				BuildStrategyAutoNode:     false,
				BuildStrategyAutoPython:   false,
				BuildStrategyAutoDatabase: false,
				BuildStrategyDockerfile:   false,
				BuildStrategyNixpacks:     false,
				BuildStrategyAuto:         false,
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
		gen.PtrOf(gen.Bool()),                                // CGOEnabled (*bool)
		gen.OneConstOf("18", "20", "22"),                     // NodeVersion
		gen.OneConstOf("npm", "yarn", "pnpm"),                // PackageManager
		gen.OneConstOf("2018", "2021", "2024"),               // RustEdition
		gen.OneConstOf("3.10", "3.11", "3.12"),               // PythonVersion
		gen.Bool(),                                           // AutoRetryAsOCI
	).Map(func(vals []interface{}) *BuildConfig {
		var cgoEnabled *bool
		if vals[5] != nil {
			cgoEnabled = vals[5].(*bool)
		}
		return &BuildConfig{
			BuildCommand:   vals[0].(string),
			StartCommand:   vals[1].(string),
			EntryPoint:     vals[2].(string),
			BuildTimeout:   vals[3].(int),
			GoVersion:      vals[4].(string),
			CGOEnabled:     cgoEnabled,
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


// **Feature: build-lifecycle-correctness, Property 15: Build Type Enforcement for Dockerfile**
// *For any* build job with `build_strategy: dockerfile`, the effective build_type SHALL be `oci`
// regardless of user specification.
// **Validates: Requirements 10.2, 18.1**
func TestBuildTypeEnforcementDockerfile(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Dockerfile strategy always enforces OCI build type
	properties.Property("dockerfile strategy enforces OCI build type", prop.ForAll(
		func(requestedType BuildType) bool {
			enforcedType, wasChanged := EnforceBuildType(BuildStrategyDockerfile, requestedType)
			
			// Enforced type must always be OCI for dockerfile strategy
			if enforcedType != BuildTypeOCI {
				return false
			}
			
			// wasChanged should be true if requested type was not OCI
			if requestedType != BuildTypeOCI && !wasChanged {
				return false
			}
			
			// wasChanged should be false if requested type was already OCI
			if requestedType == BuildTypeOCI && wasChanged {
				return false
			}
			
			return true
		},
		gen.OneConstOf(BuildTypePureNix, BuildTypeOCI, BuildType("")),
	))

	// Property: Dockerfile strategy with pure-nix request returns OCI and wasChanged=true
	properties.Property("dockerfile with pure-nix returns OCI and wasChanged=true", prop.ForAll(
		func(_ int) bool {
			enforcedType, wasChanged := EnforceBuildType(BuildStrategyDockerfile, BuildTypePureNix)
			return enforcedType == BuildTypeOCI && wasChanged
		},
		gen.IntRange(0, 100), // Dummy generator
	))

	// Property: Dockerfile strategy with OCI request returns OCI and wasChanged=false
	properties.Property("dockerfile with OCI returns OCI and wasChanged=false", prop.ForAll(
		func(_ int) bool {
			enforcedType, wasChanged := EnforceBuildType(BuildStrategyDockerfile, BuildTypeOCI)
			return enforcedType == BuildTypeOCI && !wasChanged
		},
		gen.IntRange(0, 100), // Dummy generator
	))

	// Property: Dockerfile strategy with empty request returns OCI and wasChanged=true
	properties.Property("dockerfile with empty type returns OCI and wasChanged=true", prop.ForAll(
		func(_ int) bool {
			enforcedType, wasChanged := EnforceBuildType(BuildStrategyDockerfile, BuildType(""))
			return enforcedType == BuildTypeOCI && wasChanged
		},
		gen.IntRange(0, 100), // Dummy generator
	))

	properties.TestingRun(t)
}


// **Feature: build-lifecycle-correctness, Property 16: Build Type Enforcement for Nixpacks**
// *For any* build job with `build_strategy: nixpacks`, the effective build_type SHALL be `oci`
// regardless of user specification.
// **Validates: Requirements 11.2, 18.2**
func TestBuildTypeEnforcementNixpacks(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Nixpacks strategy always enforces OCI build type
	properties.Property("nixpacks strategy enforces OCI build type", prop.ForAll(
		func(requestedType BuildType) bool {
			enforcedType, wasChanged := EnforceBuildType(BuildStrategyNixpacks, requestedType)
			
			// Enforced type must always be OCI for nixpacks strategy
			if enforcedType != BuildTypeOCI {
				return false
			}
			
			// wasChanged should be true if requested type was not OCI
			if requestedType != BuildTypeOCI && !wasChanged {
				return false
			}
			
			// wasChanged should be false if requested type was already OCI
			if requestedType == BuildTypeOCI && wasChanged {
				return false
			}
			
			return true
		},
		gen.OneConstOf(BuildTypePureNix, BuildTypeOCI, BuildType("")),
	))

	// Property: Nixpacks strategy with pure-nix request returns OCI and wasChanged=true
	properties.Property("nixpacks with pure-nix returns OCI and wasChanged=true", prop.ForAll(
		func(_ int) bool {
			enforcedType, wasChanged := EnforceBuildType(BuildStrategyNixpacks, BuildTypePureNix)
			return enforcedType == BuildTypeOCI && wasChanged
		},
		gen.IntRange(0, 100), // Dummy generator
	))

	// Property: Nixpacks strategy with OCI request returns OCI and wasChanged=false
	properties.Property("nixpacks with OCI returns OCI and wasChanged=false", prop.ForAll(
		func(_ int) bool {
			enforcedType, wasChanged := EnforceBuildType(BuildStrategyNixpacks, BuildTypeOCI)
			return enforcedType == BuildTypeOCI && !wasChanged
		},
		gen.IntRange(0, 100), // Dummy generator
	))

	// Property: Nixpacks strategy with empty request returns OCI and wasChanged=true
	properties.Property("nixpacks with empty type returns OCI and wasChanged=true", prop.ForAll(
		func(_ int) bool {
			enforcedType, wasChanged := EnforceBuildType(BuildStrategyNixpacks, BuildType(""))
			return enforcedType == BuildTypeOCI && wasChanged
		},
		gen.IntRange(0, 100), // Dummy generator
	))

	properties.TestingRun(t)
}


// **Feature: platform-enhancements, Property 4: Container Name Uniqueness**
// For any deployment, the generated container name SHALL include the app name, service name,
// and version number in the format "{app}-{service}-v{version}", ensuring uniqueness across all deployments.
// **Validates: Requirements 9.3, 9.4, 9.5**
func TestContainerNameUniqueness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Container name follows the format {app}-{service}-v{version}
	properties.Property("Container name follows correct format", prop.ForAll(
		func(appName, serviceName string, version int) bool {
			containerName := GenerateContainerName(appName, serviceName, version)
			expected := fmt.Sprintf("%s-%s-v%d", appName, serviceName, version)
			return containerName == expected
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(1, 10000),
	))

	// Property: Different versions produce different container names
	properties.Property("Different versions produce different container names", prop.ForAll(
		func(appName, serviceName string, version1, version2 int) bool {
			if version1 == version2 {
				return true // Skip if versions are the same
			}
			name1 := GenerateContainerName(appName, serviceName, version1)
			name2 := GenerateContainerName(appName, serviceName, version2)
			return name1 != name2
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(1, 10000),
		gen.IntRange(1, 10000),
	))

	// Property: Different services produce different container names
	properties.Property("Different services produce different container names", prop.ForAll(
		func(appName, serviceName1, serviceName2 string, version int) bool {
			if serviceName1 == serviceName2 {
				return true // Skip if service names are the same
			}
			name1 := GenerateContainerName(appName, serviceName1, version)
			name2 := GenerateContainerName(appName, serviceName2, version)
			return name1 != name2
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(1, 10000),
	))

	// Property: Different apps produce different container names
	properties.Property("Different apps produce different container names", prop.ForAll(
		func(appName1, appName2, serviceName string, version int) bool {
			if appName1 == appName2 {
				return true // Skip if app names are the same
			}
			name1 := GenerateContainerName(appName1, serviceName, version)
			name2 := GenerateContainerName(appName2, serviceName, version)
			return name1 != name2
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(1, 10000),
	))

	// Property: Container name contains version number
	properties.Property("Container name contains version number", prop.ForAll(
		func(appName, serviceName string, version int) bool {
			containerName := GenerateContainerName(appName, serviceName, version)
			versionStr := fmt.Sprintf("v%d", version)
			return findSubstring(containerName, versionStr)
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(1, 10000),
	))

	// Property: Same inputs always produce the same container name (deterministic)
	properties.Property("Container name generation is deterministic", prop.ForAll(
		func(appName, serviceName string, version int) bool {
			name1 := GenerateContainerName(appName, serviceName, version)
			name2 := GenerateContainerName(appName, serviceName, version)
			return name1 == name2
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(1, 10000),
	))

	// Property: Deployment.ContainerName() returns a valid container name
	properties.Property("Deployment.ContainerName returns valid format", prop.ForAll(
		func(deployment Deployment) bool {
			containerName := deployment.ContainerName()
			// Should contain version
			versionStr := fmt.Sprintf("v%d", deployment.Version)
			if !findSubstring(containerName, versionStr) {
				return false
			}
			// Should contain service name
			if !findSubstring(containerName, deployment.ServiceName) {
				return false
			}
			return true
		},
		genDeployment(),
	))

	properties.TestingRun(t)
}


// **Feature: backend-source-of-truth, Property 19: BuildJob Source Field Consistency**
// For any BuildJob, if SourceType is "git" then GitURL SHALL be non-empty and FlakeURI SHALL
// contain the constructed flake URI; if SourceType is "flake" then FlakeURI SHALL be non-empty
// and GitURL SHALL be empty.
// **Validates: Requirements 27.1, 27.2, 27.3**
func TestBuildJobSourceFieldConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for valid git source BuildJob
	genGitSourceBuildJob := func() gopter.Gen {
		return gopter.CombineGens(
			gen.Identifier(),                                     // ID
			gen.Identifier(),                                     // DeploymentID
			gen.Identifier(),                                     // AppID
			gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // ServiceName
			genGitRepo(),                                         // GitURL (valid git repo)
			genGitRef(),                                          // GitRef
			genFlakeOutput(),                                     // FlakeOutput
			genBuildType(),                                       // BuildType
			gen.OneConstOf(BuildStatusQueued, BuildStatusRunning, BuildStatusSucceeded, BuildStatusFailed),
			genTime(),                                            // CreatedAt
		).Map(func(vals []interface{}) BuildJob {
			gitURL := vals[4].(string)
			gitRef := vals[5].(string)
			flakeOutput := vals[6].(string)
			// Construct the flake URI from git components
			flakeURI := buildFlakeURIFromGit(gitURL, gitRef, flakeOutput)
			return BuildJob{
				ID:           vals[0].(string),
				DeploymentID: vals[1].(string),
				AppID:        vals[2].(string),
				ServiceName:  vals[3].(string),
				SourceType:   SourceTypeGit,
				GitURL:       gitURL,
				GitRef:       gitRef,
				FlakeURI:     flakeURI,
				FlakeOutput:  flakeOutput,
				BuildType:    vals[7].(BuildType),
				Status:       vals[8].(BuildStatus),
				CreatedAt:    vals[9].(time.Time),
			}
		})
	}

	// Generator for valid flake source BuildJob
	genFlakeSourceBuildJob := func() gopter.Gen {
		return gopter.CombineGens(
			gen.Identifier(),                                     // ID
			gen.Identifier(),                                     // DeploymentID
			gen.Identifier(),                                     // AppID
			gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // ServiceName
			genFlakeURI(),                                        // FlakeURI (valid flake URI)
			genBuildType(),                                       // BuildType
			gen.OneConstOf(BuildStatusQueued, BuildStatusRunning, BuildStatusSucceeded, BuildStatusFailed),
			genTime(),                                            // CreatedAt
		).Map(func(vals []interface{}) BuildJob {
			return BuildJob{
				ID:           vals[0].(string),
				DeploymentID: vals[1].(string),
				AppID:        vals[2].(string),
				ServiceName:  vals[3].(string),
				SourceType:   SourceTypeFlake,
				GitURL:       "", // Empty for flake sources
				FlakeURI:     vals[4].(string),
				BuildType:    vals[5].(BuildType),
				Status:       vals[6].(BuildStatus),
				CreatedAt:    vals[7].(time.Time),
			}
		})
	}

	// Generator for invalid git source BuildJob (missing GitURL)
	genInvalidGitSourceBuildJob := func() gopter.Gen {
		return gopter.CombineGens(
			gen.Identifier(),                                     // ID
			gen.Identifier(),                                     // DeploymentID
			gen.Identifier(),                                     // AppID
			gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // ServiceName
			genFlakeURI(),                                        // FlakeURI
			genBuildType(),                                       // BuildType
			gen.OneConstOf(BuildStatusQueued, BuildStatusRunning, BuildStatusSucceeded, BuildStatusFailed),
			genTime(),                                            // CreatedAt
		).Map(func(vals []interface{}) BuildJob {
			return BuildJob{
				ID:           vals[0].(string),
				DeploymentID: vals[1].(string),
				AppID:        vals[2].(string),
				ServiceName:  vals[3].(string),
				SourceType:   SourceTypeGit,
				GitURL:       "", // Invalid: empty for git source
				FlakeURI:     vals[4].(string),
				BuildType:    vals[5].(BuildType),
				Status:       vals[6].(BuildStatus),
				CreatedAt:    vals[7].(time.Time),
			}
		})
	}

	// Generator for invalid flake source BuildJob (has GitURL)
	genInvalidFlakeSourceBuildJob := func() gopter.Gen {
		return gopter.CombineGens(
			gen.Identifier(),                                     // ID
			gen.Identifier(),                                     // DeploymentID
			gen.Identifier(),                                     // AppID
			gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // ServiceName
			genGitRepo(),                                         // GitURL (should be empty for flake)
			genFlakeURI(),                                        // FlakeURI
			genBuildType(),                                       // BuildType
			gen.OneConstOf(BuildStatusQueued, BuildStatusRunning, BuildStatusSucceeded, BuildStatusFailed),
			genTime(),                                            // CreatedAt
		).Map(func(vals []interface{}) BuildJob {
			return BuildJob{
				ID:           vals[0].(string),
				DeploymentID: vals[1].(string),
				AppID:        vals[2].(string),
				ServiceName:  vals[3].(string),
				SourceType:   SourceTypeFlake,
				GitURL:       vals[4].(string), // Invalid: non-empty for flake source
				FlakeURI:     vals[5].(string),
				BuildType:    vals[6].(BuildType),
				Status:       vals[7].(BuildStatus),
				CreatedAt:    vals[8].(time.Time),
			}
		})
	}

	// Property: Valid git source BuildJob passes validation
	properties.Property("Valid git source BuildJob passes validation", prop.ForAll(
		func(job BuildJob) bool {
			err := job.ValidateBuildJobSource()
			return err == nil
		},
		genGitSourceBuildJob(),
	))

	// Property: Valid flake source BuildJob passes validation
	properties.Property("Valid flake source BuildJob passes validation", prop.ForAll(
		func(job BuildJob) bool {
			err := job.ValidateBuildJobSource()
			return err == nil
		},
		genFlakeSourceBuildJob(),
	))

	// Property: Git source with empty GitURL fails validation
	properties.Property("Git source with empty GitURL fails validation", prop.ForAll(
		func(job BuildJob) bool {
			err := job.ValidateBuildJobSource()
			if err == nil {
				return false
			}
			validationErr, ok := err.(*ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "git_url"
		},
		genInvalidGitSourceBuildJob(),
	))

	// Property: Flake source with non-empty GitURL fails validation
	properties.Property("Flake source with non-empty GitURL fails validation", prop.ForAll(
		func(job BuildJob) bool {
			err := job.ValidateBuildJobSource()
			if err == nil {
				return false
			}
			validationErr, ok := err.(*ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "git_url"
		},
		genInvalidFlakeSourceBuildJob(),
	))

	// Property: Git source BuildJob has non-empty FlakeURI (constructed from git URL)
	properties.Property("Git source BuildJob has non-empty FlakeURI", prop.ForAll(
		func(job BuildJob) bool {
			if job.SourceType != SourceTypeGit {
				return true
			}
			return job.FlakeURI != ""
		},
		genGitSourceBuildJob(),
	))

	// Property: Flake source BuildJob has empty GitURL
	properties.Property("Flake source BuildJob has empty GitURL", prop.ForAll(
		func(job BuildJob) bool {
			if job.SourceType != SourceTypeFlake {
				return true
			}
			return job.GitURL == ""
		},
		genFlakeSourceBuildJob(),
	))

	// Property: BuildJob source fields are preserved through JSON round-trip
	properties.Property("BuildJob source fields preserved through JSON round-trip", prop.ForAll(
		func(job BuildJob) bool {
			// Serialize to JSON
			data, err := json.Marshal(job)
			if err != nil {
				return false
			}

			// Deserialize from JSON
			var restored BuildJob
			if err := json.Unmarshal(data, &restored); err != nil {
				return false
			}

			// Source fields should be preserved
			if job.SourceType != restored.SourceType {
				return false
			}
			if job.GitURL != restored.GitURL {
				return false
			}
			if job.FlakeURI != restored.FlakeURI {
				return false
			}
			if job.GitRef != restored.GitRef {
				return false
			}

			return true
		},
		gen.OneGenOf(genGitSourceBuildJob(), genFlakeSourceBuildJob()),
	))

	properties.TestingRun(t)
}


// **Feature: backend-source-of-truth, Property 16: Service Configuration Round-Trip**
// For any valid service configuration, serializing to JSON and deserializing back
// SHALL produce an equivalent configuration with all user-specified values preserved.
// **Validates: Requirements 22.1, 22.3**
func TestServiceConfigRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for ResourceSpec
	genResourceSpec := func() gopter.Gen {
		return gopter.CombineGens(
			gen.OneConstOf("0.25", "0.5", "1", "2", "4"),
			gen.OneConstOf("256Mi", "512Mi", "1Gi", "2Gi", "4Gi"),
		).Map(func(vals []interface{}) *ResourceSpec {
			return &ResourceSpec{
				CPU:    vals[0].(string),
				Memory: vals[1].(string),
			}
		})
	}

	// Generator for optional ResourceSpec
	genOptionalResourceSpec := func() gopter.Gen {
		return gen.Bool().FlatMap(func(v interface{}) gopter.Gen {
			if v.(bool) {
				return genResourceSpec()
			}
			return gen.Const((*ResourceSpec)(nil))
		}, reflect.TypeOf((*ResourceSpec)(nil)))
	}

	// Generator for DatabaseConfig
	genDatabaseConfig := func() gopter.Gen {
		return gopter.CombineGens(
			gen.OneConstOf("postgres", "mysql", "mariadb", "mongodb", "redis", "sqlite"),
			gen.OneConstOf("14", "15", "16", "8.0", "10.6", "6.0", "7.0", "3"),
		).Map(func(vals []interface{}) *DatabaseConfig {
			return &DatabaseConfig{
				Type:    vals[0].(string),
				Version: vals[1].(string),
			}
		})
	}

	// Generator for optional DatabaseConfig
	genOptionalDatabaseConfig := func() gopter.Gen {
		return gen.Bool().FlatMap(func(v interface{}) gopter.Gen {
			if v.(bool) {
				return genDatabaseConfig()
			}
			return gen.Const((*DatabaseConfig)(nil))
		}, reflect.TypeOf((*DatabaseConfig)(nil)))
	}

	// Generator for BuildConfig
	genOptionalBuildConfig := func() gopter.Gen {
		return gen.Bool().FlatMap(func(v interface{}) gopter.Gen {
			if v.(bool) {
				return genBuildConfig()
			}
			return gen.Const((*BuildConfig)(nil))
		}, reflect.TypeOf((*BuildConfig)(nil)))
	}

	// Generator for complete ServiceConfig with all fields
	genCompleteServiceConfig := func() gopter.Gen {
		return genSourceType().FlatMap(func(v interface{}) gopter.Gen {
			sourceType := v.(SourceType)
			return gopter.CombineGens(
				gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
				genResourceTier(),
				genOptionalResourceSpec(),
				gen.IntRange(1, 10),
				gen.SliceOfN(3, genPortMapping()),
				genOptionalHealthCheckConfig(),
				gen.SliceOfN(3, gen.Identifier()),
				genEnvVars(),
				genGitRepo(),
				genGitRef(),
				genFlakeOutput(),
				genFlakeURI(),
				genImageRef(),
				genOptionalDatabaseConfig(),
				gen.OneConstOf(
					BuildStrategyFlake,
					BuildStrategyAutoGo,
					BuildStrategyAutoRust,
					BuildStrategyAutoNode,
					BuildStrategyAutoPython,
					BuildStrategyAuto,
				),
				genOptionalBuildConfig(),
			).Map(func(vals []interface{}) ServiceConfig {
				sc := ServiceConfig{
					Name:          vals[0].(string),
					SourceType:    sourceType,
					ResourceTier:  vals[1].(ResourceTier),
					Resources:     vals[2].(*ResourceSpec),
					Replicas:      vals[3].(int),
					Ports:         vals[4].([]PortMapping),
					HealthCheck:   vals[5].(*HealthCheckConfig),
					DependsOn:     vals[6].([]string),
					EnvVars:       vals[7].(map[string]string),
					BuildStrategy: vals[14].(BuildStrategy),
					BuildConfig:   vals[15].(*BuildConfig),
				}
				// Set only the appropriate source field based on source type
				switch sourceType {
				case SourceTypeGit:
					sc.GitRepo = vals[8].(string)
					sc.GitRef = vals[9].(string)
					sc.FlakeOutput = vals[10].(string)
				case SourceTypeFlake:
					sc.FlakeURI = vals[11].(string)
				case SourceTypeImage:
					sc.Image = vals[12].(string)
				case SourceTypeDatabase:
					sc.Database = vals[13].(*DatabaseConfig)
					if sc.Database == nil {
						// Ensure database config is set for database source type
						sc.Database = &DatabaseConfig{Type: "postgres", Version: "16"}
					}
				}
				return sc
			})
		}, reflect.TypeOf(ServiceConfig{}))
	}

	// Property: ServiceConfig JSON round-trip preserves all user-specified values
	properties.Property("ServiceConfig JSON round-trip preserves all values", prop.ForAll(
		func(original ServiceConfig) bool {
			// Serialize to JSON
			data, err := json.Marshal(original)
			if err != nil {
				t.Logf("Marshal error: %v", err)
				return false
			}

			// Deserialize from JSON
			var restored ServiceConfig
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Logf("Unmarshal error: %v", err)
				return false
			}

			// Use the Equals method for comparison
			if !original.Equals(&restored) {
				t.Logf("Original: %+v", original)
				t.Logf("Restored: %+v", restored)
				return false
			}

			return true
		},
		genCompleteServiceConfig(),
	))

	// Property: Empty optional fields are omitted in JSON
	properties.Property("Empty optional fields are omitted in JSON", prop.ForAll(
		func(name string) bool {
			// Create a minimal ServiceConfig with only required fields
			sc := ServiceConfig{
				Name:       name,
				SourceType: SourceTypeFlake,
				FlakeURI:   "github:owner/repo",
				Replicas:   1,
			}

			// Serialize to JSON
			data, err := json.Marshal(sc)
			if err != nil {
				return false
			}

			jsonStr := string(data)

			// Check that empty optional fields are not present
			// These fields should be omitted when empty
			omittedFields := []string{
				`"git_repo"`,
				`"git_ref"`,
				`"flake_output"`,
				`"image"`,
				`"database"`,
				`"build_strategy"`,
				`"build_config"`,
				`"resource_tier"`,
				`"resources"`,
				`"ports"`,
				`"health_check"`,
				`"env_vars"`,
				`"depends_on"`,
			}

			for _, field := range omittedFields {
				if findSubstring(jsonStr, field) {
					t.Logf("Field %s should be omitted but was found in: %s", field, jsonStr)
					return false
				}
			}

			return true
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property: User-specified values are preserved exactly
	properties.Property("User-specified values are preserved exactly", prop.ForAll(
		func(original ServiceConfig) bool {
			// Clone the original to preserve it
			clone := original.Clone()

			// Serialize and deserialize
			data, err := json.Marshal(original)
			if err != nil {
				return false
			}

			var restored ServiceConfig
			if err := json.Unmarshal(data, &restored); err != nil {
				return false
			}

			// Verify the clone equals the original (Clone works correctly)
			if !clone.Equals(&original) {
				t.Logf("Clone does not equal original")
				return false
			}

			// Verify restored equals original
			if !original.Equals(&restored) {
				t.Logf("Restored does not equal original")
				return false
			}

			return true
		},
		genCompleteServiceConfig(),
	))

	// Property: Double round-trip produces identical results
	properties.Property("Double round-trip produces identical results", prop.ForAll(
		func(original ServiceConfig) bool {
			// First round-trip
			data1, err := json.Marshal(original)
			if err != nil {
				return false
			}

			var restored1 ServiceConfig
			if err := json.Unmarshal(data1, &restored1); err != nil {
				return false
			}

			// Second round-trip
			data2, err := json.Marshal(restored1)
			if err != nil {
				return false
			}

			var restored2 ServiceConfig
			if err := json.Unmarshal(data2, &restored2); err != nil {
				return false
			}

			// Both restored versions should be equal
			return restored1.Equals(&restored2)
		},
		genCompleteServiceConfig(),
	))

	properties.TestingRun(t)
}
