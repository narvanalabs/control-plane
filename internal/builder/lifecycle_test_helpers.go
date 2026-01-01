// Package builder provides build lifecycle test helpers and mock implementations.
package builder

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/narvanalabs/control-plane/internal/builder/executor"
	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/queue"
	"github.com/narvanalabs/control-plane/internal/store"
)

// MockNixBuilder is a mock implementation of the NixBuilder interface for testing.
type MockNixBuilder struct {
	mu           sync.Mutex
	BuildCalls   []MockBuildCall
	BuildResult  *executor.NixBuildResult
	BuildError   error
	BuildDelay   time.Duration
	ShouldFail   bool
	FailureError error
}

// MockBuildCall records a call to the mock builder.
type MockBuildCall struct {
	Job       *models.BuildJob
	Timestamp time.Time
}

// NewMockNixBuilder creates a new MockNixBuilder with default success behavior.
func NewMockNixBuilder() *MockNixBuilder {
	return &MockNixBuilder{
		BuildResult: &executor.NixBuildResult{
			StorePath: "/nix/store/mock-hash-package",
			Logs:      "Mock build completed successfully",
			ExitCode:  0,
		},
	}
}

// BuildWithLogCallback implements the NixBuilder interface.
func (m *MockNixBuilder) BuildWithLogCallback(ctx context.Context, job *models.BuildJob, callback func(line string)) (*executor.NixBuildResult, error) {
	m.mu.Lock()
	m.BuildCalls = append(m.BuildCalls, MockBuildCall{Job: job, Timestamp: time.Now()})
	m.mu.Unlock()

	if m.BuildDelay > 0 {
		select {
		case <-time.After(m.BuildDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if callback != nil {
		callback("=== Mock Nix Build ===")
		callback("Building...")
	}

	if m.ShouldFail {
		if m.FailureError != nil {
			return nil, m.FailureError
		}
		return nil, errors.New("mock build failed")
	}

	if m.BuildError != nil {
		return nil, m.BuildError
	}

	return m.BuildResult, nil
}

// GetBuildCalls returns the recorded build calls.
func (m *MockNixBuilder) GetBuildCalls() []MockBuildCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MockBuildCall{}, m.BuildCalls...)
}

// Reset clears all recorded calls.
func (m *MockNixBuilder) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BuildCalls = nil
}

// MockOCIBuilder is a mock implementation of the OCIBuilder interface for testing.
type MockOCIBuilder struct {
	mu           sync.Mutex
	BuildCalls   []MockBuildCall
	BuildResult  *executor.OCIBuildResult
	BuildError   error
	BuildDelay   time.Duration
	ShouldFail   bool
	FailureError error
}

// NewMockOCIBuilder creates a new MockOCIBuilder with default success behavior.
func NewMockOCIBuilder() *MockOCIBuilder {
	return &MockOCIBuilder{
		BuildResult: &executor.OCIBuildResult{
			ImageTag:  "registry.example.com/app:latest",
			StorePath: "/nix/store/mock-oci-hash",
			Logs:      "Mock OCI build completed successfully",
			ExitCode:  0,
		},
	}
}


// BuildWithLogCallback implements the OCIBuilder interface.
func (m *MockOCIBuilder) BuildWithLogCallback(ctx context.Context, job *models.BuildJob, callback func(line string)) (*executor.OCIBuildResult, error) {
	m.mu.Lock()
	m.BuildCalls = append(m.BuildCalls, MockBuildCall{Job: job, Timestamp: time.Now()})
	m.mu.Unlock()

	if m.BuildDelay > 0 {
		select {
		case <-time.After(m.BuildDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if callback != nil {
		callback("=== Mock OCI Build ===")
		callback("Building container image...")
	}

	if m.ShouldFail {
		if m.FailureError != nil {
			return nil, m.FailureError
		}
		return nil, errors.New("mock OCI build failed")
	}

	if m.BuildError != nil {
		return nil, m.BuildError
	}

	return m.BuildResult, nil
}

// GetBuildCalls returns the recorded build calls.
func (m *MockOCIBuilder) GetBuildCalls() []MockBuildCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MockBuildCall{}, m.BuildCalls...)
}

// Reset clears all recorded calls.
func (m *MockOCIBuilder) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BuildCalls = nil
}

// MockAtticClient is a mock implementation of the AtticClient for testing.
type MockAtticClient struct {
	mu          sync.Mutex
	PushCalls   []MockPushCall
	PushResultVal  *PushResult
	PushError   error
	ShouldFail  bool
}

// MockPushCall records a call to the mock Attic client.
type MockPushCall struct {
	StorePath string
	Timestamp time.Time
}

// NewMockAtticClient creates a new MockAtticClient with default success behavior.
func NewMockAtticClient() *MockAtticClient {
	return &MockAtticClient{
		PushResultVal: &PushResult{
			CacheURL:  "https://cache.example.com",
			StorePath: "/nix/store/mock-hash-package",
		},
	}
}

// PushWithDependencies implements the AtticClient interface.
func (m *MockAtticClient) PushWithDependencies(ctx context.Context, storePath string) (*PushResult, error) {
	m.mu.Lock()
	m.PushCalls = append(m.PushCalls, MockPushCall{StorePath: storePath, Timestamp: time.Now()})
	m.mu.Unlock()

	if m.ShouldFail {
		return nil, errors.New("mock Attic push failed")
	}

	if m.PushError != nil {
		return nil, m.PushError
	}

	return m.PushResultVal, nil
}

