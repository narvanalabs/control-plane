// Package e2e provides end-to-end testing framework for the control-plane.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

// TestAppDeploymentE2E tests the complete app deployment workflow.
// Simulates: create app → add service → deploy → verify
// **Validates: Requirements 12.2**
func TestAppDeploymentE2E(t *testing.T) {
	ctx := context.Background()

	// Create test environment
	env := NewTestEnvironment(DefaultTestConfig())
	user := NewUserSimulator(env)
	workflow := NewWorkflowExecutor(env)

	// Step 1: Register a node
	node, err := user.RegisterNode(ctx, "node-1", "192.168.1.100", &models.NodeResources{
		CPUTotal:        4,
		CPUAvailable:    4,
		MemoryTotal:     8 << 30, // 8GB
		MemoryAvailable: 8 << 30,
		DiskTotal:       100 << 30, // 100GB
		DiskAvailable:   100 << 30,
	})
	if err != nil {
		t.Fatalf("failed to register node: %v", err)
	}
	t.Logf("Registered node: %s", node.ID)

	// Step 2: Create an application
	app, err := user.CreateApp(ctx, "my-web-app", "A sample web application")
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}
	t.Logf("Created app: %s (%s)", app.Name, app.ID)

	// Step 3: Add a service to the application
	service := models.ServiceConfig{
		Name:       "web",
		SourceType: models.SourceTypeGit,
		GitRepo:    "github.com/example/web-app",
		GitRef:     "main",
		Replicas:   1,
		Resources: &models.ResourceSpec{
			CPU:    "0.5",
			Memory: "512Mi",
		},
		Ports: []models.PortMapping{
			{ContainerPort: 8080, Protocol: "tcp"},
		},
	}
	if err := user.AddService(ctx, app.ID, service); err != nil {
		t.Fatalf("failed to add service: %v", err)
	}
	t.Logf("Added service: %s", service.Name)

	// Step 4: Set a secret for the application
	if err := user.SetSecret(ctx, app.ID, "DATABASE_URL", "postgres://localhost/mydb"); err != nil {
		t.Fatalf("failed to set secret: %v", err)
	}
	t.Log("Set secret: DATABASE_URL")

	// Step 5: Trigger a deployment
	deployment, err := user.Deploy(ctx, app.ID, "web", "main")
	if err != nil {
		t.Fatalf("failed to create deployment: %v", err)
	}
	t.Logf("Created deployment: %s (version %d)", deployment.ID, deployment.Version)

	// Step 6: Process the deployment (simulates backend workflow)
	if err := workflow.ProcessDeployment(ctx, deployment.ID); err != nil {
		t.Fatalf("failed to process deployment: %v", err)
	}

	// Step 7: Verify the deployment is running
	deployment, err = user.GetDeployment(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	if deployment.Status != models.DeploymentStatusRunning {
		t.Errorf("expected deployment status running, got %s", deployment.Status)
	}

	if deployment.NodeID == "" {
		t.Error("expected deployment to be assigned to a node")
	}

	if deployment.Artifact == "" {
		t.Error("expected deployment to have an artifact")
	}

	t.Logf("Deployment running on node %s with artifact %s", deployment.NodeID, deployment.Artifact)

	// Step 8: Verify events were recorded correctly
	events := env.GetEvents()
	expectedEventTypes := []EventType{
		EventNodeRegistered,
		EventAppCreated,
		EventServiceAdded,
		EventSecretSet,
		EventDeploymentCreated,
		EventBuildStarted,
		EventBuildCompleted,
		EventDeploymentStarted,
		EventDeploymentRunning,
	}

	if err := VerifyEventSequence(events, expectedEventTypes); err != nil {
		t.Errorf("event sequence verification failed: %v", err)
	}

	t.Logf("Total events recorded: %d", len(events))
}

