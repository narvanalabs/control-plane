// Package builder provides build execution for Nix-based applications.
package builder

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/podman"
)

// NixBuilder executes Nix builds inside Podman containers.
type NixBuilder struct {
	podmanClient *podman.Client
	workDir      string
	nixImage     string
	atticURL     string // Attic binary cache URL
	atticCache   string // Attic cache name
	atticToken   string // Attic JWT token
	logger       *slog.Logger
}

// NixBuilderConfig holds configuration for the Nix builder.
type NixBuilderConfig struct {
	WorkDir      string
	PodmanSocket string
	NixImage     string // Docker image with Nix installed
	AtticURL     string // Attic binary cache URL (e.g., "http://localhost:5000")
	AtticCache   string // Attic cache name (e.g., "narvana")
	AtticToken   string // Attic JWT token for authentication
}

// DefaultNixBuilderConfig returns a NixBuilderConfig with sensible defaults.
func DefaultNixBuilderConfig() *NixBuilderConfig {
	return &NixBuilderConfig{
		WorkDir:      "/tmp/narvana-builds",
		PodmanSocket: "unix:///run/user/1000/podman/podman.sock",
		NixImage:     "docker.io/nixos/nix:latest",
		AtticURL:     "http://localhost:5000",
		AtticCache:   "narvana",
		// Default dev token - generated with HS256 secret from attic-dev.toml
		// Permissions: full access to all caches (r, w, cc, cd)
		// Expiry: 2030
		AtticToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJuYXJ2YW5hLWJ1aWxkZXIiLCJpYXQiOjE3MDQwNjcyMDAsImV4cCI6MTg5MzQ1NjAwMCwiaHR0cHM6Ly9qd3QuYXR0aWMucnMvdjEiOnsiY2FjaGVzIjp7IioiOnsiciI6MSwidyI6MSwiY2MiOjEsImNkIjoxfX19fQ.ChlWrCl0KDrQsH4ZVQIfB3qD0EAfQHClvB-45MAn3js",
	}
}

// NewNixBuilder creates a new NixBuilder instance.
func NewNixBuilder(cfg *NixBuilderConfig, logger *slog.Logger) (*NixBuilder, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Ensure work directory exists
	if err := os.MkdirAll(cfg.WorkDir, 0755); err != nil {
		return nil, fmt.Errorf("creating work directory: %w", err)
	}

	podmanClient := podman.NewClient(cfg.PodmanSocket, logger)

	return &NixBuilder{
		podmanClient: podmanClient,
		workDir:      cfg.WorkDir,
		nixImage:     cfg.NixImage,
		atticURL:     cfg.AtticURL,
		atticCache:   cfg.AtticCache,
		atticToken:   cfg.AtticToken,
		logger:       logger,
	}, nil
}


// NixBuildResult holds the result of a Nix build.
type NixBuildResult struct {
	StorePath string        // The Nix store path of the built derivation
	Logs      string        // Build logs
	Duration  time.Duration // Build duration
	ExitCode  int           // Exit code from the build
}