// GetPushCalls returns the recorded push calls.
func (m *MockAtticClient) GetPushCalls() []MockPushCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MockPushCall{}, m.PushCalls...)
}

// Reset clears all recorded calls.
func (m *MockAtticClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PushCalls = nil
}

// MockStore is a mock implementation of the Store interface for testing.
type MockStore struct {
	orgs           *MockOrgStore
	apps           *MockAppStore
	deployments    *MockDeploymentStore
	nodes          *MockNodeStore
	builds         *MockBuildStore
	secrets        *MockSecretStore
	logs           *LifecycleMockLogStore
	users          *MockUserStore
	github         *MockGitHubStore
	githubAccounts *MockGitHubAccountStore
	settings       *MockSettingsStore
	domains        *MockDomainStore
}

// NewMockStore creates a new MockStore.
func NewMockStore() *MockStore {
	return &MockStore{
		orgs:        NewMockOrgStore(),
		apps:        NewMockAppStore(),
		deployments: NewMockDeploymentStore(),
		nodes:       NewMockNodeStore(),
		builds:      NewMockBuildStore(),
		secrets:     NewMockSecretStore(),
		logs:        NewLifecycleMockLogStore(),
		users:       NewMockUserStore(),
		github:      NewMockGitHubStore(),
		githubAccounts: NewMockGitHubAccountStore(),
		settings:    NewMockSettingsStore(),
		domains:     NewMockDomainStore(),
	}
}

func (m *MockStore) Orgs() store.OrgStore           { return m.orgs }
func (m *MockStore) Apps() store.AppStore           { return m.apps }
func (m *MockStore) Deployments() store.DeploymentStore { return m.deployments }
func (m *MockStore) Nodes() store.NodeStore         { return m.nodes }
func (m *MockStore) Builds() store.BuildStore       { return m.builds }
func (m *MockStore) Secrets() store.SecretStore     { return m.secrets }
func (m *MockStore) Logs() store.LogStore           { return m.logs }
func (m *MockStore) Users() store.UserStore         { return m.users }
func (m *MockStore) GitHub() store.GitHubStore      { return m.github }
func (m *MockStore) GitHubAccounts() store.GitHubAccountStore { return m.githubAccounts }
func (m *MockStore) Settings() store.SettingsStore      { return m.settings }
func (m *MockStore) Domains() store.DomainStore         { return m.domains }
func (m *MockStore) Invitations() store.InvitationStore { return nil }
func (m *MockStore) WithTx(ctx context.Context, fn func(store.Store) error) error { return fn(m) }
func (m *MockStore) Close() error                   { return nil }

// MockOrgStore is a mock implementation of OrgStore for testing.
type MockOrgStore struct {
	mu   sync.Mutex
	orgs map[string]*models.Organization
}

// NewMockOrgStore creates a new MockOrgStore.
func NewMockOrgStore() *MockOrgStore {
	return &MockOrgStore{
		orgs: make(map[string]*models.Organization),
	}
}

func (m *MockOrgStore) Create(ctx context.Context, org *models.Organization) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orgs[org.ID] = org
	return nil
}

func (m *MockOrgStore) Get(ctx context.Context, id string) (*models.Organization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if org, ok := m.orgs[id]; ok {
		return org, nil
	}
	return nil, errors.New("not found")
}

func (m *MockOrgStore) GetBySlug(ctx context.Context, slug string) (*models.Organization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, org := range m.orgs {
		if org.Slug == slug {
			return org, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *MockOrgStore) List(ctx context.Context, userID string) ([]*models.Organization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.Organization
	for _, org := range m.orgs {
		result = append(result, org)
	}
	return result, nil
}

func (m *MockOrgStore) Update(ctx context.Context, org *models.Organization) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orgs[org.ID] = org
	return nil
}

func (m *MockOrgStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.orgs, id)
	return nil
}

func (m *MockOrgStore) AddMember(ctx context.Context, orgID, userID string, role models.Role) error {
	return nil
}

func (m *MockOrgStore) RemoveMember(ctx context.Context, orgID, userID string) error {
	return nil
}

func (m *MockOrgStore) GetDefault(ctx context.Context) (*models.Organization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, org := range m.orgs {
		return org, nil
	}
	return nil, errors.New("not found")
}

func (m *MockOrgStore) Count(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.orgs), nil
}

func (m *MockOrgStore) ListMembers(ctx context.Context, orgID string) ([]*models.OrgMembership, error) {
	return nil, nil
}


// MockBuildStore is a mock implementation of BuildStore for testing.
type MockBuildStore struct {
	mu     sync.Mutex
	builds map[string]*models.BuildJob
	// Track state transitions for verification
	StateTransitions []StateTransition
}

// StateTransition records a state change for a build job.
type StateTransition struct {
	BuildID   string
	FromState models.BuildStatus
	ToState   models.BuildStatus
	Timestamp time.Time
}

// NewMockBuildStore creates a new MockBuildStore.
func NewMockBuildStore() *MockBuildStore {
	return &MockBuildStore{
		builds:           make(map[string]*models.BuildJob),
		StateTransitions: make([]StateTransition, 0),
	}
}

