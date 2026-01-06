// Package models provides data structures for the Narvana platform.
package models

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// Role represents a user's role within an organization.
type Role string

const (
	RoleOwner  Role = "owner"  // Full access, can invite users
	RoleMember Role = "member" // Standard access, no admin functions
)

// Organization represents a top-level grouping for all resources.
type Organization struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"` // URL-friendly identifier
	Description string    `json:"description,omitempty"`
	IconURL     string    `json:"icon_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// OrgMembership links users to organizations with roles.
type OrgMembership struct {
	OrgID     string    `json:"org_id"`
	UserID    string    `json:"user_id"`
	Role      Role      `json:"role"` // owner or member
	CreatedAt time.Time `json:"created_at"`
}

// Validation errors for organizations.
var (
	ErrOrgNameRequired = errors.New("organization name is required")
	ErrOrgNameTooLong  = errors.New("organization name must be 63 characters or less")
	ErrOrgSlugRequired = errors.New("organization slug is required")
	ErrOrgSlugTooLong  = errors.New("organization slug must be 63 characters or less")
	ErrOrgSlugInvalid  = errors.New("organization slug must contain only lowercase letters, numbers, and hyphens")
	ErrOrgSlugStartEnd = errors.New("organization slug must start and end with a letter or number")
	ErrLastOrgDelete   = errors.New("cannot delete the last organization")
)

// slugPattern matches valid slug characters: lowercase letters, numbers, and hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

// ValidateName validates the organization name.
func (o *Organization) ValidateName() error {
	name := strings.TrimSpace(o.Name)
	if name == "" {
		return ErrOrgNameRequired
	}
	if len(name) > 63 {
		return ErrOrgNameTooLong
	}
	return nil
}

// ValidateSlug validates the organization slug.
func (o *Organization) ValidateSlug() error {
	slug := strings.TrimSpace(o.Slug)
	if slug == "" {
		return ErrOrgSlugRequired
	}
	if len(slug) > 63 {
		return ErrOrgSlugTooLong
	}
	if !slugPattern.MatchString(slug) {
		// Check if it's a start/end issue vs invalid characters
		if len(slug) > 0 && (slug[0] == '-' || slug[len(slug)-1] == '-') {
			return ErrOrgSlugStartEnd
		}
		return ErrOrgSlugInvalid
	}
	return nil
}

// Validate validates the organization fields.
func (o *Organization) Validate() error {
	if err := o.ValidateName(); err != nil {
		return err
	}
	if err := o.ValidateSlug(); err != nil {
		return err
	}
	return nil
}

// GenerateSlug generates a URL-friendly slug from the organization name.
func GenerateSlug(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(strings.TrimSpace(name))

	// Replace spaces and underscores with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")

	// Remove any characters that aren't lowercase letters, numbers, or hyphens
	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	slug = result.String()

	// Remove consecutive hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}

	// Trim leading and trailing hyphens
	slug = strings.Trim(slug, "-")

	// Truncate to 63 characters
	if len(slug) > 63 {
		slug = slug[:63]
		// Ensure we don't end with a hyphen after truncation
		slug = strings.TrimRight(slug, "-")
	}

	return slug
}
