package main

/*
#include <stdlib.h>
#include <stdio.h>

void hello_from_c() {
    printf("Hello from C!\n");
}
*/
import "C"

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// Call C function
	C.hello_from_c()

	// Use SQLite (requires CGO)
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("CGO application running successfully!")
}
