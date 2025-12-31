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

	fmt.Println("Checking ALL recent deployments...")

	rows, err := db.QueryContext(context.Background(), 
		"SELECT id, app_id, service_name, status, created_at FROM deployments ORDER BY created_at DESC LIMIT 10")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, appID, name, status, created string
		if err := rows.Scan(&id, &appID, &name, &status, &created); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("[%s] App:%s %s: %s (%s)\n", created, appID[:8], name, status, id)
	}
}
