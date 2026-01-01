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

	appID := "d340a467-0e0e-456c-9a95-569ea83816aa"
	fmt.Printf("Checking recent deployments for app: %s\n", appID)

	rows, err := db.QueryContext(context.Background(), 
		"SELECT id, service_name, status, created_at FROM deployments WHERE app_id = $1 ORDER BY created_at DESC LIMIT 5", 
		appID)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, status, created string
		if err := rows.Scan(&id, &name, &status, &created); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("[%s] %s: %s (%s)\n", created, name, status, id)
	}
}
