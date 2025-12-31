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

	rows, err := db.QueryContext(context.Background(), "SELECT column_name FROM information_schema.columns WHERE table_name = 'apps'")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	fmt.Println("Columns in 'apps':")
	for rows.Next() {
		var name string
		rows.Scan(&name)
		fmt.Println("- " + name)
	}
}
