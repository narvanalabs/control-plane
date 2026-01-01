package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/narvanalabs/control-plane/internal/api/middleware"
	"github.com/narvanalabs/control-plane/internal/auth"
	"github.com/narvanalabs/control-plane/internal/store"
)

// InvitationsHandler handles invitation HTTP requests.
type InvitationsHandler struct {
	store       store.Store
	rbacService *auth.RBACService
	authService *auth.Service
	logger      *slog.Logger
}

// NewInvitationsHandler creates a new invitations handler.
func NewInvitationsHandler(st store.Store, authSvc *auth.Service, logger *slog.Logger) *InvitationsHandler {
	return &InvitationsHandler{
		store:       st,
		rbacService: auth.NewRBACService(st, logger),
		authService: authSvc,
		logger:      logger,
	}
}

// CreateInvitationRequest represents the request body for creating an invitation.
type CreateInvitationRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// Create handles POST /v1/invitations - creates a new invitation (admin only).
func (h *InvitationsHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "unauthorized")
		return
	}

	// Check permission
	if err := h.rbacService.CheckPermission(r.Context(), userID, auth.PermissionManageUsers); err != nil {
		WriteError(w, http.StatusForbidden, ErrCodeForbidden, "permission denied")
		return
	}

	var req CreateInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "invalid request body")
		return
	}

	if req.Email == "" {
		WriteBadRequest(w, "email is required")
		return
	}

	// Default to member role if not specified
	role := store.RoleMember
	if req.Role == "owner" {
		role = store.RoleOwner
	}

	invitation, err := h.rbacService.CreateInvitation(r.Context(), req.Email, userID, role)
	if err != nil {
		if err == auth.ErrEmailAlreadyInvited {
			WriteError(w, http.StatusConflict, ErrCodeConflict, "email already invited")
			return
		}
		h.logger.Error("failed to create invitation", "error", err)
		WriteInternalError(w, "failed to create invitation")
		return
	}

	WriteJSON(w, http.StatusCreated, invitation)
}

// List handles GET /v1/invitations - lists all invitations (admin only).
func (h *InvitationsHandler) List(w http.ResponseWriter, r *http.Request) {
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

	invitations, err := h.rbacService.ListInvitations(r.Context())
	if err != nil {
		h.logger.Error("failed to list invitations", "error", err)
		WriteInternalError(w, "failed to list invitations")
		return
	}

	WriteJSON(w, http.StatusOK, invitations)
}

// Revoke handles DELETE /v1/invitations/{invitationID} - revokes an invitation (admin only).
func (h *InvitationsHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "unauthorized")
		return
	}

	// Check permission
	if err := h.rbacService.CheckPermission(r.Context(), userID, auth.PermissionManageUsers); err != nil {
		WriteError(w, http.StatusForbidden, ErrCodeForbidden, "permission denied")
		return
	}

	invitationID := chi.URLParam(r, "invitationID")
	if invitationID == "" {
		WriteBadRequest(w, "invitation ID required")
		return
	}

	if err := h.rbacService.RevokeInvitation(r.Context(), invitationID); err != nil {
		if err == auth.ErrInvitationNotFound {
			WriteError(w, http.StatusNotFound, ErrCodeNotFound, "invitation not found")
			return
		}
		h.logger.Error("failed to revoke invitation", "error", err, "invitation_id", invitationID)
		WriteInternalError(w, "failed to revoke invitation")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// AcceptInvitationRequest represents the request body for accepting an invitation.
type AcceptInvitationRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// Accept handles POST /auth/invite/accept - accepts an invitation (public).
func (h *InvitationsHandler) Accept(w http.ResponseWriter, r *http.Request) {
	var req AcceptInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "invalid request body")
		return
	}

	if req.Token == "" {
		WriteBadRequest(w, "token is required")
		return
	}

	if req.Password == "" {
		WriteBadRequest(w, "password is required")
		return
	}

	if len(req.Password) < 8 {
		WriteBadRequest(w, "password must be at least 8 characters")
		return
	}

	user, err := h.rbacService.AcceptInvitation(r.Context(), req.Token, req.Password)
	if err != nil {
		if err == auth.ErrInvitationNotFound {
			WriteError(w, http.StatusNotFound, ErrCodeNotFound, "invitation not found")
			return
		}
		if err == auth.ErrInvitationExpired {
			WriteError(w, http.StatusGone, "invitation_expired", "invitation has expired")
			return
		}
		if err == auth.ErrInvitationUsed {
			WriteError(w, http.StatusConflict, ErrCodeConflict, "invitation already used")
			return
		}
		h.logger.Error("failed to accept invitation", "error", err)
		WriteInternalError(w, "failed to accept invitation")
		return
	}

	// Generate token for the new user
	token, err := h.authService.GenerateToken(user.ID, user.Email)
	if err != nil {
		h.logger.Error("failed to generate token", "error", err)
		WriteInternalError(w, "failed to generate token")
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"user_id": user.ID,
		"email":   user.Email,
		"token":   token,
		"role":    user.Role,
	})
}

// GetByToken handles GET /auth/invite/{token} - gets invitation details (public).
func (h *InvitationsHandler) GetByToken(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		WriteBadRequest(w, "token required")
		return
	}

	invitation, err := h.store.Invitations().GetByToken(r.Context(), token)
	if err != nil {
		h.logger.Error("failed to get invitation", "error", err)
		WriteInternalError(w, "failed to get invitation")
		return
	}

	if invitation == nil {
		WriteError(w, http.StatusNotFound, ErrCodeNotFound, "invitation not found")
		return
	}

	// Return limited info for security
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"email":      invitation.Email,
		"status":     invitation.Status,
		"expires_at": invitation.ExpiresAt,
		"is_valid":   invitation.IsValid(),
	})
}
