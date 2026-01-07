.PHONY: build build-api build-worker build-ui test test-unit test-property clean migrate migrate-up migrate-down lint proto dev dev-api dev-worker dev-web dev-all stop-db help

# Proto generation
proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/agent.proto
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/controlplane.proto

# Build targets
build: build-ui build-api build-worker

build-ui:
	@echo "Building web UI..."
	cd web && templ generate
	cd web && tailwindcss -i ./assets/css/input.css -o ./assets/css/output.css --minify
	go build -o bin/web ./cmd/web
	@echo "Web UI built to bin/web"

build-api:
	go build -o bin/api ./cmd/api

build-worker:
	go build -o bin/worker ./cmd/worker

# Test targets
test:
	go test -v ./...

test-unit:
	go test -v -short ./...

test-property:
	go test -v -run Property ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf ui/dist/*
	touch ui/dist/.gitkeep
	go clean

# Database targets (requires nix develop shell)
db-start:
	@if [ -z "$$PGDATA" ]; then \
		echo "Error: Run 'nix develop' first to set up PostgreSQL environment"; \
		exit 1; \
	fi
	@if ! pg_isready -q 2>/dev/null; then \
		echo "Starting PostgreSQL..."; \
		pg_ctl start -l "$$PGDATA/postgres.log" -o "-k $$PGHOST -h ''"; \
		sleep 2; \
	else \
		echo "PostgreSQL already running"; \
	fi

db-stop:
	@if [ -n "$$PGDATA" ] && pg_isready -q 2>/dev/null; then \
		echo "Stopping PostgreSQL..."; \
		pg_ctl stop; \
	fi

db-status:
	@pg_isready && echo "PostgreSQL is running" || echo "PostgreSQL is not running"

# Migration targets (requires nix develop shell + running db)
migrate-up: db-start
	@if [ -z "$$DATABASE_URL" ]; then \
		echo "Error: Run 'nix develop' first"; \
		exit 1; \
	fi
	@echo "Applying migrations..."; \
	for f in migrations/*.sql; do \
		echo "Applying $$f..."; \
		psql $$DATABASE_URL -f "$$f" || exit 1; \
	done; \
	echo "Migrations complete"

migrate-down:
	@echo "Manual rollback required - check migrations/ for down scripts"

migrate: migrate-up

# Lint
lint:
	golangci-lint run ./...

# OpenAPI validation
# Requirements: 9.5
validate-openapi:
	@echo "Validating OpenAPI specification..."
	go run scripts/validate-openapi.go

# Copy OpenAPI spec to docs directory for embedding
copy-openapi:
	@echo "Copying OpenAPI spec to docs directory..."
	mkdir -p internal/api/handlers/docs
	cp api/openapi.yaml internal/api/handlers/docs/

# Development targets (run inside nix develop)

# Run all services with overmind (single terminal!)
dev-all: db-start migrate-up
	@echo "Starting all services with overmind..."
	@echo "Press Ctrl+C to stop all services"
	@echo ""
	overmind start -f Procfile.dev

# Run individual services (foreground)
dev-api: db-start
	go run ./cmd/api

dev-worker: db-start
	go run ./cmd/worker

dev-web:
	cd web && task dev

# Stop services
dev-stop:
	@overmind quit 2>/dev/null || true
	@pkill -f "go run ./cmd/api" 2>/dev/null || true
	@pkill -f "go run ./cmd/worker" 2>/dev/null || true
	@echo "Services stopped"

# Legacy aliases
run-api: dev-api
run-worker: dev-worker

# Help
help:
	@echo "Narvana Control Plane"
	@echo ""
	@echo "Prerequisites: Run 'nix develop' first to enter dev shell"
	@echo ""
	@echo "Quick Start:"
	@echo "  make dev-all     - Run ALL services in one terminal (recommended)"
	@echo ""
	@echo "Individual Services:"
	@echo "  make dev-api     - Run API server only"
	@echo "  make dev-worker  - Run build worker only"
	@echo "  make dev-web     - Run web UI in dev mode (separate from API)"
	@echo "  make dev-stop    - Stop all services"
	@echo ""
	@echo "Database:"
	@echo "  make db-start    - Start PostgreSQL"
	@echo "  make db-stop     - Stop PostgreSQL"
	@echo "  make migrate-up  - Run migrations"
	@echo ""
	@echo "Build & Test:"
	@echo "  make build       - Build all binaries (includes web UI)"
	@echo "  make build-ui    - Build web UI only"
	@echo "  make test        - Run all tests"
	@echo "  make lint        - Run linter"
	@echo ""
	@echo "API Documentation:"
	@echo "  make validate-openapi - Validate OpenAPI specification"
	@echo "  make copy-openapi     - Copy OpenAPI spec for embedding"
