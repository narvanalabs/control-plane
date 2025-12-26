// Package podman provides a client wrapper for interacting with Podman.
package podman

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

// ResourceLimits defines resource constraints for a container.
type ResourceLimits struct {
	CPUQuota   float64 // CPU quota in cores (e.g., 0.5 = half a core)
	MemoryMB   int64   // Memory limit in megabytes
	PidsLimit  int64   // Maximum number of PIDs
}

// ContainerConfig holds configuration for creating a container.
type ContainerConfig struct {
	Name         string
	Image        string
	Command      []string
	WorkDir      string
	Env          map[string]string
	Mounts       []Mount
	Limits       *ResourceLimits
	User         string
	NetworkMode  string
	Remove       bool   // Remove container after exit
	Privileged   bool   // Run container in privileged mode
	UserNS       string // User namespace mode (e.g., "keep-id", "host")
}

// Mount defines a bind mount for a container.
type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// ContainerResult holds the result of a container execution.
type ContainerResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// Client provides methods for interacting with Podman.
type Client struct {
	socketPath string
	logger     *slog.Logger
}

// NewClient creates a new Podman client.
func NewClient(socketPath string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		socketPath: socketPath,
		logger:     logger,
	}
}


// ResourceLimitsForTier returns resource limits for a given resource tier.
func ResourceLimitsForTier(tier models.ResourceTier) *ResourceLimits {
	switch tier {
	case models.ResourceTierNano:
		return &ResourceLimits{CPUQuota: 0.25, MemoryMB: 256, PidsLimit: 100}
	case models.ResourceTierSmall:
		return &ResourceLimits{CPUQuota: 0.5, MemoryMB: 512, PidsLimit: 200}
	case models.ResourceTierMedium:
		return &ResourceLimits{CPUQuota: 1.0, MemoryMB: 1024, PidsLimit: 500}
	case models.ResourceTierLarge:
		return &ResourceLimits{CPUQuota: 2.0, MemoryMB: 2048, PidsLimit: 1000}
	case models.ResourceTierXLarge:
		return &ResourceLimits{CPUQuota: 4.0, MemoryMB: 4096, PidsLimit: 2000}
	default:
		return &ResourceLimits{CPUQuota: 0.5, MemoryMB: 512, PidsLimit: 200}
	}
}

// Run creates and runs a container, waiting for it to complete.
// Returns the container result including exit code and output.
func (c *Client) Run(ctx context.Context, cfg *ContainerConfig) (*ContainerResult, error) {
	start := time.Now()

	args := c.buildRunArgs(cfg)
	c.logger.Debug("running podman container",
		"name", cfg.Name,
		"image", cfg.Image,
		"args", args,
	)

	cmd := exec.CommandContext(ctx, "podman", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	result := &ContainerResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("running container: %w", err)
		}
	}

	c.logger.Debug("container completed",
		"name", cfg.Name,
		"exit_code", result.ExitCode,
		"duration", duration,
	)

	return result, nil
}

// RunWithStreaming creates and runs a container, streaming output to the provided writers.
func (c *Client) RunWithStreaming(ctx context.Context, cfg *ContainerConfig, stdout, stderr io.Writer) (*ContainerResult, error) {
	start := time.Now()

	args := c.buildRunArgs(cfg)
	c.logger.Info("running podman container with streaming",
		"name", cfg.Name,
		"image", cfg.Image,
		"privileged", cfg.Privileged,
		"args", args,
	)

	cmd := exec.CommandContext(ctx, "podman", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	duration := time.Since(start)

	result := &ContainerResult{
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("running container: %w", err)
		}
	}

	return result, nil
}

