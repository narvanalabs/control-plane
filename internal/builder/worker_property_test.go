package builder

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

// **Feature: flexible-build-strategies, Property 11: Build Timeout Enforcement**
// For any build job, if the build exceeds the configured timeout, the Build_System
// SHALL terminate the build and return a timeout error.
// **Validates: Requirements 17.1, 17.2**

// genPositiveTimeout generates positive timeout values in seconds.
func genPositiveTimeout() gopter.Gen {
	return gen.IntRange(1, 7200) // 1 second to 2 hours
}

// genBuildJobWithTimeout generates a BuildJob with various timeout configurations.
func genBuildJobWithTimeout() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),                    // job ID
		gen.Identifier(),                    // deployment ID
		gen.IntRange(0, 3600),               // job timeout seconds (0 means not set)
		gen.IntRange(0, 3600),               // build config timeout (0 means not set)
	).Map(func(vals []interface{}) *models.BuildJob {
		jobID := vals[0].(string)
		deploymentID := vals[1].(string)
		jobTimeout := vals[2].(int)
		configTimeout := vals[3].(int)

		job := &models.BuildJob{
			ID:             jobID,
			DeploymentID:   deploymentID,
			BuildType:      models.BuildTypePureNix,
			TimeoutSeconds: jobTimeout,
		}

		if configTimeout > 0 {
			job.BuildConfig = &models.BuildConfig{
				BuildTimeout: configTimeout,
			}
		}

		return job
	})
}

// TestBuildTimeoutEnforcement tests that build timeouts are properly enforced.
func TestBuildTimeoutEnforcement(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: GetBuildTimeout returns a positive duration for any job
	properties.Property("GetBuildTimeout always returns positive duration", prop.ForAll(
		func(job *models.BuildJob) bool {
			timeout := GetBuildTimeout(job, DefaultBuildTimeout)
			return timeout > 0
		},
		genBuildJobWithTimeout(),
	))

	// Property: Job-specific timeout takes precedence over config timeout
	properties.Property("job timeout takes precedence over config timeout", prop.ForAll(
		func(jobTimeout, configTimeout int) bool {
			if jobTimeout <= 0 {
				return true // Skip when job timeout is not set
			}

			job := &models.BuildJob{
				ID:             "test-job",
				TimeoutSeconds: jobTimeout,
				BuildConfig: &models.BuildConfig{
					BuildTimeout: configTimeout,
				},
			}

			timeout := GetBuildTimeout(job, DefaultBuildTimeout)
			expected := time.Duration(jobTimeout) * time.Second
			return timeout == expected
		},
		gen.IntRange(1, 3600),
		gen.IntRange(1, 3600),
	))

	// Property: Config timeout is used when job timeout is not set
	properties.Property("config timeout used when job timeout not set", prop.ForAll(
		func(configTimeout int) bool {
			if configTimeout <= 0 {
				return true // Skip when config timeout is not set
			}

			job := &models.BuildJob{
				ID:             "test-job",
				TimeoutSeconds: 0, // Not set
				BuildConfig: &models.BuildConfig{
					BuildTimeout: configTimeout,
				},
			}

			timeout := GetBuildTimeout(job, DefaultBuildTimeout)
			expected := time.Duration(configTimeout) * time.Second
			return timeout == expected
		},
		gen.IntRange(1, 3600),
	))

	// Property: Default timeout is used when neither job nor config timeout is set
	properties.Property("default timeout used when no timeout configured", prop.ForAll(
		func(defaultTimeout int) bool {
			if defaultTimeout <= 0 {
				return true // Skip invalid defaults
			}

			job := &models.BuildJob{
				ID:             "test-job",
				TimeoutSeconds: 0, // Not set
				BuildConfig:    nil,
			}

			timeout := GetBuildTimeout(job, defaultTimeout)
			expected := time.Duration(defaultTimeout) * time.Second
			return timeout == expected
		},
		gen.IntRange(1, 3600),
	))

	// Property: Global default is used when all other timeouts are zero
	properties.Property("global default used when all timeouts zero", prop.ForAll(
		func(job *models.BuildJob) bool {
			// Force all timeouts to zero
			job.TimeoutSeconds = 0
			if job.BuildConfig != nil {
				job.BuildConfig.BuildTimeout = 0
			}

			timeout := GetBuildTimeout(job, 0)
			expected := time.Duration(DefaultBuildTimeout) * time.Second
			return timeout == expected
		},
		genBuildJobWithTimeout(),
	))

	properties.TestingRun(t)
}

