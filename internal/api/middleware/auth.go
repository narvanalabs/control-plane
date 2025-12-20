package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/narvanalabs/control-plane/internal/auth"
	"github.com/narvanalabs/control-plane/internal/store"
)

// Context keys for user information.
type contextKey string

const (
	// UserIDKey is the context key for the authenticated user ID.
	UserIDKey contextKey = "user_id"
	// UserEmailKey is the context key for the authenticated user email.
	UserEmailKey contextKey = "user_email"
)

// GetUserID extracts the user ID from the request context.
func GetUserID(ctx context.Context) string {
	if v := ctx.Value(UserIDKey); v != nil {
		return v.(string)
	}
	return ""
}

// GetUserEmail extracts the user email from the request context.
func GetUserEmail(ctx context.Context) string {
	if v := ctx.Value(UserEmailKey); v != nil {
		return v.(string)
	}
	return ""
}

// AuthMiddleware handles JWT and API key authentication.
type AuthMiddleware struct {
	authService  *auth.Service
	apiKeyHeader string
	logger       *slog.Logger
}

// NewAuthMiddleware creates a new authentication middleware.
func NewAuthMiddleware(authService *auth.Service, apiKeyHeader string, logger *slog.Logger) *AuthMiddleware {
	if apiKeyHeader == "" {
		apiKeyHeader = "X-API-Key"
	}
	return &AuthMiddleware{
		authService:  authService,
		apiKeyHeader: apiKeyHeader,
		logger:       logger,
	}
}


// Authenticate is a middleware that validates JWT tokens or API keys.
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var userID, email string

		// Try API key first
		apiKey := r.Header.Get(m.apiKeyHeader)
		if apiKey != "" {
			user, err := m.authService.ValidateAPIKey(r.Context(), apiKey)
			if err != nil {
				m.logger.Debug("API key validation failed", "error", err)
				writeUnauthorized(w, "Invalid API key")
				return
			}
			userID = user.ID
			email = user.Email
		} else {
			// Try JWT token from Authorization header
			authHeader := r.Header.Get("Authorization")
			token := auth.ExtractBearerToken(authHeader)
			if token == "" {
				writeUnauthorized(w, "Missing authentication")
				return
			}

			claims, err := m.authService.ValidateToken(token)
			if err != nil {
				m.logger.Debug("JWT validation failed", "error", err)
				if err == auth.ErrExpiredToken {
					writeUnauthorized(w, "Token has expired")
					return
				}
				writeUnauthorized(w, "Invalid token")
				return
			}
			userID = claims.UserID
			email = claims.Email
		}

		// Add user info to context
		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		ctx = context.WithValue(ctx, UserEmailKey, email)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireOwnership returns a middleware that verifies the authenticated user owns the resource.
// It expects the appID to be in the URL path parameter.
func RequireOwnership(st store.Store, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r.Context())
			if userID == "" {
				writeUnauthorized(w, "Authentication required")
				return
			}

			appID := chi.URLParam(r, "appID")
			if appID == "" {
				// No app ID in path, continue
				next.ServeHTTP(w, r)
				return
			}

			// Get the app and verify ownership
			app, err := st.Apps().Get(r.Context(), appID)
			if err != nil {
				logger.Debug("failed to get app for ownership check", "error", err, "app_id", appID)
				writeNotFound(w, "Application not found")
				return
			}

			if app.OwnerID != userID {
				logger.Debug("ownership check failed",
					"user_id", userID,
					"owner_id", app.OwnerID,
					"app_id", appID,
				)
				writeForbidden(w, "Access denied")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"code":"unauthorized","message":"` + escapeJSON(message) + `"}`))
}

func writeForbidden(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(`{"code":"forbidden","message":"` + escapeJSON(message) + `"}`))
}

func writeNotFound(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`{"code":"not_found","message":"` + escapeJSON(message) + `"}`))
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
