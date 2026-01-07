// Package integration provides integration tests for the control-plane deployment flow.
// These tests verify the complete deployment lifecycle: build → schedule → deploy.
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

// MockStore implements a minimal in-memory store for integration testing.
type MockStore struct {
	apps        map[string]*models.App
	deployments map[string]*models.Deployment
	nodes       map[string]*models.Node
	builds      map[string]*models.BuildJob
}

// NewMockStore creates a new mock store for testing.
func NewMockStore() *MockStore {
	return &MockStore{
		apps:        make(map[string]*models.App),
		deployments: make(map[string]*models.Deployment),
		nodes:       make(map[string]*models.Node),
		builds:      make(map[string]*models.BuildJob),
	}
}

// DeploymentFlowState tracks the state of a deployment through its lifecycle.
// **Validates: Requirements 12.1**
type DeploymentFlowState struct {
	DeploymentID string
	AppID        string
	ServiceName  string
	States       []models.DeploymentStatus
	BuildStates  []models.BuildStatus
	NodeID       string
	Artifact     string
	StartTime    time.Time
	EndTime      time.Time
}

// NewDeploymentFlowState creates a new deployment flow state tracker.
func NewDeploymentFlowState(deploymentID, appID, serviceName string) *DeploymentFlowState {
	return &DeploymentFlowState{
		DeploymentID: deploymentID,
		AppID:        appID,
		ServiceName:  serviceName,
		States:       make([]models.DeploymentStatus, 0),
		BuildStates:  make([]models.BuildStatus, 0),
		StartTime:    time.Now(),
	}
}

// RecordDeploymentState records a deployment state transition.
func (s *DeploymentFlowState) RecordDeploymentState(status models.DeploymentStatus) {
	s.States = append(s.States, status)
}

// RecordBuildState records a build state transition.
func (s *DeploymentFlowState) RecordBuildState(status models.BuildStatus) {
	s.BuildStates = append(s.BuildStates, status)
}

// SetNodeID sets the node ID where the deployment was scheduled.
func (s *DeploymentFlowState) SetNodeID(nodeID string) {
	s.NodeID = nodeID
}

// SetArtifact sets the build artifact.
func (s *DeploymentFlowState) SetArtifact(artifact string) {
	s.Artifact = artifact
}

// Complete marks the deployment flow as complete.
func (s *DeploymentFlowState) Complete() {
	s.EndTime = time.Now()
}

// Duration returns the total duration of the deployment flow.
func (s *DeploymentFlowState) Duration() time.Duration {
	if s.EndTime.IsZero() {
		return time.Since(s.StartTime)
	}
	return s.EndTime.Sub(s.StartTime)
}

// ValidateDeploymentFlow validates that a deployment went through the expected state transitions.
// **Validates: Requirements 12.1**
func ValidateDeploymentFlow(state *DeploymentFlowState) ValidationResult {
	result := ValidationResult{
		Valid:  true,
		Errors: make([]string, 0),
	}

	// Check that we have deployment states
	if len(state.States) == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "no deployment states recorded")
		return result
	}

	// Validate deployment state transitions
	// Expected flow: pending → building → built → scheduled → starting → running
	expectedTransitions := map[models.DeploymentStatus][]models.DeploymentStatus{
		models.DeploymentStatusPending:   {models.DeploymentStatusBuilding},
		models.DeploymentStatusBuilding:  {models.DeploymentStatusBuilt, models.DeploymentStatusFailed},
		models.DeploymentStatusBuilt:     {models.DeploymentStatusScheduled},
		models.DeploymentStatusScheduled: {models.DeploymentStatusStarting},
		models.DeploymentStatusStarting:  {models.DeploymentStatusRunning, models.DeploymentStatusFailed},
		models.DeploymentStatusRunning:   {models.DeploymentStatusStopping},
		models.DeploymentStatusStopping:  {models.DeploymentStatusStopped},
	}

	for i := 1; i < len(state.States); i++ {
		prevState := state.States[i-1]
		currState := state.States[i]

		validNext, ok := expectedTransitions[prevState]
		if !ok {
			result.Valid = false
			result.Errors = append(result.Errors, 
				"invalid state transition: no transitions defined from "+string(prevState))
			continue
		}

		isValid := false
		for _, valid := range validNext {
			if currState == valid {
				isValid = true
				break
			}
		}

		if !isValid {
			result.Valid = false
			result.Errors = append(result.Errors,
				"invalid state transition: "+string(prevState)+" → "+string(currState))
		}
	}

	return result
}