// TestBuildTimeoutError tests that timeout errors are properly identified.
func TestBuildTimeoutError(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: IsBuildTimeoutError returns true for ErrBuildTimeout
	properties.Property("IsBuildTimeoutError identifies timeout errors", prop.ForAll(
		func(msg string) bool {
			// Create a wrapped timeout error
			err := errors.Join(ErrBuildTimeout, errors.New(msg))
			return IsBuildTimeoutError(err)
		},
		gen.AlphaString(),
	))

	// Property: IsBuildTimeoutError returns false for non-timeout errors
	properties.Property("IsBuildTimeoutError rejects non-timeout errors", prop.ForAll(
		func(msg string) bool {
			if msg == "" {
				return true // Skip empty messages
			}
			err := errors.New(msg)
			return !IsBuildTimeoutError(err)
		},
		gen.AlphaString().SuchThat(func(s string) bool {
			return s != "" && s != "build exceeded timeout limit"
		}),
	))

	properties.TestingRun(t)
}

// TestTimeoutContextCancellation tests that context cancellation works correctly.
func TestTimeoutContextCancellation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50
	properties := gopter.NewProperties(parameters)

	// Property: Context with timeout cancels after the specified duration
	properties.Property("context cancels after timeout", prop.ForAll(
		func(timeoutMs int) bool {
			if timeoutMs < 10 || timeoutMs > 500 {
				return true // Skip very short or long timeouts for test speed
			}

			timeout := time.Duration(timeoutMs) * time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			// Wait for context to be done
			select {
			case <-ctx.Done():
				return ctx.Err() == context.DeadlineExceeded
			case <-time.After(timeout + 100*time.Millisecond):
				return false // Context should have been cancelled by now
			}
		},
		gen.IntRange(10, 500),
	))

	properties.TestingRun(t)
}

// TestTimeoutPrecedence tests the timeout precedence order.
func TestTimeoutPrecedence(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Timeout precedence is: job > config > worker default > global default
	properties.Property("timeout precedence order is correct", prop.ForAll(
		func(jobTimeout, configTimeout, workerDefault int) bool {
			job := &models.BuildJob{
				ID:             "test-job",
				TimeoutSeconds: jobTimeout,
			}
			if configTimeout > 0 {
				job.BuildConfig = &models.BuildConfig{
					BuildTimeout: configTimeout,
				}
			}

			timeout := GetBuildTimeout(job, workerDefault)

			// Determine expected timeout based on precedence
			var expected time.Duration
			if jobTimeout > 0 {
				expected = time.Duration(jobTimeout) * time.Second
			} else if configTimeout > 0 {
				expected = time.Duration(configTimeout) * time.Second
			} else if workerDefault > 0 {
				expected = time.Duration(workerDefault) * time.Second
			} else {
				expected = time.Duration(DefaultBuildTimeout) * time.Second
			}

			return timeout == expected
		},
		gen.IntRange(0, 3600),
		gen.IntRange(0, 3600),
		gen.IntRange(0, 3600),
	))

	properties.TestingRun(t)
}


// **Feature: flexible-build-strategies, Property 20: Build Validation Completeness**
// For any ServiceConfig, the BuildValidator SHALL check all required fields and
// return specific errors for each invalid field.
// **Validates: Requirements 10.1**

// genValidBuildJob generates a valid BuildJob.
func genValidBuildJob() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),                    // job ID
		gen.Identifier(),                    // deployment ID
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
	).Map(func(vals []interface{}) *models.BuildJob {
		return &models.BuildJob{
			ID:           vals[0].(string),
			DeploymentID: vals[1].(string),
			BuildType:    vals[2].(models.BuildType),
		}
	})
}

