package shutdown

import (
	"context"
	"io"
	"net/http"
)

// HTTPServerComponent wraps an http.Server for graceful shutdown.
//
// **Validates: Requirements 15.1, 15.2, 15.3**
type HTTPServerComponent struct {
	name   string
	server *http.Server
}

// NewHTTPServerComponent creates a new HTTP server shutdown component.
func NewHTTPServerComponent(name string, server *http.Server) *HTTPServerComponent {
	return &HTTPServerComponent{
		name:   name,
		server: server,
	}
}

// Name returns the component name.
func (c *HTTPServerComponent) Name() string {
	return c.name
}

// Shutdown gracefully shuts down the HTTP server.
// It stops accepting new connections and waits for in-flight requests to complete.
//
// **Validates: Requirements 15.1, 15.2, 15.3**
func (c *HTTPServerComponent) Shutdown(ctx context.Context) error {
	return c.server.Shutdown(ctx)
}

// CloserComponent wraps an io.Closer for graceful shutdown.
//
// **Validates: Requirements 15.4**
type CloserComponent struct {
	name   string
	closer io.Closer
}

// NewCloserComponent creates a new closer shutdown component.
func NewCloserComponent(name string, closer io.Closer) *CloserComponent {
	return &CloserComponent{
		name:   name,
		closer: closer,
	}
}

// Name returns the component name.
func (c *CloserComponent) Name() string {
	return c.name
}

// Shutdown closes the underlying resource.
//
// **Validates: Requirements 15.4**
func (c *CloserComponent) Shutdown(ctx context.Context) error {
	return c.closer.Close()
}

// FuncComponent wraps a shutdown function as a component.
type FuncComponent struct {
	name string
	fn   func(ctx context.Context) error
}

// NewFuncComponent creates a new function-based shutdown component.
func NewFuncComponent(name string, fn func(ctx context.Context) error) *FuncComponent {
	return &FuncComponent{
		name: name,
		fn:   fn,
	}
}

// Name returns the component name.
func (c *FuncComponent) Name() string {
	return c.name
}

// Shutdown calls the wrapped function.
func (c *FuncComponent) Shutdown(ctx context.Context) error {
	return c.fn(ctx)
}

// GRPCServerShutdowner is the interface for gRPC servers that can be gracefully stopped.
type GRPCServerShutdowner interface {
	GracefulStop()
}

// GRPCServerComponent wraps a gRPC server for graceful shutdown.
type GRPCServerComponent struct {
	name   string
	server GRPCServerShutdowner
}

// NewGRPCServerComponent creates a new gRPC server shutdown component.
func NewGRPCServerComponent(name string, server GRPCServerShutdowner) *GRPCServerComponent {
	return &GRPCServerComponent{
		name:   name,
		server: server,
	}
}

// Name returns the component name.
func (c *GRPCServerComponent) Name() string {
	return c.name
}

// Shutdown gracefully stops the gRPC server.
func (c *GRPCServerComponent) Shutdown(ctx context.Context) error {
	// GracefulStop blocks until all RPCs are finished
	// We run it in a goroutine and respect the context deadline
	done := make(chan struct{})
	go func() {
		c.server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WorkerShutdowner is the interface for workers that can be stopped.
type WorkerShutdowner interface {
	Stop()
}

// WorkerComponent wraps a worker for graceful shutdown.
type WorkerComponent struct {
	name   string
	worker WorkerShutdowner
}

// NewWorkerComponent creates a new worker shutdown component.
func NewWorkerComponent(name string, worker WorkerShutdowner) *WorkerComponent {
	return &WorkerComponent{
		name:   name,
		worker: worker,
	}
}

// Name returns the component name.
func (c *WorkerComponent) Name() string {
	return c.name
}

// Shutdown stops the worker and waits for in-progress jobs to complete.
func (c *WorkerComponent) Shutdown(ctx context.Context) error {
	// Worker.Stop() already waits for in-progress jobs
	done := make(chan struct{})
	go func() {
		c.worker.Stop()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
