package handlers

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/queue"
	"github.com/narvanalabs/control-plane/internal/store"
)

// BuildHandler handles build-related HTTP requests.
type BuildHandler struct {
	store  store.Store
	queue  queue.Queue
	logger *slog.Logger
}

// NewBuildHandler creates a new build handler.
func NewBuildHandler(st store.Store, q queue.Queue, logger *slog.Logger) *BuildHandler {
	return &BuildHandler{
		store:  st,
		queue:  q,
		logger: logger,
	}
}

// List handles GET /v1/builds - lists all builds for the authenticated user.
func (h *BuildHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteUnauthorized(w, "Authentication required")
		return
	}

	builds, err := h.store.Builds().ListByUser(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to list builds", "error", err, "user_id", userID)
		WriteInternalError(w, "Failed to list builds")
		return
	}

	WriteJSON(w, http.StatusOK, builds)
}

// Get handles GET /v1/builds/{buildID} - retrieves a specific build.
func (h *BuildHandler) Get(w http.ResponseWriter, r *http.Request) {
	buildID := chi.URLParam(r, "buildID")
	if buildID == "" {
		WriteBadRequest(w, "Build ID is required")
		return
	}

	build, err := h.store.Builds().Get(r.Context(), buildID)
	if err != nil {
		h.logger.Debug("failed to get build", "error", err, "build_id", buildID)
		WriteNotFound(w, "Build not found")
		return
	}

	// Verify ownership through the app
	userID := middleware.GetUserID(r.Context())
	app, err := h.store.Apps().Get(r.Context(), build.AppID)
	if err != nil || app.OwnerID != userID {
		WriteForbidden(w, "Access denied")
		return
	}

	WriteJSON(w, http.StatusOK, build)
}

// Retry handles POST /v1/builds/{buildID}/retry - retries a failed build.
func (h *BuildHandler) Retry(w http.ResponseWriter, r *http.Request) {
	buildID := chi.URLParam(r, "buildID")
	if buildID == "" {
		WriteBadRequest(w, "Build ID is required")
		return
	}

	build, err := h.store.Builds().Get(r.Context(), buildID)
	if err != nil {
		WriteNotFound(w, "Build not found")
		return
	}

	// Verify ownership
	userID := middleware.GetUserID(r.Context())
	app, err := h.store.Apps().Get(r.Context(), build.AppID)
	if err != nil || app.OwnerID != userID {
		WriteForbidden(w, "Access denied")
		return
	}

	// Reset build job
	build.Status = "queued"
	build.RetryCount++

	if err := h.store.Builds().Update(r.Context(), build); err != nil {
		h.logger.Error("failed to update build for retry", "error", err, "build_id", buildID)
		WriteInternalError(w, "Failed to retry build")
		return
	}

	if h.queue != nil {
		if err := h.queue.Enqueue(r.Context(), build); err != nil {
			h.logger.Error("failed to enqueue build for retry", "error", err, "build_id", buildID)
		}
	}

	WriteJSON(w, http.StatusAccepted, build)
}
