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
	"github.com/narvanalabs/control-plane/internal/store"
)

// **Feature: control-plane, Property 2: Application list completeness**
// *For any* set of applications created by a user, listing applications for that user
// should return exactly those applications (no more, no fewer).
// **Validates: Requirements 1.2**

// mockAppStore implements store.AppStore for testing
type mockAppStore struct {
	apps map[string]*models.App
}

func newMockAppStore() *mockAppStore {
	return &mockAppStore{
		apps: make(map[string]*models.App),
	}
}

func (m *mockAppStore) Create(ctx context.Context, app *models.App) error {
	m.apps[app.ID] = app
	return nil
}

func (m *mockAppStore) Get(ctx context.Context, id string) (*models.App, error) {
	if app, ok := m.apps[id]; ok {
		return app, nil
	}
	return nil, nil
}

func (m *mockAppStore) GetByName(ctx context.Context, ownerID, name string) (*models.App, error) {
	for _, app := range m.apps {
		if app.OwnerID == ownerID && app.Name == name && app.DeletedAt == nil {
			return app, nil
		}
	}
	return nil, nil
}

func (m *mockAppStore) List(ctx context.Context, ownerID string) ([]*models.App, error) {
	var result []*models.App
	for _, app := range m.apps {
		if app.OwnerID == ownerID && app.DeletedAt == nil {
			result = append(result, app)
		}
	}
	return result, nil
}

func (m *mockAppStore) Update(ctx context.Context, app *models.App) error {
	m.apps[app.ID] = app
	return nil
}

func (m *mockAppStore) Delete(ctx context.Context, id string) error {
	if app, ok := m.apps[id]; ok {
		now := time.Now()
		app.DeletedAt = &now
	}
	return nil
}

func (m *mockAppStore) ListByOrg(ctx context.Context, orgID string) ([]*models.App, error) {
	var result []*models.App
	for _, app := range m.apps {
		if app.OrgID == orgID && app.DeletedAt == nil {
			result = append(result, app)
		}
	}
	return result, nil
}

// emptyDeploymentStore implements store.DeploymentStore that returns empty results
type emptyDeploymentStore struct{}

func (m *emptyDeploymentStore) Create(ctx context.Context, deployment *models.Deployment) error {
	return nil
}

func (m *emptyDeploymentStore) Get(ctx context.Context, id string) (*models.Deployment, error) {
	return nil, nil
}

func (m *emptyDeploymentStore) List(ctx context.Context, appID string) ([]*models.Deployment, error) {
	return []*models.Deployment{}, nil
}

func (m *emptyDeploymentStore) ListByNode(ctx context.Context, nodeID string) ([]*models.Deployment, error) {
	return nil, nil
}

func (m *emptyDeploymentStore) ListByStatus(ctx context.Context, status models.DeploymentStatus) ([]*models.Deployment, error) {
	return nil, nil
}

func (m *emptyDeploymentStore) Update(ctx context.Context, deployment *models.Deployment) error {
	return nil
}

func (m *emptyDeploymentStore) ListByUser(ctx context.Context, userID string) ([]*models.Deployment, error) {
	return nil, nil
}

func (m *emptyDeploymentStore) CountByStatusAndOrg(ctx context.Context, status models.DeploymentStatus, orgID string) (int, error) {
	return 0, nil
}

func (m *emptyDeploymentStore) GetNextVersion(ctx context.Context, appID, serviceName string) (int, error) {
	return 1, nil
}

// emptySecretStore implements store.SecretStore that returns empty results
type emptySecretStore struct{}

func (m *emptySecretStore) Set(ctx context.Context, appID, key string, encryptedValue []byte) error {
	return nil
}

func (m *emptySecretStore) Get(ctx context.Context, appID, key string) ([]byte, error) {
	return nil, nil
}

func (m *emptySecretStore) List(ctx context.Context, appID string) ([]string, error) {
	return []string{}, nil
}

func (m *emptySecretStore) Delete(ctx context.Context, appID, key string) error {
	return nil
}

func (m *emptySecretStore) GetAll(ctx context.Context, appID string) (map[string][]byte, error) {
	return nil, nil
}

// mockStore implements store.Store for testing
type mockStore struct {
	appStore        *mockAppStore
	deploymentStore *emptyDeploymentStore
	secretStore     *emptySecretStore
}

func newMockStore() *mockStore {
	return &mockStore{
		appStore:        newMockAppStore(),
		deploymentStore: &emptyDeploymentStore{},
		secretStore:     &emptySecretStore{},
	}
}

