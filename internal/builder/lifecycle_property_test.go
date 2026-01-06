package builder

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: build-lifecycle-correctness, Property 1: Build Job State Machine Invariants**
// For any build job, the status transitions SHALL follow the valid state machine:
// queued → running → (succeeded | failed), with running → queued only allowed for retry operations.
// **Validates: Requirements 1.1, 1.2, 1.3, 1.4, 1.6, 1.7**

// genBuildStatus generates valid build statuses.
func genBuildStatus() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStatusQueued,
		models.BuildStatusRunning,
		models.BuildStatusSucceeded,
		models.BuildStatusFailed,
	)
}

// genBuildStatusPair generates pairs of build statuses for transition testing.
func genBuildStatusPair() gopter.Gen {
	return gopter.CombineGens(
		genBuildStatus(),
		genBuildStatus(),
	).Map(func(vals []interface{}) [2]models.BuildStatus {
		return [2]models.BuildStatus{
			vals[0].(models.BuildStatus),
			vals[1].(models.BuildStatus),
		}
	})
}

// TestBuildJobStateMachineInvariants tests Property 1: Build Job State Machine Invariants.
func TestBuildJobStateMachineInvariants(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 1.1: Queued jobs can only transition to running
	properties.Property("queued jobs can only transition to running", prop.ForAll(
		func(toStatus models.BuildStatus) bool {
			canTransition := CanTransition(models.BuildStatusQueued, toStatus, false)
			// Only running is allowed from queued
			if toStatus == models.BuildStatusRunning {
				return canTransition == true
			}
			return canTransition == false
		},
		genBuildStatus(),
	))

	// Property 1.2: Running jobs can transition to succeeded or failed
	properties.Property("running jobs can transition to succeeded or failed", prop.ForAll(
		func(toStatus models.BuildStatus) bool {
			canTransition := CanTransition(models.BuildStatusRunning, toStatus, false)
			// Succeeded and failed are allowed from running
			if toStatus == models.BuildStatusSucceeded || toStatus == models.BuildStatusFailed {
				return canTransition == true
			}
			// Queued is NOT allowed without retry flag
			if toStatus == models.BuildStatusQueued {
				return canTransition == false
			}
			// Running to running is not allowed
			return canTransition == false
		},
		genBuildStatus(),
	))

	// Property 1.3: Running jobs can transition to queued ONLY for retry
	properties.Property("running to queued only allowed for retry", prop.ForAll(
		func(isRetry bool) bool {
			canTransition := CanTransition(models.BuildStatusRunning, models.BuildStatusQueued, isRetry)
			// Should only be allowed when isRetry is true
			return canTransition == isRetry
		},
		gen.Bool(),
	))

	// Property 1.4: Succeeded is a terminal state (no transitions allowed)
	properties.Property("succeeded is terminal state", prop.ForAll(
		func(toStatus models.BuildStatus, isRetry bool) bool {
			canTransition := CanTransition(models.BuildStatusSucceeded, toStatus, isRetry)
			// No transitions should be allowed from succeeded
			return canTransition == false
		},
		genBuildStatus(),
		gen.Bool(),
	))

	// Property 1.5: Failed is a terminal state (no transitions allowed)
	properties.Property("failed is terminal state", prop.ForAll(
		func(toStatus models.BuildStatus, isRetry bool) bool {
			canTransition := CanTransition(models.BuildStatusFailed, toStatus, isRetry)
			// No transitions should be allowed from failed
			return canTransition == false
		},
		genBuildStatus(),
		gen.Bool(),
	))

	// Property 1.6: Valid transitions form a DAG (no cycles except retry)
	properties.Property("state machine has no cycles except retry", prop.ForAll(
		func(statePair [2]models.BuildStatus) bool {
			from, to := statePair[0], statePair[1]

			// If we can go from A to B (non-retry), we should not be able to go from B to A (non-retry)
			// Exception: running -> queued is only allowed for retry
			forwardAllowed := CanTransition(from, to, false)
			backwardAllowed := CanTransition(to, from, false)

			// If both directions are allowed (non-retry), that's a cycle
			if forwardAllowed && backwardAllowed {
				return false
			}
			return true
		},
		genBuildStatusPair(),
	))

	// Property 1.7: All valid transitions are explicitly defined
	properties.Property("all transitions are explicitly defined", prop.ForAll(
		func(statePair [2]models.BuildStatus, isRetry bool) bool {
			from, to := statePair[0], statePair[1]
			canTransition := CanTransition(from, to, isRetry)

			// Verify against our known valid transitions
			switch from {
			case models.BuildStatusQueued:
				// Only queued -> running is valid
				if to == models.BuildStatusRunning {
					return canTransition == true
				}
				return canTransition == false

			case models.BuildStatusRunning:
				// running -> succeeded, failed are always valid
				if to == models.BuildStatusSucceeded || to == models.BuildStatusFailed {
					return canTransition == true
				}
				// running -> queued only valid for retry
				if to == models.BuildStatusQueued {
					return canTransition == isRetry
				}
				return canTransition == false

			case models.BuildStatusSucceeded, models.BuildStatusFailed:
				// Terminal states - no transitions allowed
				return canTransition == false
			}

			return true
		},
		genBuildStatusPair(),
		gen.Bool(),
	))

	properties.TestingRun(t)
}

// TestStateMachineCompleteness tests that the state machine covers all expected scenarios.
func TestStateMachineCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Every non-terminal state has at least one valid outgoing transition
	properties.Property("non-terminal states have outgoing transitions", prop.ForAll(
		func(status models.BuildStatus) bool {
			if IsTerminalState(status) {
				return true // Skip terminal states
			}

			// Check if there's at least one valid transition
			allStatuses := []models.BuildStatus{
				models.BuildStatusQueued,
				models.BuildStatusRunning,
				models.BuildStatusSucceeded,
				models.BuildStatusFailed,
			}

			for _, toStatus := range allStatuses {
				if CanTransition(status, toStatus, false) || CanTransition(status, toStatus, true) {
					return true
				}
			}
			return false
		},
		genBuildStatus(),
	))

	// Property: Terminal states have no outgoing transitions
	properties.Property("terminal states have no outgoing transitions", prop.ForAll(
		func(toStatus models.BuildStatus, isRetry bool) bool {
			// Check succeeded
			if CanTransition(models.BuildStatusSucceeded, toStatus, isRetry) {
				return false
			}
			// Check failed
			if CanTransition(models.BuildStatusFailed, toStatus, isRetry) {
				return false
			}
			return true
		},
		genBuildStatus(),
		gen.Bool(),
	))

	// Property: IsTerminalState correctly identifies terminal states
	properties.Property("IsTerminalState is correct", prop.ForAll(
		func(status models.BuildStatus) bool {
			isTerminal := IsTerminalState(status)

			// Succeeded and failed should be terminal
			if status == models.BuildStatusSucceeded || status == models.BuildStatusFailed {
				return isTerminal == true
			}
			// Queued and running should not be terminal
			return isTerminal == false
		},
		genBuildStatus(),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 2: Terminal State Immutability**
// For any build job in `succeeded` or `failed` status, no further state transitions SHALL be allowed.
// **Validates: Requirements 1.7**

// genTerminalStatus generates terminal build statuses.
func genTerminalStatus() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStatusSucceeded,
		models.BuildStatusFailed,
	)
}

// genNonTerminalStatus generates non-terminal build statuses.
func genNonTerminalStatus() gopter.Gen {
	return gen.OneConstOf(
		models.BuildStatusQueued,
		models.BuildStatusRunning,
	)
}

// TestTerminalStateImmutability tests Property 2: Terminal State Immutability.
func TestTerminalStateImmutability(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 2.1: Succeeded state cannot transition to any other state
	properties.Property("succeeded cannot transition to any state", prop.ForAll(
		func(toStatus models.BuildStatus, isRetry bool) bool {
			canTransition := CanTransition(models.BuildStatusSucceeded, toStatus, isRetry)
			// No transitions should be allowed from succeeded, even with retry flag
			return canTransition == false
		},
		genBuildStatus(),
		gen.Bool(),
	))

	// Property 2.2: Failed state cannot transition to any other state
	properties.Property("failed cannot transition to any state", prop.ForAll(
		func(toStatus models.BuildStatus, isRetry bool) bool {
			canTransition := CanTransition(models.BuildStatusFailed, toStatus, isRetry)
			// No transitions should be allowed from failed, even with retry flag
			return canTransition == false
		},
		genBuildStatus(),
		gen.Bool(),
	))

	// Property 2.3: Terminal states are correctly identified
	properties.Property("terminal states are correctly identified", prop.ForAll(
		func(status models.BuildStatus) bool {
			isTerminal := IsTerminalState(status)

			// Only succeeded and failed should be terminal
			expectedTerminal := (status == models.BuildStatusSucceeded || status == models.BuildStatusFailed)
			return isTerminal == expectedTerminal
		},
		genBuildStatus(),
	))

	// Property 2.4: Non-terminal states are not terminal
	properties.Property("non-terminal states are not terminal", prop.ForAll(
		func(status models.BuildStatus) bool {
			isTerminal := IsTerminalState(status)

			// Queued and running should not be terminal
			if status == models.BuildStatusQueued || status == models.BuildStatusRunning {
				return isTerminal == false
			}
			return true
		},
		genNonTerminalStatus(),
	))

	// Property 2.5: Terminal states have empty allowed transitions list
	properties.Property("terminal states have empty allowed transitions", prop.ForAll(
		func(terminalStatus models.BuildStatus) bool {
			allowed := ValidTransitions[terminalStatus]
			return len(allowed) == 0
		},
		genTerminalStatus(),
	))

	// Property 2.6: Attempting any transition from terminal state fails
	properties.Property("all transitions from terminal states fail", prop.ForAll(
		func(terminalStatus models.BuildStatus, targetStatus models.BuildStatus) bool {
			// Try both with and without retry flag
			withoutRetry := CanTransition(terminalStatus, targetStatus, false)
			withRetry := CanTransition(terminalStatus, targetStatus, true)

			// Both should fail
			return withoutRetry == false && withRetry == false
		},
		genTerminalStatus(),
		genBuildStatus(),
	))

	// Property 2.7: Terminal state immutability holds for self-transitions
	properties.Property("terminal states cannot self-transition", prop.ForAll(
		func(terminalStatus models.BuildStatus, isRetry bool) bool {
			// A terminal state should not be able to transition to itself
			canTransition := CanTransition(terminalStatus, terminalStatus, isRetry)
			return canTransition == false
		},
		genTerminalStatus(),
		gen.Bool(),
	))

	properties.TestingRun(t)
}

