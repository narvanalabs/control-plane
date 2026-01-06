package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// **Feature: backend-source-of-truth, Property 1: Dashboard Statistics Accuracy**
// *For any* database state with deployments, apps, and services, the dashboard statistics
// endpoint SHALL return counts that exactly match the database records filtered by the
// current organization.
// **Validates: Requirements 1.1, 1.2, 1.3**

// statsMockStore implements store.Store for stats testing
type statsMockStore struct {
	appStore        *statsAppStore
	deploymentStore *statsDeploymentStore
	nodeStore       *statsNodeStore
}

func newStatsMockStore() *statsMockStore {
	return &statsMockStore{
		appStore:        newStatsAppStore(),
		deploymentStore: newStatsDeploymentStore(),
		nodeStore:       newStatsNodeStore(),
	}
}

func (m *statsMockStore) Apps() store.AppStore                                         { return m.appStore }
func (m *statsMockStore) Deployments() store.DeploymentStore                           { return m.deploymentStore }
func (m *statsMockStore) Nodes() store.NodeStore                                       { return m.nodeStore }
func (m *statsMockStore) Orgs() store.OrgStore                                         { return nil }
func (m *statsMockStore) Builds() store.BuildStore                                     { return nil }
func (m *statsMockStore) Secrets() store.SecretStore                                   { return nil }
func (m *statsMockStore) Logs() store.LogStore                                         { return nil }
func (m *statsMockStore) Users() store.UserStore                                       { return nil }
func (m *statsMockStore) GitHub() store.GitHubStore                                    { return nil }
func (m *statsMockStore) GitHubAccounts() store.GitHubAccountStore                     { return nil }
func (m *statsMockStore) Settings() store.SettingsStore                                { return nil }
func (m *statsMockStore) Domains() store.DomainStore                                   { return nil }
func (m *statsMockStore) Invitations() store.InvitationStore                           { return nil }
func (m *statsMockStore) WithTx(ctx context.Context, fn func(store.Store) error) error { return fn(m) }
func (m *statsMockStore) Close() error                                                 { return nil }

// statsAppStore implements store.AppStore for stats testing
type statsAppStore struct {
	apps map[string]*models.App
}

func newStatsAppStore() *statsAppStore {
	return &statsAppStore{
		apps: make(map[string]*models.App),
	}
}

func (m *statsAppStore) Create(ctx context.Context, app *models.App) error {
	m.apps[app.ID] = app
	return nil
}

func (m *statsAppStore) Get(ctx context.Context, id string) (*models.App, error) {
	if app, ok := m.apps[id]; ok {
		return app, nil
	}
	return nil, nil
}

func (m *statsAppStore) GetByName(ctx context.Context, ownerID, name string) (*models.App, error) {
	return nil, nil
}

func (m *statsAppStore) List(ctx context.Context, ownerID string) ([]*models.App, error) {
	var apps []*models.App
	for _, app := range m.apps {
		if app.OwnerID == ownerID && app.DeletedAt == nil {
			apps = append(apps, app)
		}
	}
	return apps, nil
}

func (m *statsAppStore) ListByOrg(ctx context.Context, orgID string) ([]*models.App, error) {
	var apps []*models.App
	for _, app := range m.apps {
		if app.OrgID == orgID && app.DeletedAt == nil {
			apps = append(apps, app)
		}
	}
	return apps, nil
}

func (m *statsAppStore) Update(ctx context.Context, app *models.App) error {
	m.apps[app.ID] = app
	return nil
}

func (m *statsAppStore) Delete(ctx context.Context, id string) error {
	if app, ok := m.apps[id]; ok {
		now := time.Now()
		app.DeletedAt = &now
	}
	return nil
}

// statsDeploymentStore implements store.DeploymentStore for stats testing
type statsDeploymentStore struct {
	deployments map[string]*models.Deployment
	appStore    *statsAppStore
}

func newStatsDeploymentStore() *statsDeploymentStore {
	return &statsDeploymentStore{
		deployments: make(map[string]*models.Deployment),
	}
}

func (m *statsDeploymentStore) setAppStore(appStore *statsAppStore) {
	m.appStore = appStore
}