// genInvalidBuildJob generates BuildJobs with various invalid configurations.
func genInvalidBuildJob() gopter.Gen {
	return gen.Weighted([]gen.WeightedGen{
		// Missing ID
		{Weight: 2, Gen: gen.Identifier().Map(func(deploymentID string) *models.BuildJob {
			return &models.BuildJob{
				ID:           "",
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
			}
		})},
		// Missing deployment ID
		{Weight: 2, Gen: gen.Identifier().Map(func(jobID string) *models.BuildJob {
			return &models.BuildJob{
				ID:           jobID,
				DeploymentID: "",
				BuildType:    models.BuildTypePureNix,
			}
		})},
		// Missing build type
		{Weight: 2, Gen: gopter.CombineGens(
			gen.Identifier(),
			gen.Identifier(),
		).Map(func(vals []interface{}) *models.BuildJob {
			return &models.BuildJob{
				ID:           vals[0].(string),
				DeploymentID: vals[1].(string),
				BuildType:    "",
			}
		})},
		// Invalid build type
		{Weight: 2, Gen: gopter.CombineGens(
			gen.Identifier(),
			gen.Identifier(),
		).Map(func(vals []interface{}) *models.BuildJob {
			return &models.BuildJob{
				ID:           vals[0].(string),
				DeploymentID: vals[1].(string),
				BuildType:    models.BuildType("invalid-type"),
			}
		})},
		// Invalid build strategy
		{Weight: 2, Gen: gopter.CombineGens(
			gen.Identifier(),
			gen.Identifier(),
		).Map(func(vals []interface{}) *models.BuildJob {
			return &models.BuildJob{
				ID:            vals[0].(string),
				DeploymentID:  vals[1].(string),
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategy("invalid-strategy"),
			}
		})},
		// Negative timeout
		{Weight: 2, Gen: gopter.CombineGens(
			gen.Identifier(),
			gen.Identifier(),
			gen.IntRange(-1000, -1),
		).Map(func(vals []interface{}) *models.BuildJob {
			return &models.BuildJob{
				ID:             vals[0].(string),
				DeploymentID:   vals[1].(string),
				BuildType:      models.BuildTypePureNix,
				TimeoutSeconds: vals[2].(int),
			}
		})},
	})
}

// genBuildStrategy generates valid build strategies.
func genBuildStrategy() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStrategyFlake,
		models.BuildStrategyAutoGo,
		models.BuildStrategyAutoRust,
		models.BuildStrategyAutoNode,
		models.BuildStrategyAutoPython,
		models.BuildStrategyDockerfile,
		models.BuildStrategyNixpacks,
		models.BuildStrategyAuto,
	)
}

// TestBuildValidationCompleteness tests that validation checks all required fields.
func TestBuildValidationCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Valid build jobs pass validation
	properties.Property("valid build jobs pass validation", prop.ForAll(
		func(job *models.BuildJob) bool {
			result, err := ValidateBuildJob(context.Background(), job)
			if err != nil {
				return false
			}
			return result.Valid
		},
		genValidBuildJob(),
	))

	// Property: Invalid build jobs fail validation with specific errors
	properties.Property("invalid build jobs fail validation", prop.ForAll(
		func(job *models.BuildJob) bool {
			result, err := ValidateBuildJob(context.Background(), job)
			if err != nil {
				return false
			}
			// Should not be valid
			if result.Valid {
				return false
			}
			// Should have at least one error
			return len(result.Errors) > 0
		},
		genInvalidBuildJob(),
	))

	// Property: Missing ID produces specific error
	properties.Property("missing ID produces specific error", prop.ForAll(
		func(deploymentID string) bool {
			job := &models.BuildJob{
				ID:           "",
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
			}
			result, err := ValidateBuildJob(context.Background(), job)
			if err != nil {
				return false
			}
			// Should have error for ID field
			for _, e := range result.Errors {
				if e.Field == "id" {
					return true
				}
			}
			return false
		},
		gen.Identifier(),
	))

	// Property: Missing deployment ID produces specific error
	properties.Property("missing deployment ID produces specific error", prop.ForAll(
		func(jobID string) bool {
			job := &models.BuildJob{
				ID:           jobID,
				DeploymentID: "",
				BuildType:    models.BuildTypePureNix,
			}
			result, err := ValidateBuildJob(context.Background(), job)
			if err != nil {
				return false
			}
			// Should have error for deployment_id field
			for _, e := range result.Errors {
				if e.Field == "deployment_id" {
					return true
				}
			}
			return false
		},
		gen.Identifier(),
	))

	// Property: Missing build type produces specific error
	properties.Property("missing build type produces specific error", prop.ForAll(
		func(jobID, deploymentID string) bool {
			job := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    "",
			}
			result, err := ValidateBuildJob(context.Background(), job)
			if err != nil {
				return false
			}
			// Should have error for build_type field
			for _, e := range result.Errors {
				if e.Field == "build_type" {
					return true
				}
			}
			return false
		},
		gen.Identifier(),
		gen.Identifier(),
	))

	// Property: Invalid build strategy produces specific error
	properties.Property("invalid build strategy produces specific error", prop.ForAll(
		func(jobID, deploymentID string) bool {
			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategy("invalid-strategy"),
			}
			result, err := ValidateBuildJob(context.Background(), job)
			if err != nil {
				return false
			}
			// Should have error for build_strategy field
			for _, e := range result.Errors {
				if e.Field == "build_strategy" {
					return true
				}
			}
			return false
		},
		gen.Identifier(),
		gen.Identifier(),
	))

	// Property: Valid build strategies pass validation
	properties.Property("valid build strategies pass validation", prop.ForAll(
		func(jobID, deploymentID string, strategy models.BuildStrategy) bool {
			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: strategy,
			}
			result, err := ValidateBuildJob(context.Background(), job)
			if err != nil {
				return false
			}
			// Should not have error for build_strategy field
			for _, e := range result.Errors {
				if e.Field == "build_strategy" {
					return false
				}
			}
			return true
		},
		gen.Identifier(),
		gen.Identifier(),
		genBuildStrategy(),
	))

	properties.TestingRun(t)
}

