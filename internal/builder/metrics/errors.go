// Package metrics provides build performance tracking and metrics collection.
package metrics

import "errors"

var (
	// ErrNilMetrics is returned when nil metrics are provided.
	ErrNilMetrics = errors.New("metrics cannot be nil")

	// ErrEmptyBuildID is returned when an empty build ID is provided.
	ErrEmptyBuildID = errors.New("build ID cannot be empty")

	// ErrMetricsNotFound is returned when metrics for a build are not found.
	ErrMetricsNotFound = errors.New("metrics not found for build")
)