func (m *statsDeploymentStore) Create(ctx context.Context, deployment *models.Deployment) error {
	m.deployments[deployment.ID] = deployment
	return nil
}

func (m *statsDeploymentStore) Get(ctx context.Context, id string) (*models.Deployment, error) {
	if d, ok := m.deployments[id]; ok {
		return d, nil
	}
	return nil, nil
}

func (m *statsDeploymentStore) List(ctx context.Context, appID string) ([]*models.Deployment, error) {
	var deployments []*models.Deployment
	for _, d := range m.deployments {
		if d.AppID == appID {
			deployments = append(deployments, d)
		}
	}
	return deployments, nil
}

func (m *statsDeploymentStore) ListByNode(ctx context.Context, nodeID string) ([]*models.Deployment, error) {
	return nil, nil
}

func (m *statsDeploymentStore) ListByStatus(ctx context.Context, status models.DeploymentStatus) ([]*models.Deployment, error) {
	var deployments []*models.Deployment
	for _, d := range m.deployments {
		if d.Status == status {
			deployments = append(deployments, d)
		}
	}
	return deployments, nil
}

func (m *statsDeploymentStore) Update(ctx context.Context, deployment *models.Deployment) error {
	m.deployments[deployment.ID] = deployment
	return nil
}

func (m *statsDeploymentStore) ListByUser(ctx context.Context, userID string) ([]*models.Deployment, error) {
	return nil, nil
}

func (m *statsDeploymentStore) CountByStatusAndOrg(ctx context.Context, status models.DeploymentStatus, orgID string) (int, error) {
	count := 0
	for _, d := range m.deployments {
		if d.Status == status {
			// Check if the deployment's app belongs to the org
			if m.appStore != nil {
				if app, ok := m.appStore.apps[d.AppID]; ok {
					if app.OrgID == orgID && app.DeletedAt == nil {
						count++
					}
				}
			}
		}
	}
	return count, nil
}

func (m *statsDeploymentStore) GetNextVersion(ctx context.Context, appID, serviceName string) (int, error) {
	return 1, nil
}

// statsNodeStore implements store.NodeStore for stats testing
type statsNodeStore struct {
	nodes map[string]*models.Node
}

func newStatsNodeStore() *statsNodeStore {
	return &statsNodeStore{
		nodes: make(map[string]*models.Node),
	}
}

func (m *statsNodeStore) Register(ctx context.Context, node *models.Node) error {
	m.nodes[node.ID] = node
	return nil
}

func (m *statsNodeStore) Get(ctx context.Context, id string) (*models.Node, error) {
	if n, ok := m.nodes[id]; ok {
		return n, nil
	}
	return nil, nil
}