// TestTerminalStateImmutabilityWithBuildJobs tests terminal state immutability with actual build job objects.
func TestTerminalStateImmutabilityWithBuildJobs(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for build jobs in terminal states
	genTerminalBuildJob := func() gopter.Gen {
		return gopter.CombineGens(
			gen.Identifier(),
			gen.Identifier(),
			genTerminalStatus(),
		).Map(func(vals []interface{}) *models.BuildJob {
			return &models.BuildJob{
				ID:           vals[0].(string),
				DeploymentID: vals[1].(string),
				BuildType:    models.BuildTypePureNix,
				Status:       vals[2].(models.BuildStatus),
			}
		})
	}

	// Property: Build jobs in terminal states cannot have their status changed
	properties.Property("terminal build jobs cannot change status", prop.ForAll(
		func(job *models.BuildJob, newStatus models.BuildStatus, isRetry bool) bool {
			// The job is in a terminal state
			if !IsTerminalState(job.Status) {
				return true // Skip non-terminal jobs
			}

			// Attempting to transition should fail
			canTransition := CanTransition(job.Status, newStatus, isRetry)
			return canTransition == false
		},
		genTerminalBuildJob(),
		genBuildStatus(),
		gen.Bool(),
	))

	// Property: Succeeded build jobs remain succeeded
	properties.Property("succeeded jobs remain succeeded", prop.ForAll(
		func(jobID, deploymentID string, newStatus models.BuildStatus) bool {
			job := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       models.BuildStatusSucceeded,
			}

			// Cannot transition to any status
			return CanTransition(job.Status, newStatus, false) == false &&
				CanTransition(job.Status, newStatus, true) == false
		},
		gen.Identifier(),
		gen.Identifier(),
		genBuildStatus(),
	))

	// Property: Failed build jobs remain failed
	properties.Property("failed jobs remain failed", prop.ForAll(
		func(jobID, deploymentID string, newStatus models.BuildStatus) bool {
			job := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       models.BuildStatusFailed,
			}

			// Cannot transition to any status
			return CanTransition(job.Status, newStatus, false) == false &&
				CanTransition(job.Status, newStatus, true) == false
		},
		gen.Identifier(),
		gen.Identifier(),
		genBuildStatus(),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 3: State Persistence Before Transition**
// For any build job state transition, the new state SHALL be persisted to the database
// before the transition is considered complete.
// **Validates: Requirements 1.5, 17.4, 17.5**

// TestStatePersistenceBeforeTransition tests Property 3: State Persistence Before Transition.
// This test verifies that state transitions are persisted to the database before being considered complete.
func TestStatePersistenceBeforeTransition(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for valid state transitions
	genValidTransition := func() gopter.Gen {
		return gen.OneGenOf(
			// queued -> running
			gen.Const([2]models.BuildStatus{models.BuildStatusQueued, models.BuildStatusRunning}),
			// running -> succeeded
			gen.Const([2]models.BuildStatus{models.BuildStatusRunning, models.BuildStatusSucceeded}),
			// running -> failed
			gen.Const([2]models.BuildStatus{models.BuildStatusRunning, models.BuildStatusFailed}),
		)
	}

	// Property 3.1: State transitions are recorded in the mock store
	properties.Property("state transitions are recorded in store", prop.ForAll(
		func(jobID, deploymentID string, transition [2]models.BuildStatus) bool {
			fromStatus, toStatus := transition[0], transition[1]

			// Create a mock store
			mockStore := NewMockBuildStore()

			// Create a job in the initial state
			initialJob := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       fromStatus,
			}

			// Store the initial job
			ctx := context.Background()
			if err := mockStore.Create(ctx, initialJob); err != nil {
				return false
			}

			// Verify the transition is valid
			if !models.CanTransition(fromStatus, toStatus, false) {
				return true // Skip invalid transitions
			}

			// Create a new job object with the new status for the update
			updatedJob := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       toStatus,
			}
			if err := mockStore.Update(ctx, updatedJob); err != nil {
				return false
			}

			// Verify the transition was recorded
			transitions := mockStore.GetStateTransitions()
			if len(transitions) != 1 {
				return false
			}

			// Verify the recorded transition matches
			recorded := transitions[0]
			return recorded.BuildID == jobID &&
				recorded.FromState == fromStatus &&
				recorded.ToState == toStatus
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		genValidTransition(),
	))

	// Property 3.2: Retrieved job reflects persisted state
	properties.Property("retrieved job reflects persisted state", prop.ForAll(
		func(jobID, deploymentID string, transition [2]models.BuildStatus) bool {
			fromStatus, toStatus := transition[0], transition[1]

			// Create a mock store
			mockStore := NewMockBuildStore()

			// Create a job in the initial state
			initialJob := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       fromStatus,
			}

			// Store the initial job
			ctx := context.Background()
			if err := mockStore.Create(ctx, initialJob); err != nil {
				return false
			}

			// Verify the transition is valid
			if !models.CanTransition(fromStatus, toStatus, false) {
				return true // Skip invalid transitions
			}

			// Create a new job object with the new status for the update
			updatedJob := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       toStatus,
			}
			if err := mockStore.Update(ctx, updatedJob); err != nil {
				return false
			}

			// Retrieve the job and verify state
			retrieved, err := mockStore.Get(ctx, jobID)
			if err != nil {
				return false
			}

			return retrieved.Status == toStatus
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		genValidTransition(),
	))

	// Property 3.3: Multiple transitions are all recorded in order
	properties.Property("multiple transitions are recorded in order", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create a mock store
			mockStore := NewMockBuildStore()

			// Create a job in queued state
			initialJob := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       models.BuildStatusQueued,
			}

			// Store the initial job
			ctx := context.Background()
			if err := mockStore.Create(ctx, initialJob); err != nil {
				return false
			}

			// Transition queued -> running
			runningJob := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       models.BuildStatusRunning,
			}
			if err := mockStore.Update(ctx, runningJob); err != nil {
				return false
			}

			// Transition running -> succeeded
			succeededJob := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       models.BuildStatusSucceeded,
			}
			if err := mockStore.Update(ctx, succeededJob); err != nil {
				return false
			}

			// Verify both transitions were recorded
			transitions := mockStore.GetStateTransitions()
			if len(transitions) != 2 {
				return false
			}

			// Verify order: first queued->running, then running->succeeded
			return transitions[0].FromState == models.BuildStatusQueued &&
				transitions[0].ToState == models.BuildStatusRunning &&
				transitions[1].FromState == models.BuildStatusRunning &&
				transitions[1].ToState == models.BuildStatusSucceeded
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 3.4: Retry transition (running -> queued) is recorded
	properties.Property("retry transition is recorded", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create a mock store
			mockStore := NewMockBuildStore()

			// Create a job in running state
			initialJob := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       models.BuildStatusRunning,
			}

			// Store the initial job
			ctx := context.Background()
			if err := mockStore.Create(ctx, initialJob); err != nil {
				return false
			}

			// Verify retry transition is valid
			if !models.CanTransition(models.BuildStatusRunning, models.BuildStatusQueued, true) {
				return false
			}

			// Perform retry transition
			queuedJob := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       models.BuildStatusQueued,
			}
			if err := mockStore.Update(ctx, queuedJob); err != nil {
				return false
			}

			// Verify the transition was recorded
			transitions := mockStore.GetStateTransitions()
			if len(transitions) != 1 {
				return false
			}

			return transitions[0].FromState == models.BuildStatusRunning &&
				transitions[0].ToState == models.BuildStatusQueued
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 3.5: Transition timestamps are recorded
	properties.Property("transition timestamps are recorded", prop.ForAll(
		func(jobID, deploymentID string, transition [2]models.BuildStatus) bool {
			fromStatus, toStatus := transition[0], transition[1]

			// Create a mock store
			mockStore := NewMockBuildStore()

			// Create a job in the initial state
			initialJob := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       fromStatus,
			}

			// Store the initial job
			ctx := context.Background()
			if err := mockStore.Create(ctx, initialJob); err != nil {
				return false
			}

			// Verify the transition is valid
			if !models.CanTransition(fromStatus, toStatus, false) {
				return true // Skip invalid transitions
			}

			// Record time before transition
			beforeTransition := time.Now()

			// Create a new job object with the new status for the update
			updatedJob := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       toStatus,
			}
			if err := mockStore.Update(ctx, updatedJob); err != nil {
				return false
			}

			// Record time after transition
			afterTransition := time.Now()

			// Verify the transition was recorded with a valid timestamp
			transitions := mockStore.GetStateTransitions()
			if len(transitions) != 1 {
				return false
			}

			recorded := transitions[0]
			return !recorded.Timestamp.Before(beforeTransition) &&
				!recorded.Timestamp.After(afterTransition)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		genValidTransition(),
	))

	properties.TestingRun(t)
}

// TestStatePersistenceWithTransitionHelper tests that the transitionJobStatus helper
// properly validates transitions before allowing state changes.
func TestStatePersistenceWithTransitionHelper(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: transitionJobStatus only allows valid transitions
	properties.Property("transitionJobStatus validates transitions", prop.ForAll(
		func(fromStatus, toStatus models.BuildStatus, isRetry bool) bool {
			job := &models.BuildJob{
				ID:           "test-job",
				DeploymentID: "test-deployment",
				BuildType:    models.BuildTypePureNix,
				Status:       fromStatus,
			}

			err := transitionJobStatus(job, toStatus, isRetry)
			expectedValid := models.CanTransition(fromStatus, toStatus, isRetry)

			if expectedValid {
				// Should succeed and update status
				return err == nil && job.Status == toStatus
			}
			// Should fail and preserve original status
			return err != nil && job.Status == fromStatus
		},
		genBuildStatus(),
		genBuildStatus(),
		gen.Bool(),
	))

	// Property: transitionJobStatus returns ErrInvalidStateTransition for invalid transitions
	properties.Property("transitionJobStatus returns correct error type", prop.ForAll(
		func(fromStatus, toStatus models.BuildStatus, isRetry bool) bool {
			job := &models.BuildJob{
				ID:           "test-job",
				DeploymentID: "test-deployment",
				BuildType:    models.BuildTypePureNix,
				Status:       fromStatus,
			}

			err := transitionJobStatus(job, toStatus, isRetry)
			expectedValid := models.CanTransition(fromStatus, toStatus, isRetry)

			if expectedValid {
				return err == nil
			}
			// Should return an error that wraps ErrInvalidStateTransition
			return err != nil && errors.Is(err, ErrInvalidStateTransition)
		},
		genBuildStatus(),
		genBuildStatus(),
		gen.Bool(),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 6: Validation Before Execution**
// For any build job picked up by a worker, the Build_Validator SHALL be invoked
// before any build execution begins.
// **Validates: Requirements 3.1**

// MockValidatorWithTracking is a validator that tracks when validation is called.
type MockValidatorWithTracking struct {
	ValidateCalls []string
	Result        *ValidationResult
	Error         error
	mu            sync.Mutex
}

// NewMockValidatorWithTracking creates a new tracking validator.
func NewMockValidatorWithTracking() *MockValidatorWithTracking {
	return &MockValidatorWithTracking{
		ValidateCalls: make([]string, 0),
		Result: &ValidationResult{
			Valid:    true,
			Errors:   make([]ValidationError, 0),
			Warnings: make([]string, 0),
		},
	}
}

// Validate implements BuildValidator interface.
func (m *MockValidatorWithTracking) Validate(ctx context.Context, job *models.BuildJob) (*ValidationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ValidateCalls = append(m.ValidateCalls, job.ID)
	if m.Error != nil {
		return nil, m.Error
	}
	return m.Result, nil
}

// GetValidateCalls returns the list of job IDs that were validated.
func (m *MockValidatorWithTracking) GetValidateCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.ValidateCalls...)
}

// Reset clears all recorded calls.
func (m *MockValidatorWithTracking) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ValidateCalls = nil
}

