package models

import (
	"encoding/json"
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

// genServiceConfig generates a random ServiceConfig.
func genServiceConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
		genResourceTier(),
		gen.IntRange(1, 10),
		gen.SliceOfN(2, genPortMapping()),
		genOptionalHealthCheckConfig(),
		gen.SliceOfN(2, gen.AlphaString()),
	).Map(func(vals []interface{}) ServiceConfig {
		return ServiceConfig{
			Name:         vals[0].(string),
			FlakeOutput:  vals[1].(string),
			ResourceTier: vals[2].(ResourceTier),
			Replicas:     vals[3].(int),
			Ports:        vals[4].([]PortMapping),
			HealthCheck:  vals[5].(*HealthCheckConfig),
			DependsOn:    vals[6].([]string),
		}
	})
}


// genApp generates a random App.
func genApp() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),
		gen.Identifier(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
		genBuildType(),
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
			BuildType:   vals[4].(BuildType),
			Services:    vals[5].([]ServiceConfig),
			CreatedAt:   vals[6].(time.Time),
			UpdatedAt:   vals[7].(time.Time),
			DeletedAt:   vals[8].(*time.Time),
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
