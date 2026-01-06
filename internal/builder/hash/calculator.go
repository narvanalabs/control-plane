// Package hash provides vendor hash calculation for reproducible builds.
package hash

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// HashCalculator calculates vendor hashes for reproducible builds.
type HashCalculator interface {
	// CalculateGoVendorHash calculates the vendor hash for a Go module.
	CalculateGoVendorHash(ctx context.Context, repoPath string) (string, error)

	// CalculateNpmHash calculates the hash for npm dependencies.
	CalculateNpmHash(ctx context.Context, repoPath string) (string, error)

	// CalculateCargoHash calculates the hash for Cargo dependencies.
	CalculateCargoHash(ctx context.Context, repoPath string) (string, error)
}

// Calculator implements the HashCalculator interface.
type Calculator struct {
	// FakeHash is the placeholder hash used for initial builds.
	// Nix will fail with the correct hash in the error message.
	FakeHash string

	// MaxRetries is the maximum number of retry attempts for hash extraction.
	MaxRetries int

	// Timeout is the maximum time to wait for hash calculation.
	Timeout time.Duration
}

// NewCalculator creates a new Calculator with default settings.
func NewCalculator() *Calculator {
	return &Calculator{
		FakeHash:   "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		MaxRetries: 3,
		Timeout:    10 * time.Minute,
	}
}

// CalculatorOption is a functional option for configuring Calculator.
type CalculatorOption func(*Calculator)

// WithFakeHash sets a custom fake hash for initial builds.
func WithFakeHash(hash string) CalculatorOption {
	return func(c *Calculator) {
		c.FakeHash = hash
	}
}

// WithMaxRetries sets the maximum number of retry attempts.
func WithMaxRetries(retries int) CalculatorOption {
	return func(c *Calculator) {
		c.MaxRetries = retries
	}
}

// WithTimeout sets the timeout for hash calculation.
func WithTimeout(timeout time.Duration) CalculatorOption {
	return func(c *Calculator) {
		c.Timeout = timeout
	}
}