// TestMultiServiceAppDeploymentE2E tests deploying an app with multiple services.
// **Validates: Requirements 12.2**
func TestMultiServiceAppDeploymentE2E(t *testing.T) {
	ctx := context.Background()

	env := NewTestEnvironment(DefaultTestConfig())
	user := NewUserSimulator(env)
	workflow := NewWorkflowExecutor(env)

	// Register nodes
	_, err := user.RegisterNode(ctx, "node-1", "192.168.1.100", &models.NodeResources{
		CPUTotal:        8,
		CPUAvailable:    8,
		MemoryTotal:     16 << 30,
		MemoryAvailable: 16 << 30,
		DiskTotal:       200 << 30,
		DiskAvailable:   200 << 30,
	})
	if err != nil {
		t.Fatalf("failed to register node: %v", err)
	}

	// Create app
	app, err := user.CreateApp(ctx, "microservices-app", "A microservices application")
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	// Add multiple services
	services := []models.ServiceConfig{
		{
			Name:       "api",
			SourceType: models.SourceTypeGit,
			GitRepo:    "github.com/example/api",
			GitRef:     "main",
			Replicas:   1,
			Resources:  &models.ResourceSpec{CPU: "1", Memory: "1Gi"},
			Ports:      []models.PortMapping{{ContainerPort: 8080}},
		},
		{
			Name:       "worker",
			SourceType: models.SourceTypeGit,
			GitRepo:    "github.com/example/worker",
			GitRef:     "main",
			Replicas:   2,
			Resources:  &models.ResourceSpec{CPU: "0.5", Memory: "512Mi"},
		},
		{
			Name:       "frontend",
			SourceType: models.SourceTypeGit,
			GitRepo:    "github.com/example/frontend",
			GitRef:     "main",
			Replicas:   1,
			Resources:  &models.ResourceSpec{CPU: "0.25", Memory: "256Mi"},
			Ports:      []models.PortMapping{{ContainerPort: 3000}},
		},
	}

	for _, svc := range services {
		if err := user.AddService(ctx, app.ID, svc); err != nil {
			t.Fatalf("failed to add service %s: %v", svc.Name, err)
		}
	}

	// Deploy all services
	var deployments []*models.Deployment
	for _, svc := range services {
		dep, err := user.Deploy(ctx, app.ID, svc.Name, "main")
		if err != nil {
			t.Fatalf("failed to deploy service %s: %v", svc.Name, err)
		}
		deployments = append(deployments, dep)
	}

	// Process all deployments
	for _, dep := range deployments {
		if err := workflow.ProcessDeployment(ctx, dep.ID); err != nil {
			t.Fatalf("failed to process deployment %s: %v", dep.ID, err)
		}
	}

	// Verify all deployments are running
	for _, dep := range deployments {
		updated, err := user.GetDeployment(ctx, dep.ID)
		if err != nil {
			t.Fatalf("failed to get deployment %s: %v", dep.ID, err)
		}
		if updated.Status != models.DeploymentStatusRunning {
			t.Errorf("service %s: expected running, got %s", updated.ServiceName, updated.Status)
		}
	}

	t.Logf("All %d services deployed successfully", len(services))
}

