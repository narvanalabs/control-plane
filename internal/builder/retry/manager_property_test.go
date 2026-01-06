package retry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: flexible-build-strategies, Property 17: Retry Termination**
// For any failed build with RetryStrategy.MaxAttempts = N, the BuildRetryManager
// SHALL attempt at most N retries.
// **Validates: Requirements 21.1, 21.2**

// genBuildJob generates random build jobs for testing.
func genBuildJob() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // ID - must be non-empty
		gen.AlphaString(), // DeploymentID
		gen.AlphaString(), // AppID
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),                                        // BuildType
		gen.OneConstOf(models.BuildStrategyFlake, models.BuildStrategyAutoGo, models.BuildStrategyAutoNode), // BuildStrategy
	).Map(func(values []interface{}) *models.BuildJob {
		return &models.BuildJob{
			ID:            values[0].(string),
			DeploymentID:  values[1].(string),
			AppID:         values[2].(string),
			BuildType:     values[3].(models.BuildType),
			BuildStrategy: values[4].(models.BuildStrategy),
			RetryCount:    0, // Start with 0 retry count for clean testing
		}
	})
}

// genRetryableError generates errors that should trigger retries.
func genRetryableError() gopter.Gen {
	return gen.OneConstOf(
		"hash mismatch in fixed-output derivation",
		"dependency resolution failed for package",
		"network error: connection reset",
		"timeout exceeded while fetching",
		"connection refused to registry",
		"unable to fetch dependency",
		"failed to download artifact",
	).Map(func(msg string) error {
		return errors.New(msg)
	})
}

// genNonRetryableError generates errors that should NOT trigger retries.
func genNonRetryableError() gopter.Gen {
	return gen.OneConstOf(
		"syntax error in source code",
		"compilation failed",
		"invalid configuration",
		"permission denied",
		"file not found",
	).Map(func(msg string) error {
		return errors.New(msg)
	})
}

// genOCIFallbackError generates errors that should trigger OCI fallback.
func genOCIFallbackError() gopter.Gen {
	return gen.OneConstOf(
		"native dependency not found",
		"unsupported platform for binary",
		"binary not found in PATH",
		"linking failed: undefined reference",
		"undefined reference to symbol",
		"cannot find -lssl",
		"pkg-config not found",
		"cmake is required",
		"autoconf failed",
	).Map(func(msg string) error {
		return errors.New(msg)
	})
}

// genMaxAttempts generates valid max attempt values.
func genMaxAttempts() gopter.Gen {
	return gen.IntRange(1, 10)
}

func TestRetryTermination(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: ShouldRetry returns false when attempts >= MaxAttempts
	properties.Property("ShouldRetry returns false when max attempts reached", prop.ForAll(
		func(maxAttempts int, job *models.BuildJob) bool {
			strategy := &RetryStrategy{
				MaxAttempts:     maxAttempts,
				RetryAsOCI:      true,
				RetryableErrors: retryableErrorPatterns,
				BackoffDuration: time.Second,
			}
			manager := NewManager(WithRetryStrategy(strategy))

			// Record maxAttempts attempts
			for i := 0; i < maxAttempts; i++ {
				manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
					AttemptNumber: i + 1,
					Strategy:      job.BuildStrategy,
					BuildType:     job.BuildType,
					StartedAt:     time.Now(),
					Success:       false,
					Error:         "test error",
				})
			}

			// ShouldRetry should return false after max attempts
			err := errors.New("hash mismatch error")
			return !manager.ShouldRetry(context.Background(), job, err)
		},
		genMaxAttempts(),
		genBuildJob(),
	))

	// Property: ShouldRetry returns true when attempts < MaxAttempts for retryable errors
	properties.Property("ShouldRetry returns true when under max attempts for retryable errors", prop.ForAll(
		func(maxAttempts int, job *models.BuildJob, err error) bool {
			if maxAttempts < 2 {
				maxAttempts = 2 // Ensure we have room for at least one retry
			}

			strategy := &RetryStrategy{
				MaxAttempts:     maxAttempts,
				RetryAsOCI:      true,
				RetryableErrors: retryableErrorPatterns,
				BackoffDuration: time.Second,
			}
			manager := NewManager(WithRetryStrategy(strategy))

			// Record fewer than maxAttempts attempts
			attemptsToRecord := maxAttempts - 1
			for i := 0; i < attemptsToRecord; i++ {
				manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
					AttemptNumber: i + 1,
					Strategy:      job.BuildStrategy,
					BuildType:     job.BuildType,
					StartedAt:     time.Now(),
					Success:       false,
					Error:         "previous error",
				})
			}

			// ShouldRetry should return true for retryable errors
			return manager.ShouldRetry(context.Background(), job, err)
		},
		genMaxAttempts(),
		genBuildJob(),
		genRetryableError(),
	))

	// Property: Total attempts never exceed MaxAttempts
	properties.Property("total attempts never exceed MaxAttempts", prop.ForAll(
		func(maxAttempts int, job *models.BuildJob, numAttempts int) bool {
			strategy := &RetryStrategy{
				MaxAttempts:     maxAttempts,
				RetryAsOCI:      true,
				RetryableErrors: retryableErrorPatterns,
				BackoffDuration: time.Second,
			}
			manager := NewManager(WithRetryStrategy(strategy))

			// Simulate multiple retry attempts
			retryCount := 0
			for i := 0; i < numAttempts; i++ {
				err := errors.New("hash mismatch error")
				if manager.ShouldRetry(context.Background(), job, err) {
					retryCount++
					manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
						AttemptNumber: i + 1,
						Strategy:      job.BuildStrategy,
						BuildType:     job.BuildType,
						StartedAt:     time.Now(),
						Success:       false,
						Error:         err.Error(),
					})
				}
			}

			// Total recorded attempts should never exceed MaxAttempts
			return len(manager.GetAttempts(job.ID)) <= maxAttempts
		},
		genMaxAttempts(),
		genBuildJob(),
		gen.IntRange(1, 20), // Try up to 20 times
	))

	properties.TestingRun(t)
}

