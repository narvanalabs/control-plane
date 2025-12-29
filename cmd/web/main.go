package main

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/narvanalabs/control-plane/web"
)

func main() {
	// Serve CSS assets
	http.Handle("/assets/css/", http.StripPrefix("/assets/css/",
		http.FileServer(http.Dir("./web/assets/css"))))

	// Serve JS assets (for templui components)
	http.Handle("/assets/js/", http.StripPrefix("/assets/js/",
		http.FileServer(http.Dir("./web/assets/js"))))

	// Serve home page
	http.Handle("/", templ.Handler(web.Home()))

	fmt.Println("Web UI running on http://localhost:8090")
	http.ListenAndServe(":8090", nil)
}