// TestServiceDependenciesE2E tests deploying services with dependencies.
// **Validates: Requirements 12.2**
func TestServiceDependenciesE2E(t *testing.T) {
	ctx := context.Background()

	env := NewTestEnvironment(DefaultTestConfig())
	user := NewUserSimulator(env)
	workflow := NewWorkflowExecutor(env)

	// Register node
	_, err := user.RegisterNode(ctx, "node-1", "192.168.1.100", &models.NodeResources{
		CPUTotal:        4,
		CPUAvailable:    4,
		MemoryTotal:     8 << 30,
		MemoryAvailable: 8 << 30,
		DiskTotal:       100 << 30,
		DiskAvailable:   100 << 30,
	})
	if err != nil {
		t.Fatalf("failed to register node: %v", err)
	}

	// Create app
	app, err := user.CreateApp(ctx, "dependent-app", "App with service dependencies")
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	// Add database service (no dependencies)
	dbService := models.ServiceConfig{
		Name:       "database",
		SourceType: models.SourceTypeDatabase,
		Database: &models.DatabaseConfig{
			Type:    "postgres",
			Version: "16",
		},
		Replicas:  1,
		Resources: &models.ResourceSpec{CPU: "1", Memory: "1Gi"},
	}
	if err := user.AddService(ctx, app.ID, dbService); err != nil {
		t.Fatalf("failed to add database service: %v", err)
	}

	// Add API service (depends on database)
	apiService := models.ServiceConfig{
		Name:       "api",
		SourceType: models.SourceTypeGit,
		GitRepo:    "github.com/example/api",
		GitRef:     "main",
		Replicas:   1,
		Resources:  &models.ResourceSpec{CPU: "0.5", Memory: "512Mi"},
		Ports:      []models.PortMapping{{ContainerPort: 8080}},
		DependsOn:  []string{"database"},
	}
	if err := user.AddService(ctx, app.ID, apiService); err != nil {
		t.Fatalf("failed to add api service: %v", err)
	}

	// Deploy database first
	dbDep, err := user.Deploy(ctx, app.ID, "database", "main")
	if err != nil {
		t.Fatalf("failed to deploy database: %v", err)
	}
	if err := workflow.ProcessDeployment(ctx, dbDep.ID); err != nil {
		t.Fatalf("failed to process database deployment: %v", err)
	}

	// Verify database is running
	dbDep, _ = user.GetDeployment(ctx, dbDep.ID)
	if dbDep.Status != models.DeploymentStatusRunning {
		t.Fatalf("database should be running, got %s", dbDep.Status)
	}

	// Now deploy API (should succeed because database is running)
	apiDep, err := user.Deploy(ctx, app.ID, "api", "main")
	if err != nil {
		t.Fatalf("failed to deploy api: %v", err)
	}
	if err := workflow.ProcessDeployment(ctx, apiDep.ID); err != nil {
		t.Fatalf("failed to process api deployment: %v", err)
	}

	// Verify API is running
	apiDep, _ = user.GetDeployment(ctx, apiDep.ID)
	if apiDep.Status != models.DeploymentStatusRunning {
		t.Errorf("api should be running, got %s", apiDep.Status)
	}

	t.Log("Service dependency ordering verified successfully")
}

// TestDeploymentVersioningE2E tests that deployments are versioned correctly.
// **Validates: Requirements 12.2**
func TestDeploymentVersioningE2E(t *testing.T) {
	ctx := context.Background()

	env := NewTestEnvironment(DefaultTestConfig())
	user := NewUserSimulator(env)
	workflow := NewWorkflowExecutor(env)

	// Register node
	_, err := user.RegisterNode(ctx, "node-1", "192.168.1.100", &models.NodeResources{
		CPUTotal:        4,
		CPUAvailable:    4,
		MemoryTotal:     8 << 30,
		MemoryAvailable: 8 << 30,
		DiskTotal:       100 << 30,
		DiskAvailable:   100 << 30,
	})
	if err != nil {
		t.Fatalf("failed to register node: %v", err)
	}

	// Create app and service
	app, _ := user.CreateApp(ctx, "versioned-app", "App for version testing")
	service := models.ServiceConfig{
		Name:       "web",
		SourceType: models.SourceTypeGit,
		GitRepo:    "github.com/example/web",
		GitRef:     "main",
		Replicas:   1,
		Resources:  &models.ResourceSpec{CPU: "0.5", Memory: "512Mi"},
	}
	user.AddService(ctx, app.ID, service)

	// Deploy multiple versions
	var versions []int
	for i := 0; i < 3; i++ {
		dep, err := user.Deploy(ctx, app.ID, "web", "main")
		if err != nil {
			t.Fatalf("failed to deploy version %d: %v", i+1, err)
		}
		versions = append(versions, dep.Version)

		if err := workflow.ProcessDeployment(ctx, dep.ID); err != nil {
			t.Fatalf("failed to process deployment: %v", err)
		}
	}

	// Verify versions are sequential
	for i, v := range versions {
		expected := i + 1
		if v != expected {
			t.Errorf("expected version %d, got %d", expected, v)
		}
	}

	// Verify all deployments exist
	deployments, _ := user.ListDeployments(ctx, app.ID)
	if len(deployments) != 3 {
		t.Errorf("expected 3 deployments, got %d", len(deployments))
	}

	t.Logf("Deployment versioning verified: %v", versions)
}