func TestRetryableErrorDetection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	manager := NewManager()

	// Property: Retryable errors are correctly identified
	properties.Property("IsRetryableError returns true for retryable errors", prop.ForAll(
		func(err error) bool {
			return manager.IsRetryableError(err)
		},
		genRetryableError(),
	))

	// Property: Non-retryable errors are correctly rejected
	properties.Property("IsRetryableError returns false for non-retryable errors", prop.ForAll(
		func(err error) bool {
			return !manager.IsRetryableError(err)
		},
		genNonRetryableError(),
	))

	// Property: Nil error is not retryable
	properties.Property("IsRetryableError returns false for nil", prop.ForAll(
		func(_ int) bool {
			return !manager.IsRetryableError(nil)
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}

func TestPrepareRetryIncrementsCount(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: PrepareRetry increments retry count
	properties.Property("PrepareRetry increments retry count by 1", prop.ForAll(
		func(job *models.BuildJob) bool {
			manager := NewManager()
			originalCount := job.RetryCount

			retryJob, err := manager.PrepareRetry(context.Background(), job)
			if err != nil {
				return false
			}

			return retryJob.RetryCount == originalCount+1
		},
		genBuildJob(),
	))

	properties.TestingRun(t)
}

func TestAttemptRecording(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: RecordAttempt stores attempts correctly
	properties.Property("RecordAttempt stores and retrieves attempts correctly", prop.ForAll(
		func(buildID string, numAttempts int) bool {
			if buildID == "" {
				return true // Skip empty build IDs
			}
			if numAttempts < 1 {
				numAttempts = 1
			}
			if numAttempts > 10 {
				numAttempts = 10
			}

			manager := NewManager()

			// Record multiple attempts
			for i := 0; i < numAttempts; i++ {
				attempt := BuildAttempt{
					AttemptNumber: i + 1,
					Strategy:      models.BuildStrategyAutoGo,
					BuildType:     models.BuildTypePureNix,
					StartedAt:     time.Now(),
					Success:       false,
					Error:         "test error",
				}
				err := manager.RecordAttempt(context.Background(), buildID, attempt)
				if err != nil {
					return false
				}
			}

			// Verify all attempts were recorded
			attempts := manager.GetAttempts(buildID)
			return len(attempts) == numAttempts
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(1, 10),
	))

	// Property: GetLastAttempt returns the most recent attempt
	properties.Property("GetLastAttempt returns the most recent attempt", prop.ForAll(
		func(buildID string, numAttempts int) bool {
			if buildID == "" {
				return true
			}
			if numAttempts < 1 {
				numAttempts = 1
			}
			if numAttempts > 10 {
				numAttempts = 10
			}

			manager := NewManager()

			// Record multiple attempts
			for i := 0; i < numAttempts; i++ {
				attempt := BuildAttempt{
					AttemptNumber: i + 1,
					Strategy:      models.BuildStrategyAutoGo,
					BuildType:     models.BuildTypePureNix,
					StartedAt:     time.Now(),
					Success:       i == numAttempts-1, // Last one is successful
					Error:         "test error",
				}
				manager.RecordAttempt(context.Background(), buildID, attempt)
			}

			// GetLastAttempt should return the last recorded attempt
			lastAttempt := manager.GetLastAttempt(buildID)
			if lastAttempt == nil {
				return false
			}
			return lastAttempt.AttemptNumber == numAttempts
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(1, 10),
	))

	properties.TestingRun(t)
}

func TestClearAttempts(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: ClearAttempts removes all attempts for a build
	properties.Property("ClearAttempts removes all attempts", prop.ForAll(
		func(buildID string, numAttempts int) bool {
			if buildID == "" {
				return true
			}
			if numAttempts < 1 {
				numAttempts = 1
			}

			manager := NewManager()

			// Record some attempts
			for i := 0; i < numAttempts; i++ {
				manager.RecordAttempt(context.Background(), buildID, BuildAttempt{
					AttemptNumber: i + 1,
					Strategy:      models.BuildStrategyAutoGo,
					BuildType:     models.BuildTypePureNix,
					StartedAt:     time.Now(),
				})
			}

			// Clear attempts
			manager.ClearAttempts(buildID)

			// Verify attempts are cleared
			return len(manager.GetAttempts(buildID)) == 0
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(1, 10),
	))

	properties.TestingRun(t)
}

func TestManagerConfiguration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: WithRetryStrategy sets the correct strategy
	properties.Property("WithRetryStrategy configures manager correctly", prop.ForAll(
		func(maxAttempts int, backoffSeconds int) bool {
			if maxAttempts < 1 {
				maxAttempts = 1
			}
			if backoffSeconds < 1 {
				backoffSeconds = 1
			}

			strategy := &RetryStrategy{
				MaxAttempts:     maxAttempts,
				RetryAsOCI:      true,
				RetryableErrors: retryableErrorPatterns,
				BackoffDuration: time.Duration(backoffSeconds) * time.Second,
			}

			manager := NewManager(WithRetryStrategy(strategy))

			return manager.GetMaxAttempts() == maxAttempts &&
				manager.GetBackoffDuration() == time.Duration(backoffSeconds)*time.Second
		},
		gen.IntRange(1, 10),
		gen.IntRange(1, 60),
	))

	properties.TestingRun(t)
}

// **Feature: flexible-build-strategies, Property 14: OCI Fallback Notification**
// For any build that falls back from pure-nix to OCI, the Build_System SHALL
// notify the user and log the reason for fallback.
// **Validates: Requirements 21.3**

func TestOCIFallbackNotification(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: OCI fallback errors are correctly identified
	properties.Property("IsOCIFallbackError returns true for OCI fallback errors", prop.ForAll(
		func(err error) bool {
			manager := NewManager()
			return manager.IsOCIFallbackError(err)
		},
		genOCIFallbackError(),
	))

	// Property: Non-OCI-fallback errors are correctly rejected
	properties.Property("IsOCIFallbackError returns false for non-fallback errors", prop.ForAll(
		func(err error) bool {
			manager := NewManager()
			return !manager.IsOCIFallbackError(err)
		},
		genNonRetryableError(),
	))

	// Property: PrepareRetry switches to OCI for fallback errors on pure-nix builds
	properties.Property("PrepareRetry switches to OCI for fallback errors on pure-nix builds", prop.ForAll(
		func(jobID string, errMsg string) bool {
			if jobID == "" {
				return true // Skip empty job IDs
			}

			job := &models.BuildJob{
				ID:            jobID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategyAutoGo,
				RetryCount:    0,
			}

			// Record an attempt with an OCI fallback error
			manager := NewManager()
			manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
				AttemptNumber: 1,
				Strategy:      job.BuildStrategy,
				BuildType:     job.BuildType,
				StartedAt:     time.Now(),
				Success:       false,
				Error:         errMsg,
			})

			retryJob, err := manager.PrepareRetry(context.Background(), job)
			if err != nil {
				return false
			}

			// Should switch to OCI
			return retryJob.BuildType == models.BuildTypeOCI && retryJob.RetryAsOCI
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			"native dependency not found",
			"unsupported platform for binary",
			"linking failed: undefined reference",
			"cannot find -lssl",
			"pkg-config not found",
		),
	))

	// Property: PrepareRetry does NOT switch to OCI for non-fallback errors
	properties.Property("PrepareRetry does not switch to OCI for non-fallback errors", prop.ForAll(
		func(jobID string, errMsg string) bool {
			if jobID == "" {
				return true // Skip empty job IDs
			}

			job := &models.BuildJob{
				ID:            jobID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategyAutoGo,
				RetryCount:    0,
			}

			// Record an attempt with a non-fallback error
			manager := NewManager()
			manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
				AttemptNumber: 1,
				Strategy:      job.BuildStrategy,
				BuildType:     job.BuildType,
				StartedAt:     time.Now(),
				Success:       false,
				Error:         errMsg,
			})

			retryJob, err := manager.PrepareRetry(context.Background(), job)
			if err != nil {
				return false
			}

			// Should NOT switch to OCI
			return retryJob.BuildType == models.BuildTypePureNix && !retryJob.RetryAsOCI
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			"syntax error in source code",
			"compilation failed",
			"invalid configuration",
			"permission denied",
		),
	))

	// Property: Notification callback is called when fallback occurs
	properties.Property("notification callback is called on OCI fallback", prop.ForAll(
		func(jobID string, errMsg string) bool {
			if jobID == "" {
				return true // Skip empty job IDs
			}

			job := &models.BuildJob{
				ID:            jobID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategyAutoGo,
				RetryCount:    0,
			}

			// Track if notification was called
			notificationCalled := false
			var receivedNotification *FallbackNotification

			manager := NewManager(WithNotificationCallback(func(n *FallbackNotification) {
				notificationCalled = true
				receivedNotification = n
			}))

			// Record an attempt with an OCI fallback error
			manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
				AttemptNumber: 1,
				Strategy:      job.BuildStrategy,
				BuildType:     job.BuildType,
				StartedAt:     time.Now(),
				Success:       false,
				Error:         errMsg,
			})

			_, err := manager.PrepareRetry(context.Background(), job)
			if err != nil {
				return false
			}

			// Notification should have been called with correct data
			if !notificationCalled {
				return false
			}
			if receivedNotification == nil {
				return false
			}
			if receivedNotification.BuildID != jobID {
				return false
			}
			if receivedNotification.OriginalType != "pure-nix" {
				return false
			}
			if receivedNotification.FallbackType != "oci" {
				return false
			}
			if receivedNotification.Reason != errMsg {
				return false
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			"native dependency not found",
			"linking failed: undefined reference",
			"pkg-config not found",
		),
	))

	// Property: Notification callback is NOT called when no fallback occurs
	properties.Property("notification callback is not called when no fallback", prop.ForAll(
		func(jobID string, errMsg string) bool {
			if jobID == "" {
				return true // Skip empty job IDs
			}

			job := &models.BuildJob{
				ID:            jobID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategyAutoGo,
				RetryCount:    0,
			}

			// Track if notification was called
			notificationCalled := false

			manager := NewManager(WithNotificationCallback(func(n *FallbackNotification) {
				notificationCalled = true
			}))

			// Record an attempt with a non-fallback error
			manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
				AttemptNumber: 1,
				Strategy:      job.BuildStrategy,
				BuildType:     job.BuildType,
				StartedAt:     time.Now(),
				Success:       false,
				Error:         errMsg,
			})

			_, err := manager.PrepareRetry(context.Background(), job)
			if err != nil {
				return false
			}

			// Notification should NOT have been called
			return !notificationCalled
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			"syntax error in source code",
			"compilation failed",
			"invalid configuration",
		),
	))

	// Property: OCI builds do not trigger fallback
	properties.Property("OCI builds do not trigger fallback notification", prop.ForAll(
		func(jobID string, errMsg string) bool {
			if jobID == "" {
				return true // Skip empty job IDs
			}

			job := &models.BuildJob{
				ID:            jobID,
				BuildType:     models.BuildTypeOCI, // Already OCI
				BuildStrategy: models.BuildStrategyDockerfile,
				RetryCount:    0,
			}

			// Track if notification was called
			notificationCalled := false

			manager := NewManager(WithNotificationCallback(func(n *FallbackNotification) {
				notificationCalled = true
			}))

			// Record an attempt with an OCI fallback error (but job is already OCI)
			manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
				AttemptNumber: 1,
				Strategy:      job.BuildStrategy,
				BuildType:     job.BuildType,
				StartedAt:     time.Now(),
				Success:       false,
				Error:         errMsg,
			})

			retryJob, err := manager.PrepareRetry(context.Background(), job)
			if err != nil {
				return false
			}

			// Should stay OCI and no notification
			return retryJob.BuildType == models.BuildTypeOCI && !notificationCalled
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			"native dependency not found",
			"linking failed: undefined reference",
		),
	))

	properties.TestingRun(t)
}

