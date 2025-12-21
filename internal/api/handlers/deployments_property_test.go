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
		func(userID, appName, gitRef string, buildType models.BuildType) bool {
			// Create a mock store and queue
			st := newDeploymentMockStore()
			q := newMockQueue()

			// Create an app with the specified build type
			now := time.Now()
			app := &models.App{
				ID:        "app-" + appName,
				OwnerID:   userID,
				Name:      appName,
				BuildType: buildType,
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

			// Verify the deployment inherited the build type from the app
			return deployment.BuildType == buildType
		},
		genUserID(),
		genAppName(),
		gen.RegexMatch("[a-z0-9]{6,12}"), // git ref
		genBuildType(),
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
		func(userID, appName, artifact string, buildType models.BuildType) bool {
			// Create a mock store and queue
			st := newDeploymentMockStore()
			q := newMockQueue()

			// Create an app
			now := time.Now()
			app := &models.App{
				ID:        "app-" + appName,
				OwnerID:   userID,
				Name:      appName,
				BuildType: buildType,
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
				BuildType:    buildType,
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
				newDeployment.ID != originalDeployment.ID &&
				newDeployment.BuildType == buildType
		},
		genUserID(),
		genAppName(),
		gen.RegexMatch("[a-z0-9/:-]{10,30}"), // artifact (image tag or store path)
		genBuildType(),
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
		func(userID, appName, gitRef string, buildType models.BuildType, serviceCount int) bool {
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
				BuildType: buildType,
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
				// Verify deployment inherits build type
				if d.BuildType != buildType {
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
		genBuildType(),
		gen.IntRange(1, 5), // service count (1-5 services)
	))

	properties.TestingRun(t)
}
