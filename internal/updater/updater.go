// Package updater provides functionality to check for and apply updates.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
)

// Service handles version checking and updates.
type Service struct {
	currentVersion string
	githubRepo     string
	logger         *slog.Logger
	httpClient     *http.Client
}

// NewService creates a new updater service.
func NewService(currentVersion, githubRepo string, logger *slog.Logger) *Service {
	return &Service{
		currentVersion: currentVersion,
		githubRepo:     githubRepo,
		logger:         logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// UpdateInfo contains information about available updates.
type UpdateInfo struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url,omitempty"`
	ReleaseNotes    string `json:"release_notes,omitempty"`
	PublishedAt     string `json:"published_at,omitempty"`
}

// GitHubRelease represents a GitHub release from the API.
type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
}

// CheckForUpdates queries GitHub for the latest release and compares with current version.
func (s *Service) CheckForUpdates(ctx context.Context) (*UpdateInfo, error) {
	info := &UpdateInfo{
		CurrentVersion: s.currentVersion,
	}

	// Skip check if version is "dev" or empty
	if s.currentVersion == "" || s.currentVersion == "dev" {
		s.logger.Debug("skipping update check for dev version")
		return info, nil
	}

	// Fetch latest release from GitHub
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", s.githubRepo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return info, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "Narvana-Control-Plane")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return info, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return info, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return info, fmt.Errorf("failed to decode release: %w", err)
	}

	// Skip drafts and prereleases for stable version checks
	if release.Draft || release.Prerelease {
		s.logger.Debug("latest release is draft or prerelease, skipping", "tag", release.TagName)
		return info, nil
	}

	info.LatestVersion = release.TagName
	info.ReleaseURL = release.HTMLURL
	info.ReleaseNotes = release.Body
	info.PublishedAt = release.PublishedAt.Format(time.RFC3339)

	// Compare versions
	updateAvailable, err := s.isNewerVersion(release.TagName)
	if err != nil {
		s.logger.Warn("failed to compare versions", "error", err)
		return info, nil // Don't fail, just return what we have
	}

	info.UpdateAvailable = updateAvailable
	return info, nil
}

// isNewerVersion compares the latest version with current version.
func (s *Service) isNewerVersion(latestVersion string) (bool, error) {
	// Normalize version strings (remove 'v' prefix if present)
	current := strings.TrimPrefix(s.currentVersion, "v")
	latest := strings.TrimPrefix(latestVersion, "v")

	currentVer, err := semver.NewVersion(current)
	if err != nil {
		return false, fmt.Errorf("invalid current version %q: %w", current, err)
	}

	latestVer, err := semver.NewVersion(latest)
	if err != nil {
		return false, fmt.Errorf("invalid latest version %q: %w", latest, err)
	}

	return latestVer.GreaterThan(currentVer), nil
}

// ApplyUpdate performs the update by pulling new container images and restarting services.
// This is designed to work in containerized deployments using Podman/Docker Compose.
func (s *Service) ApplyUpdate(ctx context.Context, version string) error {
	s.logger.Info("applying update", "version", version)

	// Determine deployment method
	deployMethod := s.detectDeploymentMethod()
	s.logger.Info("detected deployment method", "method", deployMethod)

	switch deployMethod {
	case "compose":
		return s.updateCompose(ctx, version)
	case "systemd":
		return s.updateSystemd(ctx, version)
	default:
		return fmt.Errorf("unsupported deployment method: %s", deployMethod)
	}
}

// detectDeploymentMethod checks how Narvana is deployed.
func (s *Service) detectDeploymentMethod() string {
	// Check if running in a container (presence of /.dockerenv or /run/.containerenv)
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "compose"
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return "compose"
	}

	// Check for systemd service
	if _, err := exec.LookPath("systemctl"); err == nil {
		cmd := exec.Command("systemctl", "is-active", "narvana-api")
		if err := cmd.Run(); err == nil {
			return "systemd"
		}
	}

	return "unknown"
}

// updateCompose updates a Docker/Podman Compose deployment.
func (s *Service) updateCompose(ctx context.Context, version string) error {
	// In a containerized environment, we need to trigger an external update script
	// or communicate with the host system to restart the compose stack.
	
	// Set the desired version in the environment
	composeFile := os.Getenv("COMPOSE_FILE")
	if composeFile == "" {
		composeFile = "/app/compose.yaml" // Default location in container
	}

	// Write a flag file that the host can monitor to trigger updates
	updateFlagFile := "/var/lib/narvana/.update-requested"
	content := fmt.Sprintf("version=%s\ntimestamp=%s\n", version, time.Now().Format(time.RFC3339))
	
	if err := os.WriteFile(updateFlagFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write update flag: %w", err)
	}

	s.logger.Info("update flag written, waiting for external updater", "file", updateFlagFile, "version", version)
	
	// Note: The actual container restart must be done externally (by systemd, cron, or a watcher)
	// because the API can't restart its own container from inside.
	return nil
}

// updateSystemd updates a systemd-based deployment.
func (s *Service) updateSystemd(ctx context.Context, version string) error {
	// For systemd deployments, we would:
	// 1. Download new binaries
	// 2. Restart services
	// This is more complex and depends on the installation method
	
	return fmt.Errorf("systemd-based updates not yet implemented")
}