// TestNoNodesAvailableE2E tests behavior when no nodes are available.
// **Validates: Requirements 12.2**
func TestNoNodesAvailableE2E(t *testing.T) {
	ctx := context.Background()

	env := NewTestEnvironment(DefaultTestConfig())
	user := NewUserSimulator(env)
	workflow := NewWorkflowExecutor(env)

	// Create app without registering any nodes
	app, _ := user.CreateApp(ctx, "no-nodes-app", "App without nodes")
	service := models.ServiceConfig{
		Name:       "web",
		SourceType: models.SourceTypeGit,
		GitRepo:    "github.com/example/web",
		GitRef:     "main",
		Replicas:   1,
		Resources:  &models.ResourceSpec{CPU: "0.5", Memory: "512Mi"},
	}
	user.AddService(ctx, app.ID, service)

	// Deploy
	dep, _ := user.Deploy(ctx, app.ID, "web", "main")
	workflow.ProcessDeployment(ctx, dep.ID)

	// Verify deployment is in "built" state (queued for scheduling)
	dep, _ = user.GetDeployment(ctx, dep.ID)
	if dep.Status != models.DeploymentStatusBuilt {
		t.Errorf("expected deployment status built (queued), got %s", dep.Status)
	}

	if dep.NodeID != "" {
		t.Error("expected no node assignment when no nodes available")
	}

	t.Log("No nodes available scenario verified")
}

// TestFullWorkflowResultE2E tests the ExecuteFullWorkflow helper.
// **Validates: Requirements 12.2**
func TestFullWorkflowResultE2E(t *testing.T) {
	ctx := context.Background()

	env := NewTestEnvironment(DefaultTestConfig())
	user := NewUserSimulator(env)
	workflow := NewWorkflowExecutor(env)

	// Setup
	user.RegisterNode(ctx, "node-1", "192.168.1.100", &models.NodeResources{
		CPUTotal:        4,
		CPUAvailable:    4,
		MemoryTotal:     8 << 30,
		MemoryAvailable: 8 << 30,
		DiskTotal:       100 << 30,
		DiskAvailable:   100 << 30,
	})

	app, _ := user.CreateApp(ctx, "workflow-test-app", "Test app")
	user.AddService(ctx, app.ID, models.ServiceConfig{
		Name:       "web",
		SourceType: models.SourceTypeGit,
		GitRepo:    "github.com/example/web",
		GitRef:     "main",
		Replicas:   1,
		Resources:  &models.ResourceSpec{CPU: "0.5", Memory: "512Mi"},
	})
	user.AddService(ctx, app.ID, models.ServiceConfig{
		Name:       "api",
		SourceType: models.SourceTypeGit,
		GitRepo:    "github.com/example/api",
		GitRef:     "main",
		Replicas:   1,
		Resources:  &models.ResourceSpec{CPU: "0.5", Memory: "512Mi"},
	})

	// Deploy both services
	user.Deploy(ctx, app.ID, "web", "main")
	user.Deploy(ctx, app.ID, "api", "main")

	// Execute full workflow
	result := workflow.ExecuteFullWorkflow(ctx, app.ID)

	// Verify result
	if !result.Success {
		t.Errorf("workflow failed: %s", result.ErrorMessage)
	}

	if len(result.Deployments) != 2 {
		t.Errorf("expected 2 deployments, got %d", len(result.Deployments))
	}

	if len(result.Builds) != 2 {
		t.Errorf("expected 2 builds, got %d", len(result.Builds))
	}

	if err := VerifyAllDeploymentsRunning(result.Deployments); err != nil {
		t.Errorf("not all deployments running: %v", err)
	}

	t.Logf("Full workflow completed in %v", result.Duration)
}

