package retry

import (
	"context"
	"errors"
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
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),                                       // BuildType
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