func (m *MockBuildStore) Create(ctx context.Context, build *models.BuildJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.builds[build.ID] = build
	return nil
}

func (m *MockBuildStore) Get(ctx context.Context, id string) (*models.BuildJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if build, ok := m.builds[id]; ok {
		return build, nil
	}
	return nil, errors.New("build not found")
}

func (m *MockBuildStore) GetByDeployment(ctx context.Context, deploymentID string) (*models.BuildJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, build := range m.builds {
		if build.DeploymentID == deploymentID {
			return build, nil
		}
	}
	return nil, errors.New("build not found")
}

func (m *MockBuildStore) Update(ctx context.Context, build *models.BuildJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.builds[build.ID]; ok {
		// Record state transition if status changed
		if existing.Status != build.Status {
			m.StateTransitions = append(m.StateTransitions, StateTransition{
				BuildID:   build.ID,
				FromState: existing.Status,
				ToState:   build.Status,
				Timestamp: time.Now(),
			})
		}
	}
	m.builds[build.ID] = build
	return nil
}

func (m *MockBuildStore) ListPending(ctx context.Context) ([]*models.BuildJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var pending []*models.BuildJob
	for _, build := range m.builds {
		if build.Status == models.BuildStatusQueued {
			pending = append(pending, build)
		}
	}
	return pending, nil
}

// ListRunning retrieves all builds with status 'running'.
// Used for startup recovery to identify interrupted builds.
// **Validates: Requirements 15.1, 15.2**
func (m *MockBuildStore) ListRunning(ctx context.Context) ([]*models.BuildJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var running []*models.BuildJob
	for _, build := range m.builds {
		if build.Status == models.BuildStatusRunning {
			running = append(running, build)
		}
	}
	return running, nil
}

// ListQueued retrieves all builds with status 'queued'.
// Used for startup recovery to resume pending builds.
// **Validates: Requirements 15.1**
func (m *MockBuildStore) ListQueued(ctx context.Context) ([]*models.BuildJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var queued []*models.BuildJob
	for _, build := range m.builds {
		if build.Status == models.BuildStatusQueued {
			queued = append(queued, build)
		}
	}
	return queued, nil
}

func (m *MockBuildStore) List(ctx context.Context, appID string) ([]*models.BuildJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.BuildJob
	for _, build := range m.builds {
		if build.AppID == appID {
			result = append(result, build)
		}
	}
	return result, nil
}

func (m *MockBuildStore) ListByUser(ctx context.Context, userID string) ([]*models.BuildJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.BuildJob
	for _, build := range m.builds {
		result = append(result, build)
	}
	return result, nil
}

// GetStateTransitions returns all recorded state transitions.
func (m *MockBuildStore) GetStateTransitions() []StateTransition {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]StateTransition{}, m.StateTransitions...)
}

// Reset clears all builds and transitions.
func (m *MockBuildStore) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.builds = make(map[string]*models.BuildJob)
	m.StateTransitions = nil
}

// MockDeploymentStore is a mock implementation of DeploymentStore for testing.
type MockDeploymentStore struct {
	mu          sync.Mutex
	deployments map[string]*models.Deployment
}

// NewMockDeploymentStore creates a new MockDeploymentStore.
func NewMockDeploymentStore() *MockDeploymentStore {
	return &MockDeploymentStore{
		deployments: make(map[string]*models.Deployment),
	}
}

func (m *MockDeploymentStore) Create(ctx context.Context, deployment *models.Deployment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deployments[deployment.ID] = deployment
	return nil
}

func (m *MockDeploymentStore) Get(ctx context.Context, id string) (*models.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if deployment, ok := m.deployments[id]; ok {
		return deployment, nil
	}
	return nil, errors.New("deployment not found")
}

