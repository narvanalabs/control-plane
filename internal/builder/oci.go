package builder

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/podman"
)

// OCIBuilder builds OCI container images using Nix and pushes them to a registry.
type OCIBuilder struct {
	nixBuilder   *NixBuilder
	podmanClient *podman.Client
	registry     string
	registryAuth string
	logger       *slog.Logger
}

// OCIBuilderConfig holds configuration for the OCI builder.
type OCIBuilderConfig struct {
	NixBuilderConfig *NixBuilderConfig
	Registry         string // Registry URL (e.g., "localhost:5000" or "registry.example.com")
	RegistryAuth     string // Base64 encoded auth string for registry
	PodmanSocket     string
}

// DefaultOCIBuilderConfig returns an OCIBuilderConfig with sensible defaults.
func DefaultOCIBuilderConfig() *OCIBuilderConfig {
	return &OCIBuilderConfig{
		NixBuilderConfig: DefaultNixBuilderConfig(),
		Registry:         "localhost:5000",
		PodmanSocket:     "unix:///run/user/1000/podman/podman.sock",
	}
}

// NewOCIBuilder creates a new OCIBuilder instance.
func NewOCIBuilder(cfg *OCIBuilderConfig, logger *slog.Logger) (*OCIBuilder, error) {
	if logger == nil {
		logger = slog.Default()
	}

	nixBuilder, err := NewNixBuilder(cfg.NixBuilderConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("creating nix builder: %w", err)
	}

	podmanClient := podman.NewClient(cfg.PodmanSocket, logger)

	return &OCIBuilder{
		nixBuilder:   nixBuilder,
		podmanClient: podmanClient,
		registry:     cfg.Registry,
		registryAuth: cfg.RegistryAuth,
		logger:       logger,
	}, nil
}

// OCIBuildResult holds the result of an OCI build.
type OCIBuildResult struct {
	ImageTag  string        // Full image tag (registry/repo:tag)
	StorePath string        // Nix store path of the image archive
	Logs      string        // Build logs
	Duration  time.Duration // Total build duration
	ExitCode  int           // Exit code from the build
}

// Build executes a Nix build that produces an OCI image and pushes it to the registry.
// The Nix flake should output a Docker image archive (via dockerTools.buildImage or similar).
func (b *OCIBuilder) Build(ctx context.Context, job *models.BuildJob) (*OCIBuildResult, error) {
	b.logger.Info("starting OCI build",
		"job_id", job.ID,
		"app_id", job.AppID,
		"git_url", job.GitURL,
	)

	start := time.Now()

	// First, run the Nix build to get the image archive
	nixResult, err := b.nixBuilder.Build(ctx, job)
	if err != nil {
		return &OCIBuildResult{
			Logs:     nixResult.Logs,
			ExitCode: nixResult.ExitCode,
			Duration: time.Since(start),
		}, fmt.Errorf("nix build failed: %w", err)
	}

	// The store path should point to a Docker image archive
	imagePath := nixResult.StorePath
	b.logger.Debug("nix build produced image archive", "path", imagePath)

	// Load the image into Podman
	loadedImage, err := b.podmanClient.Load(ctx, imagePath)
	if err != nil {
		return &OCIBuildResult{
			StorePath: imagePath,
			Logs:      nixResult.Logs,
			ExitCode:  0,
			Duration:  time.Since(start),
		}, fmt.Errorf("loading image: %w", err)
	}

	// Generate the target image tag
	imageTag := b.generateImageTag(job)

	// Tag the image with the registry path
	if err := b.podmanClient.Tag(ctx, loadedImage, imageTag); err != nil {
		return &OCIBuildResult{
			StorePath: imagePath,
			Logs:      nixResult.Logs,
			ExitCode:  0,
			Duration:  time.Since(start),
		}, fmt.Errorf("tagging image: %w", err)
	}

	// Push the image to the registry
	if err := b.pushImage(ctx, imageTag); err != nil {
		return &OCIBuildResult{
			StorePath: imagePath,
			ImageTag:  imageTag,
			Logs:      nixResult.Logs,
			ExitCode:  0,
			Duration:  time.Since(start),
		}, fmt.Errorf("pushing image: %w", err)
	}

	duration := time.Since(start)
	b.logger.Info("OCI build completed",
		"job_id", job.ID,
		"image_tag", imageTag,
		"duration", duration,
	)

	return &OCIBuildResult{
		ImageTag:  imageTag,
		StorePath: imagePath,
		Logs:      nixResult.Logs,
		Duration:  duration,
		ExitCode:  0,
	}, nil
}

