package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// **Feature: control-plane, Property 20: Cross-user access denied**
// *For any* application owned by user A, user B should receive a forbidden error when attempting to access it.
// **Validates: Requirements 7.4**

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
	return nil, nil
}

func (m *mockAppStore) List(ctx context.Context, ownerID string) ([]*models.App, error) {
	return nil, nil
}

func (m *mockAppStore) Update(ctx context.Context, app *models.App) error {
	return nil
}

func (m *mockAppStore) Delete(ctx context.Context, id string) error {
	return nil
}


// mockStore implements store.Store for testing
type mockStore struct {
	appStore *mockAppStore
}

func newMockStore() *mockStore {
	return &mockStore{
		appStore: newMockAppStore(),
	}
}

func (m *mockStore) Apps() store.AppStore {
	return m.appStore
}

func (m *mockStore) Deployments() store.DeploymentStore {
	return nil
}

func (m *mockStore) Nodes() store.NodeStore {
	return nil
}

func (m *mockStore) Builds() store.BuildStore {
	return nil
}

func (m *mockStore) Secrets() store.SecretStore {
	return nil
}

func (m *mockStore) Logs() store.LogStore {
	return nil
}

func (m *mockStore) Users() store.UserStore {
	return nil
}

func (m *mockStore) WithTx(ctx context.Context, fn func(store.Store) error) error {
	return fn(m)
}

func (m *mockStore) Close() error {
	return nil
}

// genUserID generates valid user IDs (non-empty alphanumeric strings)
func genUserID() gopter.Gen {
	return gen.RegexMatch("[a-zA-Z][a-zA-Z0-9]{5,15}")
}

// genAppID generates valid app IDs (non-empty alphanumeric strings)
func genAppID() gopter.Gen {
	return gen.RegexMatch("[a-zA-Z][a-zA-Z0-9]{5,15}")
}

func TestCrossUserAccessDenied(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Create a logger for the middleware
	logger := slog.Default()

	properties.Property("Cross-user access is denied", prop.ForAll(
		func(ownerID, attackerID, appID string) bool {
			// Skip if owner and attacker are the same
			if ownerID == attackerID {
				return true
			}

			// Create a mock store with an app owned by ownerID
			st := newMockStore()
			app := &models.App{
				ID:        appID,
				OwnerID:   ownerID,
				Name:      "test-app",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			st.appStore.apps[appID] = app

			// Create the ownership middleware
			middleware := RequireOwnership(st, logger)

			// Create a test handler that should not be reached
			handlerReached := false
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerReached = true
				w.WriteHeader(http.StatusOK)
			})

			// Create a request with the attacker's user ID in context
			req := httptest.NewRequest("GET", "/v1/apps/"+appID, nil)
			ctx := context.WithValue(req.Context(), UserIDKey, attackerID)
			
			// Add the appID to chi URL params
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("appID", appID)
			ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
			
			req = req.WithContext(ctx)

			// Create a response recorder
			rr := httptest.NewRecorder()

			// Execute the middleware
			middleware(testHandler).ServeHTTP(rr, req)

			// The handler should NOT have been reached (access denied)
			// and the response should be 403 Forbidden
			return !handlerReached && rr.Code == http.StatusForbidden
		},
		genUserID(),
		genUserID(),
		genAppID(),
	))

	properties.TestingRun(t)
}

func TestOwnerAccessAllowed(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Create a logger for the middleware
	logger := slog.Default()

	properties.Property("Owner access is allowed", prop.ForAll(
		func(ownerID, appID string) bool {
			// Create a mock store with an app owned by ownerID
			st := newMockStore()
			app := &models.App{
				ID:        appID,
				OwnerID:   ownerID,
				Name:      "test-app",
				BuildType: models.BuildTypeOCI,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			st.appStore.apps[appID] = app

			// Create the ownership middleware
			middleware := RequireOwnership(st, logger)

			// Create a test handler that should be reached
			handlerReached := false
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerReached = true
				w.WriteHeader(http.StatusOK)
			})

			// Create a request with the owner's user ID in context
			req := httptest.NewRequest("GET", "/v1/apps/"+appID, nil)
			ctx := context.WithValue(req.Context(), UserIDKey, ownerID)
			
			// Add the appID to chi URL params
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("appID", appID)
			ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
			
			req = req.WithContext(ctx)

			// Create a response recorder
			rr := httptest.NewRecorder()

			// Execute the middleware
			middleware(testHandler).ServeHTTP(rr, req)

			// The handler SHOULD have been reached (access allowed)
			// and the response should be 200 OK
			return handlerReached && rr.Code == http.StatusOK
		},
		genUserID(),
		genAppID(),
	))

	properties.TestingRun(t)
}

// Ensure mockAppStore implements store.AppStore
var _ store.AppStore = (*mockAppStore)(nil)

// Ensure mockStore implements store.Store
var _ store.Store = (*mockStore)(nil)

func init() {
	// Verify interface implementations at compile time
	_ = reflect.TypeOf((*store.AppStore)(nil)).Elem()
	_ = reflect.TypeOf((*store.Store)(nil)).Elem()
}