// Build executes a Nix build for the given job inside a Podman container.
// It clones the repository, runs nix build, and returns the store path.
func (b *NixBuilder) Build(ctx context.Context, job *models.BuildJob) (*NixBuildResult, error) {
	b.logger.Info("starting nix build",
		"job_id", job.ID,
		"git_url", job.GitURL,
		"git_ref", job.GitRef,
		"flake_output", job.FlakeOutput,
		"has_generated_flake", job.GeneratedFlake != "",
	)

	// Create a unique build directory
	buildDir := filepath.Join(b.workDir, job.ID)
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return nil, fmt.Errorf("creating build directory: %w", err)
	}
	defer os.RemoveAll(buildDir) // Clean up after build

	var flakeRef string

	// Check if we have a generated flake - if so, write it to the build directory
	// and build from there instead of the git URL
	if job.GeneratedFlake != "" {
		flakePath := filepath.Join(buildDir, "flake.nix")
		if err := os.WriteFile(flakePath, []byte(job.GeneratedFlake), 0644); err != nil {
			return nil, fmt.Errorf("writing generated flake: %w", err)
		}
		b.logger.Info("wrote generated flake to build directory",
			"job_id", job.ID,
			"flake_path", flakePath,
		)
		// Build from the local directory
		flakeRef = "/build"
		if job.FlakeOutput != "" {
			flakeRef = fmt.Sprintf("/build#%s", job.FlakeOutput)
		}
	} else {
		// Build from the git URL
		flakeRef = job.GitURL
		if job.GitRef != "" {
			flakeRef = fmt.Sprintf("%s?ref=%s", job.GitURL, job.GitRef)
		}
		if job.FlakeOutput != "" {
			flakeRef = fmt.Sprintf("%s#%s", flakeRef, job.FlakeOutput)
		}
	}

	// Create a log buffer to capture output
	var logBuffer bytes.Buffer
	logWriter := io.MultiWriter(&logBuffer, os.Stdout)

	// Execute the build in a container
	result, err := b.buildInContainer(ctx, job, buildDir, flakeRef, logWriter)
	if err != nil {
		return &NixBuildResult{
			Logs:     logBuffer.String(),
			ExitCode: -1,
		}, fmt.Errorf("build failed: %w", err)
	}

	result.Logs = logBuffer.String()
	return result, nil
}

