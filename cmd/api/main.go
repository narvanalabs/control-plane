// Package main provides the entry point for the API server.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/narvanalabs/control-plane/internal/api"
	"github.com/narvanalabs/control-plane/internal/auth"
	"github.com/narvanalabs/control-plane/internal/deploy"
	grpcserver "github.com/narvanalabs/control-plane/internal/grpc"
	"github.com/narvanalabs/control-plane/internal/models"
	pgqueue "github.com/narvanalabs/control-plane/internal/queue/postgres"
	"github.com/narvanalabs/control-plane/internal/scheduler"
	"github.com/narvanalabs/control-plane/internal/secrets"
	"github.com/narvanalabs/control-plane/internal/shutdown"
	pgstore "github.com/narvanalabs/control-plane/internal/store/postgres"
	"github.com/narvanalabs/control-plane/pkg/config"
	"github.com/narvanalabs/control-plane/pkg/logger"
)

func main() {
	// Initialize logger
	log := logger.New(slog.LevelInfo, true)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize database connection for queue
	db, err := sql.Open("pgx", cfg.DatabaseDSN)
	if err != nil {
		log.Error("failed to open database connection", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Initialize database store
	storeCfg := pgstore.DefaultConfig(cfg.DatabaseDSN)
	store, err := pgstore.NewPostgresStore(storeCfg, log.Logger)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Initialize build queue
	queue := pgqueue.NewPostgresQueue(db, log.Logger)

	// Initialize auth service
	authCfg := &auth.Config{
		JWTSecret:   []byte(cfg.JWTSecret),
		TokenExpiry: cfg.JWTExpiry,
	}
	authService := auth.NewService(authCfg, nil, log.Logger) // No API key store for now

	// Create and start the API server
	server := api.NewServer(cfg, store, queue, authService, log.Logger)

	// Create gRPC server configuration
	grpcCfg := &grpcserver.Config{
		Port:                 cfg.GRPCPort,
		MaxConcurrentStreams: 1000,
		KeepaliveTime:        30 * time.Second,
		KeepaliveTimeout:     10 * time.Second,
		MaxRecvMsgSize:       16 * 1024 * 1024, // 16MB
	}

	// Create gRPC server (shares store and auth service with HTTP server)
	grpcServer, err := grpcserver.NewServer(grpcCfg, store, authService, log.Logger)
	if err != nil {
		log.Error("failed to create gRPC server", "error", err)
		os.Exit(1)
	}

	// Create scheduler with gRPC agent client
	grpcAgentClient := scheduler.NewGRPCAgentClient(grpcServer.NodeManager(), nil)
	sched := scheduler.NewScheduler(store, grpcAgentClient, &config.SchedulerConfig{
		HealthThreshold: cfg.Scheduler.HealthThreshold,
		MaxRetries:      cfg.Scheduler.MaxRetries,
		RetryBackoff:    cfg.Scheduler.RetryBackoff,
	}, log.Logger)

	// Initialize SOPS service for secret decryption (if configured)
	// **Validates: Requirements 6.1, 6.2, 6.3**
	var sopsService *secrets.SOPSService
	if cfg.SOPS.AgePublicKey != "" || cfg.SOPS.AgePrivateKey != "" {
		var err error
		sopsService, err = secrets.NewSOPSService(&secrets.Config{
			AgePublicKey:  cfg.SOPS.AgePublicKey,
			AgePrivateKey: cfg.SOPS.AgePrivateKey,
		}, log.Logger)
		if err != nil {
			log.Warn("failed to initialize SOPS service, secrets will not be decrypted", "error", err)
		}
	}

	// Initialize EnvMerger for merging app-level secrets with service-level env vars
	// **Validates: Requirements 6.1, 6.2, 6.3**
	envMerger := deploy.NewEnvMerger(store, sopsService, log.Logger)
	sched.SetEnvMerger(envMerger)

	// Create shutdown coordinator with configurable timeout
	// **Validates: Requirements 15.1, 15.2, 15.3, 15.4, 15.5**
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

	// Create HTTP server for API
	addr := fmt.Sprintf("%s:%d", cfg.APIHost, cfg.APIPort)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      server.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Register HTTP server for graceful shutdown
	coordinator.Register(shutdown.NewHTTPServerComponent("api-http-server", httpServer))

	// Register gRPC server for graceful shutdown
	coordinator.Register(shutdown.NewFuncComponent("grpc-server", func(ctx context.Context) error {
		return grpcServer.Stop(ctx)
	}))

	// Start the gRPC server in a goroutine
	grpcErrCh := make(chan error, 1)
	go func() {
		log.Info("starting gRPC server",
			"host", cfg.APIHost,
			"port", cfg.GRPCPort,
		)
		if err := grpcServer.Start(context.Background()); err != nil {
			grpcErrCh <- err
		}
		close(grpcErrCh)
	}()

	// Start the scheduler loop in a goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runSchedulerLoop(ctx, store, sched, log)

	// Start the HTTP API server in a goroutine
	httpErrCh := make(chan error, 1)
	go func() {
		log.Info("starting API server",
			"host", cfg.APIHost,
			"port", cfg.APIPort,
		)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			httpErrCh <- err
		}
		close(httpErrCh)
	}()

	// Wait for either server to error or shutdown signal
	go func() {
		select {
		case err := <-grpcErrCh:
			if err != nil {
				log.Error("gRPC server error", "error", err)
				coordinator.Shutdown()
			}
		case err := <-httpErrCh:
			if err != nil {
				log.Error("HTTP server error", "error", err)
				coordinator.Shutdown()
			}
		}
	}()

	// Wait for shutdown signal and perform graceful shutdown
	// **Validates: Requirements 15.1, 15.2, 15.3, 15.4, 15.5**
	coordinator.WaitForSignal()
	coordinator.Wait()

	// Cancel scheduler context
	cancel()

	log.Info("API server shutdown complete")
	os.Exit(coordinator.ExitCode())
}

// runSchedulerLoop periodically checks for built deployments and schedules them.
func runSchedulerLoop(ctx context.Context, store *pgstore.PostgresStore, sched *scheduler.Scheduler, log *logger.Logger) {
	log.Info("starting scheduler loop")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("scheduler loop stopped")
			return
		case <-ticker.C:
			// Find deployments that are built but not yet scheduled
			deployments, err := store.Deployments().ListByStatus(ctx, models.DeploymentStatusBuilt)
			if err != nil {
				log.Error("failed to list built deployments", "error", err)
				continue
			}

			for _, deployment := range deployments {
				log.Info("scheduling deployment",
					"deployment_id", deployment.ID,
					"service_name", deployment.ServiceName,
					"app_id", deployment.AppID,
				)

				if err := sched.ScheduleAndAssign(ctx, deployment); err != nil {
					log.Error("failed to schedule deployment",
						"deployment_id", deployment.ID,
						"error", err,
					)
					continue
				}

				log.Info("deployment scheduled successfully",
					"deployment_id", deployment.ID,
					"node_id", deployment.NodeID,
				)
			}
		}
	}
}