// TestValidationErrorFields tests that validation errors have proper field information.
func TestValidationErrorFields(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: All validation errors have non-empty field names
	properties.Property("validation errors have non-empty field names", prop.ForAll(
		func(job *models.BuildJob) bool {
			result, err := ValidateBuildJob(context.Background(), job)
			if err != nil {
				return false
			}
			for _, e := range result.Errors {
				if e.Field == "" {
					return false
				}
			}
			return true
		},
		genInvalidBuildJob(),
	))

	// Property: All validation errors have non-empty messages
	properties.Property("validation errors have non-empty messages", prop.ForAll(
		func(job *models.BuildJob) bool {
			result, err := ValidateBuildJob(context.Background(), job)
			if err != nil {
				return false
			}
			for _, e := range result.Errors {
				if e.Message == "" {
					return false
				}
			}
			return true
		},
		genInvalidBuildJob(),
	))

	// Property: All validation errors have non-empty codes
	properties.Property("validation errors have non-empty codes", prop.ForAll(
		func(job *models.BuildJob) bool {
			result, err := ValidateBuildJob(context.Background(), job)
			if err != nil {
				return false
			}
			for _, e := range result.Errors {
				if e.Code == "" {
					return false
				}
			}
			return true
		},
		genInvalidBuildJob(),
	))

	properties.TestingRun(t)
}

// TestDockerfileNixpacksOCIWarning tests that dockerfile/nixpacks strategies produce warnings for non-OCI builds.
func TestDockerfileNixpacksOCIWarning(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Dockerfile strategy with non-OCI build type produces warning
	properties.Property("dockerfile strategy with pure-nix produces warning", prop.ForAll(
		func(jobID, deploymentID string) bool {
			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategyDockerfile,
			}
			result, err := ValidateBuildJob(context.Background(), job)
			if err != nil {
				return false
			}
			// Should have a warning about OCI requirement
			return len(result.Warnings) > 0
		},
		gen.Identifier(),
		gen.Identifier(),
	))

	// Property: Nixpacks strategy with non-OCI build type produces warning
	properties.Property("nixpacks strategy with pure-nix produces warning", prop.ForAll(
		func(jobID, deploymentID string) bool {
			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategyNixpacks,
			}
			result, err := ValidateBuildJob(context.Background(), job)
			if err != nil {
				return false
			}
			// Should have a warning about OCI requirement
			return len(result.Warnings) > 0
		},
		gen.Identifier(),
		gen.Identifier(),
	))

	// Property: Dockerfile strategy with OCI build type produces no warning
	properties.Property("dockerfile strategy with OCI produces no OCI warning", prop.ForAll(
		func(jobID, deploymentID string) bool {
			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				BuildType:     models.BuildTypeOCI,
				BuildStrategy: models.BuildStrategyDockerfile,
			}
			result, err := ValidateBuildJob(context.Background(), job)
			if err != nil {
				return false
			}
			// Should not have a warning about OCI requirement
			for _, w := range result.Warnings {
				if contains(w, "OCI") || contains(w, "oci") {
					return false
				}
			}
			return true
		},
		gen.Identifier(),
		gen.Identifier(),
	))

	properties.TestingRun(t)
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