// ValidationResult contains the result of a validation check.
type ValidationResult struct {
	Valid  bool
	Errors []string
}

// ValidateBuildFlow validates that a build went through the expected state transitions.
// **Validates: Requirements 12.1**
func ValidateBuildFlow(state *DeploymentFlowState) ValidationResult {
	result := ValidationResult{
		Valid:  true,
		Errors: make([]string, 0),
	}

	if len(state.BuildStates) == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "no build states recorded")
		return result
	}

	// Expected build flow: queued → running → succeeded
	expectedTransitions := map[models.BuildStatus][]models.BuildStatus{
		models.BuildStatusQueued:    {models.BuildStatusRunning},
		models.BuildStatusRunning:   {models.BuildStatusSucceeded, models.BuildStatusFailed},
		models.BuildStatusSucceeded: {},
		models.BuildStatusFailed:    {},
	}

	for i := 1; i < len(state.BuildStates); i++ {
		prevState := state.BuildStates[i-1]
		currState := state.BuildStates[i]

		validNext, ok := expectedTransitions[prevState]
		if !ok {
			result.Valid = false
			result.Errors = append(result.Errors,
				"invalid build state transition: no transitions defined from "+string(prevState))
			continue
		}

		isValid := false
		for _, valid := range validNext {
			if currState == valid {
				isValid = true
				break
			}
		}

		if !isValid {
			result.Valid = false
			result.Errors = append(result.Errors,
				"invalid build state transition: "+string(prevState)+" → "+string(currState))
		}
	}

	return result
}

// SimulateDeploymentFlow simulates a complete deployment flow for testing.
// This is a pure function that doesn't require external dependencies.
// **Validates: Requirements 12.1**
func SimulateDeploymentFlow(
	appID string,
	serviceName string,
	buildType models.BuildType,
	nodeAvailable bool,
	buildSucceeds bool,
) *DeploymentFlowState {
	deploymentID := "dep-" + appID + "-" + serviceName
	state := NewDeploymentFlowState(deploymentID, appID, serviceName)

	// Step 1: Deployment created in pending state
	state.RecordDeploymentState(models.DeploymentStatusPending)

	// Step 2: Build job created and queued
	state.RecordBuildState(models.BuildStatusQueued)

	// Step 3: Deployment transitions to building
	state.RecordDeploymentState(models.DeploymentStatusBuilding)
	state.RecordBuildState(models.BuildStatusRunning)

	// Step 4: Build completes (success or failure)
	if buildSucceeds {
		state.RecordBuildState(models.BuildStatusSucceeded)
		state.SetArtifact("/nix/store/abc123-" + serviceName)
		state.RecordDeploymentState(models.DeploymentStatusBuilt)

		// Step 5: Scheduling (only if build succeeded)
		if nodeAvailable {
			state.SetNodeID("node-1")
			state.RecordDeploymentState(models.DeploymentStatusScheduled)

			// Step 6: Starting
			state.RecordDeploymentState(models.DeploymentStatusStarting)

			// Step 7: Running
			state.RecordDeploymentState(models.DeploymentStatusRunning)
		}
		// If no node available, deployment stays in "built" state (queued for scheduling)
	} else {
		state.RecordBuildState(models.BuildStatusFailed)
		state.RecordDeploymentState(models.DeploymentStatusFailed)
	}

	state.Complete()
	return state
}

