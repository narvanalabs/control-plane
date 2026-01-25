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
	"github.com/narvanalabs/control-plane/internal/api/health"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/auth"
	"github.com/narvanalabs/control-plane/internal/cleanup"
	"github.com/narvanalabs/control-plane/internal/podman"
	"github.com/narvanalabs/control-plane/internal/queue"
	"github.com/narvanalabs/control-plane/internal/secrets"
	"github.com/narvanalabs/control-plane/internal/store"
	"github.com/narvanalabs/control-plane/internal/updater"
	"github.com/narvanalabs/control-plane/pkg/config"
)

// Version is the current version of the API server.
// This should be set at build time using ldflags.
var Version = "dev"

// Server represents the HTTP API server.
type Server struct {
	router        chi.Router
	httpServer    *http.Server
	store         store.Store
	queue         queue.Queue
	auth          *auth.Service
	sopsService   *secrets.SOPSService
	config        *config.Config
	logger        *slog.Logger
	healthChecker *health.Checker
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

	// Initialize health checker
	s.healthChecker = health.NewChecker(st, Version)

	// Initialize SOPS service if configured
	if cfg.SOPS.AgePublicKey != "" || cfg.SOPS.AgePrivateKey != "" {
		sopsService, err := secrets.NewSOPSService(&secrets.Config{
			AgePublicKey:  cfg.SOPS.AgePublicKey,
			AgePrivateKey: cfg.SOPS.AgePrivateKey,
		}, logger)
		if err != nil {
			logger.Error("failed to initialize SOPS service", "error", err)
		} else {
			s.sopsService = sopsService
			logger.Info("SOPS service initialized", "can_encrypt", sopsService.CanEncrypt(), "can_decrypt", sopsService.CanDecrypt())
		}
	} else {
		logger.Warn("SOPS not configured, secrets will be stored without encryption")
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
	r.Get("/health", s.healthChecker.Handler())

	// API Documentation endpoints (no auth required)
	// Requirements: 9.1
	docsHandler := handlers.NewDocsHandler(s.logger)
	r.Get("/api/docs", docsHandler.ServeSwaggerUI)
	r.Get("/api/docs/openapi.yaml", docsHandler.ServeOpenAPISpec)

	// Auth routes (no auth required)
	authHandler := handlers.NewAuthHandler(s.store, s.auth, s.logger)
	invitationsPublicHandler := handlers.NewInvitationsHandler(s.store, s.auth, s.logger)
	r.Route("/auth", func(r chi.Router) {
		r.Get("/setup", authHandler.SetupCheck)
		r.Get("/can-register", authHandler.CanRegister)
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/device/start", authHandler.DeviceAuthStart)
		r.Get("/device/poll", authHandler.DeviceAuthPoll)
		r.Post("/device/approve", authHandler.DeviceAuthApprove)
		// Invitation acceptance (public)
		r.Get("/invite/{token}", invitationsPublicHandler.GetByToken)
		r.Post("/invite/accept", invitationsPublicHandler.Accept)
	})

	// GitHub callbacks (public)
	githubHandler := handlers.NewGitHubHandler(s.store, s.logger)
	r.Get("/github/callback", githubHandler.ManifestCallback)
	r.Get("/github/oauth/callback", githubHandler.OAuthCallback)
	r.Get("/github/post-install", githubHandler.PostInstallation)
	r.Post("/github/webhook", githubHandler.Webhook)

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

		// Platform configuration endpoint
		// Requirements: 2.1
		configHandler := handlers.NewConfigHandler(s.store, s.logger)
		r.Get("/config", configHandler.GetConfig)
		r.Get("/config/defaults", configHandler.GetDefaults)

		// Dashboard statistics endpoint
		// Requirements: 1.1
		statsHandler := handlers.NewStatsHandler(s.store, s.logger)
		r.Route("/dashboard", func(r chi.Router) {
			r.Use(middleware.OrgContext(s.store, s.logger))
			r.Get("/stats", statsHandler.GetDashboardStats)
		})

		// Detection endpoint
		detectHandler := handlers.NewDetectHandler(s.logger)
		r.Post("/detect", detectHandler.Detect)

		// App routes
		podmanClient := podman.NewClient(s.config.Worker.PodmanSocket, s.logger)
		appHandler := handlers.NewAppHandler(s.store, s.logger)
		r.Route("/apps", func(r chi.Router) {
			r.Post("/", appHandler.Create)
			r.Get("/", appHandler.List)
			r.Route("/{appID}", func(r chi.Router) {
				r.Use(middleware.RequireOwnership(s.store, s.logger))
				r.Get("/", appHandler.Get)
				r.Patch("/", appHandler.Update)
				r.Delete("/", appHandler.Delete)

				// Deployment routes nested under apps
				deploymentHandler := handlers.NewDeploymentHandler(s.store, s.queue, s.logger)
				r.Post("/deploy", deploymentHandler.Create)
				r.Get("/deployments", deploymentHandler.List)

				// Service routes nested under apps
				serviceHandler := handlers.NewServiceHandler(s.store, podmanClient, s.sopsService, s.logger)
				r.Route("/services", func(r chi.Router) {
					r.Post("/", serviceHandler.Create)
					r.Get("/", serviceHandler.List)
					r.Get("/{serviceName}", serviceHandler.Get)
					r.Patch("/{serviceName}", serviceHandler.Update)
					r.Delete("/{serviceName}", serviceHandler.Delete)
					r.Post("/{serviceName}/deploy", deploymentHandler.CreateForService)

					// Service lifecycle actions
					r.Post("/{serviceName}/stop", serviceHandler.StopService)
					r.Post("/{serviceName}/start", serviceHandler.StartService)
					r.Post("/{serviceName}/reload", serviceHandler.ReloadService)
					r.Post("/{serviceName}/retry", serviceHandler.RetryService)

					// Environment variable endpoints
					r.Get("/{serviceName}/env", serviceHandler.ListEnvVars)
					r.Post("/{serviceName}/env", serviceHandler.AddEnvVar)
					r.Put("/{serviceName}/env/{key}", serviceHandler.UpdateEnvVar)
					r.Delete("/{serviceName}/env/{key}", serviceHandler.DeleteEnvVar)

					// Preview endpoint for build preview
					previewHandler, err := handlers.NewPreviewHandler(s.store, s.logger)
					if err != nil {
						s.logger.Error("failed to create preview handler", "error", err)
					} else {
						r.Post("/{serviceName}/preview", previewHandler.Preview)
					}

					r.Get("/{serviceName}/terminal/ws", serviceHandler.TerminalWS)
				})

				// Log routes nested under apps
				logHandler := handlers.NewLogHandler(s.store, s.logger)
				r.Get("/logs", logHandler.Get)

				// Real-time log streaming via SSE
				logStreamHandler := handlers.NewLogStreamHandler(s.store, s.logger)
				r.Get("/logs/stream", logStreamHandler.Stream)

				// Secret routes nested under apps
				secretHandler := handlers.NewSecretHandler(s.store, s.sopsService, s.logger)
				r.Route("/secrets", func(r chi.Router) {
					r.Post("/", secretHandler.Create)
					r.Get("/", secretHandler.List)
					r.Delete("/{key}", secretHandler.Delete)
				})

				// Domain routes nested under apps
				domainHandler := handlers.NewDomainHandler(s.store, s.logger)
				r.Route("/domains", func(r chi.Router) {
					r.Post("/", domainHandler.Create)
					r.Get("/", domainHandler.List)
					r.Delete("/{domainID}", domainHandler.Delete)
				})
			})
		})

		// Deployment routes
		deploymentHandler := handlers.NewDeploymentHandler(s.store, s.queue, s.logger)
		r.Route("/deployments", func(r chi.Router) {
			r.Get("/", deploymentHandler.ListAll)
			r.Route("/{deploymentID}", func(r chi.Router) {
				r.Get("/", deploymentHandler.Get)
				r.Post("/rollback", deploymentHandler.Rollback)
			})
		})

		// Global domain routes (list all domains across apps)
		globalDomainHandler := handlers.NewDomainHandler(s.store, s.logger)
		r.Route("/domains", func(r chi.Router) {
			r.Get("/", globalDomainHandler.ListAll)
			r.Post("/", globalDomainHandler.CreateGlobal)
			r.Delete("/{domainID}", globalDomainHandler.DeleteGlobal)
		})

		// Node routes
		nodeHandler := handlers.NewNodeHandler(s.store, s.logger)
		r.Route("/nodes", func(r chi.Router) {
			r.Get("/", nodeHandler.List)
			r.Post("/register", nodeHandler.Register)
			r.Post("/heartbeat", nodeHandler.Heartbeat)
			r.Route("/{nodeID}", func(r chi.Router) {
				r.Get("/", func(w http.ResponseWriter, req *http.Request) {
					nodeID := chi.URLParam(req, "nodeID")
					nodeHandler.Get(w, req, nodeID)
				})
				r.Get("/details", func(w http.ResponseWriter, req *http.Request) {
					nodeID := chi.URLParam(req, "nodeID")
					nodeHandler.GetDetails(w, req, nodeID)
				})
				r.Post("/heartbeat", func(w http.ResponseWriter, req *http.Request) {
					nodeID := chi.URLParam(req, "nodeID")
					nodeHandler.HeartbeatByID(w, req, nodeID)
				})
			})
		})

		// GitHub routes (under /v1 for consistency)
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

		// Build routes
		buildHandler := handlers.NewBuildHandler(s.store, s.queue, s.logger)
		r.Route("/builds", func(r chi.Router) {
			r.Get("/", buildHandler.List)
			r.Route("/{buildID}", func(r chi.Router) {
				r.Get("/", buildHandler.Get)
				r.Post("/retry", buildHandler.Retry)
			})
		})

		// User routes
		userHandler := handlers.NewUserHandler(s.store, s.logger)
		r.Route("/user", func(r chi.Router) {
			r.Get("/profile", userHandler.GetProfile)
			r.Patch("/profile", userHandler.UpdateProfile)
		})

		// Organization routes
		orgHandler := handlers.NewOrgHandler(s.store, s.logger)
		r.Route("/orgs", func(r chi.Router) {
			r.Post("/", orgHandler.Create)
			r.Get("/", orgHandler.List)
			r.Get("/slug/{slug}", orgHandler.GetBySlug)
			r.Route("/{orgID}", func(r chi.Router) {
				r.Get("/", orgHandler.Get)
				r.Patch("/", orgHandler.Update)
				r.Delete("/", orgHandler.Delete)
			})
		})

		// Users management routes (admin only)
		usersHandler := handlers.NewUsersHandler(s.store, s.logger)
		r.Route("/users", func(r chi.Router) {
			r.Get("/", usersHandler.List)
			r.Delete("/{userID}", usersHandler.Delete)
		})

		// Invitations routes (admin only)
		invitationsHandler := handlers.NewInvitationsHandler(s.store, s.auth, s.logger)
		r.Route("/invitations", func(r chi.Router) {
			r.Post("/", invitationsHandler.Create)
			r.Get("/", invitationsHandler.List)
			r.Delete("/{invitationID}", invitationsHandler.Revoke)
		})

		// Server management routes
		serverLogsHandler := handlers.NewServerLogsHandler(s.logger)
		r.Get("/server/logs/stream", serverLogsHandler.Stream)
		r.Get("/server/logs/download", serverLogsHandler.Download)
		r.Post("/server/restart", serverLogsHandler.Restart)
		r.Get("/server/console/ws", serverLogsHandler.TerminalWS)

		serverStatsHandler := handlers.NewServerStatsHandler(s.logger, Version)
		r.Get("/server/stats", serverStatsHandler.Get)
		r.Get("/server/stats/stream", serverStatsHandler.Stream)

		// Update routes
		updaterService := updater.NewService(Version, "narvanalabs/control-plane", s.logger)
		updatesHandler := handlers.NewUpdatesHandler(updaterService, s.logger)
		r.Get("/updates/check", updatesHandler.CheckForUpdates)
		r.Post("/updates/apply", updatesHandler.TriggerUpdate)

		// Admin cleanup routes
		// Requirements: 19.1, 19.2, 19.3, 19.4, 25.4, 26.4
		podmanClientForCleanup := podman.NewClient(s.config.Worker.PodmanSocket, s.logger)
		cleanupService := cleanup.NewService(s.store, podmanClientForCleanup, s.logger)
		cleanupHandler := handlers.NewCleanupHandler(s.store, cleanupService, s.logger)
		r.Route("/admin/cleanup", func(r chi.Router) {
			r.Post("/containers", cleanupHandler.CleanupContainers)
			r.Post("/images", cleanupHandler.CleanupImages)
			r.Post("/nix-gc", cleanupHandler.NixGC)
			r.Post("/deployments", cleanupHandler.ArchiveDeployments)
			r.Post("/attic", cleanupHandler.CleanupAttic)
		})
	})

	// Redirect root to Web UI
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+r.Host+":8090", http.StatusTemporaryRedirect)
	})

	s.router = r
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