// TestEventTrackingE2E tests that all events are properly tracked.
// **Validates: Requirements 12.2**
func TestEventTrackingE2E(t *testing.T) {
	ctx := context.Background()

	env := NewTestEnvironment(DefaultTestConfig())
	user := NewUserSimulator(env)
	workflow := NewWorkflowExecutor(env)

	// Perform various operations
	user.RegisterNode(ctx, "node-1", "192.168.1.100", &models.NodeResources{
		CPUTotal: 4, CPUAvailable: 4,
		MemoryTotal: 8 << 30, MemoryAvailable: 8 << 30,
		DiskTotal: 100 << 30, DiskAvailable: 100 << 30,
	})

	app, _ := user.CreateApp(ctx, "event-test-app", "Test app")
	user.AddService(ctx, app.ID, models.ServiceConfig{
		Name: "web", SourceType: models.SourceTypeGit,
		GitRepo: "github.com/example/web", GitRef: "main",
		Replicas: 1, Resources: &models.ResourceSpec{CPU: "0.5", Memory: "512Mi"},
	})
	user.SetSecret(ctx, app.ID, "API_KEY", "secret123")
	dep, _ := user.Deploy(ctx, app.ID, "web", "main")
	workflow.ProcessDeployment(ctx, dep.ID)

	// Verify event counts by type
	eventCounts := make(map[EventType]int)
	for _, e := range env.GetEvents() {
		eventCounts[e.Type]++
	}

	expectedCounts := map[EventType]int{
		EventNodeRegistered:    1,
		EventAppCreated:        1,
		EventServiceAdded:      1,
		EventSecretSet:         1,
		EventDeploymentCreated: 1,
		EventBuildStarted:      1,
		EventBuildCompleted:    1,
		EventDeploymentStarted: 1,
		EventDeploymentRunning: 1,
	}

	for eventType, expected := range expectedCounts {
		if actual := eventCounts[eventType]; actual != expected {
			t.Errorf("event %s: expected %d, got %d", eventType, expected, actual)
		}
	}

	// Verify events have timestamps
	for _, e := range env.GetEvents() {
		if e.Timestamp.IsZero() {
			t.Errorf("event %s has zero timestamp", e.Type)
		}
	}

	// Verify events are in chronological order
	events := env.GetEvents()
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp.Before(events[i-1].Timestamp) {
			t.Errorf("events not in chronological order: %s before %s",
				events[i].Type, events[i-1].Type)
		}
	}

	t.Logf("Event tracking verified: %d total events", len(events))
}

// TestDeploymentWithDelaysE2E tests deployment with simulated delays.
// **Validates: Requirements 12.2**
func TestDeploymentWithDelaysE2E(t *testing.T) {
	ctx := context.Background()

	// Configure delays
	config := &TestConfig{
		SimulateBuildDelay:  10 * time.Millisecond,
		SimulateDeployDelay: 10 * time.Millisecond,
	}

	env := NewTestEnvironment(config)
	user := NewUserSimulator(env)
	workflow := NewWorkflowExecutor(env)

	// Setup
	user.RegisterNode(ctx, "node-1", "192.168.1.100", &models.NodeResources{
		CPUTotal: 4, CPUAvailable: 4,
		MemoryTotal: 8 << 30, MemoryAvailable: 8 << 30,
		DiskTotal: 100 << 30, DiskAvailable: 100 << 30,
	})

	app, _ := user.CreateApp(ctx, "delayed-app", "Test app with delays")
	user.AddService(ctx, app.ID, models.ServiceConfig{
		Name: "web", SourceType: models.SourceTypeGit,
		GitRepo: "github.com/example/web", GitRef: "main",
		Replicas: 1, Resources: &models.ResourceSpec{CPU: "0.5", Memory: "512Mi"},
	})

	// Deploy and measure time
	dep, _ := user.Deploy(ctx, app.ID, "web", "main")
	start := time.Now()
	workflow.ProcessDeployment(ctx, dep.ID)
	duration := time.Since(start)

	// Verify deployment succeeded
	dep, _ = user.GetDeployment(ctx, dep.ID)
	if dep.Status != models.DeploymentStatusRunning {
		t.Errorf("expected running, got %s", dep.Status)
	}

	// Verify delays were applied (should take at least 20ms for build + deploy)
	if duration < 20*time.Millisecond {
		t.Errorf("expected duration >= 20ms, got %v", duration)
	}

	t.Logf("Deployment with delays completed in %v", duration)
}