func (m *mockStore) Apps() store.AppStore {
	return m.appStore
}

func (m *mockStore) Deployments() store.DeploymentStore {
	return m.deploymentStore
}

func (m *mockStore) Nodes() store.NodeStore {
	return nil
}

func (m *mockStore) Builds() store.BuildStore {
	return nil
}

func (m *mockStore) Secrets() store.SecretStore {
	return m.secretStore
}

func (m *mockStore) Logs() store.LogStore {
	return nil
}

func (m *mockStore) Users() store.UserStore {
	return nil
}

func (m *mockStore) GitHub() store.GitHubStore {
	return nil
}

func (m *mockStore) GitHubAccounts() store.GitHubAccountStore {
	return nil
}

func (m *mockStore) WithTx(ctx context.Context, fn func(store.Store) error) error {
	return fn(m)
}

func (m *mockStore) Orgs() store.OrgStore {
	return nil
}

func (m *mockStore) Settings() store.SettingsStore {
	return nil
}

func (m *mockStore) Domains() store.DomainStore {
	return nil
}

func (m *mockStore) Invitations() store.InvitationStore {
	return nil
}

func (m *mockStore) Close() error {
	return nil
}

// genAppName generates valid app names
func genAppName() gopter.Gen {
	return gen.RegexMatch("[a-z][a-z0-9-]{2,20}")
}

// genUserID generates valid user IDs
func genUserID() gopter.Gen {
	return gen.RegexMatch("[a-zA-Z][a-zA-Z0-9]{5,15}")
}

func TestApplicationListCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	logger := slog.Default()

	properties.Property("Application list returns exactly the apps created by user", prop.ForAll(
		func(userID string, appNames []string) bool {
			// Skip if no app names
			if len(appNames) == 0 {
				return true
			}

			// Deduplicate app names
			nameSet := make(map[string]bool)
			uniqueNames := []string{}
			for _, name := range appNames {
				if !nameSet[name] {
					nameSet[name] = true
					uniqueNames = append(uniqueNames, name)
				}
			}

			// Create a mock store
			st := newMockStore()
			handler := NewAppHandler(st, logger)

			// Create apps via the handler
			createdAppIDs := make(map[string]bool)
			for _, name := range uniqueNames {
				reqBody := CreateAppRequest{
					Name: name,
				}
				body, _ := json.Marshal(reqBody)

				req := httptest.NewRequest("POST", "/v1/apps", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
				req = req.WithContext(ctx)

				rr := httptest.NewRecorder()
				handler.Create(rr, req)

				if rr.Code == http.StatusCreated {
					var app models.App
					json.NewDecoder(rr.Body).Decode(&app)
					createdAppIDs[app.ID] = true
				}
			}

			// List apps via the handler
			req := httptest.NewRequest("GET", "/v1/apps", nil)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.List(rr, req)

			if rr.Code != http.StatusOK {
				return false
			}

			var listedApps []*models.App
			json.NewDecoder(rr.Body).Decode(&listedApps)

			// Check that we got exactly the apps we created
			if len(listedApps) != len(createdAppIDs) {
				return false
			}

			// Check that all listed apps are ones we created
			for _, app := range listedApps {
				if !createdAppIDs[app.ID] {
					return false
				}
			}

			return true
		},
		genUserID(),
		gen.SliceOfN(5, genAppName()),
	))

	properties.TestingRun(t)
}

func TestApplicationListIsolation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	logger := slog.Default()

	properties.Property("User A cannot see User B's apps in list", prop.ForAll(
		func(userA, userB, appName string) bool {
			// Skip if users are the same
			if userA == userB {
				return true
			}

			// Create a mock store
			st := newMockStore()
			handler := NewAppHandler(st, logger)

			// User A creates an app
			reqBody := CreateAppRequest{
				Name: appName,
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest("POST", "/v1/apps", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userA)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.Create(rr, req)

			if rr.Code != http.StatusCreated {
				return false
			}

			// User B lists apps - should see nothing
			req = httptest.NewRequest("GET", "/v1/apps", nil)
			ctx = context.WithValue(req.Context(), middleware.UserIDKey, userB)
			req = req.WithContext(ctx)

			rr = httptest.NewRecorder()
			handler.List(rr, req)

			if rr.Code != http.StatusOK {
				return false
			}

			var listedApps []*models.App
			json.NewDecoder(rr.Body).Decode(&listedApps)

			// User B should see no apps
			return len(listedApps) == 0
		},
		genUserID(),
		genUserID(),
		genAppName(),
	))

	properties.TestingRun(t)
}

