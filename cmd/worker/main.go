// Package main provides the entry point for the build worker.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/narvanalabs/control-plane/internal/builder"
	postgresqueue "github.com/narvanalabs/control-plane/internal/queue/postgres"
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

	// Get default config for AtticToken
	defaultNixCfg := builder.DefaultNixBuilderConfig()

	// Configure the worker
	workerCfg := &builder.WorkerConfig{
		Concurrency: cfg.Worker.MaxConcurrency,
		NixConfig: &builder.NixBuilderConfig{
			WorkDir:      cfg.Worker.WorkDir,
			PodmanSocket: cfg.Worker.PodmanSocket,
			NixImage:     "docker.io/nixos/nix:latest",
			AtticURL:     cfg.AtticEndpoint,
			AtticCache:   "narvana",
			AtticToken:   defaultNixCfg.AtticToken, // Use default dev token
		},
		OCIConfig: &builder.OCIBuilderConfig{
			NixBuilderConfig: &builder.NixBuilderConfig{
				WorkDir:      cfg.Worker.WorkDir,
				PodmanSocket: cfg.Worker.PodmanSocket,
				NixImage:     "docker.io/nixos/nix:latest",
				AtticURL:     cfg.AtticEndpoint,
				AtticCache:   "narvana",
				AtticToken:   defaultNixCfg.AtticToken,
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

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start the worker
	log.Info("starting build worker",
		"concurrency", cfg.Worker.MaxConcurrency,
		"work_dir", cfg.Worker.WorkDir,
	)

	if err := worker.Start(ctx); err != nil {
		log.Error("failed to start worker", "error", err)
		os.Exit(1)
	}

	// Wait for shutdown signal
	sig := <-sigCh
	log.Info("received shutdown signal", "signal", sig)

	// Stop the worker gracefully
	worker.Stop()

	log.Info("build worker shutdown complete")
}
