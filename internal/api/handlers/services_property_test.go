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
	"github.com/google/uuid"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// serviceMockStore implements store.Store for service testing
type serviceMockStore struct {
	appStore        *mockAppStore
	deploymentStore *mockDeploymentStore
}

func newServiceMockStore() *serviceMockStore {
	return &serviceMockStore{
		appStore:        newMockAppStore(),
		deploymentStore: newMockDeploymentStore(),
	}
}

func (m *serviceMockStore) Apps() store.AppStore {
	return m.appStore
}

func (m *serviceMockStore) Deployments() store.DeploymentStore {
	return m.deploymentStore
}

func (m *serviceMockStore) Nodes() store.NodeStore {
	return nil
}

func (m *serviceMockStore) Builds() store.BuildStore {
	return nil
}

func (m *serviceMockStore) Secrets() store.SecretStore {
	return nil
}

func (m *serviceMockStore) Logs() store.LogStore {
	return nil
}

func (m *serviceMockStore) Users() store.UserStore {
	return nil
}

func (m *serviceMockStore) WithTx(ctx context.Context, fn func(store.Store) error) error {
	return fn(m)
}

func (m *serviceMockStore) Close() error {
	return nil
}

// genServiceName generates valid service names
func genServiceName() gopter.Gen {
	return gen.RegexMatch("[a-z][a-z0-9-]{2,20}")
}

// genGitRepo generates valid git repository URLs
func genGitRepo() gopter.Gen {
	return gen.OneConstOf(
		"github.com/owner/repo",
		"github.com/company/backend",
		"github.com/org/frontend",
		"gitlab.com/team/project",
	)
}