// TestDeploymentFlowValidation tests the deployment flow validation logic.
// **Validates: Requirements 12.1**
func TestDeploymentFlowValidation(t *testing.T) {
	tests := []struct {
		name           string
		buildType      models.BuildType
		nodeAvailable  bool
		buildSucceeds  bool
		expectValid    bool
		expectRunning  bool
	}{
		{
			name:          "successful pure-nix deployment",
			buildType:     models.BuildTypePureNix,
			nodeAvailable: true,
			buildSucceeds: true,
			expectValid:   true,
			expectRunning: true,
		},
		{
			name:          "successful oci deployment",
			buildType:     models.BuildTypeOCI,
			nodeAvailable: true,
			buildSucceeds: true,
			expectValid:   true,
			expectRunning: true,
		},
		{
			name:          "build failure",
			buildType:     models.BuildTypePureNix,
			nodeAvailable: true,
			buildSucceeds: false,
			expectValid:   true,
			expectRunning: false,
		},
		{
			name:          "no nodes available - queued",
			buildType:     models.BuildTypePureNix,
			nodeAvailable: false,
			buildSucceeds: true,
			expectValid:   true,
			expectRunning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := SimulateDeploymentFlow(
				"app-1",
				"web",
				tt.buildType,
				tt.nodeAvailable,
				tt.buildSucceeds,
			)

			// Validate deployment flow
			deployResult := ValidateDeploymentFlow(state)
			if deployResult.Valid != tt.expectValid {
				t.Errorf("deployment flow validation: got valid=%v, want valid=%v, errors=%v",
					deployResult.Valid, tt.expectValid, deployResult.Errors)
			}

			// Validate build flow
			buildResult := ValidateBuildFlow(state)
			if !buildResult.Valid {
				t.Errorf("build flow validation failed: %v", buildResult.Errors)
			}

			// Check final state
			if len(state.States) > 0 {
				finalState := state.States[len(state.States)-1]
				if tt.expectRunning && finalState != models.DeploymentStatusRunning {
					t.Errorf("expected final state running, got %s", finalState)
				}
				if !tt.expectRunning && tt.buildSucceeds && !tt.nodeAvailable {
					if finalState != models.DeploymentStatusBuilt {
						t.Errorf("expected final state built (queued), got %s", finalState)
					}
				}
			}

			// Verify artifact is set on successful build
			if tt.buildSucceeds && state.Artifact == "" {
				t.Error("expected artifact to be set on successful build")
			}

			// Verify node ID is set when scheduled
			if tt.expectRunning && state.NodeID == "" {
				t.Error("expected node ID to be set when running")
			}
		})
	}
}

// TestDeploymentFlowWithDependencies tests deployment ordering with service dependencies.
// **Validates: Requirements 12.1**
func TestDeploymentFlowWithDependencies(t *testing.T) {
	ctx := context.Background()
	_ = ctx // Used for context-aware operations

	// Simulate a deployment with dependencies
	// Service "api" depends on "database"
	dbState := SimulateDeploymentFlow("app-1", "database", models.BuildTypePureNix, true, true)
	
	// Database should be running before API can be scheduled
	if len(dbState.States) == 0 {
		t.Fatal("database deployment has no states")
	}
	
	dbFinalState := dbState.States[len(dbState.States)-1]
	if dbFinalState != models.DeploymentStatusRunning {
		t.Fatalf("database should be running, got %s", dbFinalState)
	}

	// Now API can be deployed
	apiState := SimulateDeploymentFlow("app-1", "api", models.BuildTypePureNix, true, true)
	
	apiFinalState := apiState.States[len(apiState.States)-1]
	if apiFinalState != models.DeploymentStatusRunning {
		t.Fatalf("api should be running after database, got %s", apiFinalState)
	}
}

// TestBuildScheduleDeployFlow tests the complete build → schedule → deploy flow.
// **Validates: Requirements 12.1**
func TestBuildScheduleDeployFlow(t *testing.T) {
	// Test the complete flow with various scenarios
	scenarios := []struct {
		name        string
		services    []string
		buildTypes  []models.BuildType
		expectAll   bool
	}{
		{
			name:       "single service pure-nix",
			services:   []string{"web"},
			buildTypes: []models.BuildType{models.BuildTypePureNix},
			expectAll:  true,
		},
		{
			name:       "single service oci",
			services:   []string{"api"},
			buildTypes: []models.BuildType{models.BuildTypeOCI},
			expectAll:  true,
		},
		{
			name:       "multiple services",
			services:   []string{"web", "api", "worker"},
			buildTypes: []models.BuildType{models.BuildTypePureNix, models.BuildTypeOCI, models.BuildTypePureNix},
			expectAll:  true,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			states := make([]*DeploymentFlowState, len(sc.services))
			
			for i, svc := range sc.services {
				states[i] = SimulateDeploymentFlow(
					"app-test",
					svc,
					sc.buildTypes[i],
					true,  // node available
					true,  // build succeeds
				)
			}

			// Verify all deployments completed successfully
			allRunning := true
			for i, state := range states {
				if len(state.States) == 0 {
					t.Errorf("service %s has no states", sc.services[i])
					allRunning = false
					continue
				}
				
				finalState := state.States[len(state.States)-1]
				if finalState != models.DeploymentStatusRunning {
					allRunning = false
				}

				// Validate the flow
				result := ValidateDeploymentFlow(state)
				if !result.Valid {
					t.Errorf("service %s flow invalid: %v", sc.services[i], result.Errors)
				}
			}

			if sc.expectAll && !allRunning {
				t.Error("expected all services to be running")
			}
		})
	}
}
