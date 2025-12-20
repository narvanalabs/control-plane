// Package api provides the HTTP API server for the control plane.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/narvanalabs/control-plane/internal/api/handlers"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/auth"
	"github.com/narvanalabs/control-plane/internal/queue"
	"github.com/narvanalabs/control-plane/internal/store"
	"github.com/narvanalabs/control-plane/pkg/config"
)

// Server represents the HTTP API server.
type Server struct {
	router     chi.Router
	httpServer *http.Server
	store      store.Store
	queue      queue.Queue
	auth       *auth.Service
	config     *config.Config
	logger     *slog.Logger
}

// NewServer creates a new API server with the given dependencies.
func NewServer(cfg *config.Config, st store.Store, q queue.Queue, authSvc *auth.Service, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		store:  st,
		queue:  q,
		auth:   authSvc,
		config: cfg,
		logger: logger,
	}

	s.setupRouter()
	return s
}


// setupRouter configures the router with middleware and routes.
func (s *Server) setupRouter() {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.RequestLogger(s.logger))
	r.Use(middleware.Recovery(s.logger))
	r.Use(chimiddleware.Timeout(60 * time.Second))

	// Health check endpoint (no auth required)
	r.Get("/health", s.healthCheck)

	// API v1 routes
	r.Route("/v1", func(r chi.Router) {
		// Auth middleware for all v1 routes
		authMiddleware := middleware.NewAuthMiddleware(s.auth, s.config.APIKeyHeader, s.logger)
		r.Use(authMiddleware.Authenticate)

		// App routes
		appHandler := handlers.NewAppHandler(s.store, s.logger)
		r.Route("/apps", func(r chi.Router) {
			r.Post("/", appHandler.Create)
			r.Get("/", appHandler.List)
			r.Route("/{appID}", func(r chi.Router) {
				r.Use(middleware.RequireOwnership(s.store, s.logger))
				r.Get("/", appHandler.Get)
				r.Delete("/", appHandler.Delete)

				// Deployment routes nested under apps
				deploymentHandler := handlers.NewDeploymentHandler(s.store, s.queue, s.logger)
				r.Post("/deploy", deploymentHandler.Create)
				r.Get("/deployments", deploymentHandler.List)

				// Log routes nested under apps
				logHandler := handlers.NewLogHandler(s.store, s.logger)
				r.Get("/logs", logHandler.Get)

				// Secret routes nested under apps
				secretHandler := handlers.NewSecretHandler(s.store, s.logger)
				r.Route("/secrets", func(r chi.Router) {
					r.Post("/", secretHandler.Create)
					r.Get("/", secretHandler.List)
					r.Delete("/{key}", secretHandler.Delete)
				})
			})
		})

		// Deployment routes (direct access by ID)
		deploymentHandler := handlers.NewDeploymentHandler(s.store, s.queue, s.logger)
		r.Route("/deployments/{deploymentID}", func(r chi.Router) {
			r.Get("/", deploymentHandler.Get)
			r.Post("/rollback", deploymentHandler.Rollback)
		})

		// Node routes
		nodeHandler := handlers.NewNodeHandler(s.store, s.logger)
		r.Route("/nodes", func(r chi.Router) {
			r.Get("/", nodeHandler.List)
			r.Post("/heartbeat", nodeHandler.Heartbeat)
		})
	})

	s.router = r
}

// healthCheck handles the health check endpoint.
func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// Start starts the HTTP server.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.config.APIHost, s.config.APIPort)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.logger.Info("starting API server", "addr", addr)

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	}
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down API server")
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(shutdownCtx)
}

// Router returns the chi router for testing purposes.
func (s *Server) Router() chi.Router {
	return s.router
}
