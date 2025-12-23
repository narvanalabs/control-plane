package builder

import (
	"testing"

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
