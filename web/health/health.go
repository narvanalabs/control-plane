// Package health provides health check functionality for the web server.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Status represents the health status of a component.
type Status string

const (
	// StatusHealthy indicates the component is fully operational.
	StatusHealthy Status = "healthy"
	// StatusDegraded indicates the component is operational but with issues.
	StatusDegraded Status = "degraded"
	// StatusUnhealthy indicates the component is not operational.
	StatusUnhealthy Status = "unhealthy"
)

// ComponentStatus represents the health status of a single component.
type ComponentStatus struct {
	Status  Status `json:"status"`
	Message string `json:"message,omitempty"`
}

// Response represents the health check response.
type Response struct {
	Status     Status                     `json:"status"`
	Components map[string]ComponentStatus `json:"components"`
	Version    string                     `json:"version"`
	Uptime     string                     `json:"uptime"`
}

// WebVersion is the current version of the web server.
// This should be set at build time using ldflags.
var WebVersion = "dev"

// APIChecker is a function that checks API connectivity.
type APIChecker func(ctx context.Context) error

// Checker performs health checks for the web server.
type Checker struct {
	apiChecker APIChecker
	startTime  time.Time
	version    string
	timeout    time.Duration
	mu         sync.RWMutex
}

// NewChecker creates a new web health checker.
func NewChecker(apiChecker APIChecker, version string) *Checker {
	return &Checker{
		apiChecker: apiChecker,
		startTime:  time.Now(),
		version:    version,
		timeout:    5 * time.Second,
	}
}

// SetTimeout sets the timeout for health checks.
func (c *Checker) SetTimeout(timeout time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timeout = timeout
}

// Check performs all health checks and returns the aggregated response.
func (c *Checker) Check(ctx context.Context) *Response {
	c.mu.RLock()
	timeout := c.timeout
	c.mu.RUnlock()

	// Create a context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	components := make(map[string]ComponentStatus)

	// Check API connectivity
	apiStatus := c.checkAPI(checkCtx)
	components["api"] = apiStatus

	// Determine overall status
	overallStatus := StatusHealthy
	for _, comp := range components {
		if comp.Status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
			break
		}
		if comp.Status == StatusDegraded {
			overallStatus = StatusDegraded
		}
	}

	return &Response{
		Status:     overallStatus,
		Components: components,
		Version:    c.version,
		Uptime:     time.Since(c.startTime).Round(time.Second).String(),
	}
}

// checkAPI verifies API server connectivity.
func (c *Checker) checkAPI(ctx context.Context) ComponentStatus {
	if c.apiChecker == nil {
		return ComponentStatus{
			Status:  StatusUnhealthy,
			Message: "API checker not configured",
		}
	}

	if err := c.apiChecker(ctx); err != nil {
		return ComponentStatus{
			Status:  StatusUnhealthy,
			Message: "API check failed: " + err.Error(),
		}
	}

	return ComponentStatus{
		Status:  StatusHealthy,
		Message: "connected",
	}
}

// Handler returns an HTTP handler for health checks.
func (c *Checker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := c.Check(r.Context())

		w.Header().Set("Content-Type", "application/json")

		// Set appropriate status code based on health
		switch response.Status {
		case StatusHealthy:
			w.WriteHeader(http.StatusOK)
		case StatusDegraded:
			w.WriteHeader(http.StatusOK) // Still return 200 for degraded
		case StatusUnhealthy:
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(response)
	}
}
