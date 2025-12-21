package proto

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"google.golang.org/protobuf/proto"
)

// **Feature: grpc-node-communication, Property 1: Registration Round-Trip**
// For any valid node info, serializing to protobuf and deserializing should
// produce an equivalent message.
// **Validates: Requirements 1.1, 1.2, 1.5**

// genResourceMetrics generates a random ResourceMetrics.
func genResourceMetrics() gopter.Gen {
	return gopter.CombineGens(
		gen.Float64Range(1, 64),    // cpu_total
		gen.Float64Range(0, 64),    // cpu_available
		gen.Int64Range(1<<30, 256<<30),  // memory_total
		gen.Int64Range(0, 256<<30),      // memory_available
		gen.Int64Range(1<<30, 1<<40),    // disk_total
		gen.Int64Range(0, 1<<40),        // disk_available
	).Map(func(vals []interface{}) *ResourceMetrics {
		return &ResourceMetrics{
			CpuTotal:        vals[0].(float64),
			CpuAvailable:    vals[1].(float64),
			MemoryTotal:     vals[2].(int64),
			MemoryAvailable: vals[3].(int64),
			DiskTotal:       vals[4].(int64),
			DiskAvailable:   vals[5].(int64),
		}
	})
}

// genNodeInfo generates a random NodeInfo.
func genNodeInfo() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),                                    // id
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // hostname
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // address
		gen.Int32Range(1024, 65535),                         // grpc_port
		genResourceMetrics(),                                // resources
		gen.SliceOfN(5, gen.AlphaString()),                  // cached_paths
		gen.Int32Range(0, 100),                              // available_slots
		gen.Int32Range(0, 100),                              // active_deployments
	).Map(func(vals []interface{}) *NodeInfo {
		return &NodeInfo{
			Id:                vals[0].(string),
			Hostname:          vals[1].(string),
			Address:           vals[2].(string),
			GrpcPort:          vals[3].(int32),
			Resources:         vals[4].(*ResourceMetrics),
			CachedPaths:       vals[5].([]string),
			AvailableSlots:    vals[6].(int32),
			ActiveDeployments: vals[7].(int32),
		}
	})
}

// genNodeConfig generates a random NodeConfig.
func genNodeConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.Int32Range(5, 60),   // heartbeat_interval_seconds
		gen.Int32Range(1, 100),  // max_concurrent_deployments
		gen.Int32Range(100, 10000), // log_buffer_size
	).Map(func(vals []interface{}) *NodeConfig {
		return &NodeConfig{
			HeartbeatIntervalSeconds: vals[0].(int32),
			MaxConcurrentDeployments: vals[1].(int32),
			LogBufferSize:            vals[2].(int32),
		}
	})
}

// genRegisterRequest generates a random RegisterRequest.
func genRegisterRequest() gopter.Gen {
	return gopter.CombineGens(
		genNodeInfo(),
		gen.AlphaString(), // auth_token
	).Map(func(vals []interface{}) *RegisterRequest {
		return &RegisterRequest{
			NodeInfo:  vals[0].(*NodeInfo),
			AuthToken: vals[1].(string),
		}
	})
}

// genRegisterResponse generates a random RegisterResponse.
func genRegisterResponse() gopter.Gen {
	return gopter.CombineGens(
		gen.Bool(),        // success
		gen.Identifier(),  // node_id
		gen.AlphaString(), // message
		genNodeConfig(),   // config
	).Map(func(vals []interface{}) *RegisterResponse {
		return &RegisterResponse{
			Success: vals[0].(bool),
			NodeId:  vals[1].(string),
			Message: vals[2].(string),
			Config:  vals[3].(*NodeConfig),
		}
	})
}


// TestRegisterRequestRoundTrip tests that RegisterRequest serializes and deserializes correctly.
func TestRegisterRequestRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("RegisterRequest protobuf round-trip preserves data", prop.ForAll(
		func(original *RegisterRequest) bool {
			// Serialize to protobuf
			data, err := proto.Marshal(original)
			if err != nil {
				t.Logf("Marshal error: %v", err)
				return false
			}

			// Deserialize from protobuf
			restored := &RegisterRequest{}
			if err := proto.Unmarshal(data, restored); err != nil {
				t.Logf("Unmarshal error: %v", err)
				return false
			}

			// Compare using proto.Equal
			return proto.Equal(original, restored)
		},
		genRegisterRequest(),
	))

	properties.TestingRun(t)
}

// TestRegisterResponseRoundTrip tests that RegisterResponse serializes and deserializes correctly.
func TestRegisterResponseRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("RegisterResponse protobuf round-trip preserves data", prop.ForAll(
		func(original *RegisterResponse) bool {
			// Serialize to protobuf
			data, err := proto.Marshal(original)
			if err != nil {
				t.Logf("Marshal error: %v", err)
				return false
			}

			// Deserialize from protobuf
			restored := &RegisterResponse{}
			if err := proto.Unmarshal(data, restored); err != nil {
				t.Logf("Unmarshal error: %v", err)
				return false
			}

			// Compare using proto.Equal
			return proto.Equal(original, restored)
		},
		genRegisterResponse(),
	))

	properties.TestingRun(t)
}

// TestNodeInfoRoundTrip tests that NodeInfo serializes and deserializes correctly.
func TestNodeInfoRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("NodeInfo protobuf round-trip preserves data", prop.ForAll(
		func(original *NodeInfo) bool {
			// Serialize to protobuf
			data, err := proto.Marshal(original)
			if err != nil {
				t.Logf("Marshal error: %v", err)
				return false
			}

			// Deserialize from protobuf
			restored := &NodeInfo{}
			if err := proto.Unmarshal(data, restored); err != nil {
				t.Logf("Unmarshal error: %v", err)
				return false
			}

			// Compare using proto.Equal
			return proto.Equal(original, restored)
		},
		genNodeInfo(),
	))

	properties.TestingRun(t)
}
