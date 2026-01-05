// Package handlers provides HTTP request handlers for the API.
package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/narvanalabs/control-plane/internal/store"
	"github.com/narvanalabs/control-plane/internal/validation"
)

// ConfigHandler handles platform configuration endpoints.
type ConfigHandler struct {
	store  store.Store
	logger *slog.Logger
}

// NewConfigHandler creates a new config handler.
func NewConfigHandler(st store.Store, logger *slog.Logger) *ConfigHandler {
	return &ConfigHandler{
		store:  st,
		logger: logger,
	}
}

// PlatformConfig represents the platform configuration returned to clients.
// This provides all platform-specific values so the UI doesn't need hardcoded values.
type PlatformConfig struct {
	Domain            string                   `json:"domain"`
	DefaultPorts      map[string]int           `json:"default_ports"`
	StatusMappings    map[string]StatusMapping `json:"status_mappings"`
	DefaultResources  ResourceSpecConfig       `json:"default_resources"`
	SupportedDBTypes  []DatabaseTypeDef        `json:"supported_db_types"`
	MaxServicesPerApp int                      `json:"max_services_per_app"`
}

// ResourceSpecConfig defines default CPU and memory resource allocation.
type ResourceSpecConfig struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// DatabaseTypeDef defines a supported database type with its versions.
type DatabaseTypeDef struct {
	Type           string   `json:"type"`
	Versions       []string `json:"versions"`
	DefaultVersion string   `json:"default_version"`
}

// StatusMapping defines how a status should be displayed in the UI.
type StatusMapping struct {
	Label   string `json:"label"`
	Color   string `json:"color"`
	Icon    string `json:"icon,omitempty"`
}

// Settings keys for configuration values.
const (
	SettingServerDomain          = "server_domain"
	SettingDefaultResourceCPU    = "default_resource_cpu"
	SettingDefaultResourceMemory = "default_resource_memory"
	SettingMaxServicesPerApp     = "max_services_per_app"
)

// Default values for configuration.
const (
	DefaultDomain            = "localhost"
	DefaultResourceCPU       = "0.5"
	DefaultResourceMemory    = "512Mi"
	DefaultMaxServicesPerApp = 50
)

// GetConfig handles GET /v1/config - returns platform configuration.
// Requirements: 2.1
func (h *ConfigHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	config, err := h.buildPlatformConfig(r.Context())
	if err != nil {
		h.logger.Error("failed to build platform config", "error", err)
		WriteInternalError(w, "Failed to load platform configuration")
		return
	}

	WriteJSON(w, http.StatusOK, config)
}


// buildPlatformConfig constructs the platform configuration from settings and defaults.
// Requirements: 2.1, 2.2, 2.3, 2.4
func (h *ConfigHandler) buildPlatformConfig(ctx context.Context) (*PlatformConfig, error) {
	settings, err := h.store.Settings().GetAll(ctx)
	if err != nil {
		return nil, err
	}

	config := &PlatformConfig{
		Domain:            h.getDomain(settings),
		DefaultPorts:      h.getDefaultPorts(),
		StatusMappings:    h.getStatusMappings(),
		DefaultResources:  h.getDefaultResources(settings),
		SupportedDBTypes:  h.getSupportedDBTypes(),
		MaxServicesPerApp: h.getMaxServicesPerApp(settings),
	}

	return config, nil
}

// getDomain returns the platform domain from settings or default.
func (h *ConfigHandler) getDomain(settings map[string]string) string {
	if domain, ok := settings[SettingServerDomain]; ok && domain != "" {
		return domain
	}
	return DefaultDomain
}

// getDefaultPorts returns default ports by service type and framework.
// Requirements: 2.3
func (h *ConfigHandler) getDefaultPorts() map[string]int {
	return map[string]int{
		// Web frameworks
		"nextjs":   3000,
		"react":    3000,
		"vue":      3000,
		"angular":  4200,
		"svelte":   5173,
		"express":  3000,
		"fastify":  3000,
		"koa":      3000,
		"hono":     3000,
		
		// Backend frameworks
		"django":   8000,
		"flask":    5000,
		"fastapi":  8000,
		"rails":    3000,
		"phoenix":  4000,
		"gin":      8080,
		"echo":     8080,
		"fiber":    3000,
		"actix":    8080,
		"axum":     3000,
		
		// Databases
		"postgres": 5432,
		"mysql":    3306,
		"mariadb":  3306,
		"mongodb":  27017,
		"redis":    6379,
		"sqlite":   0, // No network port
		
		// Generic defaults
		"http":     8080,
		"https":    8443,
		"default":  8080,
	}
}