func (m *MockDeploymentStore) List(ctx context.Context, appID string) ([]*models.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.Deployment
	for _, d := range m.deployments {
		if d.AppID == appID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *MockDeploymentStore) ListByNode(ctx context.Context, nodeID string) ([]*models.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.Deployment
	for _, d := range m.deployments {
		if d.NodeID == nodeID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *MockDeploymentStore) ListByStatus(ctx context.Context, status models.DeploymentStatus) ([]*models.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.Deployment
	for _, d := range m.deployments {
		if d.Status == status {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *MockDeploymentStore) Update(ctx context.Context, deployment *models.Deployment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deployments[deployment.ID] = deployment
	return nil
}

func (m *MockDeploymentStore) GetLatestSuccessful(ctx context.Context, appID string) (*models.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var latest *models.Deployment
	for _, d := range m.deployments {
		if d.AppID == appID && d.Status == models.DeploymentStatusRunning {
			if latest == nil || d.CreatedAt.After(latest.CreatedAt) {
				latest = d
			}
		}
	}
	if latest == nil {
		return nil, errors.New("no successful deployment found")
	}
	return latest, nil
}

func (m *MockDeploymentStore) ListByUser(ctx context.Context, userID string) ([]*models.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.Deployment
	for _, d := range m.deployments {
		result = append(result, d)
	}
	return result, nil
}

// GetNextVersion returns the next version number for a service.
// Returns 1 for the first deployment, or max(version) + 1 for subsequent deployments.
func (m *MockDeploymentStore) GetNextVersion(ctx context.Context, appID, serviceName string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	maxVersion := 0
	for _, d := range m.deployments {
		if d.AppID == appID && d.ServiceName == serviceName {
			if d.Version > maxVersion {
				maxVersion = d.Version
			}
		}
	}
	return maxVersion + 1, nil
}

// Reset clears all deployments.
func (m *MockDeploymentStore) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deployments = make(map[string]*models.Deployment)
}


// MockQueue is a mock implementation of Queue for testing.
type MockQueue struct {
	mu       sync.Mutex
	jobs     []*models.BuildJob
	acked    map[string]bool
	nacked   map[string]bool
	dequeued map[string]bool
}

// NewMockQueue creates a new MockQueue.
func NewMockQueue() *MockQueue {
	return &MockQueue{
		jobs:     make([]*models.BuildJob, 0),
		acked:    make(map[string]bool),
		nacked:   make(map[string]bool),
		dequeued: make(map[string]bool),
	}
}

func (m *MockQueue) Enqueue(ctx context.Context, job *models.BuildJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs = append(m.jobs, job)
	return nil
}

func (m *MockQueue) Dequeue(ctx context.Context) (*models.BuildJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, job := range m.jobs {
		if !m.dequeued[job.ID] && !m.acked[job.ID] {
			m.dequeued[job.ID] = true
			return job, nil
		}
	}
	return nil, queue.ErrNoJobs
}

func (m *MockQueue) Ack(ctx context.Context, jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.acked[jobID] = true
	return nil
}

func (m *MockQueue) Nack(ctx context.Context, jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nacked[jobID] = true
	delete(m.dequeued, jobID)
	return nil
}

// IsAcked returns true if the job was acknowledged.
func (m *MockQueue) IsAcked(jobID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.acked[jobID]
}

// IsNacked returns true if the job was nacked.
func (m *MockQueue) IsNacked(jobID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nacked[jobID]
}

// Reset clears all queue state.
func (m *MockQueue) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs = nil
	m.acked = make(map[string]bool)
	m.nacked = make(map[string]bool)
	m.dequeued = make(map[string]bool)
}

// MockAppStore is a mock implementation of AppStore for testing.
type MockAppStore struct {
	mu   sync.Mutex
	apps map[string]*models.App
}

func NewMockAppStore() *MockAppStore {
	return &MockAppStore{apps: make(map[string]*models.App)}
}

func (m *MockAppStore) Create(ctx context.Context, app *models.App) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.apps[app.ID] = app
	return nil
}

func (m *MockAppStore) Get(ctx context.Context, id string) (*models.App, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if app, ok := m.apps[id]; ok {
		return app, nil
	}
	return nil, errors.New("app not found")
}

func (m *MockAppStore) GetByName(ctx context.Context, ownerID, name string) (*models.App, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, app := range m.apps {
		if app.OwnerID == ownerID && app.Name == name {
			return app, nil
		}
	}
	return nil, errors.New("app not found")
}

func (m *MockAppStore) List(ctx context.Context, ownerID string) ([]*models.App, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.App
	for _, app := range m.apps {
		if app.OwnerID == ownerID {
			result = append(result, app)
		}
	}
	return result, nil
}

func (m *MockAppStore) Update(ctx context.Context, app *models.App) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.apps[app.ID] = app
	return nil
}

func (m *MockAppStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.apps, id)
	return nil
}

// MockNodeStore is a mock implementation of NodeStore for testing.
type MockNodeStore struct {
	mu    sync.Mutex
	nodes map[string]*models.Node
}

func NewMockNodeStore() *MockNodeStore {
	return &MockNodeStore{nodes: make(map[string]*models.Node)}
}

func (m *MockNodeStore) Register(ctx context.Context, node *models.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodes[node.ID] = node
	return nil
}

func (m *MockNodeStore) Get(ctx context.Context, id string) (*models.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if node, ok := m.nodes[id]; ok {
		return node, nil
	}
	return nil, errors.New("node not found")
}

func (m *MockNodeStore) List(ctx context.Context) ([]*models.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.Node
	for _, node := range m.nodes {
		result = append(result, node)
	}
	return result, nil
}

func (m *MockNodeStore) UpdateHeartbeat(ctx context.Context, id string, resources *models.NodeResources) error {
	return nil
}

func (m *MockNodeStore) UpdateHealth(ctx context.Context, id string, healthy bool) error {
	return nil
}

func (m *MockNodeStore) ListHealthy(ctx context.Context) ([]*models.Node, error) {
	return m.List(ctx)
}

func (m *MockNodeStore) ListWithClosure(ctx context.Context, storePath string) ([]*models.Node, error) {
	return m.List(ctx)
}


// MockSecretStore is a mock implementation of SecretStore for testing.
type MockSecretStore struct {
	mu      sync.Mutex
	secrets map[string]map[string][]byte
}

func NewMockSecretStore() *MockSecretStore {
	return &MockSecretStore{secrets: make(map[string]map[string][]byte)}
}

func (m *MockSecretStore) Set(ctx context.Context, appID, key string, encryptedValue []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.secrets[appID] == nil {
		m.secrets[appID] = make(map[string][]byte)
	}
	m.secrets[appID][key] = encryptedValue
	return nil
}

func (m *MockSecretStore) Get(ctx context.Context, appID, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if appSecrets, ok := m.secrets[appID]; ok {
		if val, ok := appSecrets[key]; ok {
			return val, nil
		}
	}
	return nil, errors.New("secret not found")
}

