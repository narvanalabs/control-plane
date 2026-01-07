# Contributing to Narvana Control Plane

Thank you for your interest in contributing to Narvana! This document provides guidelines and instructions for contributing to the control-plane project.

## Table of Contents

- [Development Setup](#development-setup)
- [Pull Request Process](#pull-request-process)
- [Coding Standards](#coding-standards)
- [Testing Requirements](#testing-requirements)

## Development Setup

### Prerequisites

- [Nix](https://nixos.org/download.html) with flakes enabled
- Git

### Getting Started

1. **Clone the repository**
   ```bash
   git clone https://github.com/narvanalabs/control-plane.git
   cd control-plane
   ```

2. **Enter the Nix development shell**
   ```bash
   nix develop
   ```
   This automatically sets up:
   - Go 1.24+
   - PostgreSQL 15+
   - All required development tools (protoc, templ, tailwindcss, etc.)
   - Environment variables for local development

3. **Start the database and run migrations**
   ```bash
   make db-start
   make migrate-up
   ```

4. **Run all services**
   ```bash
   make dev-all
   ```
   This starts the API server, worker, and web UI using overmind.

### Available Make Commands

| Command | Description |
|---------|-------------|
| `make dev-all` | Start all services (recommended) |
| `make dev-api` | Run API server only |
| `make dev-worker` | Run build worker only |
| `make dev-web` | Run web UI in dev mode |
| `make dev-stop` | Stop all services |
| `make build` | Build all binaries |
| `make test` | Run all tests |
| `make test-unit` | Run unit tests only |
| `make test-property` | Run property-based tests |
| `make lint` | Run linter |
| `make proto` | Generate protobuf files |
| `make migrate-up` | Apply database migrations |

## Pull Request Process

### Before Submitting

1. **Create a feature branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes** following the [coding standards](#coding-standards)

3. **Run tests and linting**
   ```bash
   make test
   make lint
   ```

4. **Ensure all tests pass** before submitting

### Submitting a Pull Request

1. **Push your branch**
   ```bash
   git push origin feature/your-feature-name
   ```

2. **Open a Pull Request** against the `main` branch

3. **Fill out the PR template** with:
   - Description of changes
   - Related issue numbers (if applicable)
   - Testing performed
   - Screenshots (for UI changes)

### Review Guidelines

- PRs require at least one approval before merging
- All CI checks must pass
- Address reviewer feedback promptly
- Keep PRs focused and reasonably sized
- Squash commits before merging when appropriate

## Coding Standards

### Go Conventions

- Follow [Effective Go](https://go.dev/doc/effective_go) guidelines
- Use `gofmt` for formatting (handled by the linter)
- Keep functions focused and reasonably sized
- Use meaningful variable and function names
- Add comments for exported functions and types

### Code Organization

```
internal/           # Private application code
├── api/            # HTTP API handlers
├── auth/           # Authentication and RBAC
├── builder/        # Build system
├── grpc/           # gRPC server
├── models/         # Domain models
├── queue/          # Job queue
├── scheduler/      # Deployment scheduler
├── store/          # Database access
└── validation/     # Input validation

cmd/                # Application entry points
├── api/            # API server
├── web/            # Web UI server
└── worker/         # Build worker

pkg/                # Public packages
├── config/         # Configuration
└── logger/         # Logging
```

### Error Handling

- Return errors rather than panicking
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Use structured logging for error reporting
- Include correlation IDs in error logs

### Database

- Use parameterized queries to prevent SQL injection
- Keep migrations idempotent when possible
- Add appropriate indexes for query performance
- Use transactions for multi-step operations

## Testing Requirements

### Unit Tests

- Write unit tests for all new functions and methods
- Place tests in `*_test.go` files alongside the code
- Use table-driven tests for multiple test cases
- Mock external dependencies appropriately

Example:
```go
func TestMyFunction(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"empty input", "", ""},
        {"normal input", "hello", "HELLO"},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := MyFunction(tt.input)
            if result != tt.expected {
                t.Errorf("got %q, want %q", result, tt.expected)
            }
        })
    }
}
```

### Property-Based Tests

We use [gopter](https://github.com/leanovate/gopter) for property-based testing. Property tests verify that certain properties hold across many randomly generated inputs.

- Name property test files with `*_property_test.go` suffix
- Run property tests with `make test-property`
- Each property test should reference the requirement it validates

Example:
```go
func TestMyProperty(t *testing.T) {
    properties := gopter.NewProperties(gopter.DefaultTestParameters())
    
    // **Feature: my-feature, Property 1: Description**
    // **Validates: Requirements 1.1**
    properties.Property("property description", prop.ForAll(
        func(input string) bool {
            // Property assertion
            return len(MyFunction(input)) >= 0
        },
        gen.AnyString(),
    ))
    
    properties.TestingRun(t)
}
```

### Running Tests

```bash
# Run all tests
make test

# Run unit tests only (skip integration tests)
make test-unit

# Run property-based tests only
make test-property

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test -v ./internal/auth/...

# Run a specific test
go test -v -run TestMyFunction ./internal/mypackage/
```

### Test Coverage

- Aim for meaningful test coverage, not just high percentages
- Focus on testing critical paths and edge cases
- Integration tests should cover the deployment flow
- E2E tests should simulate real user workflows

## Questions?

If you have questions about contributing, feel free to:
- Open an issue for discussion
- Reach out to the maintainers

Thank you for contributing to Narvana!
