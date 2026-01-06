package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// **Feature: backend-source-of-truth, Property 2: Organization Access Control**
// *For any* user and organization, if the user is not a member of the organization,
// all requests to access that organization's resources SHALL be rejected with a forbidden error.
// **Validates: Requirements 3.1, 3.4, 4.1, 4.3**

// mockOrgStore implements store.OrgStore for testing
type mockOrgStore struct {
	orgs        map[string]*models.Organization
	orgsBySlug  map[string]*models.Organization
	memberships map[string]map[string]bool // orgID -> userID -> isMember
	defaultOrgs map[string]string          // userID -> orgID
}

func newMockOrgStore() *mockOrgStore {
	return &mockOrgStore{
		orgs:        make(map[string]*models.Organization),
		orgsBySlug:  make(map[string]*models.Organization),
		memberships: make(map[string]map[string]bool),
		defaultOrgs: make(map[string]string),
	}
}

func (m *mockOrgStore) Create(ctx context.Context, org *models.Organization) error {
	m.orgs[org.ID] = org
	m.orgsBySlug[org.Slug] = org
	return nil
}

func (m *mockOrgStore) Get(ctx context.Context, id string) (*models.Organization, error) {
	if org, ok := m.orgs[id]; ok {
		return org, nil
	}
	return nil, nil
}

func (m *mockOrgStore) GetBySlug(ctx context.Context, slug string) (*models.Organization, error) {
	if org, ok := m.orgsBySlug[slug]; ok {
		return org, nil
	}
	return nil, nil
}

func (m *mockOrgStore) List(ctx context.Context, userID string) ([]*models.Organization, error) {
	return nil, nil
}

func (m *mockOrgStore) Update(ctx context.Context, org *models.Organization) error {
	return nil
}

func (m *mockOrgStore) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockOrgStore) AddMember(ctx context.Context, orgID, userID string, role models.Role) error {
	if m.memberships[orgID] == nil {
		m.memberships[orgID] = make(map[string]bool)
	}
	m.memberships[orgID][userID] = true
	return nil
}

func (m *mockOrgStore) RemoveMember(ctx context.Context, orgID, userID string) error {
	if m.memberships[orgID] != nil {
		delete(m.memberships[orgID], userID)
	}
	return nil
}

func (m *mockOrgStore) IsMember(ctx context.Context, orgID, userID string) (bool, error) {
	if m.memberships[orgID] != nil {
		return m.memberships[orgID][userID], nil
	}
	return false, nil
}

func (m *mockOrgStore) GetDefault(ctx context.Context) (*models.Organization, error) {
	return nil, nil
}

func (m *mockOrgStore) GetDefaultForUser(ctx context.Context, userID string) (*models.Organization, error) {
	if orgID, ok := m.defaultOrgs[userID]; ok {
		return m.orgs[orgID], nil
	}
	return nil, nil
}

func (m *mockOrgStore) Count(ctx context.Context) (int, error) {
	return len(m.orgs), nil
}

func (m *mockOrgStore) ListMembers(ctx context.Context, orgID string) ([]*models.OrgMembership, error) {
	return nil, nil
}

// orgTestStore implements store.Store for organization testing
type orgTestStore struct {
	appStore *mockAppStore
	orgStore *mockOrgStore
}

func newOrgTestStore() *orgTestStore {
	return &orgTestStore{
		appStore: newMockAppStore(),
		orgStore: newMockOrgStore(),
	}
}

func (m *orgTestStore) Apps() store.AppStore                                         { return m.appStore }
func (m *orgTestStore) Orgs() store.OrgStore                                         { return m.orgStore }
func (m *orgTestStore) Deployments() store.DeploymentStore                           { return nil }
func (m *orgTestStore) Nodes() store.NodeStore                                       { return nil }
func (m *orgTestStore) Builds() store.BuildStore                                     { return nil }
func (m *orgTestStore) Secrets() store.SecretStore                                   { return nil }
func (m *orgTestStore) Logs() store.LogStore                                         { return nil }
func (m *orgTestStore) Users() store.UserStore                                       { return nil }
func (m *orgTestStore) GitHub() store.GitHubStore                                    { return nil }
func (m *orgTestStore) GitHubAccounts() store.GitHubAccountStore                     { return nil }
func (m *orgTestStore) Settings() store.SettingsStore                                { return nil }
func (m *orgTestStore) Domains() store.DomainStore                                   { return nil }
func (m *orgTestStore) Invitations() store.InvitationStore                           { return nil }
func (m *orgTestStore) WithTx(ctx context.Context, fn func(store.Store) error) error { return fn(m) }
func (m *orgTestStore) Close() error                                                 { return nil }

// genOrgSlug generates valid organization slugs
func genOrgSlug() gopter.Gen {
	return gen.RegexMatch("[a-z][a-z0-9]{3,10}")
}