func (m *MockSecretStore) List(ctx context.Context, appID string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keys []string
	if appSecrets, ok := m.secrets[appID]; ok {
		for k := range appSecrets {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (m *MockSecretStore) Delete(ctx context.Context, appID, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if appSecrets, ok := m.secrets[appID]; ok {
		delete(appSecrets, key)
	}
	return nil
}

func (m *MockSecretStore) GetAll(ctx context.Context, appID string) (map[string][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if appSecrets, ok := m.secrets[appID]; ok {
		result := make(map[string][]byte)
		for k, v := range appSecrets {
			result[k] = v
		}
		return result, nil
	}
	return make(map[string][]byte), nil
}

// LifecycleMockLogStore is a mock implementation of LogStore for testing.
type LifecycleMockLogStore struct {
	mu   sync.Mutex
	logs map[string][]*models.LogEntry
}

// NewLifecycleMockLogStore creates a new LifecycleMockLogStore.
func NewLifecycleMockLogStore() *LifecycleMockLogStore {
	return &LifecycleMockLogStore{logs: make(map[string][]*models.LogEntry)}
}

func (m *LifecycleMockLogStore) Create(ctx context.Context, entry *models.LogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs[entry.DeploymentID] = append(m.logs[entry.DeploymentID], entry)
	return nil
}

func (m *LifecycleMockLogStore) List(ctx context.Context, deploymentID string, limit int) ([]*models.LogEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := m.logs[deploymentID]
	if limit > 0 && len(entries) > limit {
		return entries[:limit], nil
	}
	return entries, nil
}

func (m *LifecycleMockLogStore) ListBySource(ctx context.Context, deploymentID, source string, limit int) ([]*models.LogEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.LogEntry
	for _, entry := range m.logs[deploymentID] {
		if entry.Source == source {
			result = append(result, entry)
		}
	}
	if limit > 0 && len(result) > limit {
		return result[:limit], nil
	}
	return result, nil
}

func (m *LifecycleMockLogStore) DeleteOlderThan(ctx context.Context, deploymentID string, before int64) error {
	return nil
}

// MockUserStore is a mock implementation of UserStore for testing.
type MockUserStore struct {
	mu    sync.Mutex
	users map[string]*store.User
}

func NewMockUserStore() *MockUserStore {
	return &MockUserStore{users: make(map[string]*store.User)}
}

func (m *MockUserStore) Create(ctx context.Context, email, password string, isAdmin bool) (*store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	user := &store.User{
		ID:        email,
		Email:     email,
		IsAdmin:   isAdmin,
		CreatedAt: time.Now().Unix(),
	}
	m.users[email] = user
	return user, nil
}

func (m *MockUserStore) GetByEmail(ctx context.Context, email string) (*store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if user, ok := m.users[email]; ok {
		return user, nil
	}
	return nil, errors.New("user not found")
}

func (m *MockUserStore) Authenticate(ctx context.Context, email, password string) (*store.User, error) {
	return m.GetByEmail(ctx, email)
}

func (m *MockUserStore) List(ctx context.Context) ([]*store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*store.User
	for _, user := range m.users {
		result = append(result, user)
	}
	return result, nil
}

func (m *MockUserStore) GetByID(ctx context.Context, id string) (*store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if user, ok := m.users[id]; ok {
		return user, nil
	}
	for _, user := range m.users {
		if user.ID == id {
			return user, nil
		}
	}
	return nil, errors.New("user not found")
}

func (m *MockUserStore) Update(ctx context.Context, user *store.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[user.ID] = user
	return nil
}

func (m *MockUserStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.users, id)
	return nil
}

func (m *MockUserStore) CreateWithRole(ctx context.Context, email, password string, role store.Role, invitedBy string) (*store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	user := &store.User{
		ID:        email,
		Email:     email,
		Role:      role,
		InvitedBy: invitedBy,
		CreatedAt: time.Now().Unix(),
	}
	m.users[email] = user
	return user, nil
}

func (m *MockUserStore) CountByRole(ctx context.Context, role store.Role) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, user := range m.users {
		if user.Role == role {
			count++
		}
	}
	return count, nil
}

func (m *MockUserStore) GetFirstOwner(ctx context.Context) (*store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, user := range m.users {
		if user.Role == store.RoleOwner {
			return user, nil
		}
	}
	return nil, nil
}

// MockGitHubStore is a mock implementation of GitHubStore for testing.
type MockGitHubStore struct {
	mu            sync.Mutex
	config        *models.GitHubAppConfig
	installations map[int64]*models.GitHubInstallation
}

func NewMockGitHubStore() *MockGitHubStore {
	return &MockGitHubStore{
		installations: make(map[int64]*models.GitHubInstallation),
	}
}

func (m *MockGitHubStore) GetConfig(ctx context.Context) (*models.GitHubAppConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.config, nil
}

func (m *MockGitHubStore) SaveConfig(ctx context.Context, config *models.GitHubAppConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
	return nil
}

func (m *MockGitHubStore) CreateInstallation(ctx context.Context, inst *models.GitHubInstallation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.installations[inst.ID] = inst
	return nil
}

func (m *MockGitHubStore) GetInstallation(ctx context.Context, id int64) (*models.GitHubInstallation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inst, ok := m.installations[id]; ok {
		return inst, nil
	}
	return nil, errors.New("installation not found")
}

func (m *MockGitHubStore) ListInstallations(ctx context.Context, userID string) ([]*models.GitHubInstallation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.GitHubInstallation
	for _, inst := range m.installations {
		if inst.UserID == userID {
			result = append(result, inst)
		}
	}
	return result, nil
}

func (m *MockGitHubStore) DeleteInstallation(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.installations, id)
	return nil
}

func (m *MockGitHubStore) ResetConfig(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = nil
	m.installations = make(map[int64]*models.GitHubInstallation)
	return nil
}

// MockGitHubAccountStore is a mock implementation of GitHubAccountStore for testing.
type MockGitHubAccountStore struct {
	mu       sync.Mutex
	accounts map[int64]*models.GitHubAccount
}

func NewMockGitHubAccountStore() *MockGitHubAccountStore {
	return &MockGitHubAccountStore{
		accounts: make(map[int64]*models.GitHubAccount),
	}
}

func (m *MockGitHubAccountStore) Create(ctx context.Context, acc *models.GitHubAccount) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accounts[acc.ID] = acc
	return nil
}

