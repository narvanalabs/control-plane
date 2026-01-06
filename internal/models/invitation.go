// Package models provides data structures for the Narvana platform.
package models

import (
	"time"
)

// InvitationStatus represents the status of an invitation.
type InvitationStatus string

const (
	// InvitationStatusPending indicates the invitation has not been accepted.
	InvitationStatusPending InvitationStatus = "pending"
	// InvitationStatusAccepted indicates the invitation has been accepted.
	InvitationStatusAccepted InvitationStatus = "accepted"
	// InvitationStatusExpired indicates the invitation has expired.
	InvitationStatusExpired InvitationStatus = "expired"
	// InvitationStatusRevoked indicates the invitation was revoked.
	InvitationStatusRevoked InvitationStatus = "revoked"
)

// Invitation represents an invitation to join the platform.
type Invitation struct {
	ID         string           `json:"id"`
	Email      string           `json:"email"`
	Token      string           `json:"token"` // Unique token for accepting the invitation
	InvitedBy  string           `json:"invited_by"`
	Role       Role             `json:"role"`
	Status     InvitationStatus `json:"status"`
	ExpiresAt  time.Time        `json:"expires_at"`
	AcceptedAt *time.Time       `json:"accepted_at,omitempty"`
	CreatedAt  time.Time        `json:"created_at"`
}

// IsExpired returns true if the invitation has expired.
func (i *Invitation) IsExpired() bool {
	return time.Now().After(i.ExpiresAt)
}

// IsValid returns true if the invitation can be accepted.
func (i *Invitation) IsValid() bool {
	return i.Status == InvitationStatusPending && !i.IsExpired()
}
