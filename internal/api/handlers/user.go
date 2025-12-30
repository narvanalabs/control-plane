package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/store"
)

// UserHandler handles user-related HTTP requests.
type UserHandler struct {
	store  store.Store
	logger *slog.Logger
}

// NewUserHandler creates a new user handler.
func NewUserHandler(st store.Store, logger *slog.Logger) *UserHandler {
	return &UserHandler{
		store:  st,
		logger: logger,
	}
}

// GetProfile handles GET /v1/user/profile - returns the current user's profile.
func (h *UserHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "unauthorized")
		return
	}

	user, err := h.store.Users().GetByID(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get user profile", "error", err, "user_id", userID)
		WriteInternalError(w, "failed to get user profile")
		return
	}

	if user == nil {
		WriteError(w, http.StatusNotFound, ErrCodeNotFound, "user not found")
		return
	}

	WriteJSON(w, http.StatusOK, user)
}

// UpdateProfileRequest represents the request body for profile updates.
type UpdateProfileRequest struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// UpdateProfile handles PATCH /v1/user/profile - updates the current user's profile.
func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "unauthorized")
		return
	}

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "invalid request body")
		return
	}

	user, err := h.store.Users().GetByID(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get user profile for update", "error", err, "user_id", userID)
		WriteInternalError(w, "failed to update profile")
		return
	}

	if user == nil {
		WriteError(w, http.StatusNotFound, ErrCodeNotFound, "user not found")
		return
	}

	// Update fields
	user.Name = req.Name
	user.AvatarURL = req.AvatarURL

	if err := h.store.Users().Update(r.Context(), user); err != nil {
		h.logger.Error("failed to update user profile", "error", err, "user_id", userID)
		WriteInternalError(w, "failed to update profile")
		return
	}

	WriteJSON(w, http.StatusOK, user)
}
