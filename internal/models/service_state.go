// Package models provides data models for the Narvana platform.
package models

// ServiceState represents the current operational state of a service.
// This is derived from the latest deployment status for the service.
type ServiceState string

const (
	// ServiceStateNew indicates the service has never been deployed.
	ServiceStateNew ServiceState = "new"
	// ServiceStateDeploying indicates a deployment is in progress.
	ServiceStateDeploying ServiceState = "deploying"
	// ServiceStateRunning indicates the service container is running.
	ServiceStateRunning ServiceState = "running"
	// ServiceStateStopped indicates the service was manually stopped.
	ServiceStateStopped ServiceState = "stopped"
	// ServiceStateFailed indicates the last deployment failed.
	ServiceStateFailed ServiceState = "failed"
)

// ServiceAction represents an action that can be performed on a service.
type ServiceAction string

const (
	// ServiceActionDeploy triggers a new deployment.
	ServiceActionDeploy ServiceAction = "deploy"
	// ServiceActionStop gracefully stops the service container.
	ServiceActionStop ServiceAction = "stop"
	// ServiceActionReload restarts the service without rebuilding.
	ServiceActionReload ServiceAction = "reload"
	// ServiceActionRebuild triggers a full rebuild and deployment.
	ServiceActionRebuild ServiceAction = "rebuild"
	// ServiceActionStart starts a stopped service.
	ServiceActionStart ServiceAction = "start"
	// ServiceActionRetry retries a failed deployment.
	ServiceActionRetry ServiceAction = "retry"
)

// AvailableActions returns the actions available for a service in this state.
// **Feature: platform-enhancements, Property 2: Service State Action Availability**
// **Validates: Requirements 7.1, 7.2, 7.3, 7.4, 7.5, 7.6**
func (s ServiceState) AvailableActions() []ServiceAction {
	switch s {
	case ServiceStateNew:
		// Service has never been deployed - only deploy is available
		return []ServiceAction{ServiceActionDeploy}
	case ServiceStateDeploying:
		// Deployment in progress - no actions available
		return []ServiceAction{}
	case ServiceStateRunning:
		// Service is running - can stop, reload, or rebuild
		return []ServiceAction{ServiceActionStop, ServiceActionReload, ServiceActionRebuild}
	case ServiceStateStopped:
		// Service is stopped - can start or rebuild
		return []ServiceAction{ServiceActionStart, ServiceActionRebuild}
	case ServiceStateFailed:
		// Last deployment failed - can retry or rebuild
		return []ServiceAction{ServiceActionRetry, ServiceActionRebuild}
	default:
		return []ServiceAction{}
	}
}

// String returns the string representation of the service state.
func (s ServiceState) String() string {
	return string(s)
}

// IsValid returns true if the service state is a valid state.
func (s ServiceState) IsValid() bool {
	switch s {
	case ServiceStateNew, ServiceStateDeploying, ServiceStateRunning, ServiceStateStopped, ServiceStateFailed:
		return true
	default:
		return false
	}
}

// ValidServiceStates returns all valid service states.
func ValidServiceStates() []ServiceState {
	return []ServiceState{
		ServiceStateNew,
		ServiceStateDeploying,
		ServiceStateRunning,
		ServiceStateStopped,
		ServiceStateFailed,
	}
}

// DeriveServiceState derives the service state from the latest deployment status.
// If there are no deployments, the service is in the "new" state.
func DeriveServiceState(latestDeployment *Deployment) ServiceState {
	if latestDeployment == nil {
		return ServiceStateNew
	}

	switch latestDeployment.Status {
	case DeploymentStatusPending, DeploymentStatusBuilding, DeploymentStatusBuilt,
		DeploymentStatusScheduled, DeploymentStatusStarting:
		return ServiceStateDeploying
	case DeploymentStatusRunning:
		return ServiceStateRunning
	case DeploymentStatusStopping, DeploymentStatusStopped:
		return ServiceStateStopped
	case DeploymentStatusFailed:
		return ServiceStateFailed
	default:
		return ServiceStateNew
	}
}

// HasAction returns true if the given action is available for this state.
func (s ServiceState) HasAction(action ServiceAction) bool {
	for _, a := range s.AvailableActions() {
		if a == action {
			return true
		}
	}
	return false
}
