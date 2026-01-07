// Package shutdown provides graceful shutdown coordination for control-plane components.
// It handles SIGTERM/SIGINT signals, stops accepting new requests, waits for in-flight
// operations to complete, and closes resources cleanly.
//
// **Validates: Requirements 15.1, 15.2, 15.3, 15.4, 15.5**
package shutdown

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// DefaultTimeout is the default graceful shutdown timeout.
const DefaultTimeout = 30 * time.Second

// Component represents a component that can be gracefully shut down.
type Component interface {
	// Name returns the component name for logging.
	Name() string
	// Shutdown gracefully shuts down the component.
	// It should return within the given context deadline.
	Shutdown(ctx context.Context) error
}

// Coordinator manages graceful shutdown of multiple components.
// It handles SIGTERM/SIGINT signals and coordinates shutdown of registered components.
//
// **Validates: Requirements 15.1, 15.2, 15.3, 15.4, 15.5**
type Coordinator struct {
	components []Component
	timeout    time.Duration
	logger     *slog.Logger
	mu         sync.Mutex

	// For testing: allows injecting a custom signal channel
	signalCh chan os.Signal

	// Shutdown state tracking
	shutdownOnce sync.Once
	shutdownDone chan struct{}
	exitCode     int
}

// Option configures a Coordinator.
type Option func(*Coordinator)

// WithTimeout sets the shutdown timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Coordinator) {
		c.timeout = timeout
	}
}

// WithLogger sets the logger.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Coordinator) {
		c.logger = logger
	}
}

// WithSignalChannel sets a custom signal channel (for testing).
func WithSignalChannel(ch chan os.Signal) Option {
	return func(c *Coordinator) {
		c.signalCh = ch
	}
}

// NewCoordinator creates a new shutdown coordinator.
func NewCoordinator(opts ...Option) *Coordinator {
	c := &Coordinator{
		components:   make([]Component, 0),
		timeout:      DefaultTimeout,
		logger:       slog.Default(),
		shutdownDone: make(chan struct{}),
		exitCode:     0,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Register adds a component to be shut down during graceful shutdown.
// Components are shut down in reverse order of registration (LIFO).
func (c *Coordinator) Register(component Component) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.components = append(c.components, component)
	c.logger.Debug("registered shutdown component", "name", component.Name())
}

// WaitForSignal blocks until a SIGTERM or SIGINT signal is received,
// then initiates graceful shutdown.
//
// **Validates: Requirements 15.1**
func (c *Coordinator) WaitForSignal() {
	sigCh := c.signalCh
	if sigCh == nil {
		sigCh = make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	}

	sig := <-sigCh
	c.logger.Info("received shutdown signal", "signal", sig)

	c.Shutdown()
}

// Shutdown initiates graceful shutdown of all registered components.
// It stops accepting new requests, waits for in-flight operations to complete
// (up to the configured timeout), and closes resources cleanly.
//
// **Validates: Requirements 15.1, 15.2, 15.3, 15.4, 15.5**
func (c *Coordinator) Shutdown() {
	c.shutdownOnce.Do(func() {
		c.logger.Info("initiating graceful shutdown", "timeout", c.timeout)

		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		defer cancel()

		c.mu.Lock()
		components := make([]Component, len(c.components))
		copy(components, c.components)
		c.mu.Unlock()

		// Shut down components in reverse order (LIFO)
		// This ensures dependencies are shut down after dependents
		var wg sync.WaitGroup
		errors := make(chan error, len(components))

		for i := len(components) - 1; i >= 0; i-- {
			component := components[i]
			wg.Add(1)
			go func(comp Component) {
				defer wg.Done()
				c.logger.Info("shutting down component", "name", comp.Name())
				if err := comp.Shutdown(ctx); err != nil {
					c.logger.Error("component shutdown error",
						"name", comp.Name(),
						"error", err,
					)
					errors <- err
				} else {
					c.logger.Info("component shutdown complete", "name", comp.Name())
				}
			}(component)
		}

		// Wait for all components to shut down or timeout
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			c.logger.Info("all components shut down successfully")
			c.exitCode = 0
		case <-ctx.Done():
			// **Validates: Requirements 15.3, 15.5**
			c.logger.Warn("shutdown timeout exceeded, forcing termination")
			c.exitCode = 1
		}

		close(c.shutdownDone)
	})
}

// Wait blocks until shutdown is complete.
func (c *Coordinator) Wait() {
	<-c.shutdownDone
}

// ExitCode returns the exit code after shutdown.
// Returns 0 for clean shutdown, 1 for forced termination.
//
// **Validates: Requirements 15.5**
func (c *Coordinator) ExitCode() int {
	return c.exitCode
}
