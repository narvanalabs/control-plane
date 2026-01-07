// Package clone provides git repository cloning utilities for the build system.
package clone

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Error represents a detailed error from a git clone operation.
type Error struct {
	// GitURL is the URL that was being cloned
	GitURL string

	// GitRef is the ref that was being checked out
	GitRef string

	// Stderr contains the git stderr output
	Stderr string

	// ExitCode is the exit code from git
	ExitCode int

	// Err is the underlying error
	Err error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("git clone failed (exit %d): %s", e.ExitCode, strings.TrimSpace(e.Stderr))
	}
	if e.Err != nil {
		return fmt.Sprintf("git clone failed: %v", e.Err)
	}
	return fmt.Sprintf("git clone failed with exit code %d", e.ExitCode)
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.Err
}

// Result contains the result of a successful clone operation.
type Result struct {
	// RepoPath is the path to the cloned repository
	RepoPath string

	// CommitSHA is the resolved commit SHA after checkout
	CommitSHA string
}

// Repository clones a git repository to the specified destination path.
// It uses shallow clone (--depth 1) for efficiency as specified in Requirements 4.1.
//
// Parameters:
//   - ctx: Context for cancellation
//   - gitURL: The git repository URL to clone
//   - gitRef: The git ref to checkout (branch, tag, or commit). If empty, uses default branch.
//   - destPath: The destination path for the cloned repository
//
// Returns:
//   - *Result: Contains the repo path and resolved commit SHA
//   - error: A *Error with detailed information on failure
//
// **Validates: Requirements 1.1, 4.1**
func Repository(ctx context.Context, gitURL, gitRef, destPath string) (*Result, error) {
	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return nil, &Error{
			GitURL: gitURL,
			GitRef: gitRef,
			Err:    fmt.Errorf("failed to create destination directory: %w", err),
		}
	}

	// Build the git clone command with shallow clone for efficiency
	// **Validates: Requirements 4.1** - Use shallow clone (depth=1)
	args := []string{"clone", "--depth", "1"}

	// Add branch/ref if specified
	if gitRef != "" {
		args = append(args, "--branch", gitRef)
	}

	args = append(args, gitURL, destPath)

	// Execute git clone
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}

		// If clone with branch failed, try without branch (for commit SHAs)
		if gitRef != "" && exitCode != 0 {
			// Try cloning without branch specification, then checkout
			return cloneAndCheckout(ctx, gitURL, gitRef, destPath)
		}

		return nil, &Error{
			GitURL:   gitURL,
			GitRef:   gitRef,
			Stderr:   stderr.String(),
			ExitCode: exitCode,
			Err:      err,
		}
	}

	// Get the resolved commit SHA
	commitSHA, err := getCommitSHA(ctx, destPath)
	if err != nil {
		return nil, &Error{
			GitURL: gitURL,
			GitRef: gitRef,
			Err:    fmt.Errorf("failed to get commit SHA: %w", err),
		}
	}

	return &Result{
		RepoPath:  destPath,
		CommitSHA: commitSHA,
	}, nil
}

// cloneAndCheckout handles the case where gitRef is a commit SHA rather than a branch/tag.
// It clones without a specific branch, then fetches and checks out the specific commit.
func cloneAndCheckout(ctx context.Context, gitURL, gitRef, destPath string) (*Result, error) {
	// Remove any partial clone from previous attempt
	os.RemoveAll(destPath)

	// Clone without branch specification (shallow clone of default branch)
	var stdout, stderr bytes.Buffer
	cloneCmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", gitURL, destPath)
	cloneCmd.Stdout = &stdout
	cloneCmd.Stderr = &stderr

	if err := cloneCmd.Run(); err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return nil, &Error{
			GitURL:   gitURL,
			GitRef:   gitRef,
			Stderr:   stderr.String(),
			ExitCode: exitCode,
			Err:      err,
		}
	}

	// Fetch the specific ref with depth
	stderr.Reset()
	fetchCmd := exec.CommandContext(ctx, "git", "-C", destPath, "fetch", "--depth", "1", "origin", gitRef)
	fetchCmd.Stderr = &stderr

	if err := fetchCmd.Run(); err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return nil, &Error{
			GitURL:   gitURL,
			GitRef:   gitRef,
			Stderr:   stderr.String(),
			ExitCode: exitCode,
			Err:      fmt.Errorf("failed to fetch ref %s: %w", gitRef, err),
		}
	}

	// Checkout the fetched ref
	stderr.Reset()
	checkoutCmd := exec.CommandContext(ctx, "git", "-C", destPath, "checkout", "FETCH_HEAD")
	checkoutCmd.Stderr = &stderr

	if err := checkoutCmd.Run(); err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return nil, &Error{
			GitURL:   gitURL,
			GitRef:   gitRef,
			Stderr:   stderr.String(),
			ExitCode: exitCode,
			Err:      fmt.Errorf("failed to checkout ref %s: %w", gitRef, err),
		}
	}

	// Get the resolved commit SHA
	commitSHA, err := getCommitSHA(ctx, destPath)
	if err != nil {
		return nil, &Error{
			GitURL: gitURL,
			GitRef: gitRef,
			Err:    fmt.Errorf("failed to get commit SHA: %w", err),
		}
	}

	return &Result{
		RepoPath:  destPath,
		CommitSHA: commitSHA,
	}, nil
}

// getCommitSHA returns the current HEAD commit SHA for the repository at the given path.
func getCommitSHA(ctx context.Context, repoPath string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "HEAD")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse failed: %s", stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// IsError checks if an error is a clone Error.
func IsError(err error) bool {
	_, ok := err.(*Error)
	return ok
}

// AsError attempts to convert an error to a clone Error.
func AsError(err error) (*Error, bool) {
	cloneErr, ok := err.(*Error)
	return cloneErr, ok
}
