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
	logger       *slog.Logger
}

// NixBuilderConfig holds configuration for the Nix builder.
type NixBuilderConfig struct {
	WorkDir      string
	PodmanSocket string
	NixImage     string // Docker image with Nix installed
}

// DefaultNixBuilderConfig returns a NixBuilderConfig with sensible defaults.
func DefaultNixBuilderConfig() *NixBuilderConfig {
	return &NixBuilderConfig{
		WorkDir:      "/tmp/narvana-builds",
		PodmanSocket: "unix:///run/user/1000/podman/podman.sock",
		NixImage:     "docker.io/nixos/nix:latest",
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
			gitRef = "HEAD"
		}
		cloneScript = fmt.Sprintf(`
echo "=== Cloning repository ==="
git clone --depth 1 --branch %s %s /build/src 2>&1 || git clone --depth 1 %s /build/src 2>&1
cd /build/src
# Copy the generated flake.nix into the repo
cp /build/flake.nix /build/src/flake.nix
echo "Repository cloned and flake.nix added"
`, gitRef, job.GitURL, job.GitURL)
		// Update flakeRef to point to the cloned directory
		flakeRef = "/build/src"
		if job.FlakeOutput != "" {
			flakeRef = fmt.Sprintf("/build/src#%s", job.FlakeOutput)
		}
	}

	// Build script that will run inside the container
	buildScript := fmt.Sprintf(`
set -e
echo "=== Narvana Build Started ==="
echo "Flake: %s"
echo "Build Type: %s"
echo ""

# Enable flakes
export NIX_CONFIG="experimental-features = nix-command flakes"

%s

# Run the build with sandbox enabled
echo "=== Running nix build ==="
nix build '%s' --print-out-paths --no-link 2>&1

echo ""
echo "=== Build Complete ==="
`, flakeRef, job.BuildType, cloneScript, flakeRef)

	containerName := fmt.Sprintf("narvana-build-%s", job.ID)

	cfg := &podman.ContainerConfig{
		Name:    containerName,
		Image:   b.nixImage,
		Command: []string{"sh", "-c", buildScript},
		WorkDir: "/build",
		Mounts: []podman.Mount{
			{Source: buildDir, Target: "/build", ReadOnly: false},
		},
		Limits: &podman.ResourceLimits{
			CPUQuota: 2.0,    // Allow 2 CPUs for builds
			MemoryMB: 4096,   // 4GB for builds
			PidsLimit: 1000,
		},
		Remove:      true,
		NetworkMode: "host", // Allow network access for fetching dependencies
		Env: map[string]string{
			"NIX_CONFIG": "experimental-features = nix-command flakes",
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
func (b *NixBuilder) parseStorePath(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "/nix/store/") {
			return line
		}
	}
	return ""
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
