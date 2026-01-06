package builder

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCloneRepository_ValidPublicRepo tests cloning a valid public repository.
// **Validates: Requirements 1.1, 4.1**
func TestCloneRepository_ValidPublicRepo(t *testing.T) {
	// Skip if running in CI without network access
	if os.Getenv("CI") == "true" && os.Getenv("SKIP_NETWORK_TESTS") == "true" {
		t.Skip("Skipping network test in CI")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create a temporary directory for the clone
	destPath := filepath.Join(t.TempDir(), "test-repo")

	// Clone a small public repository
	result, err := CloneRepository(ctx, "https://github.com/octocat/Hello-World.git", "master", destPath)
	if err != nil {
		t.Fatalf("CloneRepository failed: %v", err)
	}

	// Verify the result
	if result.RepoPath != destPath {
		t.Errorf("RepoPath = %q, want %q", result.RepoPath, destPath)
	}

	if result.CommitSHA == "" {
		t.Error("CommitSHA should not be empty")
	}

	// Verify the commit SHA is a valid 40-character hex string
	if len(result.CommitSHA) != 40 {
		t.Errorf("CommitSHA length = %d, want 40", len(result.CommitSHA))
	}

	// Verify the repository was cloned
	if _, err := os.Stat(filepath.Join(destPath, ".git")); os.IsNotExist(err) {
		t.Error(".git directory should exist in cloned repo")
	}
}

// TestCloneRepository_InvalidURL tests cloning with an invalid URL.
// **Validates: Requirements 1.1**
func TestCloneRepository_InvalidURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	destPath := filepath.Join(t.TempDir(), "test-repo")

	// Try to clone an invalid URL
	_, err := CloneRepository(ctx, "https://invalid-url-that-does-not-exist.example.com/repo.git", "", destPath)
	if err == nil {
		t.Fatal("CloneRepository should fail with invalid URL")
	}

	// Verify it's a CloneError
	cloneErr, ok := AsCloneError(err)
	if !ok {
		t.Fatalf("Expected CloneError, got %T", err)
	}

	// Verify error contains useful information
	if cloneErr.GitURL == "" {
		t.Error("CloneError.GitURL should not be empty")
	}

	if cloneErr.ExitCode == 0 {
		t.Error("CloneError.ExitCode should be non-zero for failed clone")
	}
}

// TestCloneRepository_NonExistentRef tests cloning with a non-existent ref.
// **Validates: Requirements 1.1**
func TestCloneRepository_NonExistentRef(t *testing.T) {
	// Skip if running in CI without network access
	if os.Getenv("CI") == "true" && os.Getenv("SKIP_NETWORK_TESTS") == "true" {
		t.Skip("Skipping network test in CI")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	destPath := filepath.Join(t.TempDir(), "test-repo")

	// Try to clone with a non-existent branch
	_, err := CloneRepository(ctx, "https://github.com/octocat/Hello-World.git", "non-existent-branch-xyz123", destPath)
	if err == nil {
		t.Fatal("CloneRepository should fail with non-existent ref")
	}

	// Verify it's a CloneError
	cloneErr, ok := AsCloneError(err)
	if !ok {
		t.Fatalf("Expected CloneError, got %T", err)
	}

	// Verify error contains the ref information
	if cloneErr.GitRef != "non-existent-branch-xyz123" {
		t.Errorf("CloneError.GitRef = %q, want %q", cloneErr.GitRef, "non-existent-branch-xyz123")
	}
}

// TestCloneRepository_ShallowClone tests that shallow clone is used.
// **Validates: Requirements 4.1**
func TestCloneRepository_ShallowClone(t *testing.T) {
	// Skip if running in CI without network access
	if os.Getenv("CI") == "true" && os.Getenv("SKIP_NETWORK_TESTS") == "true" {
		t.Skip("Skipping network test in CI")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	destPath := filepath.Join(t.TempDir(), "test-repo")

	// Clone a repository
	_, err := CloneRepository(ctx, "https://github.com/octocat/Hello-World.git", "master", destPath)
	if err != nil {
		t.Fatalf("CloneRepository failed: %v", err)
	}

	// Check for shallow clone indicator
	// A shallow clone has a .git/shallow file
	shallowFile := filepath.Join(destPath, ".git", "shallow")
	if _, err := os.Stat(shallowFile); os.IsNotExist(err) {
		t.Error("Expected shallow clone (.git/shallow should exist)")
	}
}

// TestCloneRepository_ContextCancellation tests that clone respects context cancellation.
// **Validates: Requirements 1.1**
func TestCloneRepository_ContextCancellation(t *testing.T) {
	// Create an already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	destPath := filepath.Join(t.TempDir(), "test-repo")

	// Try to clone with cancelled context
	_, err := CloneRepository(ctx, "https://github.com/octocat/Hello-World.git", "master", destPath)
	if err == nil {
		t.Fatal("CloneRepository should fail with cancelled context")
	}
}

// TestCloneError_Error tests the CloneError.Error() method.
func TestCloneError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CloneError
		contains string
	}{
		{
			name: "with stderr",
			err: &CloneError{
				GitURL:   "https://example.com/repo.git",
				GitRef:   "main",
				Stderr:   "fatal: repository not found",
				ExitCode: 128,
			},
			contains: "fatal: repository not found",
		},
		{
			name: "with underlying error",
			err: &CloneError{
				GitURL:   "https://example.com/repo.git",
				GitRef:   "main",
				Err:      context.DeadlineExceeded,
				ExitCode: 1,
			},
			contains: "context deadline exceeded",
		},
		{
			name: "exit code only",
			err: &CloneError{
				GitURL:   "https://example.com/repo.git",
				ExitCode: 128,
			},
			contains: "exit code 128",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.err.Error()
			if errMsg == "" {
				t.Error("Error() should not return empty string")
			}
			if tt.contains != "" && !containsString(errMsg, tt.contains) {
				t.Errorf("Error() = %q, should contain %q", errMsg, tt.contains)
			}
		})
	}
}

// TestIsCloneError tests the IsCloneError helper function.
func TestIsCloneError(t *testing.T) {
	cloneErr := &CloneError{GitURL: "test", ExitCode: 1}

	if !IsCloneError(cloneErr) {
		t.Error("IsCloneError should return true for CloneError")
	}

	regularErr := context.DeadlineExceeded
	if IsCloneError(regularErr) {
		t.Error("IsCloneError should return false for non-CloneError")
	}
}

// TestAsCloneError tests the AsCloneError helper function.
func TestAsCloneError(t *testing.T) {
	cloneErr := &CloneError{GitURL: "test", ExitCode: 1}

	result, ok := AsCloneError(cloneErr)
	if !ok {
		t.Error("AsCloneError should return true for CloneError")
	}
	if result != cloneErr {
		t.Error("AsCloneError should return the same error")
	}

	regularErr := context.DeadlineExceeded
	_, ok = AsCloneError(regularErr)
	if ok {
		t.Error("AsCloneError should return false for non-CloneError")
	}
}

// containsString checks if s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