// buildInContainer executes the nix build inside a Podman container.
func (b *NixBuilder) buildInContainer(ctx context.Context, job *models.BuildJob, buildDir, flakeRef string, logWriter io.Writer) (*NixBuildResult, error) {
	start := time.Now()

	// Determine if we need to clone the repo (for generated flakes, the source needs to be present)
	needsClone := job.GeneratedFlake != "" && job.GitURL != ""

	var cloneScript string
	if needsClone {
		// Clone the repo into /build/src, then copy the generated flake
		gitRef := job.GitRef
		if gitRef == "" {
			gitRef = "main"
		}
		// Strategy: Clone to temp dir, copy files (excluding vendor and .git) to clean dir,
		// init fresh git repo. This ensures nix flakes only sees what we want.
		cloneScript = fmt.Sprintf(`
echo "=== Cloning repository ==="
git clone --depth 1 --branch %s %s /build/repo 2>&1 || git clone --depth 1 %s /build/repo 2>&1

echo "=== Creating clean source directory ==="
mkdir -p /build/src

# Copy all files except vendor and .git to clean directory
cd /build/repo
for item in *; do
  if [ "$item" != "vendor" ]; then
    cp -r "$item" /build/src/
  fi
done
# Copy hidden files except .git
for item in .[!.]*; do
  if [ "$item" != ".git" ] && [ -e "$item" ]; then
    cp -r "$item" /build/src/ 2>/dev/null || true
  fi
done

# Initialize fresh git repo in clean directory
cd /build/src
git init
git config user.email "narvana@localhost"
git config user.name "Narvana Builder"

# Copy the generated flake.nix
cp /build/flake.nix /build/src/flake.nix

# Add all files and commit
git add -A
git commit -m "narvana: clean source for build"

echo "Clean source directory created without vendor/"
`, gitRef, job.GitURL, job.GitURL)
		// Update flakeRef to point to the cloned directory
		flakeRef = "/build/src"
		if job.FlakeOutput != "" {
			flakeRef = fmt.Sprintf("/build/src#%s", job.FlakeOutput)
		}
	}

	// Build script that will run inside the container
	// Note: We use single-user nix mode (build-users-group =) to work with rootless podman
	// With vendorHash = null, we don't need two-phase hash calculation
	buildScript := fmt.Sprintf(`
set -e

# Remove /homeless-shelter if it exists - Nix purity check fails if this directory exists
# This is created by Nix's sandbox but causes issues with --no-sandbox builds
rm -rf /homeless-shelter 2>/dev/null || true

# Ensure HOME is set to a real directory
export HOME=/root
mkdir -p /root

echo "=== Narvana Build Started ==="
echo "Flake: %s"
echo "Build Type: %s"
echo ""

%s

cd /build/src

# Run the actual build
echo "=== Running nix build ==="
echo "Building from: $(pwd)"
# Remove /homeless-shelter before nix build - it gets recreated by nix
rm -rf /homeless-shelter 2>/dev/null || true
# Build with impure mode and sandbox disabled
# vendorHash = null allows Go to download dependencies without hash verification
BUILD_OUTPUT=$(nix build '.#default' --print-out-paths --no-link --impure --option sandbox false --option filter-syscalls false 2>&1)
echo "$BUILD_OUTPUT"

# Extract the store path (last line that starts with /nix/store/)
STORE_PATH=$(echo "$BUILD_OUTPUT" | grep "^/nix/store/" | tail -1)

if [ -z "$STORE_PATH" ]; then
  echo "ERROR: Could not extract store path from build output"
  exit 1
fi

echo ""
echo "=== Build Output: $STORE_PATH ==="
echo ""

# Push to Attic binary cache
echo "=== Pushing to Attic cache ==="
echo "Attic URL: %s"
echo "Cache: %s"

# Install attic-client
nix profile install nixpkgs#attic-client

# Login to Attic with token
attic login narvana %s --set-default %s

# Create cache if it doesn't exist (ignore error if already exists)
attic cache create %s 2>/dev/null || true

# Push the closure with all dependencies
attic push %s "$STORE_PATH"

echo ""
echo "=== Pushed to Attic successfully ==="
echo ""

# Print the store path as the final line for parsing
echo "$STORE_PATH"

echo ""
echo "=== Build Complete ==="
`, flakeRef, job.BuildType, cloneScript, b.atticURL, b.atticCache, b.atticURL, b.atticToken, b.atticCache, b.atticCache)

	containerName := fmt.Sprintf("narvana-build-%s", job.ID)

	cfg := &podman.ContainerConfig{
		Name:       containerName,
		Image:      b.nixImage,
		Entrypoint: []string{"/root/.nix-profile/bin/bash", "-c"},
		Command:    []string{buildScript},
		WorkDir:    "/build",
		User:       "root",  // Run as root
		Privileged: true,    // Privileged mode for nix builds
		Mounts: []podman.Mount{
			{Source: buildDir, Target: "/build", ReadOnly: false},
			// No need to mount host's nix store - builds happen in container,
			// then push to Attic binary cache for distribution to nodes.
		},
		Limits: &podman.ResourceLimits{
			CPUQuota:  2.0,  // Allow 2 CPUs for builds
			MemoryMB:  4096, // 4GB for builds
			PidsLimit: 1000,
		},
		Remove:      true,
		NetworkMode: "host", // Allow network access for fetching dependencies and pushing to Attic
		Env: map[string]string{
			// Nix configuration for rootless podman container builds
			// - sandbox = false: disable sandboxing
			// - build-users-group = (empty): disable multi-user mode (fixes "changing ownership" error)
			// - filter-syscalls = false: disable syscall filtering
			"NIX_CONFIG": "experimental-features = nix-command flakes\nsandbox = false\nbuild-users-group =\nfilter-syscalls = false",
			// Set HOME to /root (matching the container's default)
			"HOME": "/root",
		},
	}

	// Create a buffer to capture stdout for parsing the store path
	var stdout bytes.Buffer
	multiWriter := io.MultiWriter(&stdout, logWriter)

	containerResult, err := b.podmanClient.RunWithStreaming(ctx, cfg, multiWriter, logWriter)
	if err != nil {
		return nil, fmt.Errorf("running build container: %w", err)
	}

	duration := time.Since(start)

	result := &NixBuildResult{
		Duration: duration,
		ExitCode: containerResult.ExitCode,
	}

	if containerResult.ExitCode != 0 {
		return result, fmt.Errorf("build failed with exit code %d", containerResult.ExitCode)
	}

	// Parse the store path from the output
	storePath := b.parseStorePath(stdout.String())
	if storePath == "" {
		return result, fmt.Errorf("could not parse store path from build output")
	}

	result.StorePath = storePath
	b.logger.Info("nix build completed",
		"job_id", job.ID,
		"store_path", storePath,
		"duration", duration,
	)

	return result, nil
}