// setupTestRouter creates a chi router with the app handler for testing
func setupTestRouter(st store.Store, logger *slog.Logger) chi.Router {
	r := chi.NewRouter()
	handler := NewAppHandler(st, logger)
	r.Route("/v1/apps", func(r chi.Router) {
		r.Post("/", handler.Create)
		r.Get("/", handler.List)
		r.Route("/{appID}", func(r chi.Router) {
			r.Get("/", handler.Get)
			r.Delete("/", handler.Delete)
		})
	})
	return r
}

// **Feature: platform-enhancements, Property 12: Application CRUD Round-Trip**
// *For any* valid application data, creating an application and then retrieving it by ID
// SHALL return equivalent data.
// **Validates: Requirements 21.1, 21.2**
func TestAppCRUDRoundTripHandler(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("App CRUD round-trip preserves data via handlers", prop.ForAll(
		func(userID, appName, description string) bool {
			// Create a mock store
			st := newMockStore()
			r := setupTestRouter(st, logger)

			// CREATE: Create the app
			reqBody := CreateAppRequest{
				Name:        appName,
				Description: description,
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest("POST", "/v1/apps", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusCreated {
				t.Logf("Create failed with status %d: %s", rr.Code, rr.Body.String())
				return false
			}

			var createdApp models.App
			if err := json.NewDecoder(rr.Body).Decode(&createdApp); err != nil {
				t.Logf("Failed to decode created app: %v", err)
				return false
			}

			// Verify created app has correct data
			if createdApp.Name != appName {
				t.Logf("Name mismatch: got %s, want %s", createdApp.Name, appName)
				return false
			}
			if createdApp.Description != description {
				t.Logf("Description mismatch: got %s, want %s", createdApp.Description, description)
				return false
			}
			if createdApp.OwnerID != userID {
				t.Logf("OwnerID mismatch: got %s, want %s", createdApp.OwnerID, userID)
				return false
			}

			// READ: Retrieve the app by ID
			req = httptest.NewRequest("GET", "/v1/apps/"+createdApp.ID, nil)
			ctx = context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr = httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Logf("Get failed with status %d: %s", rr.Code, rr.Body.String())
				return false
			}

			var retrievedApp models.App
			if err := json.NewDecoder(rr.Body).Decode(&retrievedApp); err != nil {
				t.Logf("Failed to decode retrieved app: %v", err)
				return false
			}

			// Verify retrieved app matches created app
			if retrievedApp.ID != createdApp.ID {
				t.Logf("ID mismatch: got %s, want %s", retrievedApp.ID, createdApp.ID)
				return false
			}
			if retrievedApp.Name != createdApp.Name {
				t.Logf("Name mismatch after get: got %s, want %s", retrievedApp.Name, createdApp.Name)
				return false
			}
			if retrievedApp.Description != createdApp.Description {
				t.Logf("Description mismatch after get: got %s, want %s", retrievedApp.Description, createdApp.Description)
				return false
			}
			if retrievedApp.OwnerID != createdApp.OwnerID {
				t.Logf("OwnerID mismatch after get: got %s, want %s", retrievedApp.OwnerID, createdApp.OwnerID)
				return false
			}

			// DELETE: Delete the app
			req = httptest.NewRequest("DELETE", "/v1/apps/"+createdApp.ID, nil)
			ctx = context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr = httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusNoContent {
				t.Logf("Delete failed with status %d: %s", rr.Code, rr.Body.String())
				return false
			}

			// Verify app is no longer in list
			req = httptest.NewRequest("GET", "/v1/apps", nil)
			ctx = context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr = httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Logf("List after delete failed with status %d", rr.Code)
				return false
			}

			var listedApps []*models.App
			if err := json.NewDecoder(rr.Body).Decode(&listedApps); err != nil {
				t.Logf("Failed to decode listed apps: %v", err)
				return false
			}

			// Deleted app should not appear in list
			for _, app := range listedApps {
				if app.ID == createdApp.ID {
					t.Logf("Deleted app still appears in list")
					return false
				}
			}

			return true
		},
		genUserID(),
		genAppName(),
		gen.AlphaString(), // description
	))

	properties.TestingRun(t)
}

// appMockDeploymentStore implements store.DeploymentStore for app deletion testing
type appMockDeploymentStore struct {
	deployments map[string]*models.Deployment
}

