package shutdown

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// MockComponent is a mock implementation of the Component interface for testing.
type MockComponent struct {
	name          string
	shutdownDelay time.Duration
	shouldFail    bool
	shutdownCount int32
	mu            sync.Mutex
}

func NewMockComponent(name string, delay time.Duration, shouldFail bool) *MockComponent {
	return &MockComponent{
		name:          name,
		shutdownDelay: delay,
		shouldFail:    shouldFail,
	}
}

func (m *MockComponent) Name() string {
	return m.name
}

func (m *MockComponent) Shutdown(ctx context.Context) error {
	atomic.AddInt32(&m.shutdownCount, 1)

	select {
	case <-time.After(m.shutdownDelay):
		if m.shouldFail {
			return errors.New("mock shutdown failed")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *MockComponent) ShutdownCount() int {
	return int(atomic.LoadInt32(&m.shutdownCount))
}

// **Feature: release-changelog-cicd, Property 13: Graceful Shutdown Behavior**
// *For any* in-flight request when SIGTERM is received, the request SHALL complete
// successfully before the component terminates (within the configured timeout).
// **Validates: Requirements 15.1, 15.2**
func TestPropertyGracefulShutdownBehavior(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for shutdown timeout (100ms to 2s)
	genTimeout := gen.Int64Range(100, 2000).Map(func(ms int64) time.Duration {
		return time.Duration(ms) * time.Millisecond
	})

	// Generator for component shutdown delay (10ms to 500ms)
	genComponentDelay := gen.Int64Range(10, 500).Map(func(ms int64) time.Duration {
		return time.Duration(ms) * time.Millisecond
	})

	// Generator for number of components (1 to 5)
	genNumComponents := gen.IntRange(1, 5)

	properties.Property("All components are shut down when signal is received", prop.ForAll(
		func(timeout time.Duration, componentDelay time.Duration, numComponents int) bool {
			// Create signal channel for testing
			sigCh := make(chan os.Signal, 1)

			// Create coordinator with test signal channel
			coordinator := NewCoordinator(
				WithTimeout(timeout),
				WithSignalChannel(sigCh),
			)

			// Create and register mock components
			components := make([]*MockComponent, numComponents)
			for i := 0; i < numComponents; i++ {
				comp := NewMockComponent(
					"component-"+string(rune('A'+i)),
					componentDelay/2, // Ensure delay is less than timeout
					false,
				)
				components[i] = comp
				coordinator.Register(comp)
			}

			// Start waiting for signal in a goroutine
			done := make(chan struct{})
			go func() {
				coordinator.WaitForSignal()
				coordinator.Wait()
				close(done)
			}()

			// Give time for goroutine to start
			time.Sleep(10 * time.Millisecond)

			// Send shutdown signal
			sigCh <- os.Interrupt

			// Wait for shutdown to complete
			select {
			case <-done:
				// Verify all components were shut down
				for i, comp := range components {
					if comp.ShutdownCount() != 1 {
						t.Logf("Component %d shutdown count: %d, expected 1", i, comp.ShutdownCount())
						return false
					}
				}
				return true
			case <-time.After(timeout + 500*time.Millisecond):
				t.Log("Shutdown did not complete within expected time")
				return false
			}
		},
		genTimeout,
		genComponentDelay,
		genNumComponents,
	))

	properties.Property("Shutdown completes within timeout for fast components", prop.ForAll(
		func(timeout time.Duration, componentDelay time.Duration) bool {
			// Ensure component delay is less than timeout
			if componentDelay >= timeout {
				componentDelay = timeout / 2
			}

			// Create coordinator
			coordinator := NewCoordinator(
				WithTimeout(timeout),
			)

			// Create and register a fast mock component
			comp := NewMockComponent("fast-component", componentDelay, false)
			coordinator.Register(comp)

			// Measure shutdown time
			start := time.Now()
			coordinator.Shutdown()
			coordinator.Wait()
			elapsed := time.Since(start)

			// Verify shutdown completed within timeout
			if elapsed > timeout+100*time.Millisecond {
				t.Logf("Shutdown took %v, expected < %v", elapsed, timeout)
				return false
			}

			// Verify exit code is 0 (clean shutdown)
			if coordinator.ExitCode() != 0 {
				t.Logf("Exit code: %d, expected 0", coordinator.ExitCode())
				return false
			}

			return true
		},
		genTimeout,
		genComponentDelay,
	))

	properties.Property("Shutdown times out for slow components", prop.ForAll(
		func(timeout time.Duration) bool {
			// Create coordinator with short timeout
			coordinator := NewCoordinator(
				WithTimeout(timeout),
			)

			// Create a slow mock component that exceeds timeout
			slowDelay := timeout * 3
			comp := NewMockComponent("slow-component", slowDelay, false)
			coordinator.Register(comp)

			// Measure shutdown time
			start := time.Now()
			coordinator.Shutdown()
			coordinator.Wait()
			elapsed := time.Since(start)

			// Verify shutdown completed around the timeout (with some buffer)
			if elapsed > timeout+200*time.Millisecond {
				t.Logf("Shutdown took %v, expected around %v", elapsed, timeout)
				return false
			}

			// Verify exit code is 1 (forced termination)
			if coordinator.ExitCode() != 1 {
				t.Logf("Exit code: %d, expected 1", coordinator.ExitCode())
				return false
			}

			return true
		},
		// Use shorter timeouts (50-200ms) to make tests faster
		gen.Int64Range(50, 200).Map(func(ms int64) time.Duration {
			return time.Duration(ms) * time.Millisecond
		}),
	))

	properties.Property("Shutdown is idempotent", prop.ForAll(
		func(timeout time.Duration) bool {
			// Create coordinator
			coordinator := NewCoordinator(
				WithTimeout(timeout),
			)

			// Create and register a mock component
			comp := NewMockComponent("test-component", 10*time.Millisecond, false)
			coordinator.Register(comp)

			// Call shutdown multiple times
			coordinator.Shutdown()
			coordinator.Shutdown()
			coordinator.Shutdown()
			coordinator.Wait()

			// Verify component was only shut down once
			if comp.ShutdownCount() != 1 {
				t.Logf("Component shutdown count: %d, expected 1", comp.ShutdownCount())
				return false
			}

			return true
		},
		genTimeout,
	))

	properties.TestingRun(t)
}

// TestPropertyHTTPServerGracefulShutdown tests that HTTP servers complete in-flight
// requests during graceful shutdown.
// **Feature: release-changelog-cicd, Property 13: Graceful Shutdown Behavior**
// **Validates: Requirements 15.1, 15.2**
func TestPropertyHTTPServerGracefulShutdown(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50 // Fewer tests due to HTTP server overhead
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for request processing time (10ms to 200ms)
	genRequestTime := gen.Int64Range(10, 200).Map(func(ms int64) time.Duration {
		return time.Duration(ms) * time.Millisecond
	})

	properties.Property("In-flight HTTP requests complete during shutdown", prop.ForAll(
		func(requestTime time.Duration) bool {
			// Create a handler that takes some time to process
			var requestCompleted atomic.Bool
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(requestTime)
				requestCompleted.Store(true)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			})

			// Create HTTP server
			server := httptest.NewServer(handler)
			defer server.Close()

			// Create coordinator with sufficient timeout
			coordinator := NewCoordinator(
				WithTimeout(requestTime * 3),
			)

			// Create HTTP server component
			httpComp := NewHTTPServerComponent("test-http-server", server.Config)
			coordinator.Register(httpComp)

			// Start a request in a goroutine
			var responseReceived atomic.Bool
			var responseStatus int
			go func() {
				resp, err := http.Get(server.URL)
				if err == nil {
					responseStatus = resp.StatusCode
					resp.Body.Close()
					responseReceived.Store(true)
				}
			}()

			// Give time for request to start
			time.Sleep(5 * time.Millisecond)

			// Initiate shutdown while request is in-flight
			coordinator.Shutdown()
			coordinator.Wait()

			// Give a bit more time for the response to be received
			time.Sleep(requestTime + 50*time.Millisecond)

			// Verify request completed
			if !requestCompleted.Load() {
				t.Log("Request did not complete")
				return false
			}

			// Verify response was received
			if !responseReceived.Load() {
				t.Log("Response was not received")
				return false
			}

			// Verify response status
			if responseStatus != http.StatusOK {
				t.Logf("Response status: %d, expected 200", responseStatus)
				return false
			}

			return true
		},
		genRequestTime,
	))

	properties.Property("Server stops accepting new connections after shutdown starts", prop.ForAll(
		func(requestTime time.Duration) bool {
			// Create a handler
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(requestTime)
				w.WriteHeader(http.StatusOK)
			})

			// Create HTTP server
			server := httptest.NewServer(handler)
			serverURL := server.URL
			defer server.Close()

			// Create coordinator
			coordinator := NewCoordinator(
				WithTimeout(requestTime * 2),
			)

			// Create HTTP server component
			httpComp := NewHTTPServerComponent("test-http-server", server.Config)
			coordinator.Register(httpComp)

			// Initiate shutdown
			coordinator.Shutdown()
			coordinator.Wait()

			// Try to make a new request after shutdown
			client := &http.Client{Timeout: 100 * time.Millisecond}
			_, err := client.Get(serverURL)

			// Request should fail (connection refused or timeout)
			if err == nil {
				t.Log("Request succeeded after shutdown, expected failure")
				return false
			}

			return true
		},
		genRequestTime,
	))

	properties.TestingRun(t)
}