// parseStorePath extracts the Nix store path from build output.
// The store path is printed by --print-out-paths and looks like:
// /nix/store/abc123-name
// Note: We skip .drv files as those are derivations, not build outputs.
// The actual build output is a line that ONLY contains the store path,
// not embedded in messages like "copying path '...' from".
func (b *NixBuilder) parseStorePath(output string) string {
	lines := strings.Split(output, "\n")
	var lastStorePath string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Only match lines that are purely a store path (the --print-out-paths output)
		// Skip lines that contain the path embedded in messages
		if strings.HasPrefix(line, "/nix/store/") && !strings.HasSuffix(line, ".drv") && !strings.Contains(line, " ") {
			lastStorePath = line
		}
	}
	return lastStorePath
}

// BuildWithLogCallback executes a build and calls the callback for each log line.
// This is useful for streaming logs to a database in real-time.
func (b *NixBuilder) BuildWithLogCallback(ctx context.Context, job *models.BuildJob, callback func(line string)) (*NixBuildResult, error) {
	b.logger.Info("starting nix build with log callback",
		"job_id", job.ID,
		"has_generated_flake", job.GeneratedFlake != "",
	)

	// Create a unique build directory
	buildDir := filepath.Join(b.workDir, job.ID)
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return nil, fmt.Errorf("creating build directory: %w", err)
	}
	defer os.RemoveAll(buildDir)

	var flakeRef string

	// Check if we have a generated flake - if so, write it to the build directory
	// and build from there instead of the git URL
	if job.GeneratedFlake != "" {
		callback("=== Using generated flake.nix ===")
		flakePath := filepath.Join(buildDir, "flake.nix")
		if err := os.WriteFile(flakePath, []byte(job.GeneratedFlake), 0644); err != nil {
			return nil, fmt.Errorf("writing generated flake: %w", err)
		}
		b.logger.Info("wrote generated flake to build directory",
			"job_id", job.ID,
			"flake_path", flakePath,
		)
		// Build from the local directory
		flakeRef = "/build"
		if job.FlakeOutput != "" {
			flakeRef = fmt.Sprintf("/build#%s", job.FlakeOutput)
		}
	} else {
		// Build from the git URL
		flakeRef = job.GitURL
		if job.GitRef != "" {
			flakeRef = fmt.Sprintf("%s?ref=%s", job.GitURL, job.GitRef)
		}
		if job.FlakeOutput != "" {
			flakeRef = fmt.Sprintf("%s#%s", flakeRef, job.FlakeOutput)
		}
	}

	// Create a writer that calls the callback for each line
	var logBuffer bytes.Buffer
	lineWriter := &lineCallbackWriter{
		callback: callback,
		buffer:   &logBuffer,
	}

	result, err := b.buildInContainer(ctx, job, buildDir, flakeRef, lineWriter)
	if err != nil {
		if result != nil {
			result.Logs = logBuffer.String()
		}
		return result, err
	}

	result.Logs = logBuffer.String()
	return result, nil
}

// lineCallbackWriter is an io.Writer that calls a callback for each complete line.
type lineCallbackWriter struct {
	callback func(line string)
	buffer   *bytes.Buffer
	partial  bytes.Buffer
}

func (w *lineCallbackWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	w.buffer.Write(p)

	w.partial.Write(p)
	for {
		line, err := w.partial.ReadString('\n')
		if err != nil {
			// No complete line yet, put it back
			w.partial.WriteString(line)
			break
		}
		w.callback(strings.TrimSuffix(line, "\n"))
	}

	return n, nil
}

// IsValidStorePath checks if a string is a valid Nix store path.
func IsValidStorePath(path string) bool {
	if !strings.HasPrefix(path, "/nix/store/") {
		return false
	}
	// Store paths have format: /nix/store/<hash>-<name>
	// The hash is 32 characters of base32
	remainder := strings.TrimPrefix(path, "/nix/store/")
	if len(remainder) < 33 { // 32 chars hash + 1 char dash minimum
		return false
	}
	// Check for the dash separator after the hash
	if remainder[32] != '-' {
		return false
	}
	return true
}
