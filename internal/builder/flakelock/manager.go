// Package flakelock provides flake.lock file management for reproducible builds.
package flakelock

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// FlakeLockManager manages flake.lock files for reproducibility.
type FlakeLockManager interface {
	// Generate creates a new flake.lock for a flake.nix.
	Generate(ctx context.Context, flakePath string) (string, error)

	// Update updates an existing flake.lock.
	Update(ctx context.Context, flakePath string, existingLock string) (string, error)

	// Store persists a flake.lock for a build.
	Store(ctx context.Context, buildID string, lockContent string) error

	// Retrieve gets a stored flake.lock.
	Retrieve(ctx context.Context, buildID string) (string, error)

	// ShouldRegenerate determines if lock should be regenerated.
	ShouldRegenerate(ctx context.Context, buildID string, sourceHash string) bool
}

// Manager implements the FlakeLockManager interface.
type Manager struct {
	// Timeout is the maximum time to wait for lock generation.
	Timeout time.Duration

	// storage holds flake.lock content keyed by build ID.
	storage map[string]*StoredLock

	// mu protects the storage map.
	mu sync.RWMutex
}

// StoredLock represents a stored flake.lock with metadata.
type StoredLock struct {
	Content    string    `json:"content"`
	SourceHash string    `json:"source_hash"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// NewManager creates a new Manager with default settings.
func NewManager() *Manager {
	return &Manager{
		Timeout: 5 * time.Minute,
		storage: make(map[string]*StoredLock),
	}
}

// ManagerOption is a functional option for configuring Manager.
type ManagerOption func(*Manager)

// WithTimeout sets the timeout for lock generation.
func WithTimeout(timeout time.Duration) ManagerOption {
	return func(m *Manager) {
		m.Timeout = timeout
	}
}

// NewManagerWithOptions creates a new Manager with custom options.
func NewManagerWithOptions(opts ...ManagerOption) *Manager {
	m := NewManager()
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Generate creates a new flake.lock for a flake.nix at the given path.
// It runs `nix flake lock` to generate the lock file.
func (m *Manager) Generate(ctx context.Context, flakePath string) (string, error) {
	if flakePath == "" {
		return "", ErrEmptyFlakePath
	}

	// Verify flake.nix exists
	flakeNixPath := filepath.Join(flakePath, "flake.nix")
	if _, err := os.Stat(flakeNixPath); os.IsNotExist(err) {
		return "", ErrFlakeNotFound
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()

	// Run nix flake lock to generate the lock file
	cmd := exec.CommandContext(ctx, "nix", "flake", "lock", "--no-update-lock-file")
	cmd.Dir = flakePath

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", ErrLockGenerationTimeout
		}
		return "", fmt.Errorf("%w: %s", ErrLockGenerationFailed, stderr.String())
	}

	// Read the generated flake.lock
	lockPath := filepath.Join(flakePath, "flake.lock")
	lockContent, err := os.ReadFile(lockPath)
	if err != nil {
		return "", fmt.Errorf("reading flake.lock: %w", err)
	}

	// Validate the lock file is valid JSON
	if !isValidFlakeLock(string(lockContent)) {
		return "", ErrInvalidLockFormat
	}

	return string(lockContent), nil
}

// Update updates an existing flake.lock by running `nix flake update`.
// If existingLock is provided, it writes it first before updating.
func (m *Manager) Update(ctx context.Context, flakePath string, existingLock string) (string, error) {
	if flakePath == "" {
		return "", ErrEmptyFlakePath
	}

	// Verify flake.nix exists
	flakeNixPath := filepath.Join(flakePath, "flake.nix")
	if _, err := os.Stat(flakeNixPath); os.IsNotExist(err) {
		return "", ErrFlakeNotFound
	}

	// If existing lock is provided, write it first
	if existingLock != "" {
		lockPath := filepath.Join(flakePath, "flake.lock")
		if err := os.WriteFile(lockPath, []byte(existingLock), 0644); err != nil {
			return "", fmt.Errorf("writing existing flake.lock: %w", err)
		}
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()

	// Run nix flake update to update the lock file
	cmd := exec.CommandContext(ctx, "nix", "flake", "update")
	cmd.Dir = flakePath

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", ErrLockGenerationTimeout
		}
		return "", fmt.Errorf("%w: %s", ErrLockUpdateFailed, stderr.String())
	}

	// Read the updated flake.lock
	lockPath := filepath.Join(flakePath, "flake.lock")
	lockContent, err := os.ReadFile(lockPath)
	if err != nil {
		return "", fmt.Errorf("reading updated flake.lock: %w", err)
	}

	// Validate the lock file is valid JSON
	if !isValidFlakeLock(string(lockContent)) {
		return "", ErrInvalidLockFormat
	}

	return string(lockContent), nil
}

// Store persists a flake.lock for a build.
func (m *Manager) Store(ctx context.Context, buildID string, lockContent string) error {
	if buildID == "" {
		return ErrEmptyBuildID
	}

	if lockContent == "" {
		return ErrEmptyLockContent
	}

	// Validate the lock content is valid JSON
	if !isValidFlakeLock(lockContent) {
		return ErrInvalidLockFormat
	}

	// Calculate source hash from the lock content
	sourceHash := calculateHash(lockContent)

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if existing, ok := m.storage[buildID]; ok {
		// Update existing entry
		existing.Content = lockContent
		existing.SourceHash = sourceHash
		existing.UpdatedAt = now
	} else {
		// Create new entry
		m.storage[buildID] = &StoredLock{
			Content:    lockContent,
			SourceHash: sourceHash,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
	}

	return nil
}

// Retrieve gets a stored flake.lock by build ID.
func (m *Manager) Retrieve(ctx context.Context, buildID string) (string, error) {
	if buildID == "" {
		return "", ErrEmptyBuildID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	stored, ok := m.storage[buildID]
	if !ok {
		return "", ErrLockNotFound
	}

	return stored.Content, nil
}

// ShouldRegenerate determines if the lock should be regenerated.
// Returns true if:
// - No lock exists for the build ID
// - The source hash has changed (indicating source code changes)
func (m *Manager) ShouldRegenerate(ctx context.Context, buildID string, sourceHash string) bool {
	if buildID == "" {
		return true
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	stored, ok := m.storage[buildID]
	if !ok {
		// No existing lock, should generate
		return true
	}

	// If source hash is different, should regenerate
	if sourceHash != "" && stored.SourceHash != sourceHash {
		return true
	}

	return false
}

// GetStoredLock retrieves the full stored lock metadata.
func (m *Manager) GetStoredLock(ctx context.Context, buildID string) (*StoredLock, error) {
	if buildID == "" {
		return nil, ErrEmptyBuildID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	stored, ok := m.storage[buildID]
	if !ok {
		return nil, ErrLockNotFound
	}

	// Return a copy to prevent external modification
	return &StoredLock{
		Content:    stored.Content,
		SourceHash: stored.SourceHash,
		CreatedAt:  stored.CreatedAt,
		UpdatedAt:  stored.UpdatedAt,
	}, nil
}

// Delete removes a stored flake.lock.
func (m *Manager) Delete(ctx context.Context, buildID string) error {
	if buildID == "" {
		return ErrEmptyBuildID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.storage[buildID]; !ok {
		return ErrLockNotFound
	}

	delete(m.storage, buildID)
	return nil
}

// ListBuildIDs returns all build IDs with stored locks.
func (m *Manager) ListBuildIDs(ctx context.Context) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.storage))
	for id := range m.storage {
		ids = append(ids, id)
	}
	return ids
}

// isValidFlakeLock checks if the content is a valid flake.lock JSON.
func isValidFlakeLock(content string) bool {
	if content == "" {
		return false
	}

	var lock FlakeLockFile
	if err := json.Unmarshal([]byte(content), &lock); err != nil {
		return false
	}

	// A valid flake.lock must have a version and nodes
	return lock.Version > 0 && lock.Nodes != nil
}

// calculateHash calculates a SHA256 hash of the content.
func calculateHash(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// FlakeLockFile represents the structure of a flake.lock file.
type FlakeLockFile struct {
	Version int                    `json:"version"`
	Nodes   map[string]interface{} `json:"nodes"`
	Root    string                 `json:"root,omitempty"`
}

// ParseFlakeLock parses a flake.lock content into a FlakeLockFile.
func ParseFlakeLock(content string) (*FlakeLockFile, error) {
	if content == "" {
		return nil, ErrEmptyLockContent
	}

	var lock FlakeLockFile
	if err := json.Unmarshal([]byte(content), &lock); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLockFormat, err)
	}

	if lock.Version == 0 || lock.Nodes == nil {
		return nil, ErrInvalidLockFormat
	}

	return &lock, nil
}

// GetInputRevision extracts the revision for a specific input from the lock file.
func GetInputRevision(lockContent string, inputName string) (string, error) {
	lock, err := ParseFlakeLock(lockContent)
	if err != nil {
		return "", err
	}

	node, ok := lock.Nodes[inputName]
	if !ok {
		return "", fmt.Errorf("input %q not found in flake.lock", inputName)
	}

	nodeMap, ok := node.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid node format for input %q", inputName)
	}

	locked, ok := nodeMap["locked"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("no locked info for input %q", inputName)
	}

	rev, ok := locked["rev"].(string)
	if !ok {
		return "", fmt.Errorf("no revision for input %q", inputName)
	}

	return rev, nil
}

// CompareFlakeLocks compares two flake.lock contents and returns true if they are equivalent.
func CompareFlakeLocks(lock1, lock2 string) bool {
	if lock1 == lock2 {
		return true
	}

	// Parse both locks
	parsed1, err1 := ParseFlakeLock(lock1)
	parsed2, err2 := ParseFlakeLock(lock2)

	if err1 != nil || err2 != nil {
		return false
	}

	// Compare versions
	if parsed1.Version != parsed2.Version {
		return false
	}

	// Compare nodes by re-serializing to normalize
	nodes1, _ := json.Marshal(parsed1.Nodes)
	nodes2, _ := json.Marshal(parsed2.Nodes)

	return string(nodes1) == string(nodes2)
}
