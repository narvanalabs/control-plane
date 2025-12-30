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

func (m *mockStore) GitHub() store.GitHubStore {
	return nil
}

func (m *mockStore) GitHubAccounts() store.GitHubAccountStore {
	return nil
}

func (m *mockStore) WithTx(ctx context.Context, fn func(store.Store) error) error {
	return fn(m)
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