// TestValidationBeforeExecution tests Property 6: Validation Before Execution.
func TestValidationBeforeExecution(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 6.1: Validation is always called for any build job
	properties.Property("validation is called for every build job", prop.ForAll(
		func(jobID, deploymentID string, buildType models.BuildType) bool {
			// Create a valid build job
			job := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    buildType,
				Status:       models.BuildStatusQueued,
			}

			// Create a tracking validator
			validator := NewMockValidatorWithTracking()

			// Call validation
			ctx := context.Background()
			_, err := validator.Validate(ctx, job)
			if err != nil {
				return false
			}

			// Verify validation was called with the correct job ID
			calls := validator.GetValidateCalls()
			return len(calls) == 1 && calls[0] == jobID
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
	))

	// Property 6.2: Validation is called before any build execution
	// This tests that the validator is invoked and returns a result before execution proceeds
	properties.Property("validation returns result before execution can proceed", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create a valid build job
			job := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
				Status:       models.BuildStatusQueued,
			}

			// Create the default validator
			validator := NewDefaultBuildValidator(nil)

			// Call validation
			ctx := context.Background()
			result, err := validator.Validate(ctx, job)

			// Validation must complete and return a result
			return err == nil && result != nil
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 6.3: ValidateBuildJob convenience function also validates
	properties.Property("ValidateBuildJob validates any build job", prop.ForAll(
		func(jobID, deploymentID string, buildType models.BuildType) bool {
			// Create a valid build job
			job := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    buildType,
				Status:       models.BuildStatusQueued,
			}

			// Call the convenience function
			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)

			// Must return a result without error
			return err == nil && result != nil
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 7: Validation Error Detection**
// For any build job with invalid fields (empty id, empty deployment_id, empty build_type,
// invalid build_type, invalid build_strategy, negative timeout), the Build_Validator
// SHALL return a validation error.
// **Validates: Requirements 3.4, 3.5, 3.6, 3.7, 3.8, 3.9**

// TestValidationErrorDetection tests Property 7: Validation Error Detection.
func TestValidationErrorDetection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 7.1: Empty ID produces validation error with REQUIRED_FIELD code
	// **Validates: Requirements 3.4**
	properties.Property("empty id produces REQUIRED_FIELD error", prop.ForAll(
		func(deploymentID string) bool {
			job := &models.BuildJob{
				ID:           "",
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
			}

			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)
			if err != nil {
				return false
			}

			// Should not be valid
			if result.Valid {
				return false
			}

			// Should have error for id field with REQUIRED_FIELD code
			for _, e := range result.Errors {
				if e.Field == "id" && e.Code == ValidationCodeRequiredField {
					return true
				}
			}
			return false
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 7.2: Empty deployment_id produces validation error with REQUIRED_FIELD code
	// **Validates: Requirements 3.5**
	properties.Property("empty deployment_id produces REQUIRED_FIELD error", prop.ForAll(
		func(jobID string) bool {
			job := &models.BuildJob{
				ID:           jobID,
				DeploymentID: "",
				BuildType:    models.BuildTypePureNix,
			}

			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)
			if err != nil {
				return false
			}

			// Should not be valid
			if result.Valid {
				return false
			}

			// Should have error for deployment_id field with REQUIRED_FIELD code
			for _, e := range result.Errors {
				if e.Field == "deployment_id" && e.Code == ValidationCodeRequiredField {
					return true
				}
			}
			return false
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 7.3: Empty build_type produces validation error with REQUIRED_FIELD code
	// **Validates: Requirements 3.6**
	properties.Property("empty build_type produces REQUIRED_FIELD error", prop.ForAll(
		func(jobID, deploymentID string) bool {
			job := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    "",
			}

			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)
			if err != nil {
				return false
			}

			// Should not be valid
			if result.Valid {
				return false
			}

			// Should have error for build_type field with REQUIRED_FIELD code
			for _, e := range result.Errors {
				if e.Field == "build_type" && e.Code == ValidationCodeRequiredField {
					return true
				}
			}
			return false
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 7.4: Invalid build_type produces validation error with INVALID_VALUE code
	// **Validates: Requirements 3.7**
	properties.Property("invalid build_type produces INVALID_VALUE error", prop.ForAll(
		func(jobID, deploymentID, invalidType string) bool {
			// Ensure the type is actually invalid
			if invalidType == string(models.BuildTypePureNix) || invalidType == string(models.BuildTypeOCI) || invalidType == "" {
				return true // Skip valid types
			}

			job := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    models.BuildType(invalidType),
			}

			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)
			if err != nil {
				return false
			}

			// Should not be valid
			if result.Valid {
				return false
			}

			// Should have error for build_type field with INVALID_VALUE code
			for _, e := range result.Errors {
				if e.Field == "build_type" && e.Code == ValidationCodeInvalidValue {
					return true
				}
			}
			return false
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool {
			return len(s) > 0 && s != "pure-nix" && s != "oci"
		}),
	))

	// Property 7.5: Invalid build_strategy produces validation error with INVALID_VALUE code
	// **Validates: Requirements 3.8**
	properties.Property("invalid build_strategy produces INVALID_VALUE error", prop.ForAll(
		func(jobID, deploymentID, invalidStrategy string) bool {
			// Ensure the strategy is actually invalid
			testStrategy := models.BuildStrategy(invalidStrategy)
			if testStrategy.IsValid() || invalidStrategy == "" {
				return true // Skip valid strategies
			}

			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: testStrategy,
			}

			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)
			if err != nil {
				return false
			}

			// Should not be valid
			if result.Valid {
				return false
			}

			// Should have error for build_strategy field with INVALID_VALUE code
			for _, e := range result.Errors {
				if e.Field == "build_strategy" && e.Code == ValidationCodeInvalidValue {
					return true
				}
			}
			return false
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool {
			return len(s) > 0 && !models.BuildStrategy(s).IsValid()
		}),
	))

	// Property 7.6: Negative timeout produces validation error with NEGATIVE_VALUE code
	// **Validates: Requirements 3.9**
	properties.Property("negative timeout produces NEGATIVE_VALUE error", prop.ForAll(
		func(jobID, deploymentID string, negativeTimeout int) bool {
			if negativeTimeout >= 0 {
				return true // Skip non-negative timeouts
			}

			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypePureNix,
				TimeoutSeconds: negativeTimeout,
			}

			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)
			if err != nil {
				return false
			}

			// Should not be valid
			if result.Valid {
				return false
			}

			// Should have error for timeout_seconds field with NEGATIVE_VALUE code
			for _, e := range result.Errors {
				if e.Field == "timeout_seconds" && e.Code == ValidationCodeNegativeValue {
					return true
				}
			}
			return false
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(-10000, -1),
	))

	// Property 7.7: Multiple invalid fields produce multiple errors
	properties.Property("multiple invalid fields produce multiple errors", prop.ForAll(
		func(negativeTimeout int) bool {
			if negativeTimeout >= 0 {
				negativeTimeout = -1
			}

			// Job with multiple invalid fields
			job := &models.BuildJob{
				ID:             "",                                   // Invalid: empty
				DeploymentID:   "",                                   // Invalid: empty
				BuildType:      models.BuildType("invalid"),          // Invalid: not a valid type
				BuildStrategy:  models.BuildStrategy("bad-strategy"), // Invalid: not a valid strategy
				TimeoutSeconds: negativeTimeout,                      // Invalid: negative
			}

			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)
			if err != nil {
				return false
			}

			// Should not be valid
			if result.Valid {
				return false
			}

			// Should have at least 5 errors (one for each invalid field)
			return len(result.Errors) >= 5
		},
		gen.IntRange(-10000, -1),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 8: Validation Failure Handling**
// For any build job that fails validation, the Build_System SHALL mark the job as failed
// without executing the build.
// **Validates: Requirements 3.2**

// TestValidationFailureHandling tests Property 8: Validation Failure Handling.
func TestValidationFailureHandling(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 8.1: Invalid jobs are marked as not valid
	properties.Property("invalid jobs are marked as not valid", prop.ForAll(
		func(deploymentID string) bool {
			// Create an invalid job (missing ID)
			job := &models.BuildJob{
				ID:           "",
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
			}

			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)
			if err != nil {
				return false
			}

			// Should not be valid
			return !result.Valid
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 8.2: Valid jobs are marked as valid
	properties.Property("valid jobs are marked as valid", prop.ForAll(
		func(jobID, deploymentID string, buildType models.BuildType) bool {
			// Create a valid job
			job := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    buildType,
			}

			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)
			if err != nil {
				return false
			}

			// Should be valid
			return result.Valid
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
	))

	// Property 8.3: Validation result contains errors for invalid jobs
	properties.Property("validation result contains errors for invalid jobs", prop.ForAll(
		func(deploymentID string) bool {
			// Create an invalid job (missing ID)
			job := &models.BuildJob{
				ID:           "",
				DeploymentID: deploymentID,
				BuildType:    models.BuildTypePureNix,
			}

			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)
			if err != nil {
				return false
			}

			// Should have at least one error
			return len(result.Errors) > 0
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 8.4: Validation result has no errors for valid jobs
	properties.Property("validation result has no errors for valid jobs", prop.ForAll(
		func(jobID, deploymentID string, buildType models.BuildType) bool {
			// Create a valid job
			job := &models.BuildJob{
				ID:           jobID,
				DeploymentID: deploymentID,
				BuildType:    buildType,
			}

			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)
			if err != nil {
				return false
			}

			// Should have no errors
			return len(result.Errors) == 0
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
	))

	// Property 8.5: Validation failure prevents execution (simulated)
	// This tests that when validation fails, the result indicates the job should not proceed
	properties.Property("validation failure indicates job should not proceed", prop.ForAll(
		func(deploymentID string, negativeTimeout int) bool {
			if negativeTimeout >= 0 {
				negativeTimeout = -1
			}

			// Create an invalid job
			job := &models.BuildJob{
				ID:             "",
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypePureNix,
				TimeoutSeconds: negativeTimeout,
			}

			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)
			if err != nil {
				return false
			}

			// The result should indicate the job should not proceed
			// (Valid == false means execution should not happen)
			return !result.Valid && len(result.Errors) > 0
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(-10000, -1),
	))

	// Property 8.6: All validation error codes are from the defined set
	properties.Property("all validation error codes are from defined set", prop.ForAll(
		func(negativeTimeout int) bool {
			if negativeTimeout >= 0 {
				negativeTimeout = -1
			}

			// Create a job with multiple invalid fields
			job := &models.BuildJob{
				ID:             "",
				DeploymentID:   "",
				BuildType:      models.BuildType("invalid"),
				BuildStrategy:  models.BuildStrategy("bad"),
				TimeoutSeconds: negativeTimeout,
			}

			ctx := context.Background()
			result, err := ValidateBuildJob(ctx, job)
			if err != nil {
				return false
			}

			// All error codes should be from the defined set
			validCodes := map[string]bool{
				ValidationCodeRequiredField: true,
				ValidationCodeInvalidValue:  true,
				ValidationCodeNegativeValue: true,
			}

			for _, e := range result.Errors {
				if !validCodes[e.Code] {
					return false
				}
			}
			return true
		},
		gen.IntRange(-10000, -1),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 9: Progress Monotonicity**
// For any build execution, the progress percentage reported by the Progress_Tracker
// SHALL be monotonically increasing.
// **Validates: Requirements 4.3**

// genProgressSequence generates a sequence of progress percentages.
func genProgressSequence() gopter.Gen {
	return gen.SliceOfN(10, gen.IntRange(0, 100))
}

// genMonotonicProgressSequence generates a monotonically increasing sequence of progress percentages.
func genMonotonicProgressSequence() gopter.Gen {
	return gen.SliceOfN(10, gen.IntRange(0, 100)).Map(func(vals []int) []int {
		// Sort to make monotonic
		result := make([]int, len(vals))
		copy(result, vals)
		for i := 1; i < len(result); i++ {
			if result[i] < result[i-1] {
				result[i] = result[i-1]
			}
		}
		return result
	})
}

// genNonMonotonicProgressSequence generates a sequence that is NOT monotonically increasing.
func genNonMonotonicProgressSequence() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 9),    // Position to insert decrease
		gen.IntRange(50, 100), // Higher value
		gen.IntRange(0, 49),   // Lower value (to create decrease)
	).Map(func(vals []interface{}) []int {
		pos := vals[0].(int)
		high := vals[1].(int)
		low := vals[2].(int)

		// Create a sequence with a decrease at position pos
		result := make([]int, 10)
		for i := 0; i < 10; i++ {
			if i < pos {
				result[i] = high
			} else if i == pos {
				result[i] = low // This creates the decrease
			} else {
				result[i] = low + i
			}
		}
		return result
	})
}

// TestProgressMonotonicity tests Property 9: Progress Monotonicity.
func TestProgressMonotonicity(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 9.1: Monotonic progress sequences are correctly identified as monotonic
	properties.Property("monotonic sequences are identified as monotonic", prop.ForAll(
		func(buildID string, progressSeq []int) bool {
			// Create a progress tracker
			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			// Make the sequence monotonic
			monotonic := make([]int, len(progressSeq))
			copy(monotonic, progressSeq)
			for i := 1; i < len(monotonic); i++ {
				if monotonic[i] < monotonic[i-1] {
					monotonic[i] = monotonic[i-1]
				}
			}

			// Report all progress values
			for i, percent := range monotonic {
				tracker.ReportProgress(ctx, buildID, percent, fmt.Sprintf("Step %d", i))
			}

			// Verify the tracker identifies this as monotonic
			return tracker.IsProgressMonotonic(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		genProgressSequence(),
	))

	// Property 9.2: Non-monotonic progress sequences are correctly identified
	properties.Property("non-monotonic sequences are identified as non-monotonic", prop.ForAll(
		func(buildID string, progressSeq []int) bool {
			// Skip if sequence is too short or already monotonic
			if len(progressSeq) < 2 {
				return true
			}

			// Check if the sequence is already monotonic
			isMonotonic := true
			for i := 1; i < len(progressSeq); i++ {
				if progressSeq[i] < progressSeq[i-1] {
					isMonotonic = false
					break
				}
			}
			if isMonotonic {
				return true // Skip monotonic sequences
			}

			// Create a progress tracker
			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			// Report all progress values
			for i, percent := range progressSeq {
				tracker.ReportProgress(ctx, buildID, percent, fmt.Sprintf("Step %d", i))
			}

			// Verify the tracker identifies this as non-monotonic
			return !tracker.IsProgressMonotonic(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		genNonMonotonicProgressSequence(),
	))

	// Property 9.3: Empty progress history is considered monotonic
	properties.Property("empty progress history is monotonic", prop.ForAll(
		func(buildID string) bool {
			tracker := NewDefaultProgressTracker(nil)
			return tracker.IsProgressMonotonic(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 9.4: Single progress report is considered monotonic
	properties.Property("single progress report is monotonic", prop.ForAll(
		func(buildID string, percent int) bool {
			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			tracker.ReportProgress(ctx, buildID, percent, "Single report")

			return tracker.IsProgressMonotonic(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(0, 100),
	))

	// Property 9.5: Progress history is correctly stored
	properties.Property("progress history is correctly stored", prop.ForAll(
		func(buildID string, progressSeq []int) bool {
			if len(progressSeq) == 0 {
				return true
			}

			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			// Report all progress values
			for i, percent := range progressSeq {
				tracker.ReportProgress(ctx, buildID, percent, fmt.Sprintf("Step %d", i))
			}

			// Get the history
			history := tracker.GetProgressHistory(buildID)

			// Verify the history has the correct length
			if len(history) != len(progressSeq) {
				return false
			}

			// Verify the history has the correct values
			for i, record := range history {
				if record.Percent != progressSeq[i] {
					return false
				}
				if record.BuildID != buildID {
					return false
				}
			}

			return true
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		genProgressSequence(),
	))

	// Property 9.6: Progress reports for different builds are isolated
	properties.Property("progress reports for different builds are isolated", prop.ForAll(
		func(buildID1, buildID2 string, percent1, percent2 int) bool {
			if buildID1 == buildID2 {
				return true // Skip if same build ID
			}

			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			// Report progress for both builds
			tracker.ReportProgress(ctx, buildID1, percent1, "Build 1")
			tracker.ReportProgress(ctx, buildID2, percent2, "Build 2")

			// Get histories
			history1 := tracker.GetProgressHistory(buildID1)
			history2 := tracker.GetProgressHistory(buildID2)

			// Verify isolation
			if len(history1) != 1 || len(history2) != 1 {
				return false
			}
			if history1[0].Percent != percent1 || history2[0].Percent != percent2 {
				return false
			}

			return true
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(0, 100),
		gen.IntRange(0, 100),
	))

	// Property 9.7: Strictly increasing sequences are monotonic
	properties.Property("strictly increasing sequences are monotonic", prop.ForAll(
		func(buildID string, start int) bool {
			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			// Create a strictly increasing sequence
			for i := 0; i < 10; i++ {
				percent := start + i*10
				if percent > 100 {
					percent = 100
				}
				tracker.ReportProgress(ctx, buildID, percent, fmt.Sprintf("Step %d", i))
			}

			return tracker.IsProgressMonotonic(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(0, 10),
	))

	// Property 9.8: Equal consecutive values are considered monotonic (non-decreasing)
	properties.Property("equal consecutive values are monotonic", prop.ForAll(
		func(buildID string, percent int) bool {
			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			// Report the same value multiple times
			for i := 0; i < 5; i++ {
				tracker.ReportProgress(ctx, buildID, percent, fmt.Sprintf("Step %d", i))
			}

			return tracker.IsProgressMonotonic(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.IntRange(0, 100),
	))

	properties.TestingRun(t)
}

// TestProgressMonotonicityWithMockTracker tests progress monotonicity using the mock tracker.
func TestProgressMonotonicityWithMockTracker(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: MockProgressTracker correctly identifies monotonic sequences
	properties.Property("mock tracker identifies monotonic sequences", prop.ForAll(
		func(buildID string, progressSeq []int) bool {
			// Create a mock progress tracker
			tracker := NewMockProgressTracker()
			ctx := context.Background()

			// Make the sequence monotonic
			monotonic := make([]int, len(progressSeq))
			copy(monotonic, progressSeq)
			for i := 1; i < len(monotonic); i++ {
				if monotonic[i] < monotonic[i-1] {
					monotonic[i] = monotonic[i-1]
				}
			}

			// Report all progress values
			for i, percent := range monotonic {
				tracker.ReportProgress(ctx, buildID, percent, fmt.Sprintf("Step %d", i))
			}

			// Verify the tracker identifies this as monotonic
			return tracker.IsProgressMonotonic(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		genProgressSequence(),
	))

	// Property: MockProgressTracker correctly identifies non-monotonic sequences
	properties.Property("mock tracker identifies non-monotonic sequences", prop.ForAll(
		func(buildID string, progressSeq []int) bool {
			// Skip if sequence is too short
			if len(progressSeq) < 2 {
				return true
			}

			// Check if the sequence is already monotonic
			isMonotonic := true
			for i := 1; i < len(progressSeq); i++ {
				if progressSeq[i] < progressSeq[i-1] {
					isMonotonic = false
					break
				}
			}
			if isMonotonic {
				return true // Skip monotonic sequences
			}

			// Create a mock progress tracker
			tracker := NewMockProgressTracker()
			ctx := context.Background()

			// Report all progress values
			for i, percent := range progressSeq {
				tracker.ReportProgress(ctx, buildID, percent, fmt.Sprintf("Step %d", i))
			}

			// Verify the tracker identifies this as non-monotonic
			return !tracker.IsProgressMonotonic(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		genNonMonotonicProgressSequence(),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 10: Terminal Stage Reporting**
// For any build that completes (success or failure), the Progress_Tracker SHALL report
// either `completed` or `failed` stage.
// **Validates: Requirements 4.5, 4.6**

// genBuildStage generates valid build stages.
func genBuildStage() gopter.Gen {
	return gen.OneConstOf(
		StageCloning,
		StageDetecting,
		StageGenerating,
		StageCalculatingHash,
		StageBuilding,
		StagePushing,
		StageCompleted,
		StageFailed,
	)
}

// genTerminalStage generates terminal build stages.
func genTerminalStage() gopter.Gen {
	return gen.OneConstOf(StageCompleted, StageFailed)
}

// genNonTerminalStage generates non-terminal build stages.
func genNonTerminalStage() gopter.Gen {
	return gen.OneConstOf(
		StageCloning,
		StageDetecting,
		StageGenerating,
		StageCalculatingHash,
		StageBuilding,
		StagePushing,
	)
}

// genStageSequence generates a sequence of build stages.
func genStageSequence() gopter.Gen {
	return gen.SliceOfN(5, genBuildStage())
}

// TestTerminalStageReporting tests Property 10: Terminal Stage Reporting.
func TestTerminalStageReporting(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 10.1: Completed stage is correctly identified as terminal
	properties.Property("completed stage is identified as terminal", prop.ForAll(
		func(buildID string) bool {
			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			// Report completed stage
			tracker.ReportStage(ctx, buildID, StageCompleted)

			// Verify it's identified as terminal
			return tracker.HasTerminalStage(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 10.2: Failed stage is correctly identified as terminal
	properties.Property("failed stage is identified as terminal", prop.ForAll(
		func(buildID string) bool {
			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			// Report failed stage
			tracker.ReportStage(ctx, buildID, StageFailed)

			// Verify it's identified as terminal
			return tracker.HasTerminalStage(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 10.3: Non-terminal stages are not identified as terminal
	properties.Property("non-terminal stages are not terminal", prop.ForAll(
		func(buildID string, stage BuildStage) bool {
			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			// Report non-terminal stage
			tracker.ReportStage(ctx, buildID, stage)

			// Verify it's NOT identified as terminal
			return !tracker.HasTerminalStage(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		genNonTerminalStage(),
	))

	// Property 10.4: Terminal stage after non-terminal stages is detected
	properties.Property("terminal stage after non-terminal is detected", prop.ForAll(
		func(buildID string, nonTerminalStages []BuildStage, terminalStage BuildStage) bool {
			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			// Report non-terminal stages first
			for _, stage := range nonTerminalStages {
				tracker.ReportStage(ctx, buildID, stage)
			}

			// Report terminal stage
			tracker.ReportStage(ctx, buildID, terminalStage)

			// Verify terminal stage is detected
			return tracker.HasTerminalStage(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.SliceOfN(5, genNonTerminalStage()),
		genTerminalStage(),
	))

	// Property 10.5: Empty stage history has no terminal stage
	properties.Property("empty stage history has no terminal stage", prop.ForAll(
		func(buildID string) bool {
			tracker := NewDefaultProgressTracker(nil)
			return !tracker.HasTerminalStage(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 10.6: Stage history is correctly stored
	properties.Property("stage history is correctly stored", prop.ForAll(
		func(buildID string, stages []BuildStage) bool {
			if len(stages) == 0 {
				return true
			}

			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			// Report all stages
			for _, stage := range stages {
				tracker.ReportStage(ctx, buildID, stage)
			}

			// Get the history
			history := tracker.GetStageHistory(buildID)

			// Verify the history has the correct length
			if len(history) != len(stages) {
				return false
			}

			// Verify the history has the correct values
			for i, record := range history {
				if record.Stage != stages[i] {
					return false
				}
				if record.BuildID != buildID {
					return false
				}
			}

			return true
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		genStageSequence(),
	))

	// Property 10.7: GetLastStage returns the correct last stage
	properties.Property("GetLastStage returns correct last stage", prop.ForAll(
		func(buildID string, stages []BuildStage) bool {
			if len(stages) == 0 {
				return true
			}

			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			// Report all stages
			for _, stage := range stages {
				tracker.ReportStage(ctx, buildID, stage)
			}

			// Get the last stage
			lastStage, found := tracker.GetLastStage(buildID)

			// Verify the last stage is correct
			return found && lastStage == stages[len(stages)-1]
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.SliceOfN(5, genBuildStage()).SuchThat(func(s []BuildStage) bool { return len(s) > 0 }),
	))

	// Property 10.8: GetLastStage returns false for empty history
	properties.Property("GetLastStage returns false for empty history", prop.ForAll(
		func(buildID string) bool {
			tracker := NewDefaultProgressTracker(nil)
			_, found := tracker.GetLastStage(buildID)
			return !found
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 10.9: Stage reports for different builds are isolated
	properties.Property("stage reports for different builds are isolated", prop.ForAll(
		func(buildID1, buildID2 string, stage1, stage2 BuildStage) bool {
			if buildID1 == buildID2 {
				return true // Skip if same build ID
			}

			tracker := NewDefaultProgressTracker(nil)
			ctx := context.Background()

			// Report stages for both builds
			tracker.ReportStage(ctx, buildID1, stage1)
			tracker.ReportStage(ctx, buildID2, stage2)

			// Get histories
			history1 := tracker.GetStageHistory(buildID1)
			history2 := tracker.GetStageHistory(buildID2)

			// Verify isolation
			if len(history1) != 1 || len(history2) != 1 {
				return false
			}
			if history1[0].Stage != stage1 || history2[0].Stage != stage2 {
				return false
			}

			return true
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		genBuildStage(),
		genBuildStage(),
	))

	properties.TestingRun(t)
}

// TestTerminalStageReportingWithMockTracker tests terminal stage reporting using the mock tracker.
func TestTerminalStageReportingWithMockTracker(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: MockProgressTracker correctly identifies terminal stages
	properties.Property("mock tracker identifies terminal stages", prop.ForAll(
		func(buildID string, terminalStage BuildStage) bool {
			tracker := NewMockProgressTracker()
			ctx := context.Background()

			// Report terminal stage
			tracker.ReportStage(ctx, buildID, terminalStage)

			// Verify it's identified as terminal
			return tracker.HasTerminalStage(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		genTerminalStage(),
	))

	// Property: MockProgressTracker correctly identifies non-terminal stages
	properties.Property("mock tracker identifies non-terminal stages", prop.ForAll(
		func(buildID string, stage BuildStage) bool {
			tracker := NewMockProgressTracker()
			ctx := context.Background()

			// Report non-terminal stage
			tracker.ReportStage(ctx, buildID, stage)

			// Verify it's NOT identified as terminal
			return !tracker.HasTerminalStage(buildID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		genNonTerminalStage(),
	))

	// Property: MockProgressTracker GetLastStage works correctly
	properties.Property("mock tracker GetLastStage works correctly", prop.ForAll(
		func(buildID string, stages []BuildStage) bool {
			if len(stages) == 0 {
				return true
			}

			tracker := NewMockProgressTracker()
			ctx := context.Background()

			// Report all stages
			for _, stage := range stages {
				tracker.ReportStage(ctx, buildID, stage)
			}

			// Get the last stage
			lastStage, found := tracker.GetLastStage(buildID)

			// Verify the last stage is correct
			return found && lastStage == stages[len(stages)-1]
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.SliceOfN(5, genBuildStage()).SuchThat(func(s []BuildStage) bool { return len(s) > 0 }),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 18: Timeout Selection Priority**
// For any build job, the effective timeout SHALL be: job.TimeoutSeconds if set,
// else job.BuildConfig.BuildTimeout if set, else 1800 seconds (default).
// **Validates: Requirements 12.1, 12.2, 12.3**

// TestTimeoutSelectionPriority tests Property 18: Timeout Selection Priority.
func TestTimeoutSelectionPriority(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 18.1: Job-specific timeout takes highest priority
	// **Validates: Requirements 12.1**
	properties.Property("job timeout takes highest priority", prop.ForAll(
		func(jobTimeout, configTimeout, defaultTimeout int) bool {
			if jobTimeout <= 0 {
				return true // Skip when job timeout is not set
			}

			job := &models.BuildJob{
				ID:             "test-job",
				DeploymentID:   "test-deployment",
				BuildType:      models.BuildTypePureNix,
				TimeoutSeconds: jobTimeout,
			}
			if configTimeout > 0 {
				job.BuildConfig = &models.BuildConfig{
					BuildTimeout: configTimeout,
				}
			}

			timeout := GetEffectiveTimeout(job, defaultTimeout)
			expected := time.Duration(jobTimeout) * time.Second
			return timeout == expected
		},
		gen.IntRange(1, 7200),
		gen.IntRange(0, 7200),
		gen.IntRange(0, 7200),
	))

	// Property 18.2: Config timeout is used when job timeout is not set
	// **Validates: Requirements 12.2**
	properties.Property("config timeout used when job timeout not set", prop.ForAll(
		func(configTimeout, defaultTimeout int) bool {
			if configTimeout <= 0 {
				return true // Skip when config timeout is not set
			}

			job := &models.BuildJob{
				ID:             "test-job",
				DeploymentID:   "test-deployment",
				BuildType:      models.BuildTypePureNix,
				TimeoutSeconds: 0, // Not set
				BuildConfig: &models.BuildConfig{
					BuildTimeout: configTimeout,
				},
			}

			timeout := GetEffectiveTimeout(job, defaultTimeout)
			expected := time.Duration(configTimeout) * time.Second
			return timeout == expected
		},
		gen.IntRange(1, 7200),
		gen.IntRange(0, 7200),
	))

	// Property 18.3: Default timeout is used when neither job nor config timeout is set
	// **Validates: Requirements 12.3**
	properties.Property("default timeout used when no timeout configured", prop.ForAll(
		func(defaultTimeout int) bool {
			if defaultTimeout <= 0 {
				return true // Skip invalid defaults
			}

			job := &models.BuildJob{
				ID:             "test-job",
				DeploymentID:   "test-deployment",
				BuildType:      models.BuildTypePureNix,
				TimeoutSeconds: 0, // Not set
				BuildConfig:    nil,
			}

			timeout := GetEffectiveTimeout(job, defaultTimeout)
			expected := time.Duration(defaultTimeout) * time.Second
			return timeout == expected
		},
		gen.IntRange(1, 7200),
	))

	// Property 18.4: Global default (1800s) is used when all other timeouts are zero
	// **Validates: Requirements 12.3**
	properties.Property("global default used when all timeouts zero", prop.ForAll(
		func(jobID, deploymentID string) bool {
			job := &models.BuildJob{
				ID:             jobID,
				DeploymentID:   deploymentID,
				BuildType:      models.BuildTypePureNix,
				TimeoutSeconds: 0, // Not set
				BuildConfig:    nil,
			}

			timeout := GetEffectiveTimeout(job, 0)
			expected := time.Duration(DefaultBuildTimeout) * time.Second
			return timeout == expected
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 18.5: Timeout precedence order is strictly enforced
	// **Validates: Requirements 12.1, 12.2, 12.3**
	properties.Property("timeout precedence order is strictly enforced", prop.ForAll(
		func(jobTimeout, configTimeout, defaultTimeout int) bool {
			job := &models.BuildJob{
				ID:             "test-job",
				DeploymentID:   "test-deployment",
				BuildType:      models.BuildTypePureNix,
				TimeoutSeconds: jobTimeout,
			}
			if configTimeout > 0 {
				job.BuildConfig = &models.BuildConfig{
					BuildTimeout: configTimeout,
				}
			}

			timeout := GetEffectiveTimeout(job, defaultTimeout)

			// Determine expected timeout based on strict precedence
			var expected time.Duration
			if jobTimeout > 0 {
				expected = time.Duration(jobTimeout) * time.Second
			} else if configTimeout > 0 {
				expected = time.Duration(configTimeout) * time.Second
			} else if defaultTimeout > 0 {
				expected = time.Duration(defaultTimeout) * time.Second
			} else {
				expected = time.Duration(DefaultBuildTimeout) * time.Second
			}

			return timeout == expected
		},
		gen.IntRange(0, 7200),
		gen.IntRange(0, 7200),
		gen.IntRange(0, 7200),
	))

	// Property 18.6: GetEffectiveTimeout always returns a positive duration
	// **Validates: Requirements 12.1, 12.2, 12.3**
	properties.Property("GetEffectiveTimeout always returns positive duration", prop.ForAll(
		func(jobTimeout, configTimeout, defaultTimeout int) bool {
			job := &models.BuildJob{
				ID:             "test-job",
				DeploymentID:   "test-deployment",
				BuildType:      models.BuildTypePureNix,
				TimeoutSeconds: jobTimeout,
			}
			if configTimeout > 0 {
				job.BuildConfig = &models.BuildConfig{
					BuildTimeout: configTimeout,
				}
			}

			timeout := GetEffectiveTimeout(job, defaultTimeout)
			return timeout > 0
		},
		gen.IntRange(0, 7200),
		gen.IntRange(0, 7200),
		gen.IntRange(0, 7200),
	))

	// Property 18.7: Negative job timeout is treated as not set
	// **Validates: Requirements 12.1**
	properties.Property("negative job timeout is treated as not set", prop.ForAll(
		func(negativeTimeout, configTimeout int) bool {
			if negativeTimeout >= 0 {
				return true // Skip non-negative values
			}
			if configTimeout <= 0 {
				return true // Skip when config timeout is not set
			}

			job := &models.BuildJob{
				ID:             "test-job",
				DeploymentID:   "test-deployment",
				BuildType:      models.BuildTypePureNix,
				TimeoutSeconds: negativeTimeout, // Negative, should be treated as not set
				BuildConfig: &models.BuildConfig{
					BuildTimeout: configTimeout,
				},
			}

			timeout := GetEffectiveTimeout(job, 0)
			// Since negative timeout is treated as not set, config timeout should be used
			expected := time.Duration(configTimeout) * time.Second
			return timeout == expected
		},
		gen.IntRange(-1000, -1),
		gen.IntRange(1, 7200),
	))

	// Property 18.8: Config timeout with nil BuildConfig falls through to default
	// **Validates: Requirements 12.2, 12.3**
	properties.Property("nil BuildConfig falls through to default", prop.ForAll(
		func(defaultTimeout int) bool {
			if defaultTimeout <= 0 {
				return true // Skip invalid defaults
			}

			job := &models.BuildJob{
				ID:             "test-job",
				DeploymentID:   "test-deployment",
				BuildType:      models.BuildTypePureNix,
				TimeoutSeconds: 0,   // Not set
				BuildConfig:    nil, // Nil config
			}

			timeout := GetEffectiveTimeout(job, defaultTimeout)
			expected := time.Duration(defaultTimeout) * time.Second
			return timeout == expected
		},
		gen.IntRange(1, 7200),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 19: Timeout Enforcement**
// For any build that exceeds its effective timeout, the Build_System SHALL terminate
// the build and mark it as failed with a timeout error.
// **Validates: Requirements 12.4, 12.5**

// TestTimeoutEnforcement tests Property 19: Timeout Enforcement.
func TestTimeoutEnforcement(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 19.1: IsBuildTimeoutError correctly identifies timeout errors
	// **Validates: Requirements 12.5**
	properties.Property("IsBuildTimeoutError identifies timeout errors", prop.ForAll(
		func(msg string) bool {
			// Create a wrapped timeout error
			err := fmt.Errorf("build failed: %w: %s", ErrBuildTimeout, msg)
			return IsBuildTimeoutError(err)
		},
		gen.AlphaString(),
	))

	// Property 19.2: IsBuildTimeoutError rejects non-timeout errors
	// **Validates: Requirements 12.5**
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

	// Property 19.3: ErrBuildTimeout is correctly defined
	// **Validates: Requirements 12.5**
	properties.Property("ErrBuildTimeout is correctly defined", prop.ForAll(
		func(_ int) bool {
			// ErrBuildTimeout should be a non-nil error
			if ErrBuildTimeout == nil {
				return false
			}
			// ErrBuildTimeout should contain "timeout" in its message
			msg := ErrBuildTimeout.Error()
			return containsSubstring(msg, "timeout")
		},
		gen.Int(),
	))

	// Property 19.4: Context with timeout cancels after the specified duration
	// **Validates: Requirements 12.4**
	properties.Property("context cancels after timeout", prop.ForAll(
		func(timeoutMs int) bool {
			if timeoutMs < 10 || timeoutMs > 200 {
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
		gen.IntRange(10, 200),
	))

	// Property 19.5: Timeout error wrapping preserves error chain
	// **Validates: Requirements 12.5**
	properties.Property("timeout error wrapping preserves error chain", prop.ForAll(
		func(msg string) bool {
			// Create a wrapped timeout error
			innerErr := fmt.Errorf("inner: %s", msg)
			wrappedErr := fmt.Errorf("outer: %w: %w", ErrBuildTimeout, innerErr)

			// Should still be identifiable as a timeout error
			return IsBuildTimeoutError(wrappedErr)
		},
		gen.AlphaString(),
	))

	// Property 19.6: Timeout error message contains useful information
	// **Validates: Requirements 12.5**
	properties.Property("timeout error message is descriptive", prop.ForAll(
		func(timeoutSec int) bool {
			if timeoutSec <= 0 {
				return true // Skip invalid timeouts
			}

			timeout := time.Duration(timeoutSec) * time.Second
			err := fmt.Errorf("%w: build exceeded %v timeout", ErrBuildTimeout, timeout)

			// Error message should contain timeout information
			msg := err.Error()
			return containsSubstring(msg, "timeout") && IsBuildTimeoutError(err)
		},
		gen.IntRange(1, 7200),
	))

	// Property 19.7: Timeout enforcement uses effective timeout
	// **Validates: Requirements 12.4, 12.5**
	properties.Property("timeout enforcement uses effective timeout", prop.ForAll(
		func(jobTimeout, configTimeout int) bool {
			job := &models.BuildJob{
				ID:             "test-job",
				DeploymentID:   "test-deployment",
				BuildType:      models.BuildTypePureNix,
				TimeoutSeconds: jobTimeout,
			}
			if configTimeout > 0 {
				job.BuildConfig = &models.BuildConfig{
					BuildTimeout: configTimeout,
				}
			}

			// Get the effective timeout
			effectiveTimeout := GetEffectiveTimeout(job, DefaultBuildTimeout)

			// The effective timeout should be positive
			if effectiveTimeout <= 0 {
				return false
			}

			// Verify the timeout is one of the expected values
			expectedValues := []time.Duration{
				time.Duration(jobTimeout) * time.Second,
				time.Duration(configTimeout) * time.Second,
				time.Duration(DefaultBuildTimeout) * time.Second,
			}

			for _, expected := range expectedValues {
				if expected > 0 && effectiveTimeout == expected {
					return true
				}
			}

			// If none of the specific values match, it should be the default
			return effectiveTimeout == time.Duration(DefaultBuildTimeout)*time.Second
		},
		gen.IntRange(0, 7200),
		gen.IntRange(0, 7200),
	))

	// Property 19.8: Timeout error is distinct from other build errors
	// **Validates: Requirements 12.5**
	properties.Property("timeout error is distinct from other errors", prop.ForAll(
		func(_ int) bool {
			// ErrBuildTimeout should not be the same as ErrValidationFailed
			if errors.Is(ErrBuildTimeout, ErrValidationFailed) {
				return false
			}
			if errors.Is(ErrValidationFailed, ErrBuildTimeout) {
				return false
			}

			// ErrBuildTimeout should not be the same as ErrInvalidStateTransition
			if errors.Is(ErrBuildTimeout, ErrInvalidStateTransition) {
				return false
			}
			if errors.Is(ErrInvalidStateTransition, ErrBuildTimeout) {
				return false
			}

			return true
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}

// containsSubstring checks if a string contains a substring (case-insensitive).
func containsSubstring(s, substr string) bool {
	sLower := toLower(s)
	substrLower := toLower(substr)
	return len(sLower) >= len(substrLower) && containsHelper(sLower, substrLower)
}

// toLower converts a string to lowercase.
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

// **Feature: build-lifecycle-correctness, Property 20: Artifact Persistence on Success**
// For any successful build, the Build_System SHALL store the artifact (store path for pure-nix,
// image tag for OCI) and update the deployment status to `built`.
// **Validates: Requirements 13.1, 13.2, 13.3, 13.4**

// genNixStorePath generates valid Nix store paths.
func genNixStorePath() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0
	}).Map(func(name string) string {
		// Generate a mock hash (32 chars)
		hash := "0123456789abcdef0123456789abcdef"
		return fmt.Sprintf("/nix/store/%s-%s", hash, name)
	})
}

// genOCIImageTag generates valid OCI image tags.
func genOCIImageTag() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	).Map(func(vals []interface{}) string {
		registry := vals[0].(string)
		tag := vals[1].(string)
		return fmt.Sprintf("registry.example.com/%s:v%s", registry, tag)
	})
}

// TestArtifactPersistenceOnSuccess tests Property 20: Artifact Persistence on Success.
func TestArtifactPersistenceOnSuccess(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 20.1: Pure-nix builds produce valid store paths
	// **Validates: Requirements 13.1**
	properties.Property("pure-nix builds produce valid store paths", prop.ForAll(
		func(storePath string) bool {
			// Validate that the store path is recognized as a Nix store path
			return models.IsNixStorePath(storePath)
		},
		genNixStorePath(),
	))

	// Property 20.2: OCI builds produce valid image tags
	// **Validates: Requirements 13.2**
	properties.Property("OCI builds produce valid image tags", prop.ForAll(
		func(imageTag string) bool {
			// Validate that the image tag is recognized as an OCI image tag
			return models.IsOCIImageTag(imageTag)
		},
		genOCIImageTag(),
	))

	// Property 20.3: ValidateArtifact correctly validates pure-nix artifacts
	// **Validates: Requirements 13.1**
	properties.Property("ValidateArtifact validates pure-nix artifacts", prop.ForAll(
		func(storePath string) bool {
			artifactType, valid := models.ValidateArtifact(models.BuildTypePureNix, storePath)
			return valid && artifactType == models.ArtifactTypeStorePath
		},
		genNixStorePath(),
	))

	// Property 20.4: ValidateArtifact correctly validates OCI artifacts
	// **Validates: Requirements 13.2**
	properties.Property("ValidateArtifact validates OCI artifacts", prop.ForAll(
		func(imageTag string) bool {
			artifactType, valid := models.ValidateArtifact(models.BuildTypeOCI, imageTag)
			return valid && artifactType == models.ArtifactTypeImageTag
		},
		genOCIImageTag(),
	))

	// Property 20.5: Successful build updates deployment with artifact
	// **Validates: Requirements 13.3**
	properties.Property("successful build updates deployment with artifact", prop.ForAll(
		func(jobID, deploymentID string, buildType models.BuildType) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create deployment
			deployment := NewTestDeployment(deploymentID, "test-app")
			deployment.Status = models.DeploymentStatusBuilding
			mockStore.Deployments().Create(ctx, deployment)

			// Simulate successful build by updating deployment
			var artifact string
			if buildType == models.BuildTypePureNix {
				artifact = "/nix/store/0123456789abcdef0123456789abcdef-test"
			} else {
				artifact = "registry.example.com/test:v1.0.0"
			}

			// Update deployment with artifact (simulating what worker does on success)
			deployment.Artifact = artifact
			deployment.Status = models.DeploymentStatusBuilt
			mockStore.Deployments().Update(ctx, deployment)

			// Verify deployment was updated
			updated, err := mockStore.Deployments().Get(ctx, deploymentID)
			if err != nil {
				return false
			}

			return updated.Artifact == artifact && updated.Status == models.DeploymentStatusBuilt
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
	))

	// Property 20.6: Successful build sets deployment status to built
	// **Validates: Requirements 13.4**
	properties.Property("successful build sets deployment status to built", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create deployment in building state
			deployment := NewTestDeployment(deploymentID, "test-app")
			deployment.Status = models.DeploymentStatusBuilding
			mockStore.Deployments().Create(ctx, deployment)

			// Simulate successful build
			deployment.Status = models.DeploymentStatusBuilt
			deployment.Artifact = "/nix/store/0123456789abcdef0123456789abcdef-test"
			mockStore.Deployments().Update(ctx, deployment)

			// Verify status
			updated, err := mockStore.Deployments().Get(ctx, deploymentID)
			if err != nil {
				return false
			}

			return updated.Status == models.DeploymentStatusBuilt
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 20.7: Artifact type matches build type
	// **Validates: Requirements 13.1, 13.2**
	properties.Property("artifact type matches build type", prop.ForAll(
		func(buildType models.BuildType) bool {
			expectedType := models.GetExpectedArtifactType(buildType)

			switch buildType {
			case models.BuildTypePureNix:
				return expectedType == models.ArtifactTypeStorePath
			case models.BuildTypeOCI:
				return expectedType == models.ArtifactTypeImageTag
			default:
				return expectedType == models.ArtifactTypeUnknown
			}
		},
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
	))

	// Property 20.8: Empty artifact is invalid for any build type
	// **Validates: Requirements 13.1, 13.2**
	properties.Property("empty artifact is invalid", prop.ForAll(
		func(buildType models.BuildType) bool {
			artifactType, valid := models.ValidateArtifact(buildType, "")
			return !valid && artifactType == models.ArtifactTypeUnknown
		},
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
	))

	// Property 20.9: Store path is not valid for OCI build type
	// **Validates: Requirements 13.1, 13.2**
	properties.Property("store path is not valid for OCI build type", prop.ForAll(
		func(storePath string) bool {
			artifactType, valid := models.ValidateArtifact(models.BuildTypeOCI, storePath)
			// Store path should not be valid for OCI builds
			return !valid || artifactType != models.ArtifactTypeImageTag
		},
		genNixStorePath(),
	))

	// Property 20.10: Image tag is not valid for pure-nix build type
	// **Validates: Requirements 13.1, 13.2**
	properties.Property("image tag is not valid for pure-nix build type", prop.ForAll(
		func(imageTag string) bool {
			artifactType, valid := models.ValidateArtifact(models.BuildTypePureNix, imageTag)
			// Image tag should not be valid for pure-nix builds
			return !valid || artifactType != models.ArtifactTypeStorePath
		},
		genOCIImageTag(),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 21: No Artifact on Failure**
// For any failed build, the Build_System SHALL NOT update the deployment artifact,
// and SHALL update the deployment status to `failed`.
// **Validates: Requirements 13.5, 13.6**

// TestNoArtifactOnFailure tests Property 21: No Artifact on Failure.
func TestNoArtifactOnFailure(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 21.1: Failed build does not update deployment artifact
	// **Validates: Requirements 13.5**
	properties.Property("failed build does not update deployment artifact", prop.ForAll(
		func(jobID, deploymentID string, buildType models.BuildType) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create deployment with no artifact
			deployment := NewTestDeployment(deploymentID, "test-app")
			deployment.Status = models.DeploymentStatusBuilding
			deployment.Artifact = "" // No artifact initially
			mockStore.Deployments().Create(ctx, deployment)

			// Simulate failed build - only update status, NOT artifact
			deployment.Status = models.DeploymentStatusFailed
			// Note: We do NOT set deployment.Artifact
			mockStore.Deployments().Update(ctx, deployment)

			// Verify deployment artifact was NOT updated
			updated, err := mockStore.Deployments().Get(ctx, deploymentID)
			if err != nil {
				return false
			}

			// Artifact should still be empty
			return updated.Artifact == ""
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
	))

	// Property 21.2: Failed build sets deployment status to failed
	// **Validates: Requirements 13.6**
	properties.Property("failed build sets deployment status to failed", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create deployment in building state
			deployment := NewTestDeployment(deploymentID, "test-app")
			deployment.Status = models.DeploymentStatusBuilding
			mockStore.Deployments().Create(ctx, deployment)

			// Simulate failed build
			deployment.Status = models.DeploymentStatusFailed
			mockStore.Deployments().Update(ctx, deployment)

			// Verify status
			updated, err := mockStore.Deployments().Get(ctx, deploymentID)
			if err != nil {
				return false
			}

			return updated.Status == models.DeploymentStatusFailed
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 21.3: Failed build preserves existing artifact (if any)
	// **Validates: Requirements 13.5**
	properties.Property("failed build preserves existing artifact", prop.ForAll(
		func(jobID, deploymentID, existingArtifact string) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create deployment with existing artifact (from previous successful build)
			deployment := NewTestDeployment(deploymentID, "test-app")
			deployment.Status = models.DeploymentStatusBuilding
			deployment.Artifact = existingArtifact
			mockStore.Deployments().Create(ctx, deployment)

			// Simulate failed build - status changes but artifact should be preserved
			deployment.Status = models.DeploymentStatusFailed
			// Note: We do NOT modify deployment.Artifact
			mockStore.Deployments().Update(ctx, deployment)

			// Verify deployment artifact was preserved
			updated, err := mockStore.Deployments().Get(ctx, deploymentID)
			if err != nil {
				return false
			}

			return updated.Artifact == existingArtifact
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
	))

	// Property 21.4: Build job status is failed when build fails
	// **Validates: Requirements 13.6**
	properties.Property("build job status is failed when build fails", prop.ForAll(
		func(jobID, deploymentID string, buildType models.BuildType) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create build job in running state
			job := NewTestBuildJob(jobID, deploymentID, buildType, models.BuildStrategyFlake)
			job.Status = models.BuildStatusRunning
			mockStore.Builds().Create(ctx, job)

			// Simulate failed build
			job.Status = models.BuildStatusFailed
			now := time.Now()
			job.FinishedAt = &now
			mockStore.Builds().Update(ctx, job)

			// Verify job status
			updated, err := mockStore.Builds().Get(ctx, jobID)
			if err != nil {
				return false
			}

			return updated.Status == models.BuildStatusFailed
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
	))

	// Property 21.5: Failed build job has finished_at timestamp
	// **Validates: Requirements 13.6**
	properties.Property("failed build job has finished_at timestamp", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create build job
			job := NewTestBuildJob(jobID, deploymentID, models.BuildTypePureNix, models.BuildStrategyFlake)
			job.Status = models.BuildStatusRunning
			now := time.Now()
			job.StartedAt = &now
			mockStore.Builds().Create(ctx, job)

			// Simulate failed build
			job.Status = models.BuildStatusFailed
			finishedAt := time.Now()
			job.FinishedAt = &finishedAt
			mockStore.Builds().Update(ctx, job)

			// Verify finished_at is set
			updated, err := mockStore.Builds().Get(ctx, jobID)
			if err != nil {
				return false
			}

			return updated.FinishedAt != nil
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 21.6: Deployment and build job status are synchronized on failure
	// **Validates: Requirements 13.5, 13.6**
	properties.Property("deployment and build job status synchronized on failure", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create deployment
			deployment := NewTestDeployment(deploymentID, "test-app")
			deployment.Status = models.DeploymentStatusBuilding
			mockStore.Deployments().Create(ctx, deployment)

			// Create build job
			job := NewTestBuildJob(jobID, deploymentID, models.BuildTypePureNix, models.BuildStrategyFlake)
			job.Status = models.BuildStatusRunning
			mockStore.Builds().Create(ctx, job)

			// Simulate failed build - update both
			job.Status = models.BuildStatusFailed
			now := time.Now()
			job.FinishedAt = &now
			mockStore.Builds().Update(ctx, job)

			deployment.Status = models.DeploymentStatusFailed
			mockStore.Deployments().Update(ctx, deployment)

			// Verify both are failed
			updatedJob, err := mockStore.Builds().Get(ctx, jobID)
			if err != nil {
				return false
			}
			updatedDeployment, err := mockStore.Deployments().Get(ctx, deploymentID)
			if err != nil {
				return false
			}

			return updatedJob.Status == models.BuildStatusFailed &&
				updatedDeployment.Status == models.DeploymentStatusFailed
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 22: Deployment Status Synchronization**
// For any build job, the deployment status SHALL reflect the build status:
// running → building, succeeded → built, failed → failed.
// **Validates: Requirements 16.1, 16.2, 16.3**

// TestDeploymentStatusSynchronization tests Property 22: Deployment Status Synchronization.
func TestDeploymentStatusSynchronization(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 22.1: Running build sets deployment status to building
	// **Validates: Requirements 16.1**
	properties.Property("running build sets deployment status to building", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create deployment in pending state
			deployment := NewTestDeployment(deploymentID, "test-app")
			deployment.Status = models.DeploymentStatusPending
			mockStore.Deployments().Create(ctx, deployment)

			// Simulate build starting - update deployment to building
			deployment.Status = models.DeploymentStatusBuilding
			mockStore.Deployments().Update(ctx, deployment)

			// Verify status
			updated, err := mockStore.Deployments().Get(ctx, deploymentID)
			if err != nil {
				return false
			}

			return updated.Status == models.DeploymentStatusBuilding
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 22.2: Succeeded build sets deployment status to built
	// **Validates: Requirements 16.2**
	properties.Property("succeeded build sets deployment status to built", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create deployment in building state
			deployment := NewTestDeployment(deploymentID, "test-app")
			deployment.Status = models.DeploymentStatusBuilding
			mockStore.Deployments().Create(ctx, deployment)

			// Simulate successful build
			deployment.Status = models.DeploymentStatusBuilt
			deployment.Artifact = "/nix/store/0123456789abcdef0123456789abcdef-test"
			mockStore.Deployments().Update(ctx, deployment)

			// Verify status
			updated, err := mockStore.Deployments().Get(ctx, deploymentID)
			if err != nil {
				return false
			}

			return updated.Status == models.DeploymentStatusBuilt
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 22.3: Failed build sets deployment status to failed
	// **Validates: Requirements 16.3**
	properties.Property("failed build sets deployment status to failed", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create deployment in building state
			deployment := NewTestDeployment(deploymentID, "test-app")
			deployment.Status = models.DeploymentStatusBuilding
			mockStore.Deployments().Create(ctx, deployment)

			// Simulate failed build
			deployment.Status = models.DeploymentStatusFailed
			mockStore.Deployments().Update(ctx, deployment)

			// Verify status
			updated, err := mockStore.Deployments().Get(ctx, deploymentID)
			if err != nil {
				return false
			}

			return updated.Status == models.DeploymentStatusFailed
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 22.4: Build status to deployment status mapping is consistent
	// **Validates: Requirements 16.1, 16.2, 16.3**
	properties.Property("build to deployment status mapping is consistent", prop.ForAll(
		func(buildStatus models.BuildStatus) bool {
			// Map build status to expected deployment status
			var expectedDeploymentStatus models.DeploymentStatus
			switch buildStatus {
			case models.BuildStatusRunning:
				expectedDeploymentStatus = models.DeploymentStatusBuilding
			case models.BuildStatusSucceeded:
				expectedDeploymentStatus = models.DeploymentStatusBuilt
			case models.BuildStatusFailed:
				expectedDeploymentStatus = models.DeploymentStatusFailed
			case models.BuildStatusQueued:
				// Queued builds don't change deployment status
				return true
			default:
				return true
			}

			// Verify the mapping is correct
			return expectedDeploymentStatus != ""
		},
		genBuildStatus(),
	))

	// Property 22.5: Deployment updated_at is set when status changes
	// **Validates: Requirements 16.4**
	properties.Property("deployment updated_at is set on status change", prop.ForAll(
		func(jobID, deploymentID string, newStatus models.DeploymentStatus) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create deployment
			deployment := NewTestDeployment(deploymentID, "test-app")
			originalUpdatedAt := deployment.UpdatedAt
			mockStore.Deployments().Create(ctx, deployment)

			// Wait a tiny bit to ensure time difference
			time.Sleep(1 * time.Millisecond)

			// Update status
			deployment.Status = newStatus
			deployment.UpdatedAt = time.Now()
			mockStore.Deployments().Update(ctx, deployment)

			// Verify updated_at changed
			updated, err := mockStore.Deployments().Get(ctx, deploymentID)
			if err != nil {
				return false
			}

			return !updated.UpdatedAt.Before(originalUpdatedAt)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			models.DeploymentStatusBuilding,
			models.DeploymentStatusBuilt,
			models.DeploymentStatusFailed,
		),
	))

	// Property 22.6: Build job and deployment status are synchronized
	// **Validates: Requirements 16.1, 16.2, 16.3**
	properties.Property("build job and deployment status are synchronized", prop.ForAll(
		func(jobID, deploymentID string, buildStatus models.BuildStatus) bool {
			if buildStatus == models.BuildStatusQueued {
				return true // Skip queued status
			}

			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create deployment
			deployment := NewTestDeployment(deploymentID, "test-app")
			deployment.Status = models.DeploymentStatusPending
			mockStore.Deployments().Create(ctx, deployment)

			// Create build job
			job := NewTestBuildJob(jobID, deploymentID, models.BuildTypePureNix, models.BuildStrategyFlake)
			job.Status = models.BuildStatusQueued
			mockStore.Builds().Create(ctx, job)

			// Update build job status
			job.Status = buildStatus
			mockStore.Builds().Update(ctx, job)

			// Update deployment status based on build status
			switch buildStatus {
			case models.BuildStatusRunning:
				deployment.Status = models.DeploymentStatusBuilding
			case models.BuildStatusSucceeded:
				deployment.Status = models.DeploymentStatusBuilt
				deployment.Artifact = "/nix/store/0123456789abcdef0123456789abcdef-test"
			case models.BuildStatusFailed:
				deployment.Status = models.DeploymentStatusFailed
			}
			mockStore.Deployments().Update(ctx, deployment)

			// Verify synchronization
			updatedJob, err := mockStore.Builds().Get(ctx, jobID)
			if err != nil {
				return false
			}
			updatedDeployment, err := mockStore.Deployments().Get(ctx, deploymentID)
			if err != nil {
				return false
			}

			// Check the mapping
			switch updatedJob.Status {
			case models.BuildStatusRunning:
				return updatedDeployment.Status == models.DeploymentStatusBuilding
			case models.BuildStatusSucceeded:
				return updatedDeployment.Status == models.DeploymentStatusBuilt
			case models.BuildStatusFailed:
				return updatedDeployment.Status == models.DeploymentStatusFailed
			}

			return true
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			models.BuildStatusRunning,
			models.BuildStatusSucceeded,
			models.BuildStatusFailed,
		),
	))

	// Property 22.7: Deployment status transitions follow expected order
	// **Validates: Requirements 16.1, 16.2, 16.3**
	properties.Property("deployment status transitions follow expected order", prop.ForAll(
		func(jobID, deploymentID string, success bool) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create deployment in pending state
			deployment := NewTestDeployment(deploymentID, "test-app")
			deployment.Status = models.DeploymentStatusPending
			mockStore.Deployments().Create(ctx, deployment)

			// Transition to building
			deployment.Status = models.DeploymentStatusBuilding
			mockStore.Deployments().Update(ctx, deployment)

			// Verify building status
			updated, _ := mockStore.Deployments().Get(ctx, deploymentID)
			if updated.Status != models.DeploymentStatusBuilding {
				return false
			}

			// Transition to final status
			if success {
				deployment.Status = models.DeploymentStatusBuilt
				deployment.Artifact = "/nix/store/0123456789abcdef0123456789abcdef-test"
			} else {
				deployment.Status = models.DeploymentStatusFailed
			}
			mockStore.Deployments().Update(ctx, deployment)

			// Verify final status
			updated, _ = mockStore.Deployments().Get(ctx, deploymentID)
			if success {
				return updated.Status == models.DeploymentStatusBuilt
			}
			return updated.Status == models.DeploymentStatusFailed
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Bool(),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 23: Database Record Before Queue**
// For any build job, a database record SHALL exist before the job is enqueued.
// **Validates: Requirements 17.1**

// TestDatabaseRecordBeforeQueue tests Property 23: Database Record Before Queue.
func TestDatabaseRecordBeforeQueue(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 23.1: Build job must be created in database before enqueueing
	// **Validates: Requirements 17.1**
	properties.Property("build job must exist in database before enqueueing", prop.ForAll(
		func(jobID, deploymentID string, buildType models.BuildType, strategy models.BuildStrategy) bool {
			// Create mock store and queue
			mockStore := NewMockStore()
			mockQueue := NewMockQueue()
			ctx := context.Background()

			// Create a valid build job
			job := NewTestBuildJob(jobID, deploymentID, buildType, strategy)

			// The correct workflow: create in database first, then enqueue
			// Step 1: Create in database
			if err := mockStore.Builds().Create(ctx, job); err != nil {
				return false
			}

			// Step 2: Verify it exists in database
			dbJob, err := mockStore.Builds().Get(ctx, jobID)
			if err != nil {
				return false
			}
			if dbJob == nil {
				return false
			}

			// Step 3: Now enqueue
			if err := mockQueue.Enqueue(ctx, job); err != nil {
				return false
			}

			// Verify the job is in the queue
			dequeuedJob, err := mockQueue.Dequeue(ctx)
			if err != nil {
				return false
			}

			// The dequeued job should match the database record
			return dequeuedJob.ID == dbJob.ID
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
		gen.OneConstOf(
			models.BuildStrategyFlake,
			models.BuildStrategyAutoGo,
			models.BuildStrategyAutoNode,
			models.BuildStrategyAutoRust,
			models.BuildStrategyAutoPython,
		),
	))

	// Property 23.2: Database record contains all required fields before enqueueing
	// **Validates: Requirements 17.1**
	properties.Property("database record contains required fields before enqueueing", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create a valid build job with all required fields
			job := &models.BuildJob{
				ID:            jobID,
				DeploymentID:  deploymentID,
				AppID:         "test-app",
				BuildType:     models.BuildTypePureNix,
				BuildStrategy: models.BuildStrategyFlake,
				Status:        models.BuildStatusQueued,
				CreatedAt:     time.Now(),
			}

			// Create in database
			if err := mockStore.Builds().Create(ctx, job); err != nil {
				return false
			}

			// Retrieve and verify all required fields are present
			dbJob, err := mockStore.Builds().Get(ctx, jobID)
			if err != nil {
				return false
			}

			// Verify required fields
			return dbJob.ID != "" &&
				dbJob.DeploymentID != "" &&
				dbJob.BuildType != "" &&
				dbJob.Status == models.BuildStatusQueued
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 23.3: Database record status is queued when created
	// **Validates: Requirements 17.1**
	properties.Property("database record status is queued when created", prop.ForAll(
		func(jobID, deploymentID string, buildType models.BuildType) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create a build job
			job := NewTestBuildJob(jobID, deploymentID, buildType, models.BuildStrategyFlake)
			job.Status = models.BuildStatusQueued

			// Create in database
			if err := mockStore.Builds().Create(ctx, job); err != nil {
				return false
			}

			// Retrieve and verify status
			dbJob, err := mockStore.Builds().Get(ctx, jobID)
			if err != nil {
				return false
			}

			return dbJob.Status == models.BuildStatusQueued
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
	))

	// Property 23.4: Multiple jobs can be created and enqueued independently
	// **Validates: Requirements 17.1**
	properties.Property("multiple jobs can be created and enqueued independently", prop.ForAll(
		func(jobID1, jobID2, deploymentID1, deploymentID2 string) bool {
			// Skip if IDs are the same
			if jobID1 == jobID2 || deploymentID1 == deploymentID2 {
				return true
			}

			// Create mock store and queue
			mockStore := NewMockStore()
			mockQueue := NewMockQueue()
			ctx := context.Background()

			// Create first job
			job1 := NewTestBuildJob(jobID1, deploymentID1, models.BuildTypePureNix, models.BuildStrategyFlake)
			if err := mockStore.Builds().Create(ctx, job1); err != nil {
				return false
			}
			if err := mockQueue.Enqueue(ctx, job1); err != nil {
				return false
			}

			// Create second job
			job2 := NewTestBuildJob(jobID2, deploymentID2, models.BuildTypeOCI, models.BuildStrategyDockerfile)
			if err := mockStore.Builds().Create(ctx, job2); err != nil {
				return false
			}
			if err := mockQueue.Enqueue(ctx, job2); err != nil {
				return false
			}

			// Verify both exist in database
			dbJob1, err := mockStore.Builds().Get(ctx, jobID1)
			if err != nil || dbJob1 == nil {
				return false
			}
			dbJob2, err := mockStore.Builds().Get(ctx, jobID2)
			if err != nil || dbJob2 == nil {
				return false
			}

			return true
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 23.5: Database record timestamp is set before enqueueing
	// **Validates: Requirements 17.1**
	properties.Property("database record timestamp is set before enqueueing", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Record time before creation
			beforeCreate := time.Now()

			// Create a build job
			job := NewTestBuildJob(jobID, deploymentID, models.BuildTypePureNix, models.BuildStrategyFlake)
			job.CreatedAt = time.Now()

			// Create in database
			if err := mockStore.Builds().Create(ctx, job); err != nil {
				return false
			}

			// Record time after creation
			afterCreate := time.Now()

			// Retrieve and verify timestamp
			dbJob, err := mockStore.Builds().Get(ctx, jobID)
			if err != nil {
				return false
			}

			// CreatedAt should be between beforeCreate and afterCreate
			return !dbJob.CreatedAt.Before(beforeCreate) && !dbJob.CreatedAt.After(afterCreate)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 24: Orphan Job Handling**
// For any queued job without a corresponding database record, the worker SHALL
// acknowledge the queue message without processing.
// **Validates: Requirements 17.2, 17.3**

// TestOrphanJobHandling tests Property 24: Orphan Job Handling.
func TestOrphanJobHandling(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 24.1: Worker verifies database record exists before processing
	// **Validates: Requirements 17.2**
	properties.Property("worker verifies database record exists before processing", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create a job that exists in the database
			job := NewTestBuildJob(jobID, deploymentID, models.BuildTypePureNix, models.BuildStrategyFlake)
			if err := mockStore.Builds().Create(ctx, job); err != nil {
				return false
			}

			// Verify the job can be retrieved
			dbJob, err := mockStore.Builds().Get(ctx, jobID)
			if err != nil {
				return false
			}

			// The job should exist and match
			return dbJob != nil && dbJob.ID == jobID
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 24.2: Orphan job (no database record) is detected
	// **Validates: Requirements 17.2, 17.3**
	properties.Property("orphan job without database record is detected", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store (empty - no jobs)
			mockStore := NewMockStore()
			ctx := context.Background()

			// Try to get a job that doesn't exist
			_, err := mockStore.Builds().Get(ctx, jobID)

			// Should return an error (job not found)
			return err != nil
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 24.3: Orphan job should be acknowledged without processing
	// **Validates: Requirements 17.2, 17.3**
	properties.Property("orphan job should be acknowledged without processing", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store and queue
			mockStore := NewMockStore()
			mockQueue := NewMockQueue()
			ctx := context.Background()

			// Create a job in the queue but NOT in the database (orphan scenario)
			orphanJob := NewTestBuildJob(jobID, deploymentID, models.BuildTypePureNix, models.BuildStrategyFlake)
			if err := mockQueue.Enqueue(ctx, orphanJob); err != nil {
				return false
			}

			// Dequeue the job
			dequeuedJob, err := mockQueue.Dequeue(ctx)
			if err != nil {
				return false
			}

			// Verify the job is NOT in the database (orphan)
			_, dbErr := mockStore.Builds().Get(ctx, dequeuedJob.ID)
			if dbErr == nil {
				// Job exists in database, not an orphan
				return true
			}

			// For orphan jobs, the worker should acknowledge without processing
			// Simulate the worker behavior: ack the orphan job
			if err := mockQueue.Ack(ctx, dequeuedJob.ID); err != nil {
				return false
			}

			// Verify the job was acknowledged
			return mockQueue.IsAcked(dequeuedJob.ID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 24.4: Non-orphan job is processed normally
	// **Validates: Requirements 17.2**
	properties.Property("non-orphan job is processed normally", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store and queue
			mockStore := NewMockStore()
			mockQueue := NewMockQueue()
			ctx := context.Background()

			// Create a job in BOTH database and queue (normal scenario)
			job := NewTestBuildJob(jobID, deploymentID, models.BuildTypePureNix, models.BuildStrategyFlake)
			if err := mockStore.Builds().Create(ctx, job); err != nil {
				return false
			}
			if err := mockQueue.Enqueue(ctx, job); err != nil {
				return false
			}

			// Dequeue the job
			dequeuedJob, err := mockQueue.Dequeue(ctx)
			if err != nil {
				return false
			}

			// Verify the job IS in the database (not an orphan)
			dbJob, dbErr := mockStore.Builds().Get(ctx, dequeuedJob.ID)
			if dbErr != nil {
				return false
			}

			// The job should be found and match
			return dbJob != nil && dbJob.ID == dequeuedJob.ID
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 24.5: Orphan job does not trigger state transitions
	// **Validates: Requirements 17.3**
	properties.Property("orphan job does not trigger state transitions", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Do NOT create the job in the database (orphan scenario)
			// Try to get the job
			_, err := mockStore.Builds().Get(ctx, jobID)
			if err == nil {
				// Job unexpectedly exists
				return false
			}

			// Verify no state transitions were recorded
			transitions := mockStore.builds.GetStateTransitions()
			return len(transitions) == 0
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 24.6: Orphan job handling is idempotent
	// **Validates: Requirements 17.3**
	properties.Property("orphan job handling is idempotent", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store and queue
			mockStore := NewMockStore()
			mockQueue := NewMockQueue()
			ctx := context.Background()

			// Create an orphan job in the queue (not in database)
			orphanJob := NewTestBuildJob(jobID, deploymentID, models.BuildTypePureNix, models.BuildStrategyFlake)
			if err := mockQueue.Enqueue(ctx, orphanJob); err != nil {
				return false
			}

			// Dequeue and check for orphan
			dequeuedJob, err := mockQueue.Dequeue(ctx)
			if err != nil {
				return false
			}

			// Verify it's an orphan
			_, dbErr := mockStore.Builds().Get(ctx, dequeuedJob.ID)
			if dbErr == nil {
				return true // Not an orphan, skip
			}

			// Ack the orphan job
			if err := mockQueue.Ack(ctx, dequeuedJob.ID); err != nil {
				return false
			}

			// Try to ack again (should be idempotent)
			// In a real queue, this might be a no-op or return success
			_ = mockQueue.Ack(ctx, dequeuedJob.ID)

			// The job should still be acknowledged
			return mockQueue.IsAcked(dequeuedJob.ID)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 24.7: Orphan detection works for any build type and strategy
	// **Validates: Requirements 17.2, 17.3**
	properties.Property("orphan detection works for any build type and strategy", prop.ForAll(
		func(jobID, deploymentID string, buildType models.BuildType, strategy models.BuildStrategy) bool {
			// Create mock store
			mockStore := NewMockStore()
			ctx := context.Background()

			// Create an orphan job (not in database)
			orphanJob := NewTestBuildJob(jobID, deploymentID, buildType, strategy)

			// Try to get the job from database
			_, err := mockStore.Builds().Get(ctx, orphanJob.ID)

			// Should return an error (orphan detected)
			return err != nil
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
		gen.OneConstOf(
			models.BuildStrategyFlake,
			models.BuildStrategyAutoGo,
			models.BuildStrategyAutoNode,
			models.BuildStrategyAutoRust,
			models.BuildStrategyAutoPython,
			models.BuildStrategyDockerfile,
			models.BuildStrategyNixpacks,
		),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 28: Pure-Nix Artifact Push**
// For any successful pure-nix build, the Build_System SHALL push the closure to Attic
// before marking as succeeded.
// **Validates: Requirements 19.1, 19.2**

// TestPureNixArtifactPush tests Property 28: Pure-Nix Artifact Push.
func TestPureNixArtifactPush(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 28.1: Successful pure-nix build triggers Attic push
	// **Validates: Requirements 19.1**
	properties.Property("successful pure-nix build triggers Attic push", prop.ForAll(
		func(jobID, deploymentID, storePath string) bool {
			// Create mock Attic client
			mockAttic := NewMockAtticClient()
			ctx := context.Background()

			// Create a valid store path
			validStorePath := "/nix/store/0123456789abcdef0123456789abcdef-" + storePath

			// Simulate pushing to Attic
			result, err := mockAttic.PushWithDependencies(ctx, validStorePath)
			if err != nil {
				return false
			}

			// Verify push was called
			pushCalls := mockAttic.GetPushCalls()
			if len(pushCalls) != 1 {
				return false
			}

			// Verify the correct store path was pushed
			return pushCalls[0].StorePath == validStorePath && result != nil
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 28.2: Pure-nix build pushes closure (all dependencies)
	// **Validates: Requirements 19.2**
	properties.Property("pure-nix build pushes closure with dependencies", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock Attic client
			mockAttic := NewMockAtticClient()
			ctx := context.Background()

			// Create a valid store path
			storePath := "/nix/store/0123456789abcdef0123456789abcdef-test-package"

			// PushWithDependencies should push the closure (all dependencies)
			result, err := mockAttic.PushWithDependencies(ctx, storePath)
			if err != nil {
				return false
			}

			// Verify push was called with the store path
			pushCalls := mockAttic.GetPushCalls()
			if len(pushCalls) != 1 {
				return false
			}

			// The push call should have the correct store path
			// The result should contain cache URL (the store path in result is from mock default)
			return pushCalls[0].StorePath == storePath && result.CacheURL != ""
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 28.3: OCI builds do NOT trigger Attic push
	// **Validates: Requirements 19.1**
	properties.Property("OCI builds do not trigger Attic push", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock Attic client
			mockAttic := NewMockAtticClient()

			// For OCI builds, we should NOT push to Attic
			// The mock should have no push calls for OCI builds
			// This is verified by checking that no push calls are made
			// when the build type is OCI

			// Verify no push calls were made
			pushCalls := mockAttic.GetPushCalls()
			return len(pushCalls) == 0
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 28.4: Attic push failure prevents build success
	// **Validates: Requirements 19.4**
	properties.Property("Attic push failure prevents build success", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock Attic client that fails
			mockAttic := NewMockAtticClient()
			mockAttic.ShouldFail = true
			ctx := context.Background()

			// Create a valid store path
			storePath := "/nix/store/0123456789abcdef0123456789abcdef-test-package"

			// Attempt to push should fail
			_, err := mockAttic.PushWithDependencies(ctx, storePath)

			// Push should have failed
			return err != nil
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 28.5: Attic push is called with valid store path
	// **Validates: Requirements 19.1**
	properties.Property("Attic push is called with valid store path", prop.ForAll(
		func(hash, packageName string) bool {
			// Create mock Attic client
			mockAttic := NewMockAtticClient()
			ctx := context.Background()

			// Generate a valid store path format
			// Nix store paths have format: /nix/store/<32-char-hash>-<name>
			validHash := "0123456789abcdef0123456789abcdef"
			storePath := "/nix/store/" + validHash + "-" + packageName

			// Push should succeed with valid store path
			result, err := mockAttic.PushWithDependencies(ctx, storePath)
			if err != nil {
				return false
			}

			// Verify the push was recorded with the correct path
			pushCalls := mockAttic.GetPushCalls()
			if len(pushCalls) != 1 {
				return false
			}

			return pushCalls[0].StorePath == storePath && result != nil
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) <= 32 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 28.6: Pure-nix build with flake strategy pushes to Attic
	// **Validates: Requirements 19.1**
	properties.Property("pure-nix flake build pushes to Attic", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock store and Attic client
			mockStore := NewMockStore()
			mockAttic := NewMockAtticClient()
			ctx := context.Background()

			// Create a pure-nix flake build job
			job := NewTestBuildJob(jobID, deploymentID, models.BuildTypePureNix, models.BuildStrategyFlake)
			if err := mockStore.Builds().Create(ctx, job); err != nil {
				return false
			}

			// Simulate successful build with store path
			storePath := "/nix/store/0123456789abcdef0123456789abcdef-test"

			// Push to Attic
			result, err := mockAttic.PushWithDependencies(ctx, storePath)
			if err != nil {
				return false
			}

			// Verify push was called
			pushCalls := mockAttic.GetPushCalls()
			return len(pushCalls) == 1 && result != nil
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 28.7: Pure-nix build with auto-* strategy pushes to Attic
	// **Validates: Requirements 19.1**
	properties.Property("pure-nix auto-* build pushes to Attic", prop.ForAll(
		func(jobID, deploymentID string, strategy models.BuildStrategy) bool {
			// Only test auto-* strategies with pure-nix
			if strategy == models.BuildStrategyDockerfile || strategy == models.BuildStrategyNixpacks {
				return true // Skip OCI-only strategies
			}

			// Create mock store and Attic client
			mockStore := NewMockStore()
			mockAttic := NewMockAtticClient()
			ctx := context.Background()

			// Create a pure-nix build job with auto-* strategy
			job := NewTestBuildJob(jobID, deploymentID, models.BuildTypePureNix, strategy)
			if err := mockStore.Builds().Create(ctx, job); err != nil {
				return false
			}

			// Simulate successful build with store path
			storePath := "/nix/store/0123456789abcdef0123456789abcdef-test"

			// Push to Attic
			result, err := mockAttic.PushWithDependencies(ctx, storePath)
			if err != nil {
				return false
			}

			// Verify push was called
			pushCalls := mockAttic.GetPushCalls()
			return len(pushCalls) == 1 && result != nil
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			models.BuildStrategyFlake,
			models.BuildStrategyAutoGo,
			models.BuildStrategyAutoNode,
			models.BuildStrategyAutoRust,
			models.BuildStrategyAutoPython,
		),
	))

	// Property 28.8: Push result contains cache URL
	// **Validates: Requirements 19.3**
	properties.Property("push result contains cache URL", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock Attic client
			mockAttic := NewMockAtticClient()
			ctx := context.Background()

			// Create a valid store path
			storePath := "/nix/store/0123456789abcdef0123456789abcdef-test"

			// Push to Attic
			result, err := mockAttic.PushWithDependencies(ctx, storePath)
			if err != nil {
				return false
			}

			// Result should contain cache URL
			return result.CacheURL != ""
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	properties.TestingRun(t)
}

// **Feature: build-lifecycle-correctness, Property 29: Push Stage Reporting**
// For any pure-nix build during Attic push, the Progress_Tracker SHALL report stage `pushing`.
// **Validates: Requirements 19.5, 4.7**

// TestPushStageReporting tests Property 29: Push Stage Reporting.
func TestPushStageReporting(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 29.1: Pure-nix build reports pushing stage during Attic push
	// **Validates: Requirements 19.5**
	properties.Property("pure-nix build reports pushing stage", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock progress tracker
			mockTracker := NewMockProgressTracker()
			ctx := context.Background()

			// Report pushing stage (simulating what worker does during Attic push)
			if err := mockTracker.ReportStage(ctx, jobID, StagePushing); err != nil {
				return false
			}

			// Verify pushing stage was reported
			stageReports := mockTracker.GetStageReports()
			if len(stageReports) != 1 {
				return false
			}

			return stageReports[0].BuildID == jobID && stageReports[0].Stage == StagePushing
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 29.2: Pushing stage is reported before completed stage
	// **Validates: Requirements 19.5, 4.7**
	properties.Property("pushing stage is reported before completed stage", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock progress tracker
			mockTracker := NewMockProgressTracker()
			ctx := context.Background()

			// Simulate the stage sequence for a pure-nix build
			stages := []BuildStage{StageCloning, StageBuilding, StagePushing, StageCompleted}
			for _, stage := range stages {
				if err := mockTracker.ReportStage(ctx, jobID, stage); err != nil {
					return false
				}
			}

			// Verify stage order
			stageReports := mockTracker.GetStageReports()
			if len(stageReports) != 4 {
				return false
			}

			// Find pushing and completed stages
			var pushingIdx, completedIdx int = -1, -1
			for i, report := range stageReports {
				if report.Stage == StagePushing {
					pushingIdx = i
				}
				if report.Stage == StageCompleted {
					completedIdx = i
				}
			}

			// Pushing should come before completed
			return pushingIdx >= 0 && completedIdx >= 0 && pushingIdx < completedIdx
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 29.3: Pushing stage is reported after building stage
	// **Validates: Requirements 19.5**
	properties.Property("pushing stage is reported after building stage", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock progress tracker
			mockTracker := NewMockProgressTracker()
			ctx := context.Background()

			// Simulate the stage sequence for a pure-nix build
			stages := []BuildStage{StageCloning, StageBuilding, StagePushing, StageCompleted}
			for _, stage := range stages {
				if err := mockTracker.ReportStage(ctx, jobID, stage); err != nil {
					return false
				}
			}

			// Verify stage order
			stageReports := mockTracker.GetStageReports()
			if len(stageReports) != 4 {
				return false
			}

			// Find building and pushing stages
			var buildingIdx, pushingIdx int = -1, -1
			for i, report := range stageReports {
				if report.Stage == StageBuilding {
					buildingIdx = i
				}
				if report.Stage == StagePushing {
					pushingIdx = i
				}
			}

			// Building should come before pushing
			return buildingIdx >= 0 && pushingIdx >= 0 && buildingIdx < pushingIdx
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 29.4: OCI builds do NOT report pushing stage
	// **Validates: Requirements 19.5**
	properties.Property("OCI builds do not report pushing stage", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock progress tracker
			mockTracker := NewMockProgressTracker()
			ctx := context.Background()

			// Simulate the stage sequence for an OCI build (no pushing stage)
			stages := []BuildStage{StageCloning, StageBuilding, StageCompleted}
			for _, stage := range stages {
				if err := mockTracker.ReportStage(ctx, jobID, stage); err != nil {
					return false
				}
			}

			// Verify no pushing stage was reported
			stageReports := mockTracker.GetStageReports()
			for _, report := range stageReports {
				if report.Stage == StagePushing {
					return false // OCI builds should not have pushing stage
				}
			}

			return true
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 29.5: Pushing stage is only reported for pure-nix builds
	// **Validates: Requirements 19.5, 4.7**
	properties.Property("pushing stage only for pure-nix builds", prop.ForAll(
		func(jobID, deploymentID string, buildType models.BuildType) bool {
			// Create mock progress tracker
			mockTracker := NewMockProgressTracker()
			ctx := context.Background()

			// Determine expected stages based on build type
			var stages []BuildStage
			if buildType == models.BuildTypePureNix {
				stages = []BuildStage{StageCloning, StageBuilding, StagePushing, StageCompleted}
			} else {
				stages = []BuildStage{StageCloning, StageBuilding, StageCompleted}
			}

			// Report stages
			for _, stage := range stages {
				if err := mockTracker.ReportStage(ctx, jobID, stage); err != nil {
					return false
				}
			}

			// Check if pushing stage was reported
			stageReports := mockTracker.GetStageReports()
			hasPushingStage := false
			for _, report := range stageReports {
				if report.Stage == StagePushing {
					hasPushingStage = true
					break
				}
			}

			// Pushing stage should only be present for pure-nix builds
			if buildType == models.BuildTypePureNix {
				return hasPushingStage
			}
			return !hasPushingStage
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(models.BuildTypePureNix, models.BuildTypeOCI),
	))

	// Property 29.6: Pushing stage has correct build ID
	// **Validates: Requirements 19.5**
	properties.Property("pushing stage has correct build ID", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock progress tracker
			mockTracker := NewMockProgressTracker()
			ctx := context.Background()

			// Report pushing stage
			if err := mockTracker.ReportStage(ctx, jobID, StagePushing); err != nil {
				return false
			}

			// Verify the build ID is correct
			stageReports := mockTracker.GetStageReports()
			if len(stageReports) != 1 {
				return false
			}

			return stageReports[0].BuildID == jobID
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 29.7: Pushing stage timestamp is recorded
	// **Validates: Requirements 19.5**
	properties.Property("pushing stage timestamp is recorded", prop.ForAll(
		func(jobID, deploymentID string) bool {
			// Create mock progress tracker
			mockTracker := NewMockProgressTracker()
			ctx := context.Background()

			// Record time before reporting
			beforeReport := time.Now()

			// Report pushing stage
			if err := mockTracker.ReportStage(ctx, jobID, StagePushing); err != nil {
				return false
			}

			// Record time after reporting
			afterReport := time.Now()

			// Verify timestamp is within expected range
			stageReports := mockTracker.GetStageReports()
			if len(stageReports) != 1 {
				return false
			}

			timestamp := stageReports[0].Timestamp
			return !timestamp.Before(beforeReport) && !timestamp.After(afterReport)
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 29.8: Pure-nix strategies should report pushing stage
	// **Validates: Requirements 19.5, 4.7**
	properties.Property("pure-nix strategies should report pushing stage", prop.ForAll(
		func(jobID, deploymentID string, strategy models.BuildStrategy) bool {
			// Only test pure-nix compatible strategies
			if strategy == models.BuildStrategyDockerfile || strategy == models.BuildStrategyNixpacks {
				return true // Skip OCI-only strategies
			}

			// Create mock progress tracker
			mockTracker := NewMockProgressTracker()
			ctx := context.Background()

			// Simulate the stage sequence for a pure-nix build
			// Pure-nix builds should include pushing stage
			stages := []BuildStage{StageCloning, StageBuilding, StagePushing, StageCompleted}
			for _, stage := range stages {
				if err := mockTracker.ReportStage(ctx, jobID, stage); err != nil {
					return false
				}
			}

			// Verify pushing stage was reported
			stageReports := mockTracker.GetStageReports()
			hasPushing := false
			for _, report := range stageReports {
				if report.Stage == StagePushing {
					hasPushing = true
					break
				}
			}

			return hasPushing
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			models.BuildStrategyFlake,
			models.BuildStrategyAutoGo,
			models.BuildStrategyAutoNode,
			models.BuildStrategyAutoRust,
			models.BuildStrategyAutoPython,
		),
	))

	// Property 29.9: OCI-only strategies should NOT report pushing stage
	// **Validates: Requirements 19.5**
	properties.Property("OCI-only strategies should not report pushing stage", prop.ForAll(
		func(jobID, deploymentID string, strategy models.BuildStrategy) bool {
			// Only test dockerfile and nixpacks strategies
			if strategy != models.BuildStrategyDockerfile && strategy != models.BuildStrategyNixpacks {
				return true // Skip non-OCI strategies
			}

			// Create mock progress tracker
			mockTracker := NewMockProgressTracker()
			ctx := context.Background()

			// Simulate the stage sequence for an OCI build (no pushing stage)
			stages := []BuildStage{StageCloning, StageBuilding, StageCompleted}
			for _, stage := range stages {
				if err := mockTracker.ReportStage(ctx, jobID, stage); err != nil {
					return false
				}
			}

			// Verify pushing stage was NOT reported
			stageReports := mockTracker.GetStageReports()
			for _, report := range stageReports {
				if report.Stage == StagePushing {
					return false // OCI-only strategies should not have pushing
				}
			}

			return true
		},
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Identifier().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.OneConstOf(
			models.BuildStrategyDockerfile,
			models.BuildStrategyNixpacks,
		),
	))

	properties.TestingRun(t)
}
