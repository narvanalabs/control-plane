// Package ui provides the embedded web UI for the control plane.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// dist contains the built SvelteKit static files.
// The files are embedded from the dist/ directory which is created
// by running `bun run build` in the web/ directory.
//
//go:embed dist/*
var dist embed.FS

// Handler returns an http.Handler that serves the embedded web UI.
// It handles SPA routing by falling back to index.html for non-asset routes.
func Handler() http.Handler {
	// Get the dist subdirectory
	fsys, err := fs.Sub(dist, "dist")
	if err != nil {
		panic("failed to get dist subdirectory: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Remove leading slash for file lookup
		filePath := strings.TrimPrefix(path, "/")
		if filePath == "" {
			filePath = "index.html"
		}

		// Check if the file exists
		if _, err := fs.Stat(fsys, filePath); err == nil {
			// File exists, serve it directly
			fileServer.ServeHTTP(w, r)
			return
		}

		// For SPA routing: if file doesn't exist and it's not an asset path,
		// serve index.html and let client-side routing handle it
		if !isAssetPath(path) {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}

		// Asset not found - return 404
		http.NotFound(w, r)
	})
}

// isAssetPath returns true if the path appears to be a static asset.
func isAssetPath(path string) bool {
	// SvelteKit assets are typically in _app/ or have common asset extensions
	if strings.HasPrefix(path, "/_app/") {
		return true
	}

	// Common static asset extensions
	assetExtensions := []string{
		".js", ".css", ".json", ".map",
		".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico",
		".woff", ".woff2", ".ttf", ".eot",
		".mp3", ".mp4", ".webm", ".ogg",
		".pdf", ".xml", ".txt",
	}

	for _, ext := range assetExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}

	return false
}

// Available returns true if the embedded UI is available (has files).
func Available() bool {
	entries, err := dist.ReadDir("dist")
	if err != nil {
		return false
	}
	return len(entries) > 0
}




