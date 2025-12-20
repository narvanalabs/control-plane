.PHONY: build build-api build-worker test test-unit test-property clean migrate migrate-up migrate-down lint

# Build targets
build: build-api build-worker

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
	go clean

# Migration targets (requires migrate tool)
migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down

migrate:
	migrate -path migrations -database "$(DATABASE_URL)" up

# Lint
lint:
	golangci-lint run ./...

# Development helpers
run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker
