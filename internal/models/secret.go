package models

import "time"

// Secret represents an encrypted secret associated with an application.
type Secret struct {
	ID             string    `json:"id"`
	AppID          string    `json:"app_id"`
	Key            string    `json:"key"`
	EncryptedValue []byte    `json:"-"` // Excluded from JSON serialization
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