// **Feature: service-git-repos, Property 3: Service Name Uniqueness**
// *For any* app with existing services, attempting to create a service with a duplicate name
// SHALL be rejected with a conflict error.
// **Validates: Requirements 2.2**
func TestServiceNameUniqueness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("Creating a service with duplicate name is rejected", prop.ForAll(
		func(userID, serviceName, gitRepo string) bool {
			// Create a mock store
			st := newServiceMockStore()
			handler := NewServiceHandler(st, logger)

			// Create an app first
			appID := uuid.New().String()
			now := time.Now()
			app := &models.App{
				ID:        appID,
				OwnerID:   userID,
				Name:      "test-app",
				Services:  []models.ServiceConfig{},
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.Create(context.Background(), app)

			// Create router for testing
			r := chi.NewRouter()
			r.Route("/v1/apps/{appID}/services", func(r chi.Router) {
				r.Post("/", handler.Create)
			})

			// Create first service
			reqBody := CreateServiceRequest{
				Name:    serviceName,
				GitRepo: gitRepo,
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest("POST", "/v1/apps/"+appID+"/services", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusCreated {
				// First creation should succeed
				return false
			}

			// Try to create a service with the same name
			body, _ = json.Marshal(reqBody)
			req = httptest.NewRequest("POST", "/v1/apps/"+appID+"/services", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx = context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr = httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			// Second creation should fail with conflict
			return rr.Code == http.StatusConflict
		},
		genUserID(),
		genServiceName(),
		genGitRepo(),
	))

	properties.TestingRun(t)
}

// **Feature: service-git-repos, Property 4: Git URL Validation**
// *For any* service creation request with an invalid git_repo URL format,
// the request SHALL be rejected with a validation error.
// **Validates: Requirements 2.4**
func TestGitURLValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	// Generator for invalid git URLs
	genInvalidGitURL := gen.OneConstOf(
		"not-a-url",
		"ftp://invalid.com/repo",
		"just-text",
		"http://",
		"github/missing-slash",
	)

	properties.Property("Invalid git URLs are rejected", prop.ForAll(
		func(userID, serviceName, invalidGitURL string) bool {
			// Create a mock store
			st := newServiceMockStore()
			handler := NewServiceHandler(st, logger)

			// Create an app first
			appID := uuid.New().String()
			now := time.Now()
			app := &models.App{
				ID:        appID,
				OwnerID:   userID,
				Name:      "test-app",
				Services:  []models.ServiceConfig{},
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.Create(context.Background(), app)

			// Create router for testing
			r := chi.NewRouter()
			r.Route("/v1/apps/{appID}/services", func(r chi.Router) {
				r.Post("/", handler.Create)
			})

			// Try to create service with invalid git URL
			reqBody := CreateServiceRequest{
				Name:    serviceName,
				GitRepo: invalidGitURL,
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest("POST", "/v1/apps/"+appID+"/services", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			// Should be rejected with bad request
			return rr.Code == http.StatusBadRequest
		},
		genUserID(),
		genServiceName(),
		genInvalidGitURL,
	))

	properties.TestingRun(t)
}

// **Feature: service-git-repos, Property 5: Flake Reference Validation**
// *For any* service creation request with an invalid flake_uri format,
// the request SHALL be rejected with a validation error.
// **Validates: Requirements 2.5**
func TestFlakeURIValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	// Generator for invalid flake URIs
	genInvalidFlakeURI := gen.OneConstOf(
		"not-a-flake",
		"http://invalid.com",
		"just-text",
		"github.com/owner/repo", // This is git URL format, not flake format
		"ftp://something",
	)

	properties.Property("Invalid flake URIs are rejected", prop.ForAll(
		func(userID, serviceName, invalidFlakeURI string) bool {
			// Create a mock store
			st := newServiceMockStore()
			handler := NewServiceHandler(st, logger)

			// Create an app first
			appID := uuid.New().String()
			now := time.Now()
			app := &models.App{
				ID:        appID,
				OwnerID:   userID,
				Name:      "test-app",
				Services:  []models.ServiceConfig{},
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.Create(context.Background(), app)

			// Create router for testing
			r := chi.NewRouter()
			r.Route("/v1/apps/{appID}/services", func(r chi.Router) {
				r.Post("/", handler.Create)
			})

			// Try to create service with invalid flake URI
			reqBody := CreateServiceRequest{
				Name:     serviceName,
				FlakeURI: invalidFlakeURI,
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest("POST", "/v1/apps/"+appID+"/services", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			// Should be rejected with bad request
			return rr.Code == http.StatusBadRequest
		},
		genUserID(),
		genServiceName(),
		genInvalidFlakeURI,
	))

	properties.TestingRun(t)
}

// **Feature: service-git-repos, Property 6: Service Update Preserves Unspecified Fields**
// *For any* service update request that specifies only a subset of fields,
// the fields not included in the request SHALL retain their previous values.
// **Validates: Requirements 3.3**
func TestServiceUpdatePreservesFields(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("Update preserves unspecified fields", prop.ForAll(
		func(userID, serviceName, gitRepo, newGitRef string, replicas int) bool {
			// Create a mock store
			st := newServiceMockStore()
			handler := NewServiceHandler(st, logger)

			// Create an app with a service
			appID := uuid.New().String()
			now := time.Now()
			originalService := models.ServiceConfig{
				Name:         serviceName,
				SourceType:   models.SourceTypeGit,
				GitRepo:      gitRepo,
				GitRef:       "main",
				FlakeOutput:  "packages.x86_64-linux.default",
				ResourceTier: models.ResourceTierMedium,
				Replicas:     replicas,
			}
			app := &models.App{
				ID:        appID,
				OwnerID:   userID,
				Name:      "test-app",
				Services:  []models.ServiceConfig{originalService},
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.Create(context.Background(), app)

			// Create router for testing
			r := chi.NewRouter()
			r.Route("/v1/apps/{appID}/services/{serviceName}", func(r chi.Router) {
				r.Patch("/", handler.Update)
			})

			// Update only git_ref
			reqBody := map[string]interface{}{
				"git_ref": newGitRef,
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest("PATCH", "/v1/apps/"+appID+"/services/"+serviceName, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				return false
			}

			// Parse the response
			var updatedService models.ServiceConfig
			if err := json.NewDecoder(rr.Body).Decode(&updatedService); err != nil {
				return false
			}

			// Verify git_ref was updated
			if updatedService.GitRef != newGitRef {
				return false
			}

			// Verify other fields were preserved
			return updatedService.GitRepo == gitRepo &&
				updatedService.ResourceTier == models.ResourceTierMedium &&
				updatedService.Replicas == replicas
		},
		genUserID(),
		genServiceName(),
		genGitRepo(),
		gen.RegexMatch("[a-z0-9-]{3,10}"), // new git ref
		gen.IntRange(1, 5),                // replicas
	))

	properties.TestingRun(t)
}

// **Feature: service-git-repos, Property 7: Dependency Deletion Prevention**
// *For any* service that is listed in another service's depends_on,
// attempting to delete it SHALL be rejected with a dependency error.
// **Validates: Requirements 4.2**
func TestDependencyDeletionPrevention(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("Cannot delete service that others depend on", prop.ForAll(
		func(userID string) bool {
			// Create a mock store
			st := newServiceMockStore()
			handler := NewServiceHandler(st, logger)

			// Create an app with two services where one depends on the other
			appID := uuid.New().String()
			now := time.Now()
			app := &models.App{
				ID:        appID,
				OwnerID:   userID,
				Name:      "test-app",
				Services: []models.ServiceConfig{
					{
						Name:         "database",
						SourceType:   models.SourceTypeImage,
						Image:        "postgres:16",
						ResourceTier: models.ResourceTierMedium,
						Replicas:     1,
					},
					{
						Name:         "api",
						SourceType:   models.SourceTypeGit,
						GitRepo:      "github.com/company/api",
						GitRef:       "main",
						FlakeOutput:  "packages.x86_64-linux.default",
						ResourceTier: models.ResourceTierSmall,
						Replicas:     1,
						DependsOn:    []string{"database"},
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.Create(context.Background(), app)

			// Create router for testing
			r := chi.NewRouter()
			r.Route("/v1/apps/{appID}/services/{serviceName}", func(r chi.Router) {
				r.Delete("/", handler.Delete)
			})

			// Try to delete the database service (which api depends on)
			req := httptest.NewRequest("DELETE", "/v1/apps/"+appID+"/services/database", nil)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			// Should be rejected with conflict
			return rr.Code == http.StatusConflict
		},
		genUserID(),
	))

	properties.TestingRun(t)
}

// **Feature: service-git-repos, Property 10: Empty Services Array**
// *For any* app with no services, listing services SHALL return an empty array (not null).
// **Validates: Requirements 6.3**
func TestEmptyServicesArray(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("Empty app returns empty array not null", prop.ForAll(
		func(userID string) bool {
			// Create a mock store
			st := newServiceMockStore()
			handler := NewServiceHandler(st, logger)

			// Create an app with no services
			appID := uuid.New().String()
			now := time.Now()
			app := &models.App{
				ID:        appID,
				OwnerID:   userID,
				Name:      "test-app",
				Services:  nil, // Explicitly nil
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.Create(context.Background(), app)

			// Create router for testing
			r := chi.NewRouter()
			r.Route("/v1/apps/{appID}/services", func(r chi.Router) {
				r.Get("/", handler.List)
			})

			// List services
			req := httptest.NewRequest("GET", "/v1/apps/"+appID+"/services", nil)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				return false
			}

			// Check that response is an empty array, not null
			body := rr.Body.String()
			return body == "[]\n" || body == "[]"
		},
		genUserID(),
	))

	properties.TestingRun(t)
}

// **Feature: service-git-repos, Property 11: Service Listing Completeness**
// *For any* app with N services, listing services SHALL return exactly N services
// with all required fields (source, resource_tier, replicas, depends_on).
// **Validates: Requirements 7.1, 7.2, 7.3**
func TestServiceListingCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("Service listing returns all services with required fields", prop.ForAll(
		func(userID string, serviceCount int) bool {
			// Create a mock store
			st := newServiceMockStore()
			handler := NewServiceHandler(st, logger)

			// Create services
			services := make([]models.ServiceConfig, serviceCount)
			for i := 0; i < serviceCount; i++ {
				services[i] = models.ServiceConfig{
					Name:         "service-" + string(rune('a'+i)),
					SourceType:   models.SourceTypeGit,
					GitRepo:      "github.com/company/repo",
					GitRef:       "main",
					FlakeOutput:  "packages.x86_64-linux.default",
					ResourceTier: models.ResourceTierSmall,
					Replicas:     1,
				}
			}

			// Create an app with services
			appID := uuid.New().String()
			now := time.Now()
			app := &models.App{
				ID:        appID,
				OwnerID:   userID,
				Name:      "test-app",
				Services:  services,
				CreatedAt: now,
				UpdatedAt: now,
			}
			st.appStore.Create(context.Background(), app)

			// Create router for testing
			r := chi.NewRouter()
			r.Route("/v1/apps/{appID}/services", func(r chi.Router) {
				r.Get("/", handler.List)
			})

			// List services
			req := httptest.NewRequest("GET", "/v1/apps/"+appID+"/services", nil)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				return false
			}

			// Parse response
			var listedServices []models.ServiceConfig
			if err := json.NewDecoder(rr.Body).Decode(&listedServices); err != nil {
				return false
			}

			// Verify count
			if len(listedServices) != serviceCount {
				return false
			}

			// Verify all required fields are present
			for _, svc := range listedServices {
				if svc.Name == "" {
					return false
				}
				if svc.ResourceTier == "" {
					return false
				}
				if svc.Replicas == 0 {
					return false
				}
				// Verify source is set
				if svc.GitRepo == "" && svc.FlakeURI == "" && svc.Image == "" {
					return false
				}
			}

			return true
		},
		genUserID(),
		gen.IntRange(1, 5), // service count
	))

	properties.TestingRun(t)
}
