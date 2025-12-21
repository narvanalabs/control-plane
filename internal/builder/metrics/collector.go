// Package metrics provides build performance tracking and metrics collection.
package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

// BuildMetrics contains build performance data.
type BuildMetrics struct {
	BuildID   string               `json:"build_id"`
	Strategy  models.BuildStrategy `json:"strategy"`
	Language  string               `json:"language"`
	Framework string               `json:"framework"`

	// Timing
	DetectionTime time.Duration `json:"detection_time"`
	TemplateTime  time.Duration `json:"template_time"`
	HashCalcTime  time.Duration `json:"hash_calc_time"`
	BuildTime     time.Duration `json:"build_time"`
	PushTime      time.Duration `json:"push_time"`
	TotalTime     time.Duration `json:"total_time"`

	// Resource usage
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage int64   `json:"memory_usage"`
	DiskUsage   int64   `json:"disk_usage"`

	// Outcome
	Success      bool `json:"success"`
	CacheHit     bool `json:"cache_hit"`
	RetriedAsOCI bool `json:"retried_as_oci"`

	// Timestamps
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// MetricsFilter defines criteria for filtering aggregate metrics.
type MetricsFilter struct {
	Strategy  models.BuildStrategy `json:"strategy,omitempty"`
	Language  string               `json:"language,omitempty"`
	Framework string               `json:"framework,omitempty"`
	StartTime *time.Time           `json:"start_time,omitempty"`
	EndTime   *time.Time           `json:"end_time,omitempty"`
	Success   *bool                `json:"success,omitempty"`
}

// AggregateMetrics contains aggregated build metrics for analysis.
type AggregateMetrics struct {
	TotalBuilds      int           `json:"total_builds"`
	SuccessfulBuilds int           `json:"successful_builds"`
	FailedBuilds     int           `json:"failed_builds"`
	CacheHits        int           `json:"cache_hits"`
	OCIRetries       int           `json:"oci_retries"`
	SuccessRate      float64       `json:"success_rate"`
	CacheHitRate     float64       `json:"cache_hit_rate"`
	AvgBuildTime     time.Duration `json:"avg_build_time"`
	AvgTotalTime     time.Duration `json:"avg_total_time"`
	MaxBuildTime     time.Duration `json:"max_build_time"`
	MinBuildTime     time.Duration `json:"min_build_time"`

	// Breakdown by strategy
	ByStrategy map[models.BuildStrategy]*StrategyMetrics `json:"by_strategy,omitempty"`

	// Breakdown by language
	ByLanguage map[string]*LanguageMetrics `json:"by_language,omitempty"`
}

// StrategyMetrics contains metrics for a specific build strategy.
type StrategyMetrics struct {
	TotalBuilds      int           `json:"total_builds"`
	SuccessfulBuilds int           `json:"successful_builds"`
	AvgBuildTime     time.Duration `json:"avg_build_time"`
}

// LanguageMetrics contains metrics for a specific language.
type LanguageMetrics struct {
	TotalBuilds      int           `json:"total_builds"`
	SuccessfulBuilds int           `json:"successful_builds"`
	AvgBuildTime     time.Duration `json:"avg_build_time"`
}

// BuildMetricsCollector tracks build performance data.
type BuildMetricsCollector interface {
	// RecordMetrics records metrics for a completed build.
	RecordMetrics(ctx context.Context, metrics *BuildMetrics) error

	// GetMetrics retrieves metrics for a build.
	GetMetrics(ctx context.Context, buildID string) (*BuildMetrics, error)

	// GetAggregateMetrics retrieves aggregate metrics for analysis.
	GetAggregateMetrics(ctx context.Context, filter MetricsFilter) (*AggregateMetrics, error)
}

// Collector implements the BuildMetricsCollector interface.
type Collector struct {
	// storage holds metrics keyed by build ID.
	storage map[string]*BuildMetrics

	// mu protects the storage map.
	mu sync.RWMutex

	// retentionPeriod is how long to keep metrics.
	retentionPeriod time.Duration
}

// NewCollector creates a new Collector with default settings.
func NewCollector() *Collector {
	return &Collector{
		storage:         make(map[string]*BuildMetrics),
		retentionPeriod: 30 * 24 * time.Hour, // Default 30 days
	}
}

// CollectorOption is a functional option for configuring Collector.
type CollectorOption func(*Collector)

// WithRetentionPeriod sets the retention period for metrics.
func WithRetentionPeriod(period time.Duration) CollectorOption {
	return func(c *Collector) {
		c.retentionPeriod = period
	}
}

// NewCollectorWithOptions creates a new Collector with custom options.
func NewCollectorWithOptions(opts ...CollectorOption) *Collector {
	c := NewCollector()
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// RecordMetrics records metrics for a completed build.
func (c *Collector) RecordMetrics(ctx context.Context, metrics *BuildMetrics) error {
	if metrics == nil {
		return ErrNilMetrics
	}

	if metrics.BuildID == "" {
		return ErrEmptyBuildID
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Calculate total time if not set
	if metrics.TotalTime == 0 {
		metrics.TotalTime = metrics.DetectionTime + metrics.TemplateTime +
			metrics.HashCalcTime + metrics.BuildTime + metrics.PushTime
	}

	// Set completion time if not set
	if metrics.CompletedAt == nil {
		now := time.Now()
		metrics.CompletedAt = &now
	}

	// Store a copy to prevent external modification
	stored := *metrics
	c.storage[metrics.BuildID] = &stored

	return nil
}

// GetMetrics retrieves metrics for a build.
func (c *Collector) GetMetrics(ctx context.Context, buildID string) (*BuildMetrics, error) {
	if buildID == "" {
		return nil, ErrEmptyBuildID
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	metrics, ok := c.storage[buildID]
	if !ok {
		return nil, ErrMetricsNotFound
	}

	// Return a copy to prevent external modification
	result := *metrics
	return &result, nil
}

// GetAggregateMetrics retrieves aggregate metrics for analysis.
func (c *Collector) GetAggregateMetrics(ctx context.Context, filter MetricsFilter) (*AggregateMetrics, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	agg := &AggregateMetrics{
		ByStrategy: make(map[models.BuildStrategy]*StrategyMetrics),
		ByLanguage: make(map[string]*LanguageMetrics),
	}

	var totalBuildTime time.Duration
	var totalTotalTime time.Duration
	var buildTimeCount int

	for _, metrics := range c.storage {
		// Apply filters
		if !c.matchesFilter(metrics, filter) {
			continue
		}

		agg.TotalBuilds++

		if metrics.Success {
			agg.SuccessfulBuilds++
		} else {
			agg.FailedBuilds++
		}

		if metrics.CacheHit {
			agg.CacheHits++
		}

		if metrics.RetriedAsOCI {
			agg.OCIRetries++
		}

		// Track build times
		if metrics.BuildTime > 0 {
			totalBuildTime += metrics.BuildTime
			totalTotalTime += metrics.TotalTime
			buildTimeCount++

			if agg.MaxBuildTime == 0 || metrics.BuildTime > agg.MaxBuildTime {
				agg.MaxBuildTime = metrics.BuildTime
			}
			if agg.MinBuildTime == 0 || metrics.BuildTime < agg.MinBuildTime {
				agg.MinBuildTime = metrics.BuildTime
			}
		}

		// Aggregate by strategy
		c.aggregateByStrategy(agg, metrics)

		// Aggregate by language
		c.aggregateByLanguage(agg, metrics)
	}

	// Calculate averages and rates
	if agg.TotalBuilds > 0 {
		agg.SuccessRate = float64(agg.SuccessfulBuilds) / float64(agg.TotalBuilds)
		agg.CacheHitRate = float64(agg.CacheHits) / float64(agg.TotalBuilds)
	}

	if buildTimeCount > 0 {
		agg.AvgBuildTime = totalBuildTime / time.Duration(buildTimeCount)
		agg.AvgTotalTime = totalTotalTime / time.Duration(buildTimeCount)
	}

	return agg, nil
}

// matchesFilter checks if metrics match the given filter.
func (c *Collector) matchesFilter(metrics *BuildMetrics, filter MetricsFilter) bool {
	if filter.Strategy != "" && metrics.Strategy != filter.Strategy {
		return false
	}

	if filter.Language != "" && metrics.Language != filter.Language {
		return false
	}

	if filter.Framework != "" && metrics.Framework != filter.Framework {
		return false
	}

	if filter.StartTime != nil && metrics.StartedAt.Before(*filter.StartTime) {
		return false
	}

	if filter.EndTime != nil && metrics.StartedAt.After(*filter.EndTime) {
		return false
	}

	if filter.Success != nil && metrics.Success != *filter.Success {
		return false
	}

	return true
}

// aggregateByStrategy updates strategy-specific metrics.
func (c *Collector) aggregateByStrategy(agg *AggregateMetrics, metrics *BuildMetrics) {
	if metrics.Strategy == "" {
		return
	}

	strategyMetrics, ok := agg.ByStrategy[metrics.Strategy]
	if !ok {
		strategyMetrics = &StrategyMetrics{}
		agg.ByStrategy[metrics.Strategy] = strategyMetrics
	}

	strategyMetrics.TotalBuilds++
	if metrics.Success {
		strategyMetrics.SuccessfulBuilds++
	}

	// Update average build time (running average)
	if metrics.BuildTime > 0 {
		if strategyMetrics.AvgBuildTime == 0 {
			strategyMetrics.AvgBuildTime = metrics.BuildTime
		} else {
			// Simple running average
			strategyMetrics.AvgBuildTime = (strategyMetrics.AvgBuildTime + metrics.BuildTime) / 2
		}
	}
}

// aggregateByLanguage updates language-specific metrics.
func (c *Collector) aggregateByLanguage(agg *AggregateMetrics, metrics *BuildMetrics) {
	if metrics.Language == "" {
		return
	}

	langMetrics, ok := agg.ByLanguage[metrics.Language]
	if !ok {
		langMetrics = &LanguageMetrics{}
		agg.ByLanguage[metrics.Language] = langMetrics
	}

	langMetrics.TotalBuilds++
	if metrics.Success {
		langMetrics.SuccessfulBuilds++
	}

	// Update average build time (running average)
	if metrics.BuildTime > 0 {
		if langMetrics.AvgBuildTime == 0 {
			langMetrics.AvgBuildTime = metrics.BuildTime
		} else {
			// Simple running average
			langMetrics.AvgBuildTime = (langMetrics.AvgBuildTime + metrics.BuildTime) / 2
		}
	}
}

// CleanupExpired removes metrics older than the retention period.
func (c *Collector) CleanupExpired(ctx context.Context) int {
	if c.retentionPeriod <= 0 {
		return 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cutoff := time.Now().Add(-c.retentionPeriod)
	removed := 0

	for buildID, metrics := range c.storage {
		if metrics.CompletedAt != nil && metrics.CompletedAt.Before(cutoff) {
			delete(c.storage, buildID)
			removed++
		}
	}

	return removed
}

// GetStats returns statistics about the metrics collector.
func (c *Collector) GetStats(ctx context.Context) *CollectorStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return &CollectorStats{
		TotalMetrics:    len(c.storage),
		RetentionPeriod: c.retentionPeriod,
	}
}

// CollectorStats contains statistics about the metrics collector.
type CollectorStats struct {
	TotalMetrics    int           `json:"total_metrics"`
	RetentionPeriod time.Duration `json:"retention_period"`
}

// ListBuildIDs returns all build IDs with recorded metrics.
func (c *Collector) ListBuildIDs(ctx context.Context) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ids := make([]string, 0, len(c.storage))
	for id := range c.storage {
		ids = append(ids, id)
	}
	return ids
}

// DeleteMetrics removes metrics for a specific build.
func (c *Collector) DeleteMetrics(ctx context.Context, buildID string) error {
	if buildID == "" {
		return ErrEmptyBuildID
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.storage, buildID)
	return nil
}

// UpdateMetrics updates existing metrics for a build.
func (c *Collector) UpdateMetrics(ctx context.Context, buildID string, update func(*BuildMetrics)) error {
	if buildID == "" {
		return ErrEmptyBuildID
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	metrics, ok := c.storage[buildID]
	if !ok {
		return ErrMetricsNotFound
	}

	update(metrics)
	return nil
}
