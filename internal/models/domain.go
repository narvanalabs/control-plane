package models

import "time"

// Domain represents a custom domain mapping for a service.
type Domain struct {
	ID         string    `json:"id"`
	AppID      string    `json:"app_id"`
	Service    string    `json:"service"`
	Domain     string    `json:"domain"`
	IsWildcard bool      `json:"is_wildcard"`
	Verified   bool      `json:"verified"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
