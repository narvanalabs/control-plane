// Package e2e provides end-to-end testing framework for the control-plane.
package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

// WorkflowExecutor simulates the backend processing of deployments.
// **Validates: Requirements 12.2**
type WorkflowExecutor struct {
	env *TestEnvironment
}

// NewWorkflowExecutor creates a new workflow executor.
func NewWorkflowExecutor(env *TestEnvironment) *WorkflowExecutor {
	return &WorkflowExecutor{env: env}
}

// ProcessDeployment simulates the complete deployment workflow.
// This includes: build → schedule → deploy
// **Validates: Requirements 12.2**
func (w *WorkflowExecutor) ProcessDeployment(ctx context.Context, deploymentID string) error {
	w.env.mu.Lock()
	deployment, ok := w.env.Deployments[deploymentID]
	if !ok {
		w.env.mu.Unlock()
		return fmt.Errorf("deployment %s not found", deploymentID)
	}
	w.env.mu.Unlock()

	// Step 1: Create build job
	buildJob := w.createBuildJob(deployment)

	// Step 2: Execute build
	if err := w.executeBuild(ctx, buildJob, deployment); err != nil {
		return err
	}

	// Step 3: Schedule deployment
	if err := w.scheduleDeployment(ctx, deployment); err != nil {
		return err
	}

	// Step 4: Start deployment
	if err := w.startDeployment(ctx, deployment); err != nil {
		return err
	}

	return nil
}

// createBuildJob creates a build job for a deployment.
func (w *WorkflowExecutor) createBuildJob(deployment *models.Deployment) *models.BuildJob {
	w.env.mu.Lock()
	defer w.env.mu.Unlock()

	buildJob := &models.BuildJob{
		ID:           fmt.Sprintf("build-%d", time.Now().UnixNano()),
		DeploymentID: deployment.ID,
		BuildType:    deployment.BuildType,
		Status:       models.BuildStatusQueued,
		CreatedAt:    time.Now(),
	}

	w.env.Builds[buildJob.ID] = buildJob
	return buildJob
}

// executeBuild simulates the build process.
func (w *WorkflowExecutor) executeBuild(ctx context.Context, buildJob *models.BuildJob, deployment *models.Deployment) error {
	w.env.mu.Lock()

	// Update deployment status to building
	deployment.Status = models.DeploymentStatusBuilding
	deployment.UpdatedAt = time.Now()

	// Update build status to running
	buildJob.Status = models.BuildStatusRunning
	now := time.Now()
	buildJob.StartedAt = &now

	w.env.recordEvent(EventBuildStarted, buildJob.ID, map[string]interface{}{
		"deployment_id": deployment.ID,
		"build_type":    buildJob.BuildType,
	})

	w.env.mu.Unlock()

	// Simulate build delay
	if w.env.Config.SimulateBuildDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(w.env.Config.SimulateBuildDelay):
		}
	}

	w.env.mu.Lock()
	defer w.env.mu.Unlock()

	// Check for simulated failure
	// For deterministic testing, we don't use random failures here
	// Instead, tests can set specific conditions

	// Build succeeded
	buildJob.Status = models.BuildStatusSucceeded
	finishedAt := time.Now()
	buildJob.FinishedAt = &finishedAt

	// Set artifact
	artifact := fmt.Sprintf("/nix/store/abc123-%s-v%d", deployment.ServiceName, deployment.Version)
	deployment.Artifact = artifact
	deployment.Status = models.DeploymentStatusBuilt
	deployment.UpdatedAt = finishedAt

	w.env.recordEvent(EventBuildCompleted, buildJob.ID, map[string]interface{}{
		"deployment_id": deployment.ID,
		"artifact":      artifact,
	})

	return nil
}

// scheduleDeployment simulates the scheduling process.
func (w *WorkflowExecutor) scheduleDeployment(ctx context.Context, deployment *models.Deployment) error {
	w.env.mu.Lock()
	defer w.env.mu.Unlock()

	// Check dependencies
	if len(deployment.DependsOn) > 0 {
		for _, depName := range deployment.DependsOn {
			found := false
			for _, dep := range w.env.Deployments {
				if dep.AppID == deployment.AppID && 
				   dep.ServiceName == depName && 
				   dep.Status == models.DeploymentStatusRunning {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("dependency %s is not running", depName)
			}
		}
	}

	// Find a healthy node
	var selectedNode *models.Node
	for _, node := range w.env.Nodes {
		if node.Healthy {
			selectedNode = node
			break
		}
	}

	if selectedNode == nil {
		// No nodes available - deployment stays in "built" status (queued)
		return nil
	}

	// Assign to node
	deployment.NodeID = selectedNode.ID
	deployment.Status = models.DeploymentStatusScheduled
	deployment.UpdatedAt = time.Now()

	return nil
}

// startDeployment simulates starting the deployment on a node.
func (w *WorkflowExecutor) startDeployment(ctx context.Context, deployment *models.Deployment) error {
	w.env.mu.Lock()

	if deployment.NodeID == "" {
		w.env.mu.Unlock()
		return nil // Not scheduled yet
	}

	deployment.Status = models.DeploymentStatusStarting
	deployment.UpdatedAt = time.Now()

	w.env.recordEvent(EventDeploymentStarted, deployment.ID, map[string]interface{}{
		"node_id": deployment.NodeID,
	})

	w.env.mu.Unlock()

	// Simulate deploy delay
	if w.env.Config.SimulateDeployDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(w.env.Config.SimulateDeployDelay):
		}
	}

	w.env.mu.Lock()
	defer w.env.mu.Unlock()

	// Deployment running
	deployment.Status = models.DeploymentStatusRunning
	now := time.Now()
	deployment.StartedAt = &now
	deployment.UpdatedAt = now

	w.env.recordEvent(EventDeploymentRunning, deployment.ID, map[string]interface{}{
		"node_id":  deployment.NodeID,
		"artifact": deployment.Artifact,
	})

	return nil
}

