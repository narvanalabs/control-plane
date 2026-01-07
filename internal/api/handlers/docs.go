// Package handlers provides HTTP request handlers for the API.
package handlers

import (
	"embed"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed docs/*
var docsFS embed.FS

// DocsHandler handles API documentation endpoints.
type DocsHandler struct {
	logger      *slog.Logger
	specPath    string
	swaggerHTML *template.Template
}

// NewDocsHandler creates a new docs handler.
func NewDocsHandler(logger *slog.Logger) *DocsHandler {
	// Find the OpenAPI spec file
	specPath := findOpenAPISpec()

	// Parse the Swagger UI HTML template
	tmpl := template.Must(template.New("swagger").Parse(swaggerUITemplate))

	return &DocsHandler{
		logger:      logger,
		specPath:    specPath,
		swaggerHTML: tmpl,
	}
}

// findOpenAPISpec finds the OpenAPI specification file.
func findOpenAPISpec() string {
	// Try common locations
	paths := []string{
		"api/openapi.yaml",
		"../api/openapi.yaml",
		"../../api/openapi.yaml",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Default to api/openapi.yaml
	return "api/openapi.yaml"
}

// ServeSwaggerUI serves the Swagger UI at /api/docs.
func (h *DocsHandler) ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := struct {
		SpecURL string
		Title   string
	}{
		SpecURL: "/api/docs/openapi.yaml",
		Title:   "Narvana Control Plane API",
	}

	if err := h.swaggerHTML.Execute(w, data); err != nil {
		h.logger.Error("failed to render Swagger UI", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// ServeOpenAPISpec serves the OpenAPI specification file at /api/docs/openapi.yaml.
func (h *DocsHandler) ServeOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	// Try to read from embedded FS first
	data, err := docsFS.ReadFile("docs/openapi.yaml")
	if err == nil {
		w.Header().Set("Content-Type", "application/yaml")
		w.Write(data)
		return
	}

	// Fall back to file system
	data, err = os.ReadFile(h.specPath)
	if err != nil {
		h.logger.Error("failed to read OpenAPI spec", "error", err, "path", h.specPath)
		http.Error(w, "OpenAPI specification not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/yaml")
	w.Write(data)
}

// ServeDocsAssets serves static assets for the documentation.
func (h *DocsHandler) ServeDocsAssets() http.Handler {
	subFS, err := fs.Sub(docsFS, "docs")
	if err != nil {
		h.logger.Error("failed to create sub filesystem", "error", err)
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(subFS))
}

// swaggerUITemplate is the HTML template for Swagger UI.
// Uses the official Swagger UI from CDN for simplicity.
const swaggerUITemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
    <style>
        html {
            box-sizing: border-box;
            overflow: -moz-scrollbars-vertical;
            overflow-y: scroll;
        }
        *,
        *:before,
        *:after {
            box-sizing: inherit;
        }
        body {
            margin: 0;
            background: #fafafa;
        }
        .swagger-ui .topbar {
            background-color: #f97316;
        }
        .swagger-ui .topbar .download-url-wrapper .select-label {
            color: white;
        }
        .swagger-ui .info .title {
            color: #333;
        }
        .swagger-ui .opblock.opblock-get .opblock-summary-method {
            background: #61affe;
        }
        .swagger-ui .opblock.opblock-post .opblock-summary-method {
            background: #49cc90;
        }
        .swagger-ui .opblock.opblock-put .opblock-summary-method {
            background: #fca130;
        }
        .swagger-ui .opblock.opblock-delete .opblock-summary-method {
            background: #f93e3e;
        }
        .swagger-ui .opblock.opblock-patch .opblock-summary-method {
            background: #50e3c2;
        }
        .swagger-ui .btn.authorize {
            background-color: #f97316;
            border-color: #f97316;
            color: white;
        }
        .swagger-ui .btn.authorize:hover {
            background-color: #ea580c;
            border-color: #ea580c;
        }
        .swagger-ui .btn.authorize svg {
            fill: white;
        }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            const ui = SwaggerUIBundle({
                url: "{{.SpecURL}}",
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout",
                persistAuthorization: true,
                displayRequestDuration: true,
                filter: true,
                showExtensions: true,
                showCommonExtensions: true,
                tryItOutEnabled: true
            });
            window.ui = ui;
        };
    </script>
</body>
</html>`

// CopyOpenAPISpecToDocsDir copies the OpenAPI spec to the docs directory for embedding.
// This is typically called during build time.
func CopyOpenAPISpecToDocsDir() error {
	srcPath := "api/openapi.yaml"
	dstDir := filepath.Join("internal", "api", "handlers", "docs")
	dstPath := filepath.Join(dstDir, "openapi.yaml")

	// Create docs directory if it doesn't exist
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	// Read source file
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	// Write to destination
	return os.WriteFile(dstPath, data, 0644)
}
