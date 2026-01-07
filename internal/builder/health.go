// Package builder provides build worker functionality including health checks.
package builder

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// HealthStatus represents the health status of a component.
type HealthStatus string

const (
	// HealthStatusHealthy indicates the component is fully operational.
	HealthStatusHealthy HealthStatus = "healthy"
	// HealthStatusDegraded indicates the component is operational but with issues.
	HealthStatusDegraded HealthStatus = "degraded"
	// HealthStatusUnhealthy indicates the component is not operational.
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// HealthComponentStatus represents the health status of a single component.
type HealthComponentStatus struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status     HealthStatus                     `json:"status"`
	Components map[string]HealthComponentStatus `json:"components"`
	Version    string                           `json:"version"`
	Uptime     string                           `json:"uptime"`
}

// WorkerVersion is the current version of the worker.
// This should be set at build time using ldflags.
var WorkerVersion = "dev"

// WorkerHealthChecker performs health checks for the worker.
type WorkerHealthChecker struct {
	db        *sql.DB
	startTime time.Time
	version   string
	timeout   time.Duration
	mu        sync.RWMutex
}

// NewWorkerHealthChecker creates a new worker health checker.
func NewWorkerHealthChecker(db *sql.DB, version string) *WorkerHealthChecker {
	return &WorkerHealthChecker{
		db:        db,
		startTime: time.Now(),
		version:   version,
		timeout:   5 * time.Second,
	}
}

// SetTimeout sets the timeout for health checks.
func (c *WorkerHealthChecker) SetTimeout(timeout time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timeout = timeout
}

// Check performs all health checks and returns the aggregated response.
func (c *WorkerHealthChecker) Check(ctx context.Context) *HealthResponse {
	c.mu.RLock()
	timeout := c.timeout
	c.mu.RUnlock()

	// Create a context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	components := make(map[string]HealthComponentStatus)

	// Check database connectivity
	dbStatus := c.checkDatabase(checkCtx)
	components["database"] = dbStatus

	// Check queue connectivity (uses same database)
	queueStatus := c.checkQueue(checkCtx)
	components["queue"] = queueStatus

	// Determine overall status
	overallStatus := HealthStatusHealthy
	for _, comp := range components {
		if comp.Status == HealthStatusUnhealthy {
			overallStatus = HealthStatusUnhealthy
			break
		}
		if comp.Status == HealthStatusDegraded {
			overallStatus = HealthStatusDegraded
		}
	}

	return &HealthResponse{
		Status:     overallStatus,
		Components: components,
		Version:    c.version,
		Uptime:     time.Since(c.startTime).Round(time.Second).String(),
	}
}

// checkDatabase verifies database connectivity.
func (c *WorkerHealthChecker) checkDatabase(ctx context.Context) HealthComponentStatus {
	if c.db == nil {
		return HealthComponentStatus{
			Status:  HealthStatusUnhealthy,
			Message: "database connection not configured",
		}
	}

	if err := c.db.PingContext(ctx); err != nil {
		return HealthComponentStatus{
			Status:  HealthStatusUnhealthy,
			Message: "database ping failed: " + err.Error(),
		}
	}

	return HealthComponentStatus{
		Status:  HealthStatusHealthy,
		Message: "connected",
	}
}

// checkQueue verifies queue connectivity by checking the build_queue table exists.
func (c *WorkerHealthChecker) checkQueue(ctx context.Context) HealthComponentStatus {
	if c.db == nil {
		return HealthComponentStatus{
			Status:  HealthStatusUnhealthy,
			Message: "queue connection not configured",
		}
	}

	// Check if build_queue table is accessible
	var count int
	err := c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM build_queue WHERE status = 'pending' LIMIT 1").Scan(&count)
	if err != nil {
		return HealthComponentStatus{
			Status:  HealthStatusUnhealthy,
			Message: "queue check failed: " + err.Error(),
		}
	}

	return HealthComponentStatus{
		Status:  HealthStatusHealthy,
		Message: "connected",
	}
}

// Handler returns an HTTP handler for health checks.
func (c *WorkerHealthChecker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := c.Check(r.Context())

		w.Header().Set("Content-Type", "application/json")

		// Set appropriate status code based on health
		switch response.Status {
		case HealthStatusHealthy:
			w.WriteHeader(http.StatusOK)
		case HealthStatusDegraded:
			w.WriteHeader(http.StatusOK) // Still return 200 for degraded
		case HealthStatusUnhealthy:
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(response)
	}
}
