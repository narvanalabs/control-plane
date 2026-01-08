// Package main provides the entry point for the build worker.
package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/narvanalabs/control-plane/internal/builder"
	postgresqueue "github.com/narvanalabs/control-plane/internal/queue/postgres"
	"github.com/narvanalabs/control-plane/internal/shutdown"
	"github.com/narvanalabs/control-plane/internal/store/postgres"
	"github.com/narvanalabs/control-plane/pkg/config"
	"github.com/narvanalabs/control-plane/pkg/logger"
)

func main() {
	// Initialize logger
	log := logger.Default()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize database store
	storeCfg := postgres.DefaultConfig(cfg.DatabaseDSN)
	store, err := postgres.NewPostgresStore(storeCfg, log.Logger)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Initialize queue
	queue := postgresqueue.NewPostgresQueue(store.DB(), log.Logger)

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Perform startup recovery for pending and interrupted builds
	// **Validates: Requirements 15.1, 15.2**
	recoveryService := builder.NewRecoveryService(store, queue, log.Logger)
	recoveryResult, err := recoveryService.RecoverOnStartup(ctx)
	if err != nil {
		log.Error("failed to perform startup recovery", "error", err)
		// Continue anyway - recovery errors shouldn't prevent worker from starting
	} else {
		log.Info("startup recovery completed",
			"interrupted_builds", recoveryResult.InterruptedBuilds,
			"resumed_builds", recoveryResult.ResumedBuilds,
		)
	}

	// Get Attic token from config, fall back to default dev token if not set
	atticToken := cfg.AtticToken
	if atticToken == "" {
		defaultNixCfg := builder.DefaultNixBuilderConfig()
		atticToken = defaultNixCfg.AtticToken
	}

	// Configure the worker
	workerCfg := &builder.WorkerConfig{
		Concurrency: cfg.Worker.MaxConcurrency,
		NixConfig: &builder.NixBuilderConfig{
			WorkDir:      cfg.Worker.WorkDir,
			PodmanSocket: cfg.Worker.PodmanSocket,
			NixImage:     "docker.io/nixos/nix:latest",
			AtticURL:     cfg.AtticEndpoint,
			AtticCache:   "narvana",
			AtticToken:   atticToken,
		},
		OCIConfig: &builder.OCIBuilderConfig{
			NixBuilderConfig: &builder.NixBuilderConfig{
				WorkDir:      cfg.Worker.WorkDir,
				PodmanSocket: cfg.Worker.PodmanSocket,
				NixImage:     "docker.io/nixos/nix:latest",
				AtticURL:     cfg.AtticEndpoint,
				AtticCache:   "narvana",
				AtticToken:   atticToken,
			},
			Registry:     cfg.RegistryURL,
			PodmanSocket: cfg.Worker.PodmanSocket,
		},
		AtticConfig: &builder.AtticConfig{
			Endpoint:  cfg.AtticEndpoint,
			CacheName: "narvana",
			Timeout:   cfg.Worker.BuildTimeout,
		},
	}

	// Create the worker
	worker, err := builder.NewWorker(workerCfg, store, queue, log.Logger)
	if err != nil {
		log.Error("failed to create worker", "error", err)
		os.Exit(1)
	}

	// Create shutdown coordinator with configurable timeout
	// **Validates: Requirements 15.1, 15.2, 15.4**
	shutdownTimeout := 30 * time.Second
	if cfg.ShutdownTimeout > 0 {
		shutdownTimeout = cfg.ShutdownTimeout
	}
	coordinator := shutdown.NewCoordinator(
		shutdown.WithTimeout(shutdownTimeout),
		shutdown.WithLogger(log.Logger),
	)

	// Register database connection for cleanup (registered first, closed last)
	coordinator.Register(shutdown.NewCloserComponent("database", store))

	// Start health check HTTP server
	healthChecker := builder.NewWorkerHealthChecker(store.DB(), builder.WorkerVersion)
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/health", healthChecker.Handler())

	healthServer := &http.Server{
		Addr:    ":8081",
		Handler: healthMux,
	}

	// Register health server for graceful shutdown
	coordinator.Register(shutdown.NewHTTPServerComponent("worker-health-server", healthServer))

	// Register worker for graceful shutdown (waits for in-progress builds)
	coordinator.Register(shutdown.NewWorkerComponent("build-worker", worker))

	go func() {
		log.Info("starting worker health check server", "addr", ":8081")
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("health check server error", "error", err)
		}
	}()

	// Start the worker
	log.Info("starting build worker",
		"concurrency", cfg.Worker.MaxConcurrency,
		"work_dir", cfg.Worker.WorkDir,
	)

	if err := worker.Start(ctx); err != nil {
		log.Error("failed to start worker", "error", err)
		os.Exit(1)
	}

	// Wait for shutdown signal and perform graceful shutdown
	// **Validates: Requirements 15.1, 15.2, 15.4**
	coordinator.WaitForSignal()
	coordinator.Wait()

	log.Info("build worker shutdown complete")
	os.Exit(coordinator.ExitCode())
}