func (m *statsNodeStore) List(ctx context.Context) ([]*models.Node, error) {
	var nodes []*models.Node
	for _, n := range m.nodes {
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (m *statsNodeStore) UpdateHeartbeat(ctx context.Context, id string, resources *models.NodeResources) error {
	return nil
}

func (m *statsNodeStore) UpdateHeartbeatWithDiskMetrics(ctx context.Context, id string, resources *models.NodeResources, diskMetrics *models.NodeDiskMetrics) error {
	return nil
}

func (m *statsNodeStore) UpdateHealth(ctx context.Context, id string, healthy bool) error {
	if n, ok := m.nodes[id]; ok {
		n.Healthy = healthy
	}
	return nil
}

func (m *statsNodeStore) ListHealthy(ctx context.Context) ([]*models.Node, error) {
	var nodes []*models.Node
	for _, n := range m.nodes {
		if n.Healthy {
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

func (m *statsNodeStore) ListWithClosure(ctx context.Context, storePath string) ([]*models.Node, error) {
	return nil, nil
}

// genOrgID generates valid organization IDs
func genOrgID() gopter.Gen {
	return gen.RegexMatch("[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}")
}

// genUserID generates valid user IDs for stats tests
func genStatsUserID() gopter.Gen {
	return gen.RegexMatch("[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}")
}

// TestDashboardStatsAccuracy tests that dashboard statistics match database records
func TestDashboardStatsAccuracy(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("Dashboard stats match database records for organization", prop.ForAll(
		func(numApps, numServicesPerApp, numRunningDeployments, numStoppedDeployments, numHealthyNodes, numUnhealthyNodes int) bool {
			// Create a mock store
			st := newStatsMockStore()
			st.deploymentStore.setAppStore(st.appStore)
			handler := NewStatsHandler(st, logger)

			// Generate IDs
			orgID := uuid.New().String()
			userID := uuid.New().String()

			// Create apps with services
			expectedTotalServices := 0
			for i := 0; i < numApps; i++ {
				appID := uuid.New().String()
				services := make([]models.ServiceConfig, numServicesPerApp)
				for j := 0; j < numServicesPerApp; j++ {
					services[j] = models.ServiceConfig{
						Name:       "service-" + uuid.New().String()[:8],
						SourceType: models.SourceTypeGit,
						GitRepo:    "github.com/test/repo",
					}
				}
				expectedTotalServices += numServicesPerApp

				app := &models.App{
					ID:        appID,
					OrgID:     orgID,
					OwnerID:   userID,
					Name:      "app-" + uuid.New().String()[:8],
					Services:  services,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				st.appStore.Create(context.Background(), app)

				// Create running deployments for this app
				for k := 0; k < numRunningDeployments; k++ {
					deployment := &models.Deployment{
						ID:          uuid.New().String(),
						AppID:       appID,
						ServiceName: services[0].Name,
						Status:      models.DeploymentStatusRunning,
						CreatedAt:   time.Now(),
						UpdatedAt:   time.Now(),
					}
					st.deploymentStore.Create(context.Background(), deployment)
				}

				// Create stopped deployments for this app
				for k := 0; k < numStoppedDeployments; k++ {
					deployment := &models.Deployment{
						ID:          uuid.New().String(),
						AppID:       appID,
						ServiceName: services[0].Name,
						Status:      models.DeploymentStatusStopped,
						CreatedAt:   time.Now(),
						UpdatedAt:   time.Now(),
					}
					st.deploymentStore.Create(context.Background(), deployment)
				}
			}

			// Create nodes
			for i := 0; i < numHealthyNodes; i++ {
				node := &models.Node{
					ID:           uuid.New().String(),
					Hostname:     "healthy-node-" + uuid.New().String()[:8],
					Address:      "192.168.1." + string(rune('1'+i)),
					Healthy:      true,
					RegisteredAt: time.Now(),
				}
				st.nodeStore.Register(context.Background(), node)
			}

			for i := 0; i < numUnhealthyNodes; i++ {
				node := &models.Node{
					ID:           uuid.New().String(),
					Hostname:     "unhealthy-node-" + uuid.New().String()[:8],
					Address:      "192.168.2." + string(rune('1'+i)),
					Healthy:      false,
					RegisteredAt: time.Now(),
				}
				st.nodeStore.Register(context.Background(), node)
			}

			// Create router for testing
			r := chi.NewRouter()
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Add org context
					org := &models.Organization{ID: orgID, Name: "Test Org", Slug: "test-org"}
					ctx := context.WithValue(r.Context(), middleware.OrgContextKey, org)
					next.ServeHTTP(w, r.WithContext(ctx))
				})
			})
			r.Get("/v1/dashboard/stats", handler.GetDashboardStats)

			// Make request
			req := httptest.NewRequest("GET", "/v1/dashboard/stats", nil)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Logf("Request failed with status %d: %s", rr.Code, rr.Body.String())
				return false
			}

			// Parse response
			var stats DashboardStats
			if err := json.NewDecoder(rr.Body).Decode(&stats); err != nil {
				t.Logf("Failed to decode response: %v", err)
				return false
			}

			// Verify counts
			expectedRunningDeployments := numApps * numRunningDeployments
			if stats.ActiveDeployments != expectedRunningDeployments {
				t.Logf("ActiveDeployments mismatch: got %d, want %d", stats.ActiveDeployments, expectedRunningDeployments)
				return false
			}

			if stats.TotalApps != numApps {
				t.Logf("TotalApps mismatch: got %d, want %d", stats.TotalApps, numApps)
				return false
			}

			if stats.TotalServices != expectedTotalServices {
				t.Logf("TotalServices mismatch: got %d, want %d", stats.TotalServices, expectedTotalServices)
				return false
			}

			expectedTotalNodes := numHealthyNodes + numUnhealthyNodes
			if stats.NodeHealth.Total != expectedTotalNodes {
				t.Logf("NodeHealth.Total mismatch: got %d, want %d", stats.NodeHealth.Total, expectedTotalNodes)
				return false
			}

			if stats.NodeHealth.Healthy != numHealthyNodes {
				t.Logf("NodeHealth.Healthy mismatch: got %d, want %d", stats.NodeHealth.Healthy, numHealthyNodes)
				return false
			}

			if stats.NodeHealth.Unhealthy != numUnhealthyNodes {
				t.Logf("NodeHealth.Unhealthy mismatch: got %d, want %d", stats.NodeHealth.Unhealthy, numUnhealthyNodes)
				return false
			}

			return true
		},
		gen.IntRange(0, 5), // numApps
		gen.IntRange(1, 3), // numServicesPerApp
		gen.IntRange(0, 3), // numRunningDeployments per app
		gen.IntRange(0, 3), // numStoppedDeployments per app
		gen.IntRange(0, 5), // numHealthyNodes
		gen.IntRange(0, 3), // numUnhealthyNodes
	))

	properties.TestingRun(t)
}

// TestDashboardStatsOrgIsolation tests that stats are filtered by organization
func TestDashboardStatsOrgIsolation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("Dashboard stats only include apps from current organization", prop.ForAll(
		func(numAppsInOrg, numAppsOutsideOrg int) bool {
			// Create a mock store
			st := newStatsMockStore()
			st.deploymentStore.setAppStore(st.appStore)
			handler := NewStatsHandler(st, logger)

			// Generate IDs
			targetOrgID := uuid.New().String()
			otherOrgID := uuid.New().String()
			userID := uuid.New().String()

			// Create apps in target org
			for i := 0; i < numAppsInOrg; i++ {
				appID := uuid.New().String()
				app := &models.App{
					ID:        appID,
					OrgID:     targetOrgID,
					OwnerID:   userID,
					Name:      "app-in-org-" + uuid.New().String()[:8],
					Services:  []models.ServiceConfig{{Name: "svc", SourceType: models.SourceTypeGit, GitRepo: "github.com/test/repo"}},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				st.appStore.Create(context.Background(), app)

				// Create a running deployment
				deployment := &models.Deployment{
					ID:          uuid.New().String(),
					AppID:       appID,
					ServiceName: "svc",
					Status:      models.DeploymentStatusRunning,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				st.deploymentStore.Create(context.Background(), deployment)
			}

			// Create apps in other org (should not be counted)
			for i := 0; i < numAppsOutsideOrg; i++ {
				appID := uuid.New().String()
				app := &models.App{
					ID:        appID,
					OrgID:     otherOrgID,
					OwnerID:   userID,
					Name:      "app-outside-org-" + uuid.New().String()[:8],
					Services:  []models.ServiceConfig{{Name: "svc", SourceType: models.SourceTypeGit, GitRepo: "github.com/test/repo"}},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				st.appStore.Create(context.Background(), app)

				// Create a running deployment
				deployment := &models.Deployment{
					ID:          uuid.New().String(),
					AppID:       appID,
					ServiceName: "svc",
					Status:      models.DeploymentStatusRunning,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				st.deploymentStore.Create(context.Background(), deployment)
			}

			// Create router for testing with target org context
			r := chi.NewRouter()
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Add org context for target org
					org := &models.Organization{ID: targetOrgID, Name: "Target Org", Slug: "target-org"}
					ctx := context.WithValue(r.Context(), middleware.OrgContextKey, org)
					next.ServeHTTP(w, r.WithContext(ctx))
				})
			})
			r.Get("/v1/dashboard/stats", handler.GetDashboardStats)

			// Make request
			req := httptest.NewRequest("GET", "/v1/dashboard/stats", nil)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Logf("Request failed with status %d: %s", rr.Code, rr.Body.String())
				return false
			}

			// Parse response
			var stats DashboardStats
			if err := json.NewDecoder(rr.Body).Decode(&stats); err != nil {
				t.Logf("Failed to decode response: %v", err)
				return false
			}

			// Verify only apps from target org are counted
			if stats.TotalApps != numAppsInOrg {
				t.Logf("TotalApps should only count apps in target org: got %d, want %d", stats.TotalApps, numAppsInOrg)
				return false
			}

			// Verify only deployments from target org are counted
			if stats.ActiveDeployments != numAppsInOrg {
				t.Logf("ActiveDeployments should only count deployments in target org: got %d, want %d", stats.ActiveDeployments, numAppsInOrg)
				return false
			}

			return true
		},
		gen.IntRange(0, 5), // numAppsInOrg
		gen.IntRange(0, 5), // numAppsOutsideOrg
	))

	properties.TestingRun(t)
}