// getStatusMappings returns display mappings for deployment statuses.
// Requirements: 2.4
func (h *ConfigHandler) getStatusMappings() map[string]StatusMapping {
	return map[string]StatusMapping{
		"pending": {
			Label: "Pending",
			Color: "gray",
			Icon:  "clock",
		},
		"building": {
			Label: "Building",
			Color: "blue",
			Icon:  "hammer",
		},
		"built": {
			Label: "Built",
			Color: "blue",
			Icon:  "check",
		},
		"scheduled": {
			Label: "Scheduled",
			Color: "blue",
			Icon:  "calendar",
		},
		"starting": {
			Label: "Starting",
			Color: "yellow",
			Icon:  "play",
		},
		"running": {
			Label: "Running",
			Color: "green",
			Icon:  "check-circle",
		},
		"stopping": {
			Label: "Stopping",
			Color: "yellow",
			Icon:  "pause",
		},
		"stopped": {
			Label: "Stopped",
			Color: "gray",
			Icon:  "stop",
		},
		"failed": {
			Label: "Failed",
			Color: "red",
			Icon:  "x-circle",
		},
	}
}

// getDefaultResources returns default resource specifications from settings.
// Requirements: 2.1
func (h *ConfigHandler) getDefaultResources(settings map[string]string) ResourceSpecConfig {
	cpu := DefaultResourceCPU
	memory := DefaultResourceMemory

	if v, ok := settings[SettingDefaultResourceCPU]; ok && v != "" {
		cpu = v
	}
	if v, ok := settings[SettingDefaultResourceMemory]; ok && v != "" {
		memory = v
	}

	return ResourceSpecConfig{
		CPU:    cpu,
		Memory: memory,
	}
}

// getSupportedDBTypes returns the list of supported database types with versions.
// Requirements: 2.1
func (h *ConfigHandler) getSupportedDBTypes() []DatabaseTypeDef {
	types := make([]DatabaseTypeDef, 0, len(validation.SupportedDatabaseTypes))
	
	// Define order for consistent output
	orderedTypes := []string{"postgres", "mysql", "mariadb", "mongodb", "redis", "sqlite"}
	
	for _, dbType := range orderedTypes {
		versions, ok := validation.SupportedDatabaseTypes[dbType]
		if !ok {
			continue
		}
		types = append(types, DatabaseTypeDef{
			Type:           dbType,
			Versions:       versions,
			DefaultVersion: validation.DefaultDatabaseVersions[dbType],
		})
	}

	return types
}

// getMaxServicesPerApp returns the maximum services per app from settings.
func (h *ConfigHandler) getMaxServicesPerApp(settings map[string]string) int {
	if v, ok := settings[SettingMaxServicesPerApp]; ok && v != "" {
		var maxServices int
		if _, err := fmt.Sscanf(v, "%d", &maxServices); err == nil && maxServices > 0 {
			return maxServices
		}
	}
	return DefaultMaxServicesPerApp
}

// DefaultsResponse represents the response for the defaults endpoint.
// Requirements: 30.4
type DefaultsResponse struct {
	Resources ResourceSpecConfig `json:"resources"`
}

// GetDefaults handles GET /v1/config/defaults - returns current default resource values.
// Requirements: 30.4
func (h *ConfigHandler) GetDefaults(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.Settings().GetAll(r.Context())
	if err != nil {
		h.logger.Error("failed to get settings for defaults", "error", err)
		WriteInternalError(w, "Failed to load default configuration")
		return
	}

	response := DefaultsResponse{
		Resources: h.getDefaultResources(settings),
	}

	WriteJSON(w, http.StatusOK, response)
}