// buildRunArgs constructs the podman run command arguments.
func (c *Client) buildRunArgs(cfg *ContainerConfig) []string {
	args := []string{"run"}

	// Container name
	if cfg.Name != "" {
		args = append(args, "--name", cfg.Name)
	}

	// Remove after exit
	if cfg.Remove {
		args = append(args, "--rm")
	}

	// Working directory
	if cfg.WorkDir != "" {
		args = append(args, "--workdir", cfg.WorkDir)
	}

	// User
	if cfg.User != "" {
		args = append(args, "--user", cfg.User)
	}

	// Network mode
	if cfg.NetworkMode != "" {
		args = append(args, "--network", cfg.NetworkMode)
	}

	// Privileged mode
	if cfg.Privileged {
		args = append(args, "--privileged")
	}

	// User namespace mode
	if cfg.UserNS != "" {
		args = append(args, "--userns", cfg.UserNS)
	}

	// Environment variables
	for k, v := range cfg.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Mounts
	for _, m := range cfg.Mounts {
		mountOpt := fmt.Sprintf("%s:%s", m.Source, m.Target)
		if m.ReadOnly {
			mountOpt += ":ro"
		}
		args = append(args, "-v", mountOpt)
	}

	// Resource limits
	if cfg.Limits != nil {
		if cfg.Limits.CPUQuota > 0 {
			// Convert CPU quota to period/quota format
			// Period is 100000 microseconds (100ms), quota is proportional
			period := 100000
			quota := int(cfg.Limits.CPUQuota * float64(period))
			args = append(args, "--cpu-period", fmt.Sprintf("%d", period))
			args = append(args, "--cpu-quota", fmt.Sprintf("%d", quota))
		}
		if cfg.Limits.MemoryMB > 0 {
			args = append(args, "--memory", fmt.Sprintf("%dm", cfg.Limits.MemoryMB))
		}
		if cfg.Limits.PidsLimit > 0 {
			args = append(args, "--pids-limit", fmt.Sprintf("%d", cfg.Limits.PidsLimit))
		}
	}

	// Image
	args = append(args, cfg.Image)

	// Command
	args = append(args, cfg.Command...)

	return args
}

// Pull pulls an image from a registry.
func (c *Client) Pull(ctx context.Context, image string) error {
	c.logger.Debug("pulling image", "image", image)

	cmd := exec.CommandContext(ctx, "podman", "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pulling image %s: %w\nOutput: %s", image, err, string(output))
	}

	return nil
}

// Push pushes an image to a registry.
func (c *Client) Push(ctx context.Context, image string) error {
	c.logger.Debug("pushing image", "image", image)

	cmd := exec.CommandContext(ctx, "podman", "push", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pushing image %s: %w\nOutput: %s", image, err, string(output))
	}

	return nil
}

// Tag tags an image with a new name.
func (c *Client) Tag(ctx context.Context, source, target string) error {
	c.logger.Debug("tagging image", "source", source, "target", target)

	cmd := exec.CommandContext(ctx, "podman", "tag", source, target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tagging image: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// Load loads an image from a tar archive.
func (c *Client) Load(ctx context.Context, archivePath string) (string, error) {
	c.logger.Debug("loading image", "archive", archivePath)

	cmd := exec.CommandContext(ctx, "podman", "load", "-i", archivePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("loading image: %w\nOutput: %s", err, string(output))
	}

	// Parse the loaded image name from output
	// Output format: "Loaded image: <image_name>"
	outputStr := string(output)
	if idx := strings.Index(outputStr, "Loaded image:"); idx != -1 {
		imageName := strings.TrimSpace(outputStr[idx+len("Loaded image:"):])
		imageName = strings.TrimSuffix(imageName, "\n")
		return imageName, nil
	}

	return "", fmt.Errorf("could not parse loaded image name from output: %s", outputStr)
}

// ImageExists checks if an image exists locally.
func (c *Client) ImageExists(ctx context.Context, image string) (bool, error) {
	cmd := exec.CommandContext(ctx, "podman", "image", "exists", image)
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("checking image existence: %w", err)
	}
	return true, nil
}

// RemoveImage removes an image.
func (c *Client) RemoveImage(ctx context.Context, image string) error {
	c.logger.Debug("removing image", "image", image)

	cmd := exec.CommandContext(ctx, "podman", "rmi", "-f", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("removing image: %w\nOutput: %s", err, string(output))
	}

	return nil
}