// TestDashboardStatsOnlyCountsRunningDeployments tests that only running deployments are counted
func TestDashboardStatsOnlyCountsRunningDeployments(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("Dashboard stats only count running deployments as active", prop.ForAll(
		func(numRunning, numPending, numStopped, numFailed int) bool {
			// Create a mock store
			st := newStatsMockStore()
			st.deploymentStore.setAppStore(st.appStore)
			handler := NewStatsHandler(st, logger)

			// Generate IDs
			orgID := uuid.New().String()
			userID := uuid.New().String()
			appID := uuid.New().String()

			// Create an app
			app := &models.App{
				ID:        appID,
				OrgID:     orgID,
				OwnerID:   userID,
				Name:      "test-app",
				Services:  []models.ServiceConfig{{Name: "svc", SourceType: models.SourceTypeGit, GitRepo: "github.com/test/repo"}},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			st.appStore.Create(context.Background(), app)

			// Create deployments with different statuses
			statuses := []struct {
				status models.DeploymentStatus
				count  int
			}{
				{models.DeploymentStatusRunning, numRunning},
				{models.DeploymentStatusPending, numPending},
				{models.DeploymentStatusStopped, numStopped},
				{models.DeploymentStatusFailed, numFailed},
			}

			for _, s := range statuses {
				for i := 0; i < s.count; i++ {
					deployment := &models.Deployment{
						ID:          uuid.New().String(),
						AppID:       appID,
						ServiceName: "svc",
						Status:      s.status,
						CreatedAt:   time.Now(),
						UpdatedAt:   time.Now(),
					}
					st.deploymentStore.Create(context.Background(), deployment)
				}
			}

			// Create router for testing
			r := chi.NewRouter()
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					org := &models.Organization{ID: orgID, Name: "Test Org", Slug: "test-org"}
					ctx := context.WithValue(r.Context(), middleware.OrgContextKey, org)
					next.ServeHTTP(w, r.WithContext(ctx))
				})
			})
			r.Get("/v1/dashboard/stats", handler.GetDashboardStats)

			// Make request
			req := httptest.NewRequest("GET", "/v1/dashboard/stats", nil)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Logf("Request failed with status %d: %s", rr.Code, rr.Body.String())
				return false
			}

			// Parse response
			var stats DashboardStats
			if err := json.NewDecoder(rr.Body).Decode(&stats); err != nil {
				t.Logf("Failed to decode response: %v", err)
				return false
			}

			// Verify only running deployments are counted as active
			if stats.ActiveDeployments != numRunning {
				t.Logf("ActiveDeployments should only count running: got %d, want %d", stats.ActiveDeployments, numRunning)
				return false
			}

			return true
		},
		gen.IntRange(0, 5), // numRunning
		gen.IntRange(0, 5), // numPending
		gen.IntRange(0, 5), // numStopped
		gen.IntRange(0, 5), // numFailed
	))

	properties.TestingRun(t)
}

// Ensure mock stores implement their interfaces
var _ store.Store = (*statsMockStore)(nil)
var _ store.AppStore = (*statsAppStore)(nil)
var _ store.DeploymentStore = (*statsDeploymentStore)(nil)
var _ store.NodeStore = (*statsNodeStore)(nil)
