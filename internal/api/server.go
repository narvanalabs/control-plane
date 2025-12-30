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

	// Auth routes (no auth required)
	authHandler := handlers.NewAuthHandler(s.store, s.auth, s.logger)
	r.Route("/auth", func(r chi.Router) {
		r.Get("/setup", authHandler.SetupCheck)
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/device/start", authHandler.DeviceAuthStart)
		r.Get("/device/poll", authHandler.DeviceAuthPoll)
		r.Post("/device/approve", authHandler.DeviceAuthApprove)
	})

	// GitHub callbacks (public)
	githubHandler := handlers.NewGitHubHandler(s.store, s.logger)
	r.Get("/github/callback", githubHandler.ManifestCallback)
	r.Get("/github/oauth/callback", githubHandler.OAuthCallback)

	// API v1 routes
	r.Route("/v1", func(r chi.Router) {
		// Auth middleware for all v1 routes
		authMiddleware := middleware.NewAuthMiddleware(s.auth, s.config.APIKeyHeader, s.logger)
		r.Use(authMiddleware.Authenticate)

		// Auth validation endpoint (returns OK if token is valid - middleware already validated it)
		r.Get("/auth/validate", func(w http.ResponseWriter, r *http.Request) {
			userID := middleware.GetUserID(r.Context())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok","user_id":"` + userID + `"}`))
		})

		// Detection endpoint
		detectHandler := handlers.NewDetectHandler(s.logger)
		r.Post("/detect", detectHandler.Detect)

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

				// Service routes nested under apps
				serviceHandler := handlers.NewServiceHandler(s.store, s.logger)
				r.Route("/services", func(r chi.Router) {
					r.Post("/", serviceHandler.Create)
					r.Get("/", serviceHandler.List)
					r.Get("/{serviceName}", serviceHandler.Get)
					r.Patch("/{serviceName}", serviceHandler.Update)
					r.Delete("/{serviceName}", serviceHandler.Delete)
					r.Post("/{serviceName}/deploy", deploymentHandler.CreateForService)

					// Preview endpoint for build preview
					previewHandler, err := handlers.NewPreviewHandler(s.store, s.logger)
					if err != nil {
						s.logger.Error("failed to create preview handler", "error", err)
					} else {
						r.Post("/{serviceName}/preview", previewHandler.Preview)
					}
				})

				// Log routes nested under apps
				logHandler := handlers.NewLogHandler(s.store, s.logger)
				r.Get("/logs", logHandler.Get)
				
				// Real-time log streaming via SSE
				logStreamHandler := handlers.NewLogStreamHandler(s.store, s.logger)
				r.Get("/logs/stream", logStreamHandler.Stream)

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
			r.Post("/register", nodeHandler.Register)
			r.Post("/heartbeat", nodeHandler.Heartbeat)
			r.Post("/{nodeID}/heartbeat", func(w http.ResponseWriter, req *http.Request) {
				nodeID := chi.URLParam(req, "nodeID")
				nodeHandler.HeartbeatByID(w, req, nodeID)
			})
		})

		// GitHub routes
		r.Route("/github", func(r chi.Router) {
			r.Get("/setup", githubHandler.ManifestStart)
			r.Post("/config", githubHandler.SaveConfigManual)
			r.Get("/config", githubHandler.GetConfig)
			r.Get("/install", githubHandler.AppInstall)
			r.Get("/installations", githubHandler.ListInstallations)
			r.Get("/repos", githubHandler.ListRepos)
			r.Get("/connect", githubHandler.OAuthStart)
			r.Delete("/config", githubHandler.ResetConfig)
		})

		// Settings routes
		settingsHandler := handlers.NewSettingsHandler(s.store, s.logger)
		r.Route("/settings", func(r chi.Router) {
			r.Get("/", settingsHandler.Get)
			r.Patch("/", settingsHandler.Update)
		})
	})

	// Redirect root to Web UI
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+r.Host+":8090", http.StatusTemporaryRedirect)
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
