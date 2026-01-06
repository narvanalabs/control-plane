// Package middleware provides HTTP middleware for the API.
package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/store"
)

// OrgContextKey is the context key for the organization.
const OrgContextKey contextKey = "org"

// OrgContext returns a middleware that extracts and validates organization context.
// It extracts the organization from:
// 1. X-Org-Slug header
// 2. current_org cookie
// 3. Falls back to user's default organization
//
// The middleware validates that the user is a member of the organization.
// If validation fails, it returns a forbidden error.
//
// Requirements: 3.1, 3.2, 3.3, 3.4
func OrgContext(st store.Store, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r.Context())
			if userID == "" {
				writeUnauthorized(w, "Authentication required")
				return
			}

			// Get org from header or cookie
			// Support both X-Org-ID (by ID) and X-Org-Slug (by slug)
			orgID := r.Header.Get("X-Org-ID")
			orgSlug := r.Header.Get("X-Org-Slug")
			if orgSlug == "" && orgID == "" {
				if cookie, err := r.Cookie("current_org"); err == nil {
					orgSlug = cookie.Value
				}
			}

			var org *models.Organization
			var err error

			if orgID != "" {
				// Look up by ID
				org, err = st.Orgs().Get(r.Context(), orgID)
				if err != nil {
					logger.Debug("organization not found", "id", orgID, "error", err)
					writeNotFound(w, "Organization not found")
					return
				}

				isMember, err := st.Orgs().IsMember(r.Context(), org.ID, userID)
				if err != nil {
					logger.Error("failed to check org membership", "error", err, "org_id", org.ID, "user_id", userID)
					writeInternalError(w, "Failed to verify organization membership")
					return
				}
				if !isMember {
					logger.Debug("user not member of organization",
						"user_id", userID,
						"org_id", org.ID,
					)
					writeForbidden(w, "Not a member of this organization")
					return
				}
			} else if orgSlug != "" {
				// Validate user is member of this org
				org, err = st.Orgs().GetBySlug(r.Context(), orgSlug)
				if err != nil {
					logger.Debug("organization not found", "slug", orgSlug, "error", err)
					writeNotFound(w, "Organization not found")
					return
				}

				isMember, err := st.Orgs().IsMember(r.Context(), org.ID, userID)
				if err != nil {
					logger.Error("failed to check org membership", "error", err, "org_id", org.ID, "user_id", userID)
					writeInternalError(w, "Failed to verify organization membership")
					return
				}
				if !isMember {
					logger.Debug("user not member of organization",
						"user_id", userID,
						"org_id", org.ID,
						"org_slug", orgSlug,
					)
					writeForbidden(w, "Not a member of this organization")
					return
				}
			} else {
				// Use default org for user
				org, err = st.Orgs().GetDefaultForUser(r.Context(), userID)
				if err != nil {
					logger.Error("failed to get default organization", "error", err, "user_id", userID)
					writeInternalError(w, "Failed to get default organization")
					return
				}
				if org == nil {
					logger.Error("no default organization found for user", "user_id", userID)
					writeInternalError(w, "No organization found")
					return
				}
			}

			ctx := context.WithValue(r.Context(), OrgContextKey, org)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetOrg extracts the organization from the request context.
func GetOrg(ctx context.Context) *models.Organization {
	if v := ctx.Value(OrgContextKey); v != nil {
		return v.(*models.Organization)
	}
	return nil
}

// GetOrgID extracts the organization ID from the request context.
// Returns empty string if no organization is set.
// Requirements: 3.2
func GetOrgID(ctx context.Context) string {
	if org := GetOrg(ctx); org != nil {
		return org.ID
	}
	return ""
}
