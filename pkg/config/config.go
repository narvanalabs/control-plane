// Package config provides environment-based configuration for the control plane.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the control plane.
type Config struct {
	// Database configuration
	DatabaseDSN string

	// Authentication
	JWTSecret    string
	JWTExpiry    time.Duration
	APIKeyHeader string

	// External services
	AtticEndpoint string
	RegistryURL   string

	// Server configuration
	APIPort  int
	GRPCPort int
	APIHost  string

	// Graceful shutdown timeout
	// **Validates: Requirements 15.2, 15.3**
	ShutdownTimeout time.Duration

	// Scheduler configuration
	Scheduler SchedulerConfig

	// Worker configuration
	Worker WorkerConfig

	// SOPS configuration for secrets encryption
	SOPS SOPSConfig
}

// SOPSConfig holds SOPS-Nix secrets encryption configuration.
type SOPSConfig struct {
	// AgePublicKey is the age public key for encryption (required for API server).
	// Format: age1... (Bech32 encoded)
	AgePublicKey string
	// AgePrivateKey is the age private key for decryption (required for node agents).
	// Format: AGE-SECRET-KEY-1... (Bech32 encoded)
	AgePrivateKey string
}

// SchedulerConfig holds scheduler-specific configuration.
type SchedulerConfig struct {
	HealthThreshold   time.Duration
	MaxRetries        int
	RetryBackoff      time.Duration
	DeploymentTimeout time.Duration // Timeout for deployments waiting to be scheduled
}

// WorkerConfig holds build worker-specific configuration.
type WorkerConfig struct {
	WorkDir        string
	PodmanSocket   string
	BuildTimeout   time.Duration
	MaxConcurrency int
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseDSN:     getEnv("DATABASE_URL", "postgres://localhost:5432/narvana?sslmode=disable"),
		JWTSecret:       getEnv("JWT_SECRET", ""),
		JWTExpiry:       getDurationEnv("JWT_EXPIRY", 24*time.Hour),
		APIKeyHeader:    getEnv("API_KEY_HEADER", "X-API-Key"),
		AtticEndpoint:   getEnv("ATTIC_ENDPOINT", "http://localhost:5000"),
		RegistryURL:     getEnv("REGISTRY_URL", "localhost:5000"),
		APIPort:         getIntEnv("API_PORT", 8080),
		GRPCPort:        getIntEnv("GRPC_PORT", 9090),
		APIHost:         getEnv("API_HOST", "0.0.0.0"),
		ShutdownTimeout: getDurationEnv("SHUTDOWN_TIMEOUT", 30*time.Second),
		Scheduler: SchedulerConfig{
			HealthThreshold:   getDurationEnv("SCHEDULER_HEALTH_THRESHOLD", 30*time.Second),
			MaxRetries:        getIntEnv("SCHEDULER_MAX_RETRIES", 5),
			RetryBackoff:      getDurationEnv("SCHEDULER_RETRY_BACKOFF", 5*time.Second),
			DeploymentTimeout: getDurationEnv("SCHEDULER_DEPLOYMENT_TIMEOUT", 30*time.Minute),
		},
		Worker: WorkerConfig{
			WorkDir:        getEnv("WORKER_WORKDIR", "/tmp/narvana-builds"),
			PodmanSocket:   getEnv("PODMAN_SOCKET", "unix:///run/user/1000/podman/podman.sock"),
			BuildTimeout:   getDurationEnv("BUILD_TIMEOUT", 30*time.Minute),
			MaxConcurrency: getIntEnv("WORKER_MAX_CONCURRENCY", 4),
		},
		SOPS: SOPSConfig{
			AgePublicKey:  getEnv("SOPS_AGE_PUBLIC_KEY", ""),
			AgePrivateKey: getEnv("SOPS_AGE_PRIVATE_KEY", ""),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that required configuration values are set.
func (c *Config) Validate() error {
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}
	return nil
}

// LoadWithDefaults loads configuration with defaults for development.
// It does not validate required fields, useful for testing.
func LoadWithDefaults() *Config {
	return &Config{
		DatabaseDSN:     getEnv("DATABASE_URL", "postgres://localhost:5432/narvana?sslmode=disable"),
		JWTSecret:       getEnv("JWT_SECRET", "development-secret-key-min-32-chars"),
		JWTExpiry:       getDurationEnv("JWT_EXPIRY", 24*time.Hour),
		APIKeyHeader:    getEnv("API_KEY_HEADER", "X-API-Key"),
		AtticEndpoint:   getEnv("ATTIC_ENDPOINT", "http://localhost:5000"),
		RegistryURL:     getEnv("REGISTRY_URL", "localhost:5000"),
		APIPort:         getIntEnv("API_PORT", 8080),
		GRPCPort:        getIntEnv("GRPC_PORT", 9090),
		APIHost:         getEnv("API_HOST", "0.0.0.0"),
		ShutdownTimeout: getDurationEnv("SHUTDOWN_TIMEOUT", 30*time.Second),
		Scheduler: SchedulerConfig{
			HealthThreshold:   getDurationEnv("SCHEDULER_HEALTH_THRESHOLD", 30*time.Second),
			MaxRetries:        getIntEnv("SCHEDULER_MAX_RETRIES", 5),
			RetryBackoff:      getDurationEnv("SCHEDULER_RETRY_BACKOFF", 5*time.Second),
			DeploymentTimeout: getDurationEnv("SCHEDULER_DEPLOYMENT_TIMEOUT", 30*time.Minute),
		},
		Worker: WorkerConfig{
			WorkDir:        getEnv("WORKER_WORKDIR", "/tmp/narvana-builds"),
			PodmanSocket:   getEnv("PODMAN_SOCKET", "unix:///run/user/1000/podman/podman.sock"),
			BuildTimeout:   getDurationEnv("BUILD_TIMEOUT", 30*time.Minute),
			MaxConcurrency: getIntEnv("WORKER_MAX_CONCURRENCY", 4),
		},
		SOPS: SOPSConfig{
			AgePublicKey:  getEnv("SOPS_AGE_PUBLIC_KEY", ""),
			AgePrivateKey: getEnv("SOPS_AGE_PRIVATE_KEY", ""),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
