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
	CPUQuota  float64 // CPU quota in cores (e.g., 0.5 = half a core)
	MemoryMB  int64   // Memory limit in megabytes
	PidsLimit int64   // Maximum number of PIDs
}

// ContainerConfig holds configuration for creating a container.
type ContainerConfig struct {
	Name        string
	Image       string
	Entrypoint  []string // Override container entrypoint
	Command     []string
	WorkDir     string
	Env         map[string]string
	Mounts      []Mount
	Limits      *ResourceLimits
	User        string
	NetworkMode string
	Remove      bool   // Remove container after exit
	Privileged  bool   // Run container in privileged mode
	UserNS      string // User namespace mode (e.g., "keep-id", "host")
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

// ResourceLimitsFromSpec returns resource limits from a ResourceSpec.
// If spec is nil, returns default limits (0.5 CPU, 512MB).
func ResourceLimitsFromSpec(spec *models.ResourceSpec) *ResourceLimits {
	if spec == nil {
		return &ResourceLimits{CPUQuota: 0.5, MemoryMB: 512, PidsLimit: 200}
	}

	limits := &ResourceLimits{PidsLimit: 200}

	// Parse CPU (e.g., "0.5", "1", "2")
	if spec.CPU != "" {
		var cpu float64
		fmt.Sscanf(spec.CPU, "%f", &cpu)
		if cpu > 0 {
			limits.CPUQuota = cpu
		} else {
			limits.CPUQuota = 0.5
		}
	} else {
		limits.CPUQuota = 0.5
	}

	// Parse memory (e.g., "256Mi", "1Gi", "512")
	if spec.Memory != "" {
		limits.MemoryMB = parseMemoryToMB(spec.Memory)
	} else {
		limits.MemoryMB = 512
	}

	// Scale PIDs limit based on resources
	if limits.CPUQuota >= 4 {
		limits.PidsLimit = 2000
	} else if limits.CPUQuota >= 2 {
		limits.PidsLimit = 1000
	} else if limits.CPUQuota >= 1 {
		limits.PidsLimit = 500
	}

	return limits
}

// parseMemoryToMB parses a memory string to megabytes.
func parseMemoryToMB(mem string) int64 {
	mem = strings.TrimSpace(mem)
	if mem == "" {
		return 512
	}

	// Handle Gi suffix
	if strings.HasSuffix(mem, "Gi") {
		var val float64
		fmt.Sscanf(mem, "%fGi", &val)
		return int64(val * 1024)
	}

	// Handle Mi suffix
	if strings.HasSuffix(mem, "Mi") {
		var val int64
		fmt.Sscanf(mem, "%dMi", &val)
		return val
	}

	// Handle G suffix
	if strings.HasSuffix(mem, "G") {
		var val float64
		fmt.Sscanf(mem, "%fG", &val)
		return int64(val * 1024)
	}

	// Handle M suffix
	if strings.HasSuffix(mem, "M") {
		var val int64
		fmt.Sscanf(mem, "%dM", &val)
		return val
	}

	// Try parsing as plain number (assume MB)
	var val int64
	fmt.Sscanf(mem, "%d", &val)
	if val > 0 {
		return val
	}

	return 512 // default
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

	// Entrypoint override
	if len(cfg.Entrypoint) > 0 {
		args = append(args, "--entrypoint", cfg.Entrypoint[0])
	}

	// Image
	args = append(args, cfg.Image)

	// If entrypoint has additional args, add them before command
	if len(cfg.Entrypoint) > 1 {
		args = append(args, cfg.Entrypoint[1:]...)
	}

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

// Exec prepares a podman exec command.
func (c *Client) Exec(containerName string, command []string) *exec.Cmd {
	args := []string{"exec", "-it"}
	args = append(args, containerName)
	args = append(args, command...)
	return exec.Command("podman", args...)
}

// ContainerInfo holds information about a container.
type ContainerInfo struct {
	ID        string
	Name      string
	Image     string
	Status    string
	CreatedAt time.Time
	StoppedAt time.Time
}

// ImageInfo holds information about an image.
type ImageInfo struct {
	ID        string
	Tags      []string
	Size      int64
	CreatedAt time.Time
}

// ListStoppedContainers returns all stopped containers.
func (c *Client) ListStoppedContainers(ctx context.Context) ([]ContainerInfo, error) {
	c.logger.Debug("listing stopped containers")

	// Use JSON format for reliable parsing
	cmd := exec.CommandContext(ctx, "podman", "ps", "-a", "--filter", "status=exited", "--format", "{{.ID}}|{{.Names}}|{{.Image}}|{{.Status}}|{{.CreatedAt}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing stopped containers: %w", err)
	}

	var containers []ContainerInfo
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}

		createdAt, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", parts[4])

		containers = append(containers, ContainerInfo{
			ID:        parts[0],
			Name:      parts[1],
			Image:     parts[2],
			Status:    parts[3],
			CreatedAt: createdAt,
			// StoppedAt is approximated from status or we use CreatedAt as fallback
			StoppedAt: createdAt,
		})
	}

	return containers, nil
}

// RemoveContainer removes a container by ID or name.
func (c *Client) RemoveContainer(ctx context.Context, idOrName string) error {
	c.logger.Debug("removing container", "id", idOrName)

	cmd := exec.CommandContext(ctx, "podman", "rm", "-f", idOrName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("removing container: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// ListImages returns all images.
func (c *Client) ListImages(ctx context.Context) ([]ImageInfo, error) {
	c.logger.Debug("listing images")

	cmd := exec.CommandContext(ctx, "podman", "images", "--format", "{{.ID}}|{{.Repository}}:{{.Tag}}|{{.Size}}|{{.CreatedAt}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}

	var images []ImageInfo
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		createdAt, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", parts[3])

		images = append(images, ImageInfo{
			ID:        parts[0],
			Tags:      []string{parts[1]},
			CreatedAt: createdAt,
		})
	}

	return images, nil
}

// ListDanglingImages returns images that are not tagged.
func (c *Client) ListDanglingImages(ctx context.Context) ([]ImageInfo, error) {
	c.logger.Debug("listing dangling images")

	cmd := exec.CommandContext(ctx, "podman", "images", "--filter", "dangling=true", "--format", "{{.ID}}|{{.Repository}}:{{.Tag}}|{{.Size}}|{{.CreatedAt}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing dangling images: %w", err)
	}

	var images []ImageInfo
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		createdAt, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", parts[3])

		images = append(images, ImageInfo{
			ID:        parts[0],
			Tags:      []string{parts[1]},
			CreatedAt: createdAt,
		})
	}

	return images, nil
}

// PruneContainers removes all stopped containers.
func (c *Client) PruneContainers(ctx context.Context) (int, error) {
	c.logger.Debug("pruning containers")

	cmd := exec.CommandContext(ctx, "podman", "container", "prune", "-f")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("pruning containers: %w\nOutput: %s", err, string(output))
	}

	// Count removed containers from output
	lines := strings.Split(string(output), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" && !strings.Contains(line, "Total reclaimed space") {
			count++
		}
	}

	return count, nil
}

// PruneImages removes unused images.
func (c *Client) PruneImages(ctx context.Context) (int, error) {
	c.logger.Debug("pruning images")

	cmd := exec.CommandContext(ctx, "podman", "image", "prune", "-f")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("pruning images: %w\nOutput: %s", err, string(output))
	}

	// Count removed images from output
	lines := strings.Split(string(output), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" && !strings.Contains(line, "Total reclaimed space") {
			count++
		}
	}

	return count, nil
}
