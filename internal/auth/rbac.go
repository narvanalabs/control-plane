// Package auth provides authentication and authorization services.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// RBAC errors.
var (
	ErrOwnerExists         = errors.New("owner already exists, public registration is disabled")
	ErrPermissionDenied    = errors.New("permission denied")
	ErrInvalidRole         = errors.New("invalid role")
	ErrCannotRemoveOwner   = errors.New("cannot remove the only owner")
	ErrUserNotFound        = errors.New("user not found")
	ErrInvitationNotFound  = errors.New("invitation not found")
	ErrInvitationExpired   = errors.New("invitation has expired")
	ErrInvitationUsed      = errors.New("invitation has already been used")
	ErrEmailAlreadyInvited = errors.New("email has already been invited")
)

// Permission represents an action that can be performed.
type Permission string

const (
	// PermissionManageUsers allows managing users (invite, remove, change roles).
	PermissionManageUsers Permission = "manage_users"
	// PermissionManageSettings allows managing system settings.
	PermissionManageSettings Permission = "manage_settings"
	// PermissionViewUsers allows viewing the user list.
	PermissionViewUsers Permission = "view_users"
	// PermissionManageApps allows creating and deleting apps.
	PermissionManageApps Permission = "manage_apps"
	// PermissionViewApps allows viewing apps.
	PermissionViewApps Permission = "view_apps"
	// PermissionDeploy allows deploying services.
	PermissionDeploy Permission = "deploy"
)

// InvitationExpiry is the default duration for invitation validity.
const InvitationExpiry = 7 * 24 * time.Hour // 7 days

// rolePermissions defines which permissions each role has.
var rolePermissions = map[store.Role][]Permission{
	store.RoleOwner: {
		PermissionManageUsers,
		PermissionManageSettings,
		PermissionViewUsers,
		PermissionManageApps,
		PermissionViewApps,
		PermissionDeploy,
	},
	store.RoleMember: {
		PermissionViewApps,
		PermissionDeploy,
	},
}

// RBACService provides role-based access control functionality.
type RBACService struct {
	store  store.Store
	logger *slog.Logger
}

// NewRBACService creates a new RBAC service.
func NewRBACService(st store.Store, logger *slog.Logger) *RBACService {
	if logger == nil {
		logger = slog.Default()
	}
	return &RBACService{
		store:  st,
		logger: logger,
	}
}

// CanRegister checks if public registration is allowed.
// Returns true if no owner exists, false otherwise.
func (s *RBACService) CanRegister(ctx context.Context) (bool, error) {
	count, err := s.store.Users().CountByRole(ctx, store.RoleOwner)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// CheckPermission verifies a user has permission for an action.
func (s *RBACService) CheckPermission(ctx context.Context, userID string, permission Permission) error {
	user, err := s.store.Users().GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}
	return CheckRolePermission(user.Role, permission)
}

// CheckRolePermission checks if a role has a specific permission.
func CheckRolePermission(role store.Role, permission Permission) error {
	permissions, ok := rolePermissions[role]
	if !ok {
		return ErrPermissionDenied
	}
	for _, p := range permissions {
		if p == permission {
			return nil
		}
	}
	return ErrPermissionDenied
}

// CreateInvitation creates an invitation for a new user.
func (s *RBACService) CreateInvitation(ctx context.Context, email string, invitedBy string, role store.Role) (*models.Invitation, error) {
	// Check if email is already invited
	existing, _ := s.store.Invitations().GetByEmail(ctx, email)
	if existing != nil && existing.Status == models.InvitationStatusPending {
		return nil, ErrEmailAlreadyInvited
	}

	// Check if user already exists
	existingUser, _ := s.store.Users().GetByEmail(ctx, email)
	if existingUser != nil {
		return nil, errors.New("user with this email already exists")
	}

	// Generate token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)

	invitation := &models.Invitation{
		Email:     email,
		Token:     token,
		InvitedBy: invitedBy,
		Role:      models.Role(role),
		Status:    models.InvitationStatusPending,
		ExpiresAt: time.Now().Add(InvitationExpiry),
		CreatedAt: time.Now(),
	}

	if err := s.store.Invitations().Create(ctx, invitation); err != nil {
		return nil, err
	}

	return invitation, nil
}

// AcceptInvitation accepts an invitation and creates a user.
func (s *RBACService) AcceptInvitation(ctx context.Context, token, password string) (*store.User, error) {
	invitation, err := s.store.Invitations().GetByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if invitation == nil {
		return nil, ErrInvitationNotFound
	}
	if invitation.Status != models.InvitationStatusPending {
		return nil, ErrInvitationUsed
	}
	if invitation.IsExpired() {
		return nil, ErrInvitationExpired
	}

	// Create user with the invited role
	user, err := s.store.Users().CreateWithRole(ctx, invitation.Email, password, store.Role(invitation.Role), invitation.InvitedBy)
	if err != nil {
		return nil, err
	}

	// Mark invitation as accepted
	now := time.Now()
	invitation.Status = models.InvitationStatusAccepted
	invitation.AcceptedAt = &now
	if err := s.store.Invitations().Update(ctx, invitation); err != nil {
		s.logger.Error("failed to update invitation status", "error", err)
	}

	return user, nil
}

// RevokeInvitation revokes a pending invitation.
func (s *RBACService) RevokeInvitation(ctx context.Context, invitationID string) error {
	invitation, err := s.store.Invitations().Get(ctx, invitationID)
	if err != nil {
		return err
	}
	if invitation == nil {
		return ErrInvitationNotFound
	}
	if invitation.Status != models.InvitationStatusPending {
		return errors.New("can only revoke pending invitations")
	}

	invitation.Status = models.InvitationStatusRevoked
	return s.store.Invitations().Update(ctx, invitation)
}

// ListInvitations returns all invitations.
func (s *RBACService) ListInvitations(ctx context.Context) ([]*models.Invitation, error) {
	return s.store.Invitations().List(ctx)
}

// RemoveUser removes a user from the system.
func (s *RBACService) RemoveUser(ctx context.Context, userID string) error {
	user, err := s.store.Users().GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	// Check if this is the only owner
	if user.Role == store.RoleOwner {
		count, err := s.store.Users().CountByRole(ctx, store.RoleOwner)
		if err != nil {
			return err
		}
		if count <= 1 {
			return ErrCannotRemoveOwner
		}
	}

	return s.store.Users().Delete(ctx, userID)
}
