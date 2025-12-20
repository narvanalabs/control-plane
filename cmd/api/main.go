// Package main provides the entry point for the API server.
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/narvanalabs/control-plane/internal/api"
	"github.com/narvanalabs/control-plane/internal/auth"
	pgqueue "github.com/narvanalabs/control-plane/internal/queue/postgres"
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

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	// Start the server
	log.Info("starting API server",
		"host", cfg.APIHost,
		"port", cfg.APIPort,
	)

	if err := server.Start(ctx); err != nil {
		log.Error("server error", "error", err)
		os.Exit(1)
	}

	// Give time for graceful shutdown
	time.Sleep(100 * time.Millisecond)
	log.Info("server stopped")
}
