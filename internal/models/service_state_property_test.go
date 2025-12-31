package models

import (
	"reflect"
	"sort"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: platform-enhancements, Property 2: Service State Action Availability**
// For any service state, the available actions SHALL be exactly:
// - "new" → ["deploy"]
// - "deploying" → []
// - "running" → ["stop", "reload", "rebuild"]
// - "stopped" → ["start", "rebuild"]
// - "failed" → ["retry", "rebuild"]
// **Validates: Requirements 7.1, 7.2, 7.3, 7.4, 7.5, 7.6**

// genServiceState generates a random ServiceState.
func genServiceState() gopter.Gen {
	return gen.OneConstOf(
		ServiceStateNew,
		ServiceStateDeploying,
		ServiceStateRunning,
		ServiceStateStopped,
		ServiceStateFailed,
	)
}

// expectedActionsForState returns the expected actions for a given state.
func expectedActionsForState(state ServiceState) []ServiceAction {
	switch state {
	case ServiceStateNew:
		return []ServiceAction{ServiceActionDeploy}
	case ServiceStateDeploying:
		return []ServiceAction{}
	case ServiceStateRunning:
		return []ServiceAction{ServiceActionStop, ServiceActionReload, ServiceActionRebuild}
	case ServiceStateStopped:
		return []ServiceAction{ServiceActionStart, ServiceActionRebuild}
	case ServiceStateFailed:
		return []ServiceAction{ServiceActionRetry, ServiceActionRebuild}
	default:
		return []ServiceAction{}
	}
}

// sortActions sorts a slice of ServiceAction for comparison.
func sortActions(actions []ServiceAction) []ServiceAction {
	sorted := make([]ServiceAction, len(actions))
	copy(sorted, actions)
	sort.Slice(sorted, func(i, j int) bool {
		return string(sorted[i]) < string(sorted[j])
	})
	return sorted
}

// actionsEqual compares two slices of ServiceAction for equality.
func actionsEqual(a, b []ServiceAction) bool {
	if len(a) != len(b) {
		return false
	}
	sortedA := sortActions(a)
	sortedB := sortActions(b)
	return reflect.DeepEqual(sortedA, sortedB)
}

// TestServiceStateActionAvailability tests Property 2: Service State Action Availability.
func TestServiceStateActionAvailability(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Each state returns exactly the expected actions
	properties.Property("Service state returns correct available actions", prop.ForAll(
		func(state ServiceState) bool {
			actual := state.AvailableActions()
			expected := expectedActionsForState(state)
			return actionsEqual(actual, expected)
		},
		genServiceState(),
	))

	// Property: New state only has deploy action
	properties.Property("New state only allows deploy", prop.ForAll(
		func(_ int) bool {
			state := ServiceStateNew
			actions := state.AvailableActions()
			return len(actions) == 1 && actions[0] == ServiceActionDeploy
		},
		gen.IntRange(0, 100),
	))

	// Property: Deploying state has no actions
	properties.Property("Deploying state has no available actions", prop.ForAll(
		func(_ int) bool {
			state := ServiceStateDeploying
			actions := state.AvailableActions()
			return len(actions) == 0
		},
		gen.IntRange(0, 100),
	))

	// Property: Running state has stop, reload, rebuild
	properties.Property("Running state allows stop, reload, rebuild", prop.ForAll(
		func(_ int) bool {
			state := ServiceStateRunning
			actions := state.AvailableActions()
			expected := []ServiceAction{ServiceActionStop, ServiceActionReload, ServiceActionRebuild}
			return actionsEqual(actions, expected)
		},
		gen.IntRange(0, 100),
	))

	// Property: Stopped state has start, rebuild
	properties.Property("Stopped state allows start, rebuild", prop.ForAll(
		func(_ int) bool {
			state := ServiceStateStopped
			actions := state.AvailableActions()
			expected := []ServiceAction{ServiceActionStart, ServiceActionRebuild}
			return actionsEqual(actions, expected)
		},
		gen.IntRange(0, 100),
	))

	// Property: Failed state has retry, rebuild
	properties.Property("Failed state allows retry, rebuild", prop.ForAll(
		func(_ int) bool {
			state := ServiceStateFailed
			actions := state.AvailableActions()
			expected := []ServiceAction{ServiceActionRetry, ServiceActionRebuild}
			return actionsEqual(actions, expected)
		},
		gen.IntRange(0, 100),
	))

	// Property: HasAction correctly reports action availability
	properties.Property("HasAction correctly reports action availability", prop.ForAll(
		func(state ServiceState) bool {
			actions := state.AvailableActions()
			
			// All available actions should return true for HasAction
			for _, action := range actions {
				if !state.HasAction(action) {
					return false
				}
			}
			
			// Actions not in the list should return false
			allActions := []ServiceAction{
				ServiceActionDeploy,
				ServiceActionStop,
				ServiceActionReload,
				ServiceActionRebuild,
				ServiceActionStart,
				ServiceActionRetry,
			}
			
			for _, action := range allActions {
				hasIt := state.HasAction(action)
				shouldHaveIt := false
				for _, a := range actions {
					if a == action {
						shouldHaveIt = true
						break
					}
				}
				if hasIt != shouldHaveIt {
					return false
				}
			}
			
			return true
		},
		genServiceState(),
	))

	// Property: All valid states are recognized as valid
	properties.Property("All valid states are recognized as valid", prop.ForAll(
		func(state ServiceState) bool {
			return state.IsValid()
		},
		genServiceState(),
	))

	// Property: Invalid states are not recognized as valid
	properties.Property("Invalid states are not recognized as valid", prop.ForAll(
		func(s string) bool {
			state := ServiceState(s)
			// If it's one of the valid states, skip
			for _, valid := range ValidServiceStates() {
				if state == valid {
					return true // Skip valid states
				}
			}
			return !state.IsValid()
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// TestDeriveServiceState tests the DeriveServiceState function.
func TestDeriveServiceState(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Nil deployment results in "new" state
	properties.Property("Nil deployment results in new state", prop.ForAll(
		func(_ int) bool {
			state := DeriveServiceState(nil)
			return state == ServiceStateNew
		},
		gen.IntRange(0, 100),
	))

	// Property: Pending/Building/Built/Scheduled/Starting deployments result in "deploying" state
	genDeployingStatus := func() gopter.Gen {
		return gen.OneConstOf(
			DeploymentStatusPending,
			DeploymentStatusBuilding,
			DeploymentStatusBuilt,
			DeploymentStatusScheduled,
			DeploymentStatusStarting,
		)
	}

	properties.Property("In-progress deployment statuses result in deploying state", prop.ForAll(
		func(status DeploymentStatus) bool {
			deployment := &Deployment{Status: status}
			state := DeriveServiceState(deployment)
			return state == ServiceStateDeploying
		},
		genDeployingStatus(),
	))

	// Property: Running deployment results in "running" state
	properties.Property("Running deployment results in running state", prop.ForAll(
		func(_ int) bool {
			deployment := &Deployment{Status: DeploymentStatusRunning}
			state := DeriveServiceState(deployment)
			return state == ServiceStateRunning
		},
		gen.IntRange(0, 100),
	))

	// Property: Stopped/Stopping deployment results in "stopped" state
	genStoppedStatus := func() gopter.Gen {
		return gen.OneConstOf(
			DeploymentStatusStopping,
			DeploymentStatusStopped,
		)
	}

	properties.Property("Stopped deployment statuses result in stopped state", prop.ForAll(
		func(status DeploymentStatus) bool {
			deployment := &Deployment{Status: status}
			state := DeriveServiceState(deployment)
			return state == ServiceStateStopped
		},
		genStoppedStatus(),
	))

	// Property: Failed deployment results in "failed" state
	properties.Property("Failed deployment results in failed state", prop.ForAll(
		func(_ int) bool {
			deployment := &Deployment{Status: DeploymentStatusFailed}
			state := DeriveServiceState(deployment)
			return state == ServiceStateFailed
		},
		gen.IntRange(0, 100),
	))

	properties.TestingRun(t)
}
