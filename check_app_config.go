//go:build ignore
// +build ignore

package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres:///testdb?host=/home/danny/dev/narvana/control-plane/.pg-socket&user=testuser"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	appID := "5e4d09c0-6771-419b-a01c-6dcb8c205566"
	fmt.Printf("Checking app config for: %s\n", appID)

	var config []byte
	err = db.QueryRowContext(context.Background(), "SELECT config FROM apps WHERE id = $1", appID).Scan(&config)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Config: %s\n", string(config))
}