// generateImageTag creates a unique image tag for the build.
// Format: registry/app-id:deployment-id
func (b *OCIBuilder) generateImageTag(job *models.BuildJob) string {
	// Sanitize the app ID for use in image name
	appName := sanitizeImageName(job.AppID)
	tag := job.DeploymentID
	if tag == "" {
		tag = job.ID
	}

	return fmt.Sprintf("%s/%s:%s", b.registry, appName, tag)
}

// pushImage pushes an image to the registry.
func (b *OCIBuilder) pushImage(ctx context.Context, imageTag string) error {
	b.logger.Debug("pushing image to registry", "image", imageTag)

	if err := b.podmanClient.Push(ctx, imageTag); err != nil {
		return fmt.Errorf("pushing image %s: %w", imageTag, err)
	}

	return nil
}

// BuildWithLogCallback executes an OCI build and calls the callback for each log line.
func (b *OCIBuilder) BuildWithLogCallback(ctx context.Context, job *models.BuildJob, callback func(line string)) (*OCIBuildResult, error) {
	b.logger.Info("starting OCI build with log callback",
		"job_id", job.ID,
	)

	start := time.Now()

	// Run the Nix build with log callback
	nixResult, err := b.nixBuilder.BuildWithLogCallback(ctx, job, callback)
	if err != nil {
		return &OCIBuildResult{
			Logs:     nixResult.Logs,
			ExitCode: nixResult.ExitCode,
			Duration: time.Since(start),
		}, fmt.Errorf("nix build failed: %w", err)
	}

	callback("=== Loading image into Podman ===")

	// Load the image
	loadedImage, err := b.podmanClient.Load(ctx, nixResult.StorePath)
	if err != nil {
		return &OCIBuildResult{
			StorePath: nixResult.StorePath,
			Logs:      nixResult.Logs,
			ExitCode:  0,
			Duration:  time.Since(start),
		}, fmt.Errorf("loading image: %w", err)
	}

	callback(fmt.Sprintf("Loaded image: %s", loadedImage))

	// Generate and apply tag
	imageTag := b.generateImageTag(job)
	callback(fmt.Sprintf("Tagging as: %s", imageTag))

	if err := b.podmanClient.Tag(ctx, loadedImage, imageTag); err != nil {
		return &OCIBuildResult{
			StorePath: nixResult.StorePath,
			Logs:      nixResult.Logs,
			ExitCode:  0,
			Duration:  time.Since(start),
		}, fmt.Errorf("tagging image: %w", err)
	}

	callback("=== Pushing image to registry ===")

	if err := b.pushImage(ctx, imageTag); err != nil {
		return &OCIBuildResult{
			StorePath: nixResult.StorePath,
			ImageTag:  imageTag,
			Logs:      nixResult.Logs,
			ExitCode:  0,
			Duration:  time.Since(start),
		}, fmt.Errorf("pushing image: %w", err)
	}

	callback(fmt.Sprintf("Successfully pushed: %s", imageTag))

	return &OCIBuildResult{
		ImageTag:  imageTag,
		StorePath: nixResult.StorePath,
		Logs:      nixResult.Logs,
		Duration:  time.Since(start),
		ExitCode:  0,
	}, nil
}

// sanitizeImageName converts a string to a valid Docker image name.
// Docker image names must be lowercase and can only contain [a-z0-9._-/].
func sanitizeImageName(name string) string {
	name = strings.ToLower(name)
	// Replace invalid characters with dashes
	reg := regexp.MustCompile(`[^a-z0-9._/-]`)
	name = reg.ReplaceAllString(name, "-")
	// Remove leading/trailing dashes
	name = strings.Trim(name, "-")
	return name
}

// IsValidOCIImageTag checks if a string is a valid OCI image reference.
// Valid formats: registry/repo:tag or registry/repo@digest
func IsValidOCIImageTag(tag string) bool {
	if tag == "" {
		return false
	}

	// Basic validation: must contain at least one slash (registry/repo)
	// and either a colon (for tag) or @ (for digest)
	if !strings.Contains(tag, "/") {
		return false
	}

	// Check for tag or digest
	hasTag := strings.Contains(tag, ":")
	hasDigest := strings.Contains(tag, "@")

	if !hasTag && !hasDigest {
		return false
	}

	// Validate the format more strictly
	// Pattern: [registry/]repo[:tag][@digest]
	parts := strings.Split(tag, "/")
	if len(parts) < 2 {
		return false
	}

	// The last part should contain the repo and tag/digest
	lastPart := parts[len(parts)-1]
	if hasTag {
		tagParts := strings.Split(lastPart, ":")
		if len(tagParts) != 2 || tagParts[0] == "" || tagParts[1] == "" {
			return false
		}
	}

	return true
}
