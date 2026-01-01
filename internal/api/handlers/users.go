package handlers

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/auth"
	"github.com/narvanalabs/control-plane/internal/store"
)

// UsersHandler handles user management HTTP requests.
type UsersHandler struct {
	store       store.Store
	rbacService *auth.RBACService
	logger      *slog.Logger
}

// NewUsersHandler creates a new users handler.
func NewUsersHandler(st store.Store, logger *slog.Logger) *UsersHandler {
	return &UsersHandler{
		store:       st,
		rbacService: auth.NewRBACService(st, logger),
		logger:      logger,
	}
}

// List handles GET /v1/users - returns all users (admin only).
func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "unauthorized")
		return
	}

	// Check permission
	if err := h.rbacService.CheckPermission(r.Context(), userID, auth.PermissionViewUsers); err != nil {
		WriteError(w, http.StatusForbidden, ErrCodeForbidden, "permission denied")
		return
	}

	users, err := h.store.Users().List(r.Context())
	if err != nil {
		h.logger.Error("failed to list users", "error", err)
		WriteInternalError(w, "failed to list users")
		return
	}

	WriteJSON(w, http.StatusOK, users)
}

// Delete handles DELETE /v1/users/{userID} - removes a user (admin only).
func (h *UsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	currentUserID := middleware.GetUserID(r.Context())
	if currentUserID == "" {
		WriteError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "unauthorized")
		return
	}

	// Check permission
	if err := h.rbacService.CheckPermission(r.Context(), currentUserID, auth.PermissionManageUsers); err != nil {
		WriteError(w, http.StatusForbidden, ErrCodeForbidden, "permission denied")
		return
	}

	targetUserID := chi.URLParam(r, "userID")
	if targetUserID == "" {
		WriteBadRequest(w, "user ID required")
		return
	}

	// Prevent self-deletion
	if targetUserID == currentUserID {
		WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "cannot delete yourself")
		return
	}

	if err := h.rbacService.RemoveUser(r.Context(), targetUserID); err != nil {
		if err == auth.ErrCannotRemoveOwner {
			WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "cannot remove the only owner")
			return
		}
		if err == auth.ErrUserNotFound {
			WriteError(w, http.StatusNotFound, ErrCodeNotFound, "user not found")
			return
		}
		h.logger.Error("failed to delete user", "error", err, "user_id", targetUserID)
		WriteInternalError(w, "failed to delete user")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