func newAppMockDeploymentStore() *appMockDeploymentStore {
	return &appMockDeploymentStore{
		deployments: make(map[string]*models.Deployment),
	}
}

func (m *appMockDeploymentStore) Create(ctx context.Context, deployment *models.Deployment) error {
	m.deployments[deployment.ID] = deployment
	return nil
}

func (m *appMockDeploymentStore) Get(ctx context.Context, id string) (*models.Deployment, error) {
	if d, ok := m.deployments[id]; ok {
		return d, nil
	}
	return nil, nil
}

func (m *appMockDeploymentStore) List(ctx context.Context, appID string) ([]*models.Deployment, error) {
	var result []*models.Deployment
	for _, d := range m.deployments {
		if d.AppID == appID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *appMockDeploymentStore) ListByNode(ctx context.Context, nodeID string) ([]*models.Deployment, error) {
	return nil, nil
}

func (m *appMockDeploymentStore) ListByStatus(ctx context.Context, status models.DeploymentStatus) ([]*models.Deployment, error) {
	return nil, nil
}

func (m *appMockDeploymentStore) Update(ctx context.Context, deployment *models.Deployment) error {
	m.deployments[deployment.ID] = deployment
	return nil
}

func (m *appMockDeploymentStore) ListByUser(ctx context.Context, userID string) ([]*models.Deployment, error) {
	return nil, nil
}

func (m *appMockDeploymentStore) CountByStatusAndOrg(ctx context.Context, status models.DeploymentStatus, orgID string) (int, error) {
	return 0, nil
}

func (m *appMockDeploymentStore) GetNextVersion(ctx context.Context, appID, serviceName string) (int, error) {
	return 1, nil
}

// appMockSecretStore implements store.SecretStore for app deletion testing
type appMockSecretStore struct {
	secrets map[string]map[string][]byte // appID -> key -> value
}

func newAppMockSecretStore() *appMockSecretStore {
	return &appMockSecretStore{
		secrets: make(map[string]map[string][]byte),
	}
}

func (m *appMockSecretStore) Set(ctx context.Context, appID, key string, encryptedValue []byte) error {
	if m.secrets[appID] == nil {
		m.secrets[appID] = make(map[string][]byte)
	}
	m.secrets[appID][key] = encryptedValue
	return nil
}

func (m *appMockSecretStore) Get(ctx context.Context, appID, key string) ([]byte, error) {
	if appSecrets, ok := m.secrets[appID]; ok {
		if val, ok := appSecrets[key]; ok {
			return val, nil
		}
	}
	return nil, nil
}

func (m *appMockSecretStore) List(ctx context.Context, appID string) ([]string, error) {
	var keys []string
	if appSecrets, ok := m.secrets[appID]; ok {
		for key := range appSecrets {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (m *appMockSecretStore) Delete(ctx context.Context, appID, key string) error {
	if appSecrets, ok := m.secrets[appID]; ok {
		delete(appSecrets, key)
	}
	return nil
}

func (m *appMockSecretStore) GetAll(ctx context.Context, appID string) (map[string][]byte, error) {
	if appSecrets, ok := m.secrets[appID]; ok {
		return appSecrets, nil
	}
	return nil, nil
}

// appDeletionMockStore extends mockStore with deployment and secret support for app deletion testing
type appDeletionMockStore struct {
	appStore        *mockAppStore
	deploymentStore *appMockDeploymentStore
	secretStore     *appMockSecretStore
}

func newAppDeletionMockStore() *appDeletionMockStore {
	return &appDeletionMockStore{
		appStore:        newMockAppStore(),
		deploymentStore: newAppMockDeploymentStore(),
		secretStore:     newAppMockSecretStore(),
	}
}

func (m *appDeletionMockStore) Apps() store.AppStore {
	return m.appStore
}

func (m *appDeletionMockStore) Deployments() store.DeploymentStore {
	return m.deploymentStore
}

func (m *appDeletionMockStore) Secrets() store.SecretStore {
	return m.secretStore
}

func (m *appDeletionMockStore) Nodes() store.NodeStore {
	return nil
}

func (m *appDeletionMockStore) Builds() store.BuildStore {
	return nil
}

func (m *appDeletionMockStore) Logs() store.LogStore {
	return nil
}

func (m *appDeletionMockStore) Users() store.UserStore {
	return nil
}

func (m *appDeletionMockStore) GitHub() store.GitHubStore {
	return nil
}

func (m *appDeletionMockStore) GitHubAccounts() store.GitHubAccountStore {
	return nil
}

func (m *appDeletionMockStore) WithTx(ctx context.Context, fn func(store.Store) error) error {
	return fn(m)
}

func (m *appDeletionMockStore) Orgs() store.OrgStore {
	return nil
}

func (m *appDeletionMockStore) Settings() store.SettingsStore {
	return nil
}

func (m *appDeletionMockStore) Domains() store.DomainStore {
	return nil
}

func (m *appDeletionMockStore) Invitations() store.InvitationStore {
	return nil
}

func (m *appDeletionMockStore) Close() error {
	return nil
}

// **Feature: backend-source-of-truth, Property 10: App Deletion Deployment Cleanup**
// *For any* app deletion, all deployments with status "running", "pending", or "building"
// SHALL be transitioned to "stopped" or "cancelled" before the app is soft-deleted.
// **Validates: Requirements 11.1, 11.2**
func TestAppDeletionDeploymentCleanup(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	// Generator for deployment status
	genDeploymentStatus := gen.OneConstOf(
		models.DeploymentStatusPending,
		models.DeploymentStatusBuilding,
		models.DeploymentStatusRunning,
		models.DeploymentStatusStopped,
		models.DeploymentStatusFailed,
	)

	properties.Property("App deletion stops running and cancels pending/building deployments", prop.ForAll(
		func(userID, appName string, deploymentStatuses []models.DeploymentStatus) bool {
			// Skip if no deployments
			if len(deploymentStatuses) == 0 {
				return true
			}

			// Create a mock store with deployment support
			st := newAppDeletionMockStore()
			handler := NewAppHandler(st, logger)

			// Create an app
			reqBody := CreateAppRequest{
				Name: appName,
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest("POST", "/v1/apps", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.Create(rr, req)

			if rr.Code != http.StatusCreated {
				t.Logf("Create failed with status %d", rr.Code)
				return false
			}

			var createdApp models.App
			json.NewDecoder(rr.Body).Decode(&createdApp)

			// Create deployments with various statuses
			for i, status := range deploymentStatuses {
				deployment := &models.Deployment{
					ID:          "deployment-" + string(rune('a'+i)),
					AppID:       createdApp.ID,
					ServiceName: "service-" + string(rune('a'+i)),
					Status:      status,
				}
				st.deploymentStore.Create(ctx, deployment)
			}

			// Add some secrets
			st.secretStore.Set(ctx, createdApp.ID, "DB_PASSWORD", []byte("secret1"))
			st.secretStore.Set(ctx, createdApp.ID, "API_KEY", []byte("secret2"))

			// Delete the app
			r := chi.NewRouter()
			r.Route("/v1/apps/{appID}", func(r chi.Router) {
				r.Delete("/", handler.Delete)
			})

			req = httptest.NewRequest("DELETE", "/v1/apps/"+createdApp.ID, nil)
			ctx = context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr = httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusNoContent {
				t.Logf("Delete failed with status %d: %s", rr.Code, rr.Body.String())
				return false
			}

			// Verify all deployments have been properly transitioned
			for _, deployment := range st.deploymentStore.deployments {
				if deployment.AppID != createdApp.ID {
					continue
				}

				switch deployment.Status {
				case models.DeploymentStatusRunning:
					// Running deployments should have been stopped
					t.Logf("Running deployment %s was not stopped", deployment.ID)
					return false
				case models.DeploymentStatusPending, models.DeploymentStatusBuilding:
					// Pending/building deployments should have been cancelled (failed)
					t.Logf("Pending/building deployment %s was not cancelled", deployment.ID)
					return false
				case models.DeploymentStatusStopped, models.DeploymentStatusFailed:
					// These are acceptable final states
				}
			}

			// Verify secrets were cleaned up
			secrets, _ := st.secretStore.List(ctx, createdApp.ID)
			if len(secrets) > 0 {
				t.Logf("Secrets were not cleaned up: %v", secrets)
				return false
			}

			// Verify app was soft-deleted
			if createdApp.DeletedAt == nil {
				// Check the store directly
				app, _ := st.appStore.Get(ctx, createdApp.ID)
				if app != nil && app.DeletedAt == nil {
					t.Logf("App was not soft-deleted")
					return false
				}
			}

			return true
		},
		genUserID(),
		genAppName(),
		gen.SliceOfN(5, genDeploymentStatus),
	))

	properties.TestingRun(t)
}
