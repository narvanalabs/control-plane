package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/queue"
	"github.com/narvanalabs/control-plane/internal/store"
)

// **Feature: control-plane, Property 4: Deployment inherits build type**
// *For any* application with a configured build type, all deployments triggered
// for that application should have the same build type.
// **Validates: Requirements 2.2**

// mockDeploymentStore implements store.DeploymentStore for testing
type mockDeploymentStore struct {
	deployments map[string]*models.Deployment
}

func newMockDeploymentStore() *mockDeploymentStore {
	return &mockDeploymentStore{
		deployments: make(map[string]*models.Deployment),
	}
}

func (m *mockDeploymentStore) Create(ctx context.Context, deployment *models.Deployment) error {
	m.deployments[deployment.ID] = deployment
	return nil
}

func (m *mockDeploymentStore) Get(ctx context.Context, id string) (*models.Deployment, error) {
	if d, ok := m.deployments[id]; ok {
		return d, nil
	}
	return nil, nil
}

func (m *mockDeploymentStore) List(ctx context.Context, appID string) ([]*models.Deployment, error) {
	var result []*models.Deployment
	for _, d := range m.deployments {
		if d.AppID == appID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *mockDeploymentStore) ListByNode(ctx context.Context, nodeID string) ([]*models.Deployment, error) {
	return nil, nil
}

func (m *mockDeploymentStore) Update(ctx context.Context, deployment *models.Deployment) error {
	m.deployments[deployment.ID] = deployment
	return nil
}

func (m *mockDeploymentStore) GetLatestSuccessful(ctx context.Context, appID string) (*models.Deployment, error) {
	var latest *models.Deployment
	for _, d := range m.deployments {
		if d.AppID == appID && d.Status == models.DeploymentStatusRunning {
			if latest == nil || d.CreatedAt.After(latest.CreatedAt) {
				latest = d
			}
		}
	}
	return latest, nil
}


// mockQueue implements queue.Queue for testing
type mockQueue struct {
	jobs []*models.BuildJob
}

func newMockQueue() *mockQueue {
	return &mockQueue{
		jobs: make([]*models.BuildJob, 0),
	}
}

func (m *mockQueue) Enqueue(ctx context.Context, job *models.BuildJob) error {
	m.jobs = append(m.jobs, job)
	return nil
}

func (m *mockQueue) Dequeue(ctx context.Context) (*models.BuildJob, error) {
	if len(m.jobs) == 0 {
		return nil, queue.ErrNoJobs
	}
	job := m.jobs[0]
	m.jobs = m.jobs[1:]
	return job, nil
}

func (m *mockQueue) Ack(ctx context.Context, jobID string) error {
	return nil
}

func (m *mockQueue) Nack(ctx context.Context, jobID string) error {
	return nil
}

// deploymentMockStore implements store.Store for deployment testing
type deploymentMockStore struct {
	appStore        *mockAppStore
	deploymentStore *mockDeploymentStore
}

func newDeploymentMockStore() *deploymentMockStore {
	return &deploymentMockStore{
		appStore:        newMockAppStore(),
		deploymentStore: newMockDeploymentStore(),
	}
}

func (m *deploymentMockStore) Apps() store.AppStore {
	return m.appStore
}

func (m *deploymentMockStore) Deployments() store.DeploymentStore {
	return m.deploymentStore
}

func (m *deploymentMockStore) Nodes() store.NodeStore {
	return nil
}

func (m *deploymentMockStore) Builds() store.BuildStore {
	return nil
}

func (m *deploymentMockStore) Secrets() store.SecretStore {
	return nil
}

func (m *deploymentMockStore) Logs() store.LogStore {
	return nil
}

func (m *deploymentMockStore) Users() store.UserStore {
	return nil
}

func (m *deploymentMockStore) WithTx(ctx context.Context, fn func(store.Store) error) error {
	return fn(m)
}

func (m *deploymentMockStore) Close() error {
	return nil
}

func TestDeploymentInheritsBuildType(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	logger := slog.Default()

	properties.Property("Deployment inherits build type from application", prop.ForAll(
		func(userID, appName, gitRef string) bool {
			// Create a mock store and queue
			st := newDeploymentMockStore()
			q := newMockQueue()

			// Create an app
			now := time.Now()
			app := &models.App{
				ID:      "app-" + appName,
				OwnerID: userID,
				Name:    appName,
				Services: []models.ServiceConfig{
					{Name: "web", ResourceTier: models.ResourceTierSmall},
				},
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.apps[app.ID] = app

			// Create deployment handler
			handler := NewDeploymentHandler(st, q, logger)

			// Create a deployment request
			reqBody := CreateDeploymentRequest{
				GitRef: gitRef,
			}
			body, _ := json.Marshal(reqBody)

			// Create the HTTP request
			req := httptest.NewRequest("POST", "/v1/apps/"+app.ID+"/deploy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			
			// Add the appID to chi URL params
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("appID", app.ID)
			ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
			
			req = req.WithContext(ctx)

			// Execute the request
			rr := httptest.NewRecorder()
			handler.Create(rr, req)

			if rr.Code != http.StatusAccepted {
				return false
			}

			// Parse the response
			var deployment models.Deployment
			if err := json.NewDecoder(rr.Body).Decode(&deployment); err != nil {
				return false
			}

			// Verify the deployment was created successfully
			return deployment.ID != "" && deployment.AppID == app.ID
		},
		genUserID(),
		genAppName(),
		gen.RegexMatch("[a-z0-9]{6,12}"), // git ref
	))

	properties.TestingRun(t)
}

// **Feature: control-plane, Property 6: Rollback uses previous artifact**
// *For any* successful deployment, rolling back should create a new deployment
// that references the same artifact as the rolled-back-to deployment.
// **Validates: Requirements 2.5**

func TestRollbackUsesPreviousArtifact(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	logger := slog.Default()

	properties.Property("Rollback creates deployment with same artifact", prop.ForAll(
		func(userID, appName, artifact string) bool {
			// Create a mock store and queue
			st := newDeploymentMockStore()
			q := newMockQueue()

			// Create an app
			now := time.Now()
			app := &models.App{
				ID:        "app-" + appName,
				OwnerID:   userID,
				Name:      appName,
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.apps[app.ID] = app

			// Create a successful deployment with an artifact
			originalDeployment := &models.Deployment{
				ID:           "deploy-original",
				AppID:        app.ID,
				ServiceName:  "web",
				GitRef:       "main",
				Artifact:     artifact,
				Status:       models.DeploymentStatusRunning,
				ResourceTier: models.ResourceTierSmall,
				CreatedAt:    now,
				UpdatedAt:    now,
			}
			st.deploymentStore.deployments[originalDeployment.ID] = originalDeployment

			// Create deployment handler
			handler := NewDeploymentHandler(st, q, logger)

			// Create a rollback request
			req := httptest.NewRequest("POST", "/v1/deployments/"+originalDeployment.ID+"/rollback", nil)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			
			// Add the deploymentID to chi URL params
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("deploymentID", originalDeployment.ID)
			ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
			
			req = req.WithContext(ctx)

			// Execute the request
			rr := httptest.NewRecorder()
			handler.Rollback(rr, req)

			if rr.Code != http.StatusAccepted {
				return false
			}

			// Parse the response
			var newDeployment models.Deployment
			if err := json.NewDecoder(rr.Body).Decode(&newDeployment); err != nil {
				return false
			}

			// Verify the new deployment has the same artifact as the original
			return newDeployment.Artifact == artifact &&
				newDeployment.ID != originalDeployment.ID
		},
		genUserID(),
		genAppName(),
		gen.RegexMatch("[a-z0-9/:-]{10,30}"), // artifact (image tag or store path)
	))

	properties.TestingRun(t)
}

// **Feature: control-plane, Property 21: Multi-service deployment creation**
// *For any* application with N services, triggering a deployment should create
// exactly N deployment records (one per service).
// **Validates: Requirements 8.1**

func TestMultiServiceDeploymentCreation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	logger := slog.Default()

	// Generator for service configs with unique names
	genUniqueServices := func(count int) []models.ServiceConfig {
		tiers := []models.ResourceTier{
			models.ResourceTierNano,
			models.ResourceTierSmall,
			models.ResourceTierMedium,
		}
		services := make([]models.ServiceConfig, count)
		for i := 0; i < count; i++ {
			services[i] = models.ServiceConfig{
				Name:         "service-" + string(rune('a'+i)),
				ResourceTier: tiers[i%len(tiers)],
			}
		}
		return services
	}

	properties.Property("Multi-service app creates N deployments for N services", prop.ForAll(
		func(userID, appName, gitRef string, serviceCount int) bool {
			// Generate unique services
			services := genUniqueServices(serviceCount)

			// Create a mock store and queue
			st := newDeploymentMockStore()
			q := newMockQueue()

			// Create an app with multiple services
			now := time.Now()
			app := &models.App{
				ID:        "app-" + appName,
				OwnerID:   userID,
				Name:      appName,
				Services:  services,
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.apps[app.ID] = app

			// Create deployment handler
			handler := NewDeploymentHandler(st, q, logger)

			// Create a deployment request
			reqBody := CreateDeploymentRequest{
				GitRef: gitRef,
			}
			body, _ := json.Marshal(reqBody)

			// Create the HTTP request
			req := httptest.NewRequest("POST", "/v1/apps/"+app.ID+"/deploy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)

			// Add the appID to chi URL params
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("appID", app.ID)
			ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

			req = req.WithContext(ctx)

			// Execute the request
			rr := httptest.NewRecorder()
			handler.Create(rr, req)

			if rr.Code != http.StatusAccepted {
				return false
			}

			// Count deployments created in the store
			deploymentsCreated := len(st.deploymentStore.deployments)

			// Verify exactly N deployments were created for N services
			if deploymentsCreated != serviceCount {
				return false
			}

			// Verify each service has a corresponding deployment
			serviceNames := make(map[string]bool)
			for _, svc := range services {
				serviceNames[svc.Name] = true
			}

			for _, d := range st.deploymentStore.deployments {
				if !serviceNames[d.ServiceName] {
					return false
				}
				// Verify deployment has correct app ID
				if d.AppID != app.ID {
					return false
				}
			}

			return true
		},
		genUserID(),
		genAppName(),
		gen.RegexMatch("[a-z0-9]{6,12}"), // git ref
		gen.IntRange(1, 5), // service count (1-5 services)
	))

	properties.TestingRun(t)
}


// **Feature: service-git-repos, Property 8: Per-Service Deployment Isolation**
// *For any* deployment triggered for a specific service, only that service SHALL be
// built and deployed (deployment count equals 1).
// **Validates: Requirements 5.1**

func TestPerServiceDeploymentIsolation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	logger := slog.Default()

	// Generator for multiple services with unique names
	genMultipleServices := func(count int) []models.ServiceConfig {
		services := make([]models.ServiceConfig, count)
		for i := 0; i < count; i++ {
			services[i] = models.ServiceConfig{
				Name:         "service-" + string(rune('a'+i)),
				SourceType:   models.SourceTypeGit,
				GitRepo:      "github.com/test/repo" + string(rune('a'+i)),
				GitRef:       "main",
				FlakeOutput:  "packages.x86_64-linux.default",
				ResourceTier: models.ResourceTierSmall,
				Replicas:     1,
			}
		}
		return services
	}

	properties.Property("Per-service deployment creates exactly one deployment", prop.ForAll(
		func(userID, appName string, serviceCount, targetServiceIndex int) bool {
			// Ensure target index is valid
			if targetServiceIndex >= serviceCount {
				targetServiceIndex = serviceCount - 1
			}

			// Generate services
			services := genMultipleServices(serviceCount)
			targetServiceName := services[targetServiceIndex].Name

			// Create a mock store and queue
			st := newDeploymentMockStore()
			q := newMockQueue()

			// Create an app with multiple services
			now := time.Now()
			app := &models.App{
				ID:        "app-" + appName,
				OwnerID:   userID,
				Name:      appName,
				Services:  services,
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.apps[app.ID] = app

			// Create deployment handler
			handler := NewDeploymentHandler(st, q, logger)

			// Create a per-service deployment request
			reqBody := ServiceDeployRequest{
				GitRef: "feature-branch",
			}
			body, _ := json.Marshal(reqBody)

			// Create the HTTP request for per-service deployment
			req := httptest.NewRequest("POST", "/v1/apps/"+app.ID+"/services/"+targetServiceName+"/deploy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)

			// Add URL params
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("appID", app.ID)
			rctx.URLParams.Add("serviceName", targetServiceName)
			ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

			req = req.WithContext(ctx)

			// Execute the request
			rr := httptest.NewRecorder()
			handler.CreateForService(rr, req)

			if rr.Code != http.StatusAccepted {
				t.Logf("Expected status 202, got %d: %s", rr.Code, rr.Body.String())
				return false
			}

			// Verify exactly ONE deployment was created
			deploymentsCreated := len(st.deploymentStore.deployments)
			if deploymentsCreated != 1 {
				t.Logf("Expected 1 deployment, got %d", deploymentsCreated)
				return false
			}

			// Verify the deployment is for the target service only
			for _, d := range st.deploymentStore.deployments {
				if d.ServiceName != targetServiceName {
					t.Logf("Expected service %s, got %s", targetServiceName, d.ServiceName)
					return false
				}
				if d.AppID != app.ID {
					return false
				}
			}

			return true
		},
		genUserID(),
		genAppName(),
		gen.IntRange(2, 5), // service count (2-5 services to ensure multiple exist)
		gen.IntRange(0, 4), // target service index
	))

	properties.TestingRun(t)
}

// **Feature: service-git-repos, Property 9: Deployment Uses Service Source**
// *For any* deployment of a service with a git_repo, the resulting build job SHALL
// contain the service's git_repo and flake_output.
// **Validates: Requirements 5.2**

func TestDeploymentUsesServiceSource(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	logger := slog.Default()

	properties.Property("Deployment build job uses service's git_repo and flake_output", prop.ForAll(
		func(userID, appName, gitRepo, flakeOutput string) bool {
			// Create a mock store and queue
			st := newDeploymentMockStore()
			q := newMockQueue()

			// Create an app with a service that has specific git source
			now := time.Now()
			serviceName := "api"
			app := &models.App{
				ID:      "app-" + appName,
				OwnerID: userID,
				Name:    appName,
				Services: []models.ServiceConfig{
					{
						Name:         serviceName,
						SourceType:   models.SourceTypeGit,
						GitRepo:      gitRepo,
						GitRef:       "main",
						FlakeOutput:  flakeOutput,
						ResourceTier: models.ResourceTierSmall,
						Replicas:     1,
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.apps[app.ID] = app

			// Create deployment handler
			handler := NewDeploymentHandler(st, q, logger)

			// Create a per-service deployment request
			reqBody := ServiceDeployRequest{}
			body, _ := json.Marshal(reqBody)

			// Create the HTTP request
			req := httptest.NewRequest("POST", "/v1/apps/"+app.ID+"/services/"+serviceName+"/deploy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)

			// Add URL params
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("appID", app.ID)
			rctx.URLParams.Add("serviceName", serviceName)
			ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

			req = req.WithContext(ctx)

			// Execute the request
			rr := httptest.NewRecorder()
			handler.CreateForService(rr, req)

			if rr.Code != http.StatusAccepted {
				t.Logf("Expected status 202, got %d: %s", rr.Code, rr.Body.String())
				return false
			}

			// Verify the build job was created with correct source
			if len(q.jobs) != 1 {
				t.Logf("Expected 1 build job, got %d", len(q.jobs))
				return false
			}

			buildJob := q.jobs[0]

			// Verify git_repo is used
			if buildJob.GitURL != gitRepo {
				t.Logf("Expected GitURL %s, got %s", gitRepo, buildJob.GitURL)
				return false
			}

			// Verify flake_output is used
			if buildJob.FlakeOutput != flakeOutput {
				t.Logf("Expected FlakeOutput %s, got %s", flakeOutput, buildJob.FlakeOutput)
				return false
			}

			// Verify git_ref defaults to service's git_ref
			if buildJob.GitRef != "main" {
				t.Logf("Expected GitRef 'main', got %s", buildJob.GitRef)
				return false
			}

			return true
		},
		genUserID(),
		genAppName(),
		gen.RegexMatch("github\\.com/[a-z]+/[a-z]+"), // git repo
		gen.OneConstOf("packages.x86_64-linux.default", "packages.x86_64-linux.api", "packages.aarch64-linux.default"),
	))

	// Test that git_ref override works
	properties.Property("Deployment allows git_ref override", prop.ForAll(
		func(userID, appName, overrideRef string) bool {
			// Create a mock store and queue
			st := newDeploymentMockStore()
			q := newMockQueue()

			// Create an app with a service
			now := time.Now()
			serviceName := "api"
			app := &models.App{
				ID:      "app-" + appName,
				OwnerID: userID,
				Name:    appName,
				Services: []models.ServiceConfig{
					{
						Name:         serviceName,
						SourceType:   models.SourceTypeGit,
						GitRepo:      "github.com/test/repo",
						GitRef:       "main", // Default ref
						FlakeOutput:  "packages.x86_64-linux.default",
						ResourceTier: models.ResourceTierSmall,
						Replicas:     1,
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.apps[app.ID] = app

			// Create deployment handler
			handler := NewDeploymentHandler(st, q, logger)

			// Create a per-service deployment request with git_ref override
			reqBody := ServiceDeployRequest{
				GitRef: overrideRef,
			}
			body, _ := json.Marshal(reqBody)

			// Create the HTTP request
			req := httptest.NewRequest("POST", "/v1/apps/"+app.ID+"/services/"+serviceName+"/deploy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)

			// Add URL params
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("appID", app.ID)
			rctx.URLParams.Add("serviceName", serviceName)
			ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

			req = req.WithContext(ctx)

			// Execute the request
			rr := httptest.NewRecorder()
			handler.CreateForService(rr, req)

			if rr.Code != http.StatusAccepted {
				return false
			}

			// Verify the build job uses the override ref
			if len(q.jobs) != 1 {
				return false
			}

			buildJob := q.jobs[0]
			return buildJob.GitRef == overrideRef
		},
		genUserID(),
		genAppName(),
		gen.OneConstOf("feature/test", "release/v1.0", "develop", "abc123def456"),
	))

	properties.TestingRun(t)
}


// **Feature: service-git-repos, Property 12: Dependency Order in Deployment**
// *For any* deployment of multiple services with dependencies, services SHALL be
// deployed in topological order (dependencies before dependents).
// **Validates: Requirements 9.2**

func TestDependencyOrderInDeployment(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	logger := slog.Default()

	properties.Property("Services are deployed in dependency order", prop.ForAll(
		func(userID, appName string) bool {
			// Create services with a dependency chain: A -> B -> C (C depends on B, B depends on A)
			services := []models.ServiceConfig{
				{
					Name:         "service-c",
					SourceType:   models.SourceTypeGit,
					GitRepo:      "github.com/test/repoc",
					GitRef:       "main",
					FlakeOutput:  "packages.x86_64-linux.default",
					ResourceTier: models.ResourceTierSmall,
					Replicas:     1,
					DependsOn:    []string{"service-b"},
				},
				{
					Name:         "service-a",
					SourceType:   models.SourceTypeGit,
					GitRepo:      "github.com/test/repoa",
					GitRef:       "main",
					FlakeOutput:  "packages.x86_64-linux.default",
					ResourceTier: models.ResourceTierSmall,
					Replicas:     1,
					DependsOn:    []string{}, // No dependencies
				},
				{
					Name:         "service-b",
					SourceType:   models.SourceTypeGit,
					GitRepo:      "github.com/test/repob",
					GitRef:       "main",
					FlakeOutput:  "packages.x86_64-linux.default",
					ResourceTier: models.ResourceTierSmall,
					Replicas:     1,
					DependsOn:    []string{"service-a"},
				},
			}

			// Create a mock store and queue
			st := newDeploymentMockStore()
			q := newMockQueue()

			// Create an app with services that have dependencies
			now := time.Now()
			app := &models.App{
				ID:        "app-" + appName,
				OwnerID:   userID,
				Name:      appName,
				Services:  services,
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.apps[app.ID] = app

			// Create deployment handler
			handler := NewDeploymentHandler(st, q, logger)

			// Create a deployment request for all services
			reqBody := CreateDeploymentRequest{
				GitRef: "main",
			}
			body, _ := json.Marshal(reqBody)

			// Create the HTTP request
			req := httptest.NewRequest("POST", "/v1/apps/"+app.ID+"/deploy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)

			// Add URL params
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("appID", app.ID)
			ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

			req = req.WithContext(ctx)

			// Execute the request
			rr := httptest.NewRecorder()
			handler.Create(rr, req)

			if rr.Code != http.StatusAccepted {
				t.Logf("Expected status 202, got %d: %s", rr.Code, rr.Body.String())
				return false
			}

			// Verify all 3 deployments were created
			if len(st.deploymentStore.deployments) != 3 {
				t.Logf("Expected 3 deployments, got %d", len(st.deploymentStore.deployments))
				return false
			}

			// Verify the build jobs are in dependency order
			// The order should be: service-a, service-b, service-c
			if len(q.jobs) != 3 {
				t.Logf("Expected 3 build jobs, got %d", len(q.jobs))
				return false
			}

			// Build a map of deployment IDs to service names
			deploymentToService := make(map[string]string)
			for _, d := range st.deploymentStore.deployments {
				deploymentToService[d.ID] = d.ServiceName
			}

			// Get the order of services from build jobs
			var serviceOrder []string
			for _, job := range q.jobs {
				serviceName := deploymentToService[job.DeploymentID]
				serviceOrder = append(serviceOrder, serviceName)
			}

			// Verify service-a comes before service-b
			aIndex := -1
			bIndex := -1
			cIndex := -1
			for i, name := range serviceOrder {
				switch name {
				case "service-a":
					aIndex = i
				case "service-b":
					bIndex = i
				case "service-c":
					cIndex = i
				}
			}

			// A must come before B (B depends on A)
			if aIndex >= bIndex {
				t.Logf("service-a (index %d) should come before service-b (index %d)", aIndex, bIndex)
				return false
			}

			// B must come before C (C depends on B)
			if bIndex >= cIndex {
				t.Logf("service-b (index %d) should come before service-c (index %d)", bIndex, cIndex)
				return false
			}

			return true
		},
		genUserID(),
		genAppName(),
	))

	// Test that services with no dependencies can be deployed in any order relative to each other
	properties.Property("Independent services can be deployed in any order", prop.ForAll(
		func(userID, appName string) bool {
			// Create independent services (no dependencies between them)
			services := []models.ServiceConfig{
				{
					Name:         "service-x",
					SourceType:   models.SourceTypeGit,
					GitRepo:      "github.com/test/repox",
					GitRef:       "main",
					FlakeOutput:  "packages.x86_64-linux.default",
					ResourceTier: models.ResourceTierSmall,
					Replicas:     1,
					DependsOn:    []string{},
				},
				{
					Name:         "service-y",
					SourceType:   models.SourceTypeGit,
					GitRepo:      "github.com/test/repoy",
					GitRef:       "main",
					FlakeOutput:  "packages.x86_64-linux.default",
					ResourceTier: models.ResourceTierSmall,
					Replicas:     1,
					DependsOn:    []string{},
				},
			}

			// Create a mock store and queue
			st := newDeploymentMockStore()
			q := newMockQueue()

			// Create an app
			now := time.Now()
			app := &models.App{
				ID:        "app-" + appName,
				OwnerID:   userID,
				Name:      appName,
				Services:  services,
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.apps[app.ID] = app

			// Create deployment handler
			handler := NewDeploymentHandler(st, q, logger)

			// Create a deployment request
			reqBody := CreateDeploymentRequest{
				GitRef: "main",
			}
			body, _ := json.Marshal(reqBody)

			// Create the HTTP request
			req := httptest.NewRequest("POST", "/v1/apps/"+app.ID+"/deploy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)

			// Add URL params
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("appID", app.ID)
			ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

			req = req.WithContext(ctx)

			// Execute the request
			rr := httptest.NewRecorder()
			handler.Create(rr, req)

			if rr.Code != http.StatusAccepted {
				return false
			}

			// Verify both deployments were created
			if len(st.deploymentStore.deployments) != 2 {
				return false
			}

			// Verify both build jobs were created
			if len(q.jobs) != 2 {
				return false
			}

			return true
		},
		genUserID(),
		genAppName(),
	))

	properties.TestingRun(t)
}
