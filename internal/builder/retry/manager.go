// Package retry provides build retry management functionality.
package retry

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

// Retryable error patterns that indicate a build should be retried.
var retryableErrorPatterns = []string{
	"hash mismatch",
	"dependency resolution failed",
	"network error",
	"timeout",
	"connection refused",
	"unable to fetch",
	"failed to download",
}

// OCI fallback error patterns that indicate a pure-nix build should retry as OCI.
var ociFallbackErrorPatterns = []string{
	"native dependency",
	"unsupported platform",
	"binary not found",
	"linking failed",
	"undefined reference",
	"cannot find -l",
	"pkg-config",
	"cmake",
	"autoconf",
}

// BuildAttempt records a single build attempt.
type BuildAttempt struct {
	AttemptNumber int                  `json:"attempt_number"`
	Strategy      models.BuildStrategy `json:"strategy"`
	BuildType     models.BuildType     `json:"build_type"`
	StartedAt     time.Time            `json:"started_at"`
	CompletedAt   *time.Time           `json:"completed_at,omitempty"`
	Success       bool                 `json:"success"`
	Error         string               `json:"error,omitempty"`
}

// RetryStrategy defines retry behavior.
type RetryStrategy struct {
	MaxAttempts     int           `json:"max_attempts"`      // Default: 2
	RetryAsOCI      bool          `json:"retry_as_oci"`      // Try OCI on pure-nix failure
	RetryableErrors []string      `json:"retryable_errors"`  // Error codes that trigger retry
	BackoffDuration time.Duration `json:"backoff_duration"`  // Wait between retries
}

// DefaultRetryStrategy returns the default retry strategy.
func DefaultRetryStrategy() *RetryStrategy {
	return &RetryStrategy{
		MaxAttempts:     2,
		RetryAsOCI:      true,
		RetryableErrors: retryableErrorPatterns,
		BackoffDuration: 5 * time.Second,
	}
}

// FallbackNotification contains information about a build fallback.
type FallbackNotification struct {
	BuildID       string    `json:"build_id"`
	OriginalType  string    `json:"original_type"`
	FallbackType  string    `json:"fallback_type"`
	Reason        string    `json:"reason"`
	AttemptNumber int       `json:"attempt_number"`
	Timestamp     time.Time `json:"timestamp"`
}

// Manager handles build retry logic.
type Manager struct {
	strategy *RetryStrategy
	attempts map[string][]BuildAttempt // buildID -> attempts
	// NotificationCallback is called when a fallback occurs
	NotificationCallback func(notification *FallbackNotification)
}

// ManagerOption is a functional option for configuring the Manager.
type ManagerOption func(*Manager)

// WithRetryStrategy sets a custom retry strategy.
func WithRetryStrategy(strategy *RetryStrategy) ManagerOption {
	return func(m *Manager) {
		m.strategy = strategy
	}
}

// WithNotificationCallback sets the callback for fallback notifications.
func WithNotificationCallback(callback func(*FallbackNotification)) ManagerOption {
	return func(m *Manager) {
		m.NotificationCallback = callback
	}
}

// NewManager creates a new retry manager.
func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		strategy: DefaultRetryStrategy(),
		attempts: make(map[string][]BuildAttempt),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// ShouldRetry determines if a failed build should be retried.
func (m *Manager) ShouldRetry(ctx context.Context, job *models.BuildJob, err error) bool {
	if err == nil {
		return false
	}

	// Check if we've exceeded max attempts
	attempts := m.GetAttempts(job.ID)
	if len(attempts) >= m.strategy.MaxAttempts {
		return false
	}

	// Check if the error is retryable
	errStr := err.Error()
	for _, pattern := range m.strategy.RetryableErrors {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(pattern)) {
			return true
		}
	}

	// Check if we should retry as OCI (for pure-nix failures)
	if m.strategy.RetryAsOCI && job.BuildType == models.BuildTypePureNix {
		if m.shouldFallbackToOCI(errStr) {
			return true
		}
	}

	return false
}

// shouldFallbackToOCI checks if the error indicates we should try OCI instead.
func (m *Manager) shouldFallbackToOCI(errStr string) bool {
	for _, pattern := range ociFallbackErrorPatterns {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// PrepareRetry prepares a job for retry, potentially switching to OCI.
func (m *Manager) PrepareRetry(ctx context.Context, job *models.BuildJob) (*models.BuildJob, error) {
	if job == nil {
		return nil, errors.New("job cannot be nil")
	}

	// Get the last attempt to determine what failed
	attempts := m.GetAttempts(job.ID)
	var lastError string
	if len(attempts) > 0 {
		lastError = attempts[len(attempts)-1].Error
	}

	// Create a copy of the job for retry
	retryJob := *job
	retryJob.RetryCount = job.RetryCount + 1

	// Check if we should switch to OCI
	if job.BuildType == models.BuildTypePureNix && m.strategy.RetryAsOCI {
		if m.shouldFallbackToOCI(lastError) {
			retryJob.BuildType = models.BuildTypeOCI
			retryJob.RetryAsOCI = true

			// Notify about the fallback
			m.notifyFallback(job.ID, "pure-nix", "oci", lastError, retryJob.RetryCount)
		}
	}

	return &retryJob, nil
}

// notifyFallback sends a notification about a build fallback.
func (m *Manager) notifyFallback(buildID, originalType, fallbackType, reason string, attemptNumber int) {
	if m.NotificationCallback == nil {
		return
	}

	notification := &FallbackNotification{
		BuildID:       buildID,
		OriginalType:  originalType,
		FallbackType:  fallbackType,
		Reason:        reason,
		AttemptNumber: attemptNumber,
		Timestamp:     time.Now(),
	}

	m.NotificationCallback(notification)
}

// RecordAttempt records a build attempt.
func (m *Manager) RecordAttempt(ctx context.Context, buildID string, attempt BuildAttempt) error {
	if buildID == "" {
		return errors.New("buildID cannot be empty")
	}

	m.attempts[buildID] = append(m.attempts[buildID], attempt)
	return nil
}

// GetAttempts returns all recorded attempts for a build.
func (m *Manager) GetAttempts(buildID string) []BuildAttempt {
	return m.attempts[buildID]
}

// GetLastAttempt returns the most recent attempt for a build.
func (m *Manager) GetLastAttempt(buildID string) *BuildAttempt {
	attempts := m.attempts[buildID]
	if len(attempts) == 0 {
		return nil
	}
	return &attempts[len(attempts)-1]
}

// ClearAttempts clears all recorded attempts for a build.
func (m *Manager) ClearAttempts(buildID string) {
	delete(m.attempts, buildID)
}

// GetBackoffDuration returns the backoff duration for retries.
func (m *Manager) GetBackoffDuration() time.Duration {
	return m.strategy.BackoffDuration
}

// GetMaxAttempts returns the maximum number of retry attempts.
func (m *Manager) GetMaxAttempts() int {
	return m.strategy.MaxAttempts
}

// IsRetryableError checks if an error matches any retryable pattern.
func (m *Manager) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	for _, pattern := range m.strategy.RetryableErrors {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// IsOCIFallbackError checks if an error should trigger OCI fallback.
func (m *Manager) IsOCIFallbackError(err error) bool {
	if err == nil {
		return false
	}

	return m.shouldFallbackToOCI(err.Error())
}
