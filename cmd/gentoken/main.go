// Package main provides a simple tool to generate JWT tokens for the Narvana platform.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/narvanalabs/control-plane/internal/auth"
)

func main() {
	userID := flag.String("user", "admin", "User ID for the token")
	email := flag.String("email", "admin@localhost", "Email for the token")
	secret := flag.String("secret", "", "JWT secret (or set JWT_SECRET env var)")
	expiry := flag.Duration("expiry", 24*365*time.Hour, "Token expiry duration (default: 1 year)")
	flag.Parse()

	jwtSecret := *secret
	if jwtSecret == "" {
		jwtSecret = os.Getenv("JWT_SECRET")
	}
	if jwtSecret == "" {
		fmt.Fprintln(os.Stderr, "Error: JWT secret required. Use -secret flag or set JWT_SECRET env var")
		fmt.Fprintln(os.Stderr, "Example: go run ./cmd/gentoken -secret 'your-secret-at-least-32-chars-long'")
		os.Exit(1)
	}
	if len(jwtSecret) < 32 {
		fmt.Fprintln(os.Stderr, "Error: JWT secret must be at least 32 characters")
		os.Exit(1)
	}

	cfg := &auth.Config{
		JWTSecret:   []byte(jwtSecret),
		TokenExpiry: *expiry,
	}

	svc := auth.NewService(cfg, nil, nil)
	token, err := svc.GenerateToken(*userID, *email)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating token: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(token)
}