func (m *MockGitHubAccountStore) Get(ctx context.Context, id int64) (*models.GitHubAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if acc, ok := m.accounts[id]; ok {
		return acc, nil
	}
	return nil, errors.New("account not found")
}

func (m *MockGitHubAccountStore) GetByUserID(ctx context.Context, userID string) (*models.GitHubAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, acc := range m.accounts {
		if acc.UserID == userID {
			return acc, nil
		}
	}
	return nil, errors.New("account not found")
}

func (m *MockGitHubAccountStore) Update(ctx context.Context, account *models.GitHubAccount) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accounts[account.ID] = account
	return nil
}

func (m *MockGitHubAccountStore) List(ctx context.Context, userID string) ([]*models.GitHubAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.GitHubAccount
	for _, acc := range m.accounts {
		if acc.UserID == userID {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (m *MockGitHubAccountStore) Delete(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.accounts, id)
	return nil
}

// MockProgressTracker is a mock implementation of BuildProgressTracker for testing.
// It also implements ProgressTrackerWithHistory for testing progress tracking verification.
type MockProgressTracker struct {
	mu              sync.Mutex
	StageReports    []StageReport
	ProgressReports []ProgressReport
}

// StageReport records a stage report.
type StageReport struct {
	BuildID   string
	Stage     BuildStage
	Timestamp time.Time
}

// ProgressReport records a progress report.
type ProgressReport struct {
	BuildID   string
	Percent   int
	Message   string
	Timestamp time.Time
}

// NewMockProgressTracker creates a new MockProgressTracker.
func NewMockProgressTracker() *MockProgressTracker {
	return &MockProgressTracker{
		StageReports:    make([]StageReport, 0),
		ProgressReports: make([]ProgressReport, 0),
	}
}

func (m *MockProgressTracker) ReportStage(ctx context.Context, buildID string, stage BuildStage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StageReports = append(m.StageReports, StageReport{
		BuildID:   buildID,
		Stage:     stage,
		Timestamp: time.Now(),
	})
	return nil
}

func (m *MockProgressTracker) ReportProgress(ctx context.Context, buildID string, percent int, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ProgressReports = append(m.ProgressReports, ProgressReport{
		BuildID:   buildID,
		Percent:   percent,
		Message:   message,
		Timestamp: time.Now(),
	})
	return nil
}

// GetStageReports returns all recorded stage reports.
func (m *MockProgressTracker) GetStageReports() []StageReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]StageReport{}, m.StageReports...)
}

// GetProgressReports returns all recorded progress reports.
func (m *MockProgressTracker) GetProgressReports() []ProgressReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ProgressReport{}, m.ProgressReports...)
}

// GetProgressHistory returns the progress history for a build (implements ProgressTrackerWithHistory).
func (m *MockProgressTracker) GetProgressHistory(buildID string) []ProgressRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []ProgressRecord
	for _, report := range m.ProgressReports {
		if report.BuildID == buildID {
			result = append(result, ProgressRecord{
				BuildID:   report.BuildID,
				Percent:   report.Percent,
				Message:   report.Message,
				Timestamp: report.Timestamp,
			})
		}
	}
	return result
}

// GetStageHistory returns the stage history for a build (implements ProgressTrackerWithHistory).
func (m *MockProgressTracker) GetStageHistory(buildID string) []StageRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []StageRecord
	for _, report := range m.StageReports {
		if report.BuildID == buildID {
			result = append(result, StageRecord{
				BuildID:   report.BuildID,
				Stage:     report.Stage,
				Timestamp: report.Timestamp,
			})
		}
	}
	return result
}

// IsProgressMonotonic checks if all progress reports for a build are monotonically increasing.
func (m *MockProgressTracker) IsProgressMonotonic(buildID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	var buildReports []ProgressReport
	for _, report := range m.ProgressReports {
		if report.BuildID == buildID {
			buildReports = append(buildReports, report)
		}
	}
	
	if len(buildReports) <= 1 {
		return true
	}
	
	for i := 1; i < len(buildReports); i++ {
		if buildReports[i].Percent < buildReports[i-1].Percent {
			return false
		}
	}
	return true
}

