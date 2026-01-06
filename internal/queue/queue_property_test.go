package queue

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: control-plane, Property 7: Build job JSON round-trip**
// For any valid build job, serializing to JSON and deserializing should
// produce an equivalent build job structure.
// **Validates: Requirements 3.6, 3.7**

// genBuildType generates a random BuildType.
func genBuildType() gopter.Gen {
	return gen.OneConstOf(models.BuildTypeOCI, models.BuildTypePureNix)
}

// genBuildStatus generates a random BuildStatus.
func genBuildStatus() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStatusQueued,
		models.BuildStatusRunning,
		models.BuildStatusSucceeded,
		models.BuildStatusFailed,
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

// genBuildJob generates a random BuildJob.
func genBuildJob() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(), // ID
		gen.Identifier(), // DeploymentID
		gen.Identifier(), // AppID
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // GitURL
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // GitRef
		gen.AlphaString(), // FlakeOutput
		genBuildType(),    // BuildType
		genBuildStatus(),  // Status
		genTime(),         // CreatedAt
		genOptionalTime(), // StartedAt
		genOptionalTime(), // FinishedAt
	).Map(func(vals []interface{}) models.BuildJob {
		return models.BuildJob{
			ID:           vals[0].(string),
			DeploymentID: vals[1].(string),
			AppID:        vals[2].(string),
			GitURL:       vals[3].(string),
			GitRef:       vals[4].(string),
			FlakeOutput:  vals[5].(string),
			BuildType:    vals[6].(models.BuildType),
			Status:       vals[7].(models.BuildStatus),
			CreatedAt:    vals[8].(time.Time),
			StartedAt:    vals[9].(*time.Time),
			FinishedAt:   vals[10].(*time.Time),
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

// TestBuildJobJSONRoundTrip tests that BuildJob serializes and deserializes correctly.
// This validates Property 7: Build job JSON round-trip
// **Validates: Requirements 3.6, 3.7**
func TestBuildJobJSONRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("BuildJob JSON round-trip preserves data", prop.ForAll(
		func(original models.BuildJob) bool {
			// Serialize to JSON (as done in Enqueue)
			data, err := json.Marshal(original)
			if err != nil {
				return false
			}

			// Deserialize from JSON (as done in Dequeue)
			var restored models.BuildJob
			if err := json.Unmarshal(data, &restored); err != nil {
				return false
			}

			// Compare via JSON (handles empty vs nil equivalence)
			return jsonEqual(original, restored)
		},
		genBuildJob(),
	))

	properties.TestingRun(t)
}