// TestOrgContextNonMemberDenied tests that non-members are denied access to organizations
func TestOrgContextNonMemberDenied(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("Non-member access to organization is denied", prop.ForAll(
		func(memberID, nonMemberID, orgID, orgSlug string) bool {
			// Skip if member and non-member are the same
			if memberID == nonMemberID {
				return true
			}

			// Create a mock store with an organization
			st := newOrgTestStore()
			org := &models.Organization{
				ID:        orgID,
				Name:      "Test Org",
				Slug:      orgSlug,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			st.orgStore.orgs[orgID] = org
			st.orgStore.orgsBySlug[orgSlug] = org

			// Add memberID as a member, but NOT nonMemberID
			st.orgStore.memberships[orgID] = map[string]bool{memberID: true}

			// Create the org context middleware
			middleware := OrgContext(st, logger)

			// Create a test handler that should not be reached
			handlerReached := false
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerReached = true
				w.WriteHeader(http.StatusOK)
			})

			// Create a request with the non-member's user ID and org slug header
			req := httptest.NewRequest("GET", "/v1/apps", nil)
			req.Header.Set("X-Org-Slug", orgSlug)
			ctx := context.WithValue(req.Context(), UserIDKey, nonMemberID)
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
		genOrgSlug(),
	))

	properties.TestingRun(t)
}

// TestOrgContextMemberAllowed tests that members are allowed access to organizations
func TestOrgContextMemberAllowed(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("Member access to organization is allowed", prop.ForAll(
		func(memberID, orgID, orgSlug string) bool {
			// Create a mock store with an organization
			st := newOrgTestStore()
			org := &models.Organization{
				ID:        orgID,
				Name:      "Test Org",
				Slug:      orgSlug,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			st.orgStore.orgs[orgID] = org
			st.orgStore.orgsBySlug[orgSlug] = org

			// Add memberID as a member
			st.orgStore.memberships[orgID] = map[string]bool{memberID: true}

			// Create the org context middleware
			middleware := OrgContext(st, logger)

			// Create a test handler that should be reached
			handlerReached := false
			var contextOrg *models.Organization
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerReached = true
				contextOrg = GetOrg(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			// Create a request with the member's user ID and org slug header
			req := httptest.NewRequest("GET", "/v1/apps", nil)
			req.Header.Set("X-Org-Slug", orgSlug)
			ctx := context.WithValue(req.Context(), UserIDKey, memberID)
			req = req.WithContext(ctx)

			// Create a response recorder
			rr := httptest.NewRecorder()

			// Execute the middleware
			middleware(testHandler).ServeHTTP(rr, req)

			// The handler SHOULD have been reached (access allowed)
			// and the response should be 200 OK
			// and the org should be in context
			return handlerReached && rr.Code == http.StatusOK && contextOrg != nil && contextOrg.ID == orgID
		},
		genUserID(),
		genAppID(),
		genOrgSlug(),
	))

	properties.TestingRun(t)
}

// TestRequireOwnershipOrgMemberAllowed tests that org members can access apps in their org
func TestRequireOwnershipOrgMemberAllowed(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("Org member can access apps in their organization", prop.ForAll(
		func(ownerID, memberID, appID, orgID string) bool {
			// Skip if owner and member are the same (that's covered by owner test)
			if ownerID == memberID {
				return true
			}

			// Create a mock store with an app owned by ownerID in orgID
			st := newOrgTestStore()
			app := &models.App{
				ID:        appID,
				OwnerID:   ownerID,
				OrgID:     orgID,
				Name:      "test-app",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			st.appStore.apps[appID] = app

			// Add memberID as a member of the org (but not the owner)
			st.orgStore.memberships[orgID] = map[string]bool{memberID: true}

			// Create the ownership middleware
			middleware := RequireOwnership(st, logger)

			// Create a test handler that should be reached
			handlerReached := false
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerReached = true
				w.WriteHeader(http.StatusOK)
			})

			// Create a request with the member's user ID in context
			req := httptest.NewRequest("GET", "/v1/apps/"+appID, nil)
			ctx := context.WithValue(req.Context(), UserIDKey, memberID)

			// Add the appID to chi URL params
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("appID", appID)
			ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

			req = req.WithContext(ctx)

			// Create a response recorder
			rr := httptest.NewRecorder()

			// Execute the middleware
			middleware(testHandler).ServeHTTP(rr, req)

			// The handler SHOULD have been reached (access allowed via org membership)
			// and the response should be 200 OK
			return handlerReached && rr.Code == http.StatusOK
		},
		genUserID(),
		genUserID(),
		genAppID(),
		genAppID(),
	))

	properties.TestingRun(t)
}

// TestRequireOwnershipNonOrgMemberDenied tests that non-org members cannot access apps
func TestRequireOwnershipNonOrgMemberDenied(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)
	logger := slog.Default()

	properties.Property("Non-org member cannot access apps in organization", prop.ForAll(
		func(ownerID, attackerID, appID, orgID string) bool {
			// Skip if owner and attacker are the same
			if ownerID == attackerID {
				return true
			}

			// Create a mock store with an app owned by ownerID in orgID
			st := newOrgTestStore()
			app := &models.App{
				ID:        appID,
				OwnerID:   ownerID,
				OrgID:     orgID,
				Name:      "test-app",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			st.appStore.apps[appID] = app

			// Do NOT add attackerID as a member of the org
			st.orgStore.memberships[orgID] = map[string]bool{ownerID: true}

			// Create the ownership middleware
			middleware := RequireOwnership(st, logger)

			// Create a test handler that should NOT be reached
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
		genAppID(),
	))

	properties.TestingRun(t)
}

// Ensure mockOrgStore implements store.OrgStore
var _ store.OrgStore = (*mockOrgStore)(nil)

// Ensure orgTestStore implements store.Store
var _ store.Store = (*orgTestStore)(nil)