// HasTerminalStage checks if the build has reported a terminal stage (completed or failed).
func (m *MockProgressTracker) HasTerminalStage(buildID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for _, report := range m.StageReports {
		if report.BuildID == buildID {
			if report.Stage == StageCompleted || report.Stage == StageFailed {
				return true
			}
		}
	}
	return false
}

// GetLastStage returns the last reported stage for a build.
func (m *MockProgressTracker) GetLastStage(buildID string) (BuildStage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	var lastStage BuildStage
	found := false
	for _, report := range m.StageReports {
		if report.BuildID == buildID {
			lastStage = report.Stage
			found = true
		}
	}
	return lastStage, found
}

// Reset clears all recorded reports.
func (m *MockProgressTracker) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StageReports = nil
	m.ProgressReports = nil
}


// Test fixtures for build jobs with various configurations.

// NewTestBuildJob creates a valid build job for testing.
func NewTestBuildJob(id, deploymentID string, buildType models.BuildType, strategy models.BuildStrategy) *models.BuildJob {
	return &models.BuildJob{
		ID:            id,
		DeploymentID:  deploymentID,
		AppID:         "test-app-id",
		ServiceName:   "test-service",
		GitURL:        "https://github.com/example/repo.git",
		GitRef:        "main",
		FlakeOutput:   "packages.x86_64-linux.default",
		BuildType:     buildType,
		BuildStrategy: strategy,
		Status:        models.BuildStatusQueued,
		CreatedAt:     time.Now(),
	}
}

