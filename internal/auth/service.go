// Package auth provides authentication and authorization services.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Common errors returned by the auth service.
var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token has expired")
	ErrInvalidAPIKey    = errors.New("invalid API key")
	ErrMissingClaims    = errors.New("missing required claims")
	ErrInvalidSignature = errors.New("invalid token signature")
)

// User represents an authenticated user.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// Claims represents the JWT claims structure.
type Claims struct {
	UserID string    `json:"user_id"`
	Email  string    `json:"email"`
	Exp    time.Time `json:"exp"`
}

// APIKey represents a stored API key.
type APIKey struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	KeyHash   string    `json:"-"` // SHA256 hash of the key
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// APIKeyStore defines the interface for API key storage.
type APIKeyStore interface {
	// GetByHash retrieves an API key by its hash.
	GetByHash(ctx context.Context, hash string) (*APIKey, error)
	// Create creates a new API key.
	Create(ctx context.Context, key *APIKey) error
	// Delete removes an API key.
	Delete(ctx context.Context, id string) error
	// ListByUser retrieves all API keys for a user.
	ListByUser(ctx context.Context, userID string) ([]*APIKey, error)
}

// Config holds authentication configuration.
type Config struct {
	JWTSecret   []byte
	TokenExpiry time.Duration
}

// Service provides authentication and authorization functionality.
type Service struct {
	jwtSecret   []byte
	tokenExpiry time.Duration
	apiKeyStore APIKeyStore
	logger      *slog.Logger
}

// NewService creates a new authentication service.
func NewService(cfg *Config, apiKeyStore APIKeyStore, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		jwtSecret:   cfg.JWTSecret,
		tokenExpiry: cfg.TokenExpiry,
		apiKeyStore: apiKeyStore,
		logger:      logger,
	}
}

// GenerateToken creates a new JWT token for the given user.
func (s *Service) GenerateToken(userID, email string) (string, error) {
	if userID == "" {
		return "", ErrMissingClaims
	}

	now := time.Now()
	exp := now.Add(s.tokenExpiry)

	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"iat":   now.Unix(),
		"exp":   exp.Unix(),
		"nbf":   now.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(s.jwtSecret)
	if err != nil {
		s.logger.Error("failed to sign token", "error", err)
		return "", fmt.Errorf("signing token: %w", err)
	}

	return signedToken, nil
}

// ValidateToken validates a JWT token and returns the claims.
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, ErrInvalidToken
	}

	// Parse and validate the token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		if errors.Is(err, jwt.ErrSignatureInvalid) {
			return nil, ErrInvalidSignature
		}
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	// Extract claims
	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	// Extract user ID from "sub" claim
	userID, ok := mapClaims["sub"].(string)
	if !ok || userID == "" {
		return nil, ErrMissingClaims
	}

	// Extract email (optional)
	email, _ := mapClaims["email"].(string)

	// Extract expiration
	expFloat, ok := mapClaims["exp"].(float64)
	if !ok {
		return nil, ErrMissingClaims
	}
	exp := time.Unix(int64(expFloat), 0)

	return &Claims{
		UserID: userID,
		Email:  email,
		Exp:    exp,
	}, nil
}

// ValidateAPIKey validates an API key and returns the associated user.
func (s *Service) ValidateAPIKey(ctx context.Context, apiKey string) (*User, error) {
	if apiKey == "" {
		return nil, ErrInvalidAPIKey
	}

	// Hash the provided key
	hash := HashAPIKey(apiKey)

	// Look up the key in the store
	if s.apiKeyStore == nil {
		return nil, ErrInvalidAPIKey
	}

	storedKey, err := s.apiKeyStore.GetByHash(ctx, hash)
	if err != nil {
		s.logger.Debug("API key lookup failed", "error", err)
		return nil, ErrInvalidAPIKey
	}

	if storedKey == nil {
		return nil, ErrInvalidAPIKey
	}

	// Check if the key has expired
	if !storedKey.ExpiresAt.IsZero() && time.Now().After(storedKey.ExpiresAt) {
		return nil, ErrExpiredToken
	}

	return &User{
		ID: storedKey.UserID,
	}, nil
}

// GenerateAPIKey generates a new API key and returns the raw key.
// The raw key should be shown to the user once and never stored.
func GenerateAPIKey() (string, error) {
	// Generate 32 random bytes
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}

	// Encode as base64 with a prefix for identification
	key := "nrv_" + base64.RawURLEncoding.EncodeToString(bytes)
	return key, nil
}

// HashAPIKey creates a SHA256 hash of an API key for storage.
func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// ExtractBearerToken extracts the token from a Bearer authorization header.
func ExtractBearerToken(authHeader string) string {
	if authHeader == "" {
		return ""
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return strings.TrimSpace(parts[1])
}

// SecureCompare performs a constant-time comparison of two strings.
// This helps prevent timing attacks.
func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