// NewCalculatorWithOptions creates a new Calculator with custom options.
func NewCalculatorWithOptions(opts ...CalculatorOption) *Calculator {
	c := NewCalculator()
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// CalculateGoVendorHash calculates the vendor hash for a Go module.
// It uses the fake-hash retry mechanism: first attempts with a fake hash,
// then extracts the real hash from the Nix error output.
func (c *Calculator) CalculateGoVendorHash(ctx context.Context, repoPath string) (string, error) {
	// Check if go.mod exists
	goModPath := filepath.Join(repoPath, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		return "", fmt.Errorf("go.mod not found in %s", repoPath)
	}

	// Check if go.sum exists (required for hash calculation)
	goSumPath := filepath.Join(repoPath, "go.sum")
	if _, err := os.Stat(goSumPath); os.IsNotExist(err) {
		return "", fmt.Errorf("go.sum not found in %s", repoPath)
	}

	// Try to calculate hash using nix-prefetch-url or similar
	// First, try the fake hash approach
	hash, err := c.calculateGoHashWithFakeRetry(ctx, repoPath)
	if err != nil {
		return "", fmt.Errorf("calculating Go vendor hash: %w", err)
	}

	return hash, nil
}

// calculateGoHashWithFakeRetry implements the fake-hash retry mechanism.
// It creates a minimal flake with a fake hash, runs nix build, and extracts
// the correct hash from the error output.
func (c *Calculator) calculateGoHashWithFakeRetry(ctx context.Context, repoPath string) (string, error) {
	// Create a temporary directory for the hash calculation
	tmpDir, err := os.MkdirTemp("", "narvana-hash-*")
	if err != nil {
		return "", fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a minimal flake.nix for hash calculation
	flakeContent := c.generateGoHashFlake(repoPath)
	flakePath := filepath.Join(tmpDir, "flake.nix")
	if err := os.WriteFile(flakePath, []byte(flakeContent), 0644); err != nil {
		return "", fmt.Errorf("writing flake.nix: %w", err)
	}

	// Run nix build with the fake hash - it will fail with the correct hash
	var attempt int
	for attempt = 0; attempt < c.MaxRetries; attempt++ {
		hash, err := c.tryExtractHashFromNixError(ctx, tmpDir)
		if err == nil && hash != "" {
			return hash, nil
		}
		// If we got an error that's not about hash mismatch, return it
		if err != nil && !isHashMismatchError(err) {
			return "", err
		}
	}

	return "", fmt.Errorf("failed to extract vendor hash after %d attempts", c.MaxRetries)
}

// generateGoHashFlake generates a minimal flake.nix for Go vendor hash calculation.
func (c *Calculator) generateGoHashFlake(repoPath string) string {
	return fmt.Sprintf(`{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }: let
    system = "x86_64-linux";
    pkgs = nixpkgs.legacyPackages.${system};
  in {
    packages.${system}.default = pkgs.buildGoModule {
      pname = "hash-check";
      version = "0.0.1";
      src = %s;
      vendorHash = "%s";
    };
  };
}`, repoPath, c.FakeHash)
}

// tryExtractHashFromNixError runs nix build and extracts the hash from error output.
func (c *Calculator) tryExtractHashFromNixError(ctx context.Context, flakeDir string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "nix", "build", "--no-link", "-L")
	cmd.Dir = flakeDir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// We expect this to fail with a hash mismatch
	err := cmd.Run()
	if err == nil {
		// Build succeeded with fake hash - this shouldn't happen normally
		// but could mean there are no dependencies
		return c.FakeHash, nil
	}

	// Extract the correct hash from the error output
	hash := extractHashFromNixOutput(stderr.String())
	if hash != "" {
		return hash, nil
	}

	return "", fmt.Errorf("nix build failed: %s", stderr.String())
}

// extractHashFromNixOutput extracts the SRI hash from Nix error output.
// Nix outputs something like: "got: sha256-ABC123..."
func extractHashFromNixOutput(output string) string {
	// Pattern for SRI hash in Nix error output
	// Matches: got: sha256-<base64>
	patterns := []string{
		`got:\s+(sha256-[A-Za-z0-9+/]+=*)`,
		`specified:\s+sha256-[A-Za-z0-9+/]+=*\s+got:\s+(sha256-[A-Za-z0-9+/]+=*)`,
		`hash mismatch.*got:\s+(sha256-[A-Za-z0-9+/]+=*)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(output)
		if len(matches) > 1 {
			return matches[1]
		}
	}

	return ""
}

// isHashMismatchError checks if the error is a hash mismatch error.
func isHashMismatchError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "hash mismatch") ||
		strings.Contains(errStr, "got:") ||
		strings.Contains(errStr, "specified:")
}

// CalculateNpmHash calculates the hash for npm dependencies from package-lock.json.
// This uses the SRI hash format compatible with Nix.
func (c *Calculator) CalculateNpmHash(ctx context.Context, repoPath string) (string, error) {
	// Check for package-lock.json
	lockPath := filepath.Join(repoPath, "package-lock.json")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		// Try yarn.lock
		yarnLockPath := filepath.Join(repoPath, "yarn.lock")
		if _, err := os.Stat(yarnLockPath); os.IsNotExist(err) {
			// Try pnpm-lock.yaml
			pnpmLockPath := filepath.Join(repoPath, "pnpm-lock.yaml")
			if _, err := os.Stat(pnpmLockPath); os.IsNotExist(err) {
				return "", fmt.Errorf("no lock file found (package-lock.json, yarn.lock, or pnpm-lock.yaml)")
			}
			lockPath = pnpmLockPath
		} else {
			lockPath = yarnLockPath
		}
	}

	// Calculate SHA256 hash of the lock file
	hash, err := c.calculateFileHash(lockPath)
	if err != nil {
		return "", fmt.Errorf("calculating npm hash: %w", err)
	}

	return hash, nil
}

// CalculateCargoHash calculates the hash for Cargo dependencies from Cargo.lock.
func (c *Calculator) CalculateCargoHash(ctx context.Context, repoPath string) (string, error) {
	// Check for Cargo.lock
	lockPath := filepath.Join(repoPath, "Cargo.lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		return "", fmt.Errorf("Cargo.lock not found in %s", repoPath)
	}

	// Calculate SHA256 hash of the lock file
	hash, err := c.calculateFileHash(lockPath)
	if err != nil {
		return "", fmt.Errorf("calculating Cargo hash: %w", err)
	}

	return hash, nil
}

// calculateFileHash calculates the SHA256 hash of a file in SRI format.
func (c *Calculator) calculateFileHash(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	// Format as SRI hash: sha256-<base64>
	hashBytes := h.Sum(nil)
	sriHash := fmt.Sprintf("sha256-%s", base64.StdEncoding.EncodeToString(hashBytes))

	return sriHash, nil
}

// HashResult contains the result of a hash calculation.
type HashResult struct {
	Hash       string `json:"hash"`
	Algorithm  string `json:"algorithm"`
	SourceFile string `json:"source_file,omitempty"`
	Retries    int    `json:"retries,omitempty"`
}

// CalculateGoVendorHashWithResult calculates the Go vendor hash and returns detailed result.
func (c *Calculator) CalculateGoVendorHashWithResult(ctx context.Context, repoPath string) (*HashResult, error) {
	hash, err := c.CalculateGoVendorHash(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	return &HashResult{
		Hash:       hash,
		Algorithm:  "sha256",
		SourceFile: "go.sum",
	}, nil
}

// CalculateNpmHashWithResult calculates the npm hash and returns detailed result.
func (c *Calculator) CalculateNpmHashWithResult(ctx context.Context, repoPath string) (*HashResult, error) {
	hash, err := c.CalculateNpmHash(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	// Determine which lock file was used
	sourceFile := "package-lock.json"
	if _, err := os.Stat(filepath.Join(repoPath, "package-lock.json")); os.IsNotExist(err) {
		if _, err := os.Stat(filepath.Join(repoPath, "yarn.lock")); err == nil {
			sourceFile = "yarn.lock"
		} else {
			sourceFile = "pnpm-lock.yaml"
		}
	}

	return &HashResult{
		Hash:       hash,
		Algorithm:  "sha256",
		SourceFile: sourceFile,
	}, nil
}

// CalculateCargoHashWithResult calculates the Cargo hash and returns detailed result.
func (c *Calculator) CalculateCargoHashWithResult(ctx context.Context, repoPath string) (*HashResult, error) {
	hash, err := c.CalculateCargoHash(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	return &HashResult{
		Hash:       hash,
		Algorithm:  "sha256",
		SourceFile: "Cargo.lock",
	}, nil
}

// IsValidSRIHash checks if a string is a valid SRI hash.
func IsValidSRIHash(hash string) bool {
	// SRI hash format: algorithm-base64
	if !strings.HasPrefix(hash, "sha256-") {
		return false
	}

	base64Part := strings.TrimPrefix(hash, "sha256-")
	if len(base64Part) == 0 {
		return false
	}

	// Check if it's valid base64
	_, err := base64.StdEncoding.DecodeString(base64Part)
	return err == nil
}

// IsFakeHash checks if the hash is the placeholder fake hash.
func (c *Calculator) IsFakeHash(hash string) bool {
	return hash == c.FakeHash
}