// NewTestDeployment creates a valid deployment for testing.
func NewTestDeployment(id, appID string) *models.Deployment {
	return &models.Deployment{
		ID:        id,
		AppID:     appID,
		Status:    models.DeploymentStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// BuildJobFixtures provides pre-configured build jobs for testing.
type BuildJobFixtures struct {
	// ValidPureNixFlake is a valid pure-nix build with flake strategy.
	ValidPureNixFlake *models.BuildJob
	// ValidOCIDockerfile is a valid OCI build with dockerfile strategy.
	ValidOCIDockerfile *models.BuildJob
	// ValidAutoGo is a valid auto-go build.
	ValidAutoGo *models.BuildJob
	// ValidAutoNode is a valid auto-node build.
	ValidAutoNode *models.BuildJob
	// ValidAutoRust is a valid auto-rust build.
	ValidAutoRust *models.BuildJob
	// ValidAutoPython is a valid auto-python build.
	ValidAutoPython *models.BuildJob
	// ValidNixpacks is a valid nixpacks build.
	ValidNixpacks *models.BuildJob
	// InvalidMissingID is a build job with missing ID.
	InvalidMissingID *models.BuildJob
	// InvalidMissingDeploymentID is a build job with missing deployment ID.
	InvalidMissingDeploymentID *models.BuildJob
	// InvalidBuildType is a build job with invalid build type.
	InvalidBuildType *models.BuildJob
	// InvalidStrategy is a build job with invalid strategy.
	InvalidStrategy *models.BuildJob
	// InvalidNegativeTimeout is a build job with negative timeout.
	InvalidNegativeTimeout *models.BuildJob
}

// NewBuildJobFixtures creates a set of build job fixtures for testing.
func NewBuildJobFixtures() *BuildJobFixtures {
	return &BuildJobFixtures{
		ValidPureNixFlake: &models.BuildJob{
			ID:            "valid-pure-nix-flake",
			DeploymentID:  "deployment-1",
			AppID:         "app-1",
			GitURL:        "https://github.com/example/repo.git",
			GitRef:        "main",
			BuildType:     models.BuildTypePureNix,
			BuildStrategy: models.BuildStrategyFlake,
			Status:        models.BuildStatusQueued,
			CreatedAt:     time.Now(),
		},
		ValidOCIDockerfile: &models.BuildJob{
			ID:            "valid-oci-dockerfile",
			DeploymentID:  "deployment-2",
			AppID:         "app-2",
			GitURL:        "https://github.com/example/repo.git",
			GitRef:        "main",
			BuildType:     models.BuildTypeOCI,
			BuildStrategy: models.BuildStrategyDockerfile,
			Status:        models.BuildStatusQueued,
			CreatedAt:     time.Now(),
		},
		ValidAutoGo: &models.BuildJob{
			ID:            "valid-auto-go",
			DeploymentID:  "deployment-3",
			AppID:         "app-3",
			GitURL:        "https://github.com/example/go-app.git",
			GitRef:        "main",
			BuildType:     models.BuildTypePureNix,
			BuildStrategy: models.BuildStrategyAutoGo,
			Status:        models.BuildStatusQueued,
			CreatedAt:     time.Now(),
			BuildConfig: &models.BuildConfig{
				GoVersion: "1.21",
			},
		},
		ValidAutoNode: &models.BuildJob{
			ID:            "valid-auto-node",
			DeploymentID:  "deployment-4",
			AppID:         "app-4",
			GitURL:        "https://github.com/example/node-app.git",
			GitRef:        "main",
			BuildType:     models.BuildTypePureNix,
			BuildStrategy: models.BuildStrategyAutoNode,
			Status:        models.BuildStatusQueued,
			CreatedAt:     time.Now(),
			BuildConfig: &models.BuildConfig{
				NodeVersion:    "20",
				PackageManager: "npm",
			},
		},
		ValidAutoRust: &models.BuildJob{
			ID:            "valid-auto-rust",
			DeploymentID:  "deployment-5",
			AppID:         "app-5",
			GitURL:        "https://github.com/example/rust-app.git",
			GitRef:        "main",
			BuildType:     models.BuildTypePureNix,
			BuildStrategy: models.BuildStrategyAutoRust,
			Status:        models.BuildStatusQueued,
			CreatedAt:     time.Now(),
			BuildConfig: &models.BuildConfig{
				RustEdition: "2021",
			},
		},
		ValidAutoPython: &models.BuildJob{
			ID:            "valid-auto-python",
			DeploymentID:  "deployment-6",
			AppID:         "app-6",
			GitURL:        "https://github.com/example/python-app.git",
			GitRef:        "main",
			BuildType:     models.BuildTypePureNix,
			BuildStrategy: models.BuildStrategyAutoPython,
			Status:        models.BuildStatusQueued,
			CreatedAt:     time.Now(),
			BuildConfig: &models.BuildConfig{
				PythonVersion: "3.11",
			},
		},
		ValidNixpacks: &models.BuildJob{
			ID:            "valid-nixpacks",
			DeploymentID:  "deployment-7",
			AppID:         "app-7",
			GitURL:        "https://github.com/example/generic-app.git",
			GitRef:        "main",
			BuildType:     models.BuildTypeOCI,
			BuildStrategy: models.BuildStrategyNixpacks,
			Status:        models.BuildStatusQueued,
			CreatedAt:     time.Now(),
		},
		InvalidMissingID: &models.BuildJob{
			ID:           "",
			DeploymentID: "deployment-invalid-1",
			BuildType:    models.BuildTypePureNix,
			Status:       models.BuildStatusQueued,
			CreatedAt:    time.Now(),
		},
		InvalidMissingDeploymentID: &models.BuildJob{
			ID:           "invalid-missing-deployment",
			DeploymentID: "",
			BuildType:    models.BuildTypePureNix,
			Status:       models.BuildStatusQueued,
			CreatedAt:    time.Now(),
		},
		InvalidBuildType: &models.BuildJob{
			ID:           "invalid-build-type",
			DeploymentID: "deployment-invalid-2",
			BuildType:    models.BuildType("invalid-type"),
			Status:       models.BuildStatusQueued,
			CreatedAt:    time.Now(),
		},
		InvalidStrategy: &models.BuildJob{
			ID:            "invalid-strategy",
			DeploymentID:  "deployment-invalid-3",
			BuildType:     models.BuildTypePureNix,
			BuildStrategy: models.BuildStrategy("invalid-strategy"),
			Status:        models.BuildStatusQueued,
			CreatedAt:     time.Now(),
		},
		InvalidNegativeTimeout: &models.BuildJob{
			ID:             "invalid-negative-timeout",
			DeploymentID:   "deployment-invalid-4",
			BuildType:      models.BuildTypePureNix,
			TimeoutSeconds: -100,
			Status:         models.BuildStatusQueued,
			CreatedAt:      time.Now(),
		},
	}
}

// ValidTransitions is an alias to models.ValidStatusTransitions for backward compatibility in tests.
var ValidTransitions = models.ValidStatusTransitions

// CanTransition delegates to models.CanTransition for state transition validation.
func CanTransition(from, to models.BuildStatus, isRetry bool) bool {
	return models.CanTransition(from, to, isRetry)
}

// IsTerminalState delegates to models.IsTerminalState for terminal state checking.
func IsTerminalState(status models.BuildStatus) bool {
	return models.IsTerminalState(status)
}

// MockSettingsStore is a mock implementation of SettingsStore for testing.
type MockSettingsStore struct {
	mu       sync.Mutex
	settings map[string]string
}

func NewMockSettingsStore() *MockSettingsStore {
	return &MockSettingsStore{settings: make(map[string]string)}
}

func (m *MockSettingsStore) Get(ctx context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings[key], nil
}

func (m *MockSettingsStore) Set(ctx context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[key] = value
	return nil
}

func (m *MockSettingsStore) GetAll(ctx context.Context) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make(map[string]string)
	for k, v := range m.settings {
		res[k] = v
	}
	return res, nil
}

// MockDomainStore is a mock implementation of DomainStore for testing.
type MockDomainStore struct {
	mu      sync.Mutex
	domains map[string]*models.Domain
}

func NewMockDomainStore() *MockDomainStore {
	return &MockDomainStore{domains: make(map[string]*models.Domain)}
}

func (m *MockDomainStore) Create(ctx context.Context, domain *models.Domain) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domains[domain.ID] = domain
	return nil
}

func (m *MockDomainStore) Get(ctx context.Context, id string) (*models.Domain, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if domain, ok := m.domains[id]; ok {
		return domain, nil
	}
	return nil, errors.New("domain not found")
}

func (m *MockDomainStore) List(ctx context.Context, appID string) ([]*models.Domain, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.Domain
	for _, domain := range m.domains {
		if domain.AppID == appID {
			result = append(result, domain)
		}
	}
	return result, nil
}

func (m *MockDomainStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.domains, id)
	return nil
}

func (m *MockDomainStore) GetByDomain(ctx context.Context, domainName string) (*models.Domain, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, domain := range m.domains {
		if domain.Domain == domainName {
			return domain, nil
		}
	}
	return nil, nil // Return nil if not found, consistent with store interface
}