// TestPropertyExitCodeBehavior tests that exit codes are set correctly.
// **Feature: release-changelog-cicd, Property 13: Graceful Shutdown Behavior**
// **Validates: Requirements 15.5**
func TestPropertyExitCodeBehavior(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	properties.Property("Exit code is 0 for clean shutdown", prop.ForAll(
		func(timeout int64) bool {
			timeoutDuration := time.Duration(timeout) * time.Millisecond

			// Create coordinator
			coordinator := NewCoordinator(
				WithTimeout(timeoutDuration),
			)

			// Create a fast component
			comp := NewMockComponent("fast-component", timeoutDuration/4, false)
			coordinator.Register(comp)

			// Shutdown
			coordinator.Shutdown()
			coordinator.Wait()

			// Verify exit code is 0
			return coordinator.ExitCode() == 0
		},
		gen.Int64Range(100, 1000),
	))

	properties.Property("Exit code is 1 for forced termination", prop.ForAll(
		func(timeout int64) bool {
			timeoutDuration := time.Duration(timeout) * time.Millisecond

			// Create coordinator with short timeout
			coordinator := NewCoordinator(
				WithTimeout(timeoutDuration),
			)

			// Create a slow component that exceeds timeout
			comp := NewMockComponent("slow-component", timeoutDuration*3, false)
			coordinator.Register(comp)

			// Shutdown
			coordinator.Shutdown()
			coordinator.Wait()

			// Verify exit code is 1
			return coordinator.ExitCode() == 1
		},
		gen.Int64Range(50, 200),
	))

	properties.TestingRun(t)
}