// ProcessAllPendingDeployments processes all pending deployments.
func (w *WorkflowExecutor) ProcessAllPendingDeployments(ctx context.Context) error {
	w.env.mu.RLock()
	var pendingIDs []string
	for id, dep := range w.env.Deployments {
		if dep.Status == models.DeploymentStatusPending {
			pendingIDs = append(pendingIDs, id)
		}
	}
	w.env.mu.RUnlock()

	for _, id := range pendingIDs {
		if err := w.ProcessDeployment(ctx, id); err != nil {
			return err
		}
	}

	return nil
}

// WorkflowResult contains the result of a workflow execution.
type WorkflowResult struct {
	Success      bool
	Deployments  []*models.Deployment
	Builds       []*models.BuildJob
	Events       []Event
	Duration     time.Duration
	ErrorMessage string
}

// ExecuteFullWorkflow executes a complete workflow and returns the result.
// **Validates: Requirements 12.2**
func (w *WorkflowExecutor) ExecuteFullWorkflow(ctx context.Context, appID string) *WorkflowResult {
	startTime := time.Now()
	result := &WorkflowResult{
		Success:     true,
		Deployments: make([]*models.Deployment, 0),
		Builds:      make([]*models.BuildJob, 0),
	}

	// Get all deployments for the app
	w.env.mu.RLock()
	var deploymentIDs []string
	for id, dep := range w.env.Deployments {
		if dep.AppID == appID && dep.Status == models.DeploymentStatusPending {
			deploymentIDs = append(deploymentIDs, id)
		}
	}
	w.env.mu.RUnlock()

	// Process each deployment
	for _, id := range deploymentIDs {
		if err := w.ProcessDeployment(ctx, id); err != nil {
			result.Success = false
			result.ErrorMessage = err.Error()
			break
		}
	}

	// Collect results
	w.env.mu.RLock()
	for _, dep := range w.env.Deployments {
		if dep.AppID == appID {
			result.Deployments = append(result.Deployments, dep)
		}
	}
	for _, build := range w.env.Builds {
		for _, dep := range result.Deployments {
			if build.DeploymentID == dep.ID {
				result.Builds = append(result.Builds, build)
			}
		}
	}
	result.Events = make([]Event, len(w.env.Events))
	copy(result.Events, w.env.Events)
	w.env.mu.RUnlock()

	result.Duration = time.Since(startTime)
	return result
}

// VerifyDeploymentState verifies that a deployment is in the expected state.
// **Validates: Requirements 12.2**
func VerifyDeploymentState(deployment *models.Deployment, expectedStatus models.DeploymentStatus) error {
	if deployment.Status != expectedStatus {
		return fmt.Errorf("expected deployment status %s, got %s", expectedStatus, deployment.Status)
	}
	return nil
}

// VerifyAllDeploymentsRunning verifies that all deployments for an app are running.
// **Validates: Requirements 12.2**
func VerifyAllDeploymentsRunning(deployments []*models.Deployment) error {
	for _, dep := range deployments {
		if dep.Status != models.DeploymentStatusRunning {
			return fmt.Errorf("deployment %s is not running (status: %s)", dep.ID, dep.Status)
		}
	}
	return nil
}

// VerifyEventSequence verifies that events occurred in the expected order.
// **Validates: Requirements 12.2**
func VerifyEventSequence(events []Event, expectedTypes []EventType) error {
	if len(events) < len(expectedTypes) {
		return fmt.Errorf("expected at least %d events, got %d", len(expectedTypes), len(events))
	}

	typeIndex := 0
	for _, event := range events {
		if typeIndex >= len(expectedTypes) {
			break
		}
		if event.Type == expectedTypes[typeIndex] {
			typeIndex++
		}
	}

	if typeIndex < len(expectedTypes) {
		return fmt.Errorf("missing expected event type: %s", expectedTypes[typeIndex])
	}

	return nil
}
