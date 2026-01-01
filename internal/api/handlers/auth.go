// Package handlers provides HTTP handlers for the API.
package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/narvanalabs/control-plane/internal/auth"
	"github.com/narvanalabs/control-plane/internal/store"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	store       store.Store
	authService *auth.Service
	rbacService *auth.RBACService
	logger      *slog.Logger
	
	// Device auth state (in-memory for simplicity)
	deviceCodes   map[string]*deviceAuthState
	deviceCodesMu sync.RWMutex
}

type deviceAuthState struct {
	UserCode  string    `json:"user_code"`
	ExpiresAt time.Time `json:"expires_at"`
	Token     string    `json:"token,omitempty"`
	Approved  bool      `json:"approved"`
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(st store.Store, authSvc *auth.Service, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{
		store:       st,
		authService: authSvc,
		rbacService: auth.NewRBACService(st, logger),
		logger:      logger,
		deviceCodes: make(map[string]*deviceAuthState),
	}
}

// SetupCheck returns whether initial setup is complete.
func (h *AuthHandler) SetupCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// Check if any users exist
	users, err := h.store.Users().List(ctx)
	if err != nil {
		h.logger.Error("failed to check users", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"setup_complete": len(users) > 0,
		"user_count":     len(users),
	})
}

// CanRegister returns whether public registration is allowed.
func (h *AuthHandler) CanRegister(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	canRegister, err := h.rbacService.CanRegister(ctx)
	if err != nil {
		h.logger.Error("failed to check registration status", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"can_register": canRegister,
	})
}


// Register handles user registration (only allowed if no users exist or for additional users).
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	
	if req.Email == "" || req.Password == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password required"})
		return
	}
	
	if len(req.Password) < 8 {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}
	
	ctx := r.Context()
	
	// Check if public registration is allowed (no owner exists)
	canRegister, err := h.rbacService.CanRegister(ctx)
	if err != nil {
		h.logger.Error("failed to check registration status", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	
	if !canRegister {
		// Owner exists, public registration is disabled
		WriteJSON(w, http.StatusForbidden, map[string]string{
			"error": "registration_disabled",
			"message": "Public registration is disabled. Please contact an administrator for an invitation.",
		})
		return
	}
	
	// Check if email already exists
	existing, _ := h.store.Users().GetByEmail(ctx, req.Email)
	if existing != nil {
		WriteJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
		return
	}
	
	// First user becomes owner, subsequent users become members
	// (but this path is only reached when no owner exists)
	role := store.RoleOwner
	
	// Create user with role
	user, err := h.store.Users().CreateWithRole(ctx, req.Email, req.Password, role, "")
	if err != nil {
		h.logger.Error("failed to create user", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create user"})
		return
	}
	
	// Generate token
	token, err := h.authService.GenerateToken(user.ID, user.Email)
	if err != nil {
		h.logger.Error("failed to generate token", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}
	
	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"user_id":  user.ID,
		"email":    user.Email,
		"token":    token,
		"role":     user.Role,
		"is_admin": user.Role == store.RoleOwner,
	})
}

// Login handles user login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	
	if req.Email == "" || req.Password == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password required"})
		return
	}
	
	ctx := r.Context()
	
	// Verify credentials
	user, err := h.store.Users().Authenticate(ctx, req.Email, req.Password)
	if err != nil {
		WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	
	// Generate token
	token, err := h.authService.GenerateToken(user.ID, user.Email)
	if err != nil {
		h.logger.Error("failed to generate token", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}
	
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"user_id": user.ID,
		"email":   user.Email,
		"token":   token,
	})
}


// DeviceAuthStart initiates device authorization flow (for CLI).
func (h *AuthHandler) DeviceAuthStart(w http.ResponseWriter, r *http.Request) {
	// Generate device code and user code
	deviceCode := generateCode(32)
	userCode := generateCode(6)
	
	h.deviceCodesMu.Lock()
	h.deviceCodes[deviceCode] = &deviceAuthState{
		UserCode:  userCode,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	h.deviceCodesMu.Unlock()
	
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"device_code":      deviceCode,
		"user_code":        userCode,
		"expires_in":       600,
		"interval":         5,
		"verification_uri": "/auth/device",
	})
}

// DeviceAuthPoll polls for device authorization status (CLI calls this).
func (h *AuthHandler) DeviceAuthPoll(w http.ResponseWriter, r *http.Request) {
	deviceCode := r.URL.Query().Get("device_code")
	if deviceCode == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "device_code required"})
		return
	}
	
	h.deviceCodesMu.RLock()
	state, exists := h.deviceCodes[deviceCode]
	h.deviceCodesMu.RUnlock()
	
	if !exists {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "invalid_device_code"})
		return
	}
	
	if time.Now().After(state.ExpiresAt) {
		h.deviceCodesMu.Lock()
		delete(h.deviceCodes, deviceCode)
		h.deviceCodesMu.Unlock()
		WriteJSON(w, http.StatusGone, map[string]string{"error": "expired_token"})
		return
	}
	
	if !state.Approved {
		WriteJSON(w, http.StatusAccepted, map[string]string{"error": "authorization_pending"})
		return
	}
	
	// Clean up and return token
	h.deviceCodesMu.Lock()
	delete(h.deviceCodes, deviceCode)
	h.deviceCodesMu.Unlock()
	
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"token": state.Token,
	})
}

// DeviceAuthApprove approves a device auth request (called from web UI after login).
func (h *AuthHandler) DeviceAuthApprove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserCode string `json:"user_code"`
		Token    string `json:"token"` // User's token from login
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	
	// Validate the user's token
	claims, err := h.authService.ValidateToken(req.Token)
	if err != nil {
		WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return
	}
	
	// Find the device code by user code
	h.deviceCodesMu.Lock()
	defer h.deviceCodesMu.Unlock()
	
	for _, state := range h.deviceCodes {
		if state.UserCode == req.UserCode && !state.Approved {
			// Generate a new token for the CLI
			cliToken, err := h.authService.GenerateToken(claims.UserID, claims.Email)
			if err != nil {
				WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
				return
			}
			state.Token = cliToken
			state.Approved = true
			WriteJSON(w, http.StatusOK, map[string]string{"status": "approved"})
			return
		}
	}
	
	WriteJSON(w, http.StatusNotFound, map[string]string{"error": "invalid user code"})
}

func generateCode(length int) string {
	bytes := make([]byte, length/2+1)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}