// TestFallbackNotificationContent tests the content of fallback notifications.
func TestFallbackNotificationContent(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Notification contains correct timestamp
	properties.Property("notification timestamp is recent", prop.ForAll(
		func(jobID string) bool {
			if jobID == "" {
				return true
			}

			job := &models.BuildJob{
				ID:            jobID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategyAutoGo,
				RetryCount:    0,
			}

			var receivedNotification *FallbackNotification
			beforeTime := time.Now()

			manager := NewManager(WithNotificationCallback(func(n *FallbackNotification) {
				receivedNotification = n
			}))

			manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
				AttemptNumber: 1,
				Strategy:      job.BuildStrategy,
				BuildType:     job.BuildType,
				StartedAt:     time.Now(),
				Success:       false,
				Error:         "native dependency not found",
			})

			manager.PrepareRetry(context.Background(), job)
			afterTime := time.Now()

			if receivedNotification == nil {
				return false
			}

			// Timestamp should be between before and after
			return !receivedNotification.Timestamp.Before(beforeTime) &&
				!receivedNotification.Timestamp.After(afterTime)
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property: Notification attempt number matches retry count
	properties.Property("notification attempt number is correct", prop.ForAll(
		func(jobID string, initialRetryCount int) bool {
			if jobID == "" {
				return true
			}
			if initialRetryCount < 0 {
				initialRetryCount = 0
			}
			if initialRetryCount > 5 {
				initialRetryCount = 5
			}

			job := &models.BuildJob{
				ID:            jobID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategyAutoGo,
				RetryCount:    initialRetryCount,
			}

			var receivedNotification *FallbackNotification

			manager := NewManager(WithNotificationCallback(func(n *FallbackNotification) {
				receivedNotification = n
			}))

			manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
				AttemptNumber: 1,
				Strategy:      job.BuildStrategy,
				BuildType:     job.BuildType,
				StartedAt:     time.Now(),
				Success:       false,
				Error:         "native dependency not found",
			})

			manager.PrepareRetry(context.Background(), job)

			if receivedNotification == nil {
				return false
			}

			// Attempt number should be initialRetryCount + 1
			return receivedNotification.AttemptNumber == initialRetryCount+1
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(0, 5),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 25: Validation Error Non-Retry**
// For any build that fails due to validation error, the Build_System SHALL NOT retry the build.
// **Validates: Requirements 15.7**

// genValidationError generates errors that are validation errors and should NOT trigger retries.
func genValidationError() gopter.Gen {
	return gen.OneGenOf(
		// Wrapped ErrValidationFailed
		gen.Const(fmt.Errorf("%w: missing required field", ErrValidationFailed)),
		gen.Const(fmt.Errorf("%w: invalid build type", ErrValidationFailed)),
		gen.Const(fmt.Errorf("%w: negative timeout", ErrValidationFailed)),
		// Error messages containing validation patterns
		gen.Const(errors.New("validation failed: missing id")),
		gen.Const(errors.New("validation error: invalid configuration")),
		gen.Const(errors.New("required field is missing")),
		gen.Const(errors.New("invalid value for build_type")),
		gen.Const(errors.New("negative value for timeout")),
	)
}

func TestValidationErrorNonRetry(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: IsValidationError returns true for validation errors
	properties.Property("IsValidationError returns true for validation errors", prop.ForAll(
		func(err error) bool {
			return IsValidationError(err)
		},
		genValidationError(),
	))

	// Property: IsValidationError returns false for non-validation errors
	properties.Property("IsValidationError returns false for non-validation errors", prop.ForAll(
		func(err error) bool {
			return !IsValidationError(err)
		},
		genRetryableError(),
	))

	// Property: IsValidationError returns false for nil
	properties.Property("IsValidationError returns false for nil", prop.ForAll(
		func(_ int) bool {
			return !IsValidationError(nil)
		},
		gen.Int(),
	))

	// Property: ShouldRetry returns false for validation errors regardless of retry count
	properties.Property("ShouldRetry returns false for validation errors", prop.ForAll(
		func(job *models.BuildJob, err error) bool {
			manager := NewManager()
			// ShouldRetry should always return false for validation errors
			return !manager.ShouldRetry(context.Background(), job, err)
		},
		genBuildJob(),
		genValidationError(),
	))

	// Property: ShouldRetry returns false for validation errors even with no prior attempts
	properties.Property("ShouldRetry returns false for validation errors even with no prior attempts", prop.ForAll(
		func(job *models.BuildJob) bool {
			manager := NewManager()
			// Create a fresh job with no retry history
			job.RetryCount = 0

			// Test with various validation errors
			validationErrors := []error{
				fmt.Errorf("%w: test", ErrValidationFailed),
				errors.New("validation failed: test"),
				errors.New("validation error: test"),
				errors.New("required field missing"),
				errors.New("invalid value"),
			}

			for _, err := range validationErrors {
				if manager.ShouldRetry(context.Background(), job, err) {
					return false
				}
			}
			return true
		},
		genBuildJob(),
	))

	// Property: ShouldRetry returns false for validation errors even with RetryAsOCI enabled
	properties.Property("ShouldRetry returns false for validation errors even with RetryAsOCI", prop.ForAll(
		func(job *models.BuildJob, err error) bool {
			strategy := &RetryStrategy{
				MaxAttempts:     10, // High max attempts
				RetryAsOCI:      true,
				RetryableErrors: retryableErrorPatterns,
				BackoffDuration: time.Second,
			}
			manager := NewManager(WithRetryStrategy(strategy))

			// Even with RetryAsOCI enabled and high max attempts, validation errors should not retry
			return !manager.ShouldRetry(context.Background(), job, err)
		},
		genBuildJob(),
		genValidationError(),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 26: Max Retry Enforcement**
// For any build job with retry_count >= MaxRetries (default 2), the Build_System
// SHALL NOT retry and SHALL mark as permanently failed.
// **Validates: Requirements 15.5**

func TestMaxRetryEnforcement(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: ShouldRetry returns false when job.RetryCount >= MaxRetries
	properties.Property("ShouldRetry returns false when retry count >= MaxRetries", prop.ForAll(
		func(job *models.BuildJob, retryCount int) bool {
			// Ensure retry count is at or above MaxRetries
			if retryCount < MaxRetries {
				retryCount = MaxRetries
			}
			job.RetryCount = retryCount

			manager := NewManager()
			err := errors.New("hash mismatch error") // A retryable error

			// ShouldRetry should return false when retry count >= MaxRetries
			return !manager.ShouldRetry(context.Background(), job, err)
		},
		genBuildJob(),
		gen.IntRange(MaxRetries, MaxRetries+10),
	))

	// Property: ShouldRetry returns true when job.RetryCount < MaxRetries for retryable errors
	properties.Property("ShouldRetry returns true when retry count < MaxRetries for retryable errors", prop.ForAll(
		func(job *models.BuildJob, err error) bool {
			// Ensure retry count is below MaxRetries
			job.RetryCount = 0

			manager := NewManager()

			// ShouldRetry should return true for retryable errors when under max retries
			return manager.ShouldRetry(context.Background(), job, err)
		},
		genBuildJob(),
		genRetryableError(),
	))

	// Property: MaxRetries constant equals 2 (as per requirements)
	properties.Property("MaxRetries constant equals 2", prop.ForAll(
		func(_ int) bool {
			return MaxRetries == 2
		},
		gen.Int(),
	))

	// Property: ShouldRetry returns false when recorded attempts >= strategy.MaxAttempts
	properties.Property("ShouldRetry returns false when recorded attempts >= MaxAttempts", prop.ForAll(
		func(job *models.BuildJob, maxAttempts int) bool {
			if maxAttempts < 1 {
				maxAttempts = 1
			}
			if maxAttempts > 10 {
				maxAttempts = 10
			}

			strategy := &RetryStrategy{
				MaxAttempts:     maxAttempts,
				RetryAsOCI:      true,
				RetryableErrors: retryableErrorPatterns,
				BackoffDuration: time.Second,
			}
			manager := NewManager(WithRetryStrategy(strategy))

			// Record maxAttempts attempts
			for i := 0; i < maxAttempts; i++ {
				manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
					AttemptNumber: i + 1,
					Strategy:      job.BuildStrategy,
					BuildType:     job.BuildType,
					StartedAt:     time.Now(),
					Success:       false,
					Error:         "test error",
				})
			}

			err := errors.New("hash mismatch error") // A retryable error
			return !manager.ShouldRetry(context.Background(), job, err)
		},
		genBuildJob(),
		gen.IntRange(1, 10),
	))

	// Property: Both job.RetryCount and recorded attempts are checked
	properties.Property("both job.RetryCount and recorded attempts are checked", prop.ForAll(
		func(job *models.BuildJob) bool {
			manager := NewManager()

			// Case 1: job.RetryCount >= MaxRetries, no recorded attempts
			job.RetryCount = MaxRetries
			err := errors.New("hash mismatch error")
			if manager.ShouldRetry(context.Background(), job, err) {
				return false // Should not retry
			}

			// Case 2: job.RetryCount < MaxRetries, but recorded attempts >= MaxAttempts
			job.RetryCount = 0
			for i := 0; i < manager.GetMaxAttempts(); i++ {
				manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
					AttemptNumber: i + 1,
					Strategy:      job.BuildStrategy,
					BuildType:     job.BuildType,
					StartedAt:     time.Now(),
					Success:       false,
					Error:         "test error",
				})
			}
			if manager.ShouldRetry(context.Background(), job, err) {
				return false // Should not retry
			}

			return true
		},
		genBuildJob(),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 27: OCI Fallback on Retry**
// For any build job configured with AutoRetryAsOCI and retrying due to dependency error,
// the Build_System SHALL switch build_type to OCI.
// **Validates: Requirements 15.3, 15.9**

// genDependencyError generates errors that should trigger OCI fallback.
func genDependencyError() gopter.Gen {
	return gen.OneConstOf(
		"native dependency not found",
		"unsupported platform for binary",
		"binary not found in PATH",
		"linking failed: undefined reference",
		"undefined reference to symbol",
		"cannot find -lssl",
		"pkg-config not found",
		"cmake is required",
		"autoconf failed",
	).Map(func(msg string) error {
		return errors.New(msg)
	})
}

func TestOCIFallbackOnRetry(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: ShouldRetry returns true for dependency errors on pure-nix builds with RetryAsOCI
	properties.Property("ShouldRetry returns true for dependency errors on pure-nix with RetryAsOCI", prop.ForAll(
		func(job *models.BuildJob, err error) bool {
			// Ensure job is pure-nix and has no prior retries
			job.BuildType = models.BuildTypePureNix
			job.RetryCount = 0

			strategy := &RetryStrategy{
				MaxAttempts:     5,
				RetryAsOCI:      true,
				RetryableErrors: retryableErrorPatterns,
				BackoffDuration: time.Second,
			}
			manager := NewManager(WithRetryStrategy(strategy))

			// ShouldRetry should return true for dependency errors on pure-nix builds
			return manager.ShouldRetry(context.Background(), job, err)
		},
		genBuildJob(),
		genDependencyError(),
	))

	// Property: PrepareRetry switches build_type to OCI for dependency errors on pure-nix builds
	properties.Property("PrepareRetry switches to OCI for dependency errors on pure-nix", prop.ForAll(
		func(jobID string, errMsg string) bool {
			if jobID == "" {
				return true // Skip empty job IDs
			}

			job := &models.BuildJob{
				ID:            jobID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategyAutoGo,
				RetryCount:    0,
			}

			manager := NewManager()

			// Record an attempt with a dependency error
			manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
				AttemptNumber: 1,
				Strategy:      job.BuildStrategy,
				BuildType:     job.BuildType,
				StartedAt:     time.Now(),
				Success:       false,
				Error:         errMsg,
			})

			retryJob, err := manager.PrepareRetry(context.Background(), job)
			if err != nil {
				return false
			}

			// Should switch to OCI and set RetryAsOCI flag
			return retryJob.BuildType == models.BuildTypeOCI && retryJob.RetryAsOCI
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			"native dependency not found",
			"linking failed: undefined reference",
			"cannot find -lssl",
			"pkg-config not found",
		),
	))

	// Property: PrepareRetry does NOT switch to OCI for non-dependency errors
	properties.Property("PrepareRetry does not switch to OCI for non-dependency errors", prop.ForAll(
		func(jobID string, errMsg string) bool {
			if jobID == "" {
				return true // Skip empty job IDs
			}

			job := &models.BuildJob{
				ID:            jobID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategyAutoGo,
				RetryCount:    0,
			}

			manager := NewManager()

			// Record an attempt with a non-dependency error
			manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
				AttemptNumber: 1,
				Strategy:      job.BuildStrategy,
				BuildType:     job.BuildType,
				StartedAt:     time.Now(),
				Success:       false,
				Error:         errMsg,
			})

			retryJob, err := manager.PrepareRetry(context.Background(), job)
			if err != nil {
				return false
			}

			// Should NOT switch to OCI
			return retryJob.BuildType == models.BuildTypePureNix && !retryJob.RetryAsOCI
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			"hash mismatch in derivation",
			"network error: connection reset",
			"timeout exceeded",
		),
	))

	// Property: OCI builds do not trigger OCI fallback (already OCI)
	properties.Property("OCI builds do not trigger OCI fallback", prop.ForAll(
		func(jobID string, errMsg string) bool {
			if jobID == "" {
				return true // Skip empty job IDs
			}

			job := &models.BuildJob{
				ID:            jobID,
				BuildType:     models.BuildTypeOCI, // Already OCI
				BuildStrategy: models.BuildStrategyDockerfile,
				RetryCount:    0,
			}

			manager := NewManager()

			// Record an attempt with a dependency error
			manager.RecordAttempt(context.Background(), job.ID, BuildAttempt{
				AttemptNumber: 1,
				Strategy:      job.BuildStrategy,
				BuildType:     job.BuildType,
				StartedAt:     time.Now(),
				Success:       false,
				Error:         errMsg,
			})

			retryJob, err := manager.PrepareRetry(context.Background(), job)
			if err != nil {
				return false
			}

			// Should stay OCI (no change)
			return retryJob.BuildType == models.BuildTypeOCI
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			"native dependency not found",
			"linking failed: undefined reference",
		),
	))

	// Property: PrepareRetry increments retry count
	properties.Property("PrepareRetry increments retry count", prop.ForAll(
		func(job *models.BuildJob) bool {
			manager := NewManager()
			originalCount := job.RetryCount

			retryJob, err := manager.PrepareRetry(context.Background(), job)
			if err != nil {
				return false
			}

			return retryJob.RetryCount == originalCount+1
		},
		genBuildJob(),
	))

	// Property: ShouldRetry returns false for dependency errors when RetryAsOCI is disabled
	properties.Property("ShouldRetry returns false for dependency errors when RetryAsOCI disabled", prop.ForAll(
		func(job *models.BuildJob, errMsg string) bool {
			// Ensure job is pure-nix
			job.BuildType = models.BuildTypePureNix
			job.RetryCount = 0

			strategy := &RetryStrategy{
				MaxAttempts:     5,
				RetryAsOCI:      false, // Disabled
				RetryableErrors: retryableErrorPatterns,
				BackoffDuration: time.Second,
			}
			manager := NewManager(WithRetryStrategy(strategy))

			err := errors.New(errMsg)

			// ShouldRetry should return false for dependency errors when RetryAsOCI is disabled
			// (unless the error matches a retryable pattern)
			result := manager.ShouldRetry(context.Background(), job, err)

			// If the error is a retryable pattern, it should still retry
			isRetryablePattern := false
			for _, pattern := range retryableErrorPatterns {
				if strings.Contains(strings.ToLower(errMsg), strings.ToLower(pattern)) {
					isRetryablePattern = true
					break
				}
			}

			if isRetryablePattern {
				return result == true
			}
			return result == false
		},
		genBuildJob(),
		gen.OneConstOf(
			"native dependency not found",
			"linking failed: undefined reference",
			"pkg-config not found",
		),
	))

	properties.TestingRun(t)
}
