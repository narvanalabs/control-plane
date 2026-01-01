package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"message": "Hello from Go backend!"}`)
	})

	fmt.Println("Backend starting on :8080")
	http.ListenAndServe(":8080", nil)
}
