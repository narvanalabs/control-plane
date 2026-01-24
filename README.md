# Narvana Control Plane

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Podman](https://img.shields.io/badge/Podman-Container_Runtime-892CA0?style=flat&logo=podman&logoColor=white)](https://podman.io/)
[![Nix](https://img.shields.io/badge/Nix-Flakes-5277C3?style=flat&logo=nixos&logoColor=white)](https://nixos.org/)
[![gRPC](https://img.shields.io/badge/gRPC-Protocol-244c5a?style=flat&logo=google&logoColor=white)](https://grpc.io/)
[![Tests](https://img.shields.io/badge/Tests-Passing-success?style=flat)](#running-tests)

A self-hosted Platform-as-a-Service (PaaS) control plane built with Go, designed for deploying and managing applications using Nix flakes and OCI containers. Narvana provides a modern, Nix-native approach to application deployment with automatic build detection, multi-tenancy support, and distributed node management.

## Features

- **Nix-Native Builds**: First-class support for Nix flakes with automatic flake generation for Go, Rust, Node.js, Python, and database services
- **Dual Build Modes**: Support for both pure Nix store paths and OCI container images
- **Auto-Detection**: Automatic language and framework detection with intelligent build strategy selection
- **Multi-Tenancy**: Organization-based resource isolation with role-based access control
- **Distributed Scheduling**: Intelligent node placement based on health, capacity, and cache locality
- **gRPC Node Communication**: Bidirectional streaming for real-time deployment commands and status updates
- **Binary Caching**: Integration with [Attic](https://github.com/zhaofengli/attic) for Nix binary cache acceleration
- **Secrets Management**: SOPS-based encryption for application secrets using age keys
- **Custom Domains**: Support for custom domain mappings with wildcard certificates
- **Real-time Logs**: SSE-based log streaming for builds and runtime

## Architecture

### System Overview

```mermaid
graph TB
    subgraph "Control Plane"
        WebUI["Web UI<br/>:8090"]
        API["API Server<br/>:8080 HTTP<br/>:9090 gRPC"]
        Worker["Build Worker"]
        Scheduler["Scheduler"]
    end
    
    subgraph "Data Layer"
        PG[(PostgreSQL)]
        Attic["Attic<br/>Binary Cache"]
    end
    
    subgraph "Compute Nodes"
        Node1["Node Agent 1<br/>(Podman)"]
        Node2["Node Agent 2<br/>(Podman)"]
        Node3["Node Agent N<br/>(Podman)"]
    end
    
    WebUI --> API
    API --> PG
    API --> Scheduler
    Worker --> PG
    Worker --> Attic
    Scheduler --> PG
    
    API <-->|"gRPC<br/>Bidirectional"| Node1
    API <-->|"gRPC<br/>Bidirectional"| Node2
    API <-->|"gRPC<br/>Bidirectional"| Node3
    
    Node1 --> Attic
    Node2 --> Attic
    Node3 --> Attic
```

### Deployment Flow

```mermaid
sequenceDiagram
    participant User
    participant API
    participant Queue
    participant Worker
    participant Scheduler
    participant Node
    participant Attic
    
    User->>API: POST /deploy
    API->>Queue: Enqueue build job
    API-->>User: 202 Accepted
    
    Worker->>Queue: Dequeue job
    Worker->>Worker: Detect strategy
    Worker->>Worker: Generate flake
    Worker->>Worker: Build artifact
    Worker->>Attic: Push to cache
    Worker->>API: Update status: built
    
    Scheduler->>API: Poll built deployments
    Scheduler->>Scheduler: Select optimal node
    Scheduler->>Node: Deploy command (gRPC)
    
    Node->>Attic: Pull artifact
    Node->>Node: Start container
    Node->>API: Status: running
```

### Build Strategy Selection

```mermaid
flowchart TD
    Start([Source Code]) --> HasFlake{Has flake.nix?}
    
    HasFlake -->|Yes| UseFlake[Use Flake Strategy]
    HasFlake -->|No| HasDockerfile{Has Dockerfile?}
    
    HasDockerfile -->|Yes| UseDockerfile[Use Dockerfile Strategy]
    HasDockerfile -->|No| DetectLang[Detect Language]
    
    DetectLang --> Go{Go?}
    DetectLang --> Rust{Rust?}
    DetectLang --> Node{Node.js?}
    DetectLang --> Python{Python?}
    
    Go -->|Yes| AutoGo[auto-go]
    Rust -->|Yes| AutoRust[auto-rust]
    Node -->|Yes| AutoNode[auto-node]
    Python -->|Yes| AutoPython[auto-python]
    
    Go -->|No| Rust
    Rust -->|No| Node
    Node -->|No| Python
    Python -->|No| Nixpacks[Use Nixpacks]
    
    UseFlake --> Build([Build Artifact])
    UseDockerfile --> Build
    AutoGo --> Build
    AutoRust --> Build
    AutoNode --> Build
    AutoPython --> Build
    Nixpacks --> Build
```

### Deployment State Machine

```mermaid
stateDiagram-v2
    [*] --> pending: Create deployment
    pending --> building: Worker picks up
    building --> built: Build succeeds
    building --> failed: Build fails
    built --> scheduled: Node assigned
    scheduled --> starting: Agent starts container
    starting --> running: Container healthy
    starting --> failed: Start fails
    running --> stopping: Stop requested
    stopping --> stopped: Container stopped
    running --> failed: Container crashes
    stopped --> [*]
    failed --> [*]
```

### Data Model

```mermaid
erDiagram
    Organization ||--o{ App : contains
    Organization ||--o{ User : has_members
    User ||--o{ App : owns
    App ||--o{ Service : contains
    App ||--o{ Secret : has
    App ||--o{ Domain : has
    Service ||--o{ Deployment : has
    Deployment ||--o| BuildJob : triggers
    Deployment ||--o| Node : runs_on
    Node ||--o{ Deployment : hosts
    
    Organization {
        uuid id PK
        string name
        string slug UK
        string description
    }
    
    App {
        uuid id PK
        uuid org_id FK
        string owner_id FK
        string name
        int version
        jsonb services
    }
    
    Deployment {
        uuid id PK
        uuid app_id FK
        string service_name
        int version
        string status
        uuid node_id FK
        string artifact
    }
    
    Node {
        uuid id PK
        string hostname
        string address
        boolean healthy
        jsonb resources
    }
```

## Requirements

- [Nix](https://nixos.org/download.html) with flakes enabled
- Go 1.24+
- PostgreSQL 15+
- Podman (for OCI builds and container runtime)

## Quick Start

### Using Nix (Recommended)

```bash
# Clone the repository
git clone https://github.com/narvanalabs/control-plane.git
cd control-plane

# Enter the development shell (starts PostgreSQL automatically)
nix develop

# Run database migrations
make migrate-up

# Start all services (API, Worker, Web UI, Attic cache)
make dev-all
```

The development environment will be available at:
- Web UI: http://localhost:8090
- API: http://localhost:8080
- gRPC: localhost:9090

### Manual Setup

```bash
# Install dependencies
go mod download

# Set required environment variables
export DATABASE_URL="postgres://user:pass@localhost:5432/narvana?sslmode=disable"
export JWT_SECRET="your-secret-key-minimum-32-characters-long"

# Run migrations
make migrate-up

# Build and run
make build
./bin/api &
./bin/worker &
./bin/web &
```

## Configuration

Configuration is managed through environment variables:

### Core Settings

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://localhost:5432/narvana?sslmode=disable` |
| `JWT_SECRET` | Secret key for JWT tokens (min 32 chars) | Required |
| `JWT_EXPIRY` | Token expiration duration | `24h` |
| `API_PORT` | HTTP API server port | `8080` |
| `GRPC_PORT` | gRPC server port | `9090` |
| `API_HOST` | API server bind address | `0.0.0.0` |

### Build Worker Settings

| Variable | Description | Default |
|----------|-------------|---------|
| `WORKER_WORKDIR` | Build working directory | `/tmp/narvana-builds` |
| `WORKER_MAX_CONCURRENCY` | Max concurrent builds | `4` |
| `BUILD_TIMEOUT` | Build timeout duration | `30m` |
| `PODMAN_SOCKET` | Podman socket path | `unix:///run/user/1000/podman/podman.sock` |
| `ATTIC_ENDPOINT` | Attic binary cache URL | `http://localhost:5000` |

### Scheduler Settings

| Variable | Description | Default |
|----------|-------------|---------|
| `SCHEDULER_HEALTH_THRESHOLD` | Node health check threshold | `30s` |
| `SCHEDULER_MAX_RETRIES` | Max deployment retries | `5` |
| `SCHEDULER_RETRY_BACKOFF` | Retry backoff duration | `5s` |
| `SCHEDULER_DEPLOYMENT_TIMEOUT` | Deployment scheduling timeout | `30m` |

### Secrets Encryption (SOPS)

| Variable | Description | Default |
|----------|-------------|---------|
| `SOPS_AGE_PUBLIC_KEY` | Age public key for encryption | Optional |
| `SOPS_AGE_PRIVATE_KEY` | Age private key for decryption | Optional |

## Project Structure

```
.
├── api/proto/              # gRPC protocol definitions
├── cmd/
│   ├── api/                # API server entry point
│   ├── web/                # Web UI server entry point
│   └── worker/             # Build worker entry point
├── internal/
│   ├── api/                # HTTP API handlers and middleware
│   ├── auth/               # Authentication and RBAC
│   ├── builder/            # Build system (Nix, OCI, strategies)
│   ├── cleanup/            # Resource cleanup services
│   ├── grpc/               # gRPC server and node management
│   ├── models/             # Domain models
│   ├── queue/              # Build job queue
│   ├── scheduler/          # Deployment scheduler
│   ├── secrets/            # SOPS secrets management
│   ├── store/              # Database access layer
│   └── validation/         # Input validation services
├── migrations/             # SQL migrations
├── pkg/
│   ├── config/             # Configuration loading
│   └── logger/             # Structured logging
├── web/                    # Web UI (templ templates)
├── flake.nix               # Nix flake for development
└── Makefile                # Build and development commands
```

## Build Strategies

Narvana supports multiple build strategies:

| Strategy | Description | Build Type |
|----------|-------------|------------|
| `flake` | Use existing `flake.nix` in repository | Nix or OCI |
| `auto-go` | Auto-generate flake for Go projects | Nix or OCI |
| `auto-rust` | Auto-generate flake for Rust projects | Nix or OCI |
| `auto-node` | Auto-generate flake for Node.js projects | Nix or OCI |
| `auto-python` | Auto-generate flake for Python projects | Nix or OCI |
| `auto-database` | Auto-generate flake for database services | Nix |
| `dockerfile` | Build from Dockerfile | OCI only |
| `nixpacks` | Use Nixpacks for detection and building | OCI only |
| `auto` | Automatic strategy detection | Varies |

## API Overview

### Authentication

```bash
# Register (first user becomes owner)
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@example.com", "password": "secure-password"}'

# Login
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@example.com", "password": "secure-password"}'
```

### Apps and Services

```bash
# Create an app
curl -X POST http://localhost:8080/v1/apps \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "my-app", "description": "My application"}'

# Create a service
curl -X POST http://localhost:8080/v1/apps/$APP_ID/services \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "api",
    "source_type": "git",
    "git_repo": "github.com/myorg/myrepo",
    "build_strategy": "auto-go"
  }'

# Deploy a service
curl -X POST http://localhost:8080/v1/apps/$APP_ID/services/api/deploy \
  -H "Authorization: Bearer $TOKEN"
```

### Node Management

```bash
# List nodes
curl http://localhost:8080/v1/nodes \
  -H "Authorization: Bearer $TOKEN"

# Register a node (from node agent)
curl -X POST http://localhost:8080/v1/nodes/register \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "hostname": "node-1",
    "address": "192.168.1.100",
    "grpc_port": 9090
  }'
```

## Development

### Running Tests

```bash
# Run all tests
make test

# Run unit tests only
make test-unit

# Run property-based tests
make test-property
```

### Code Generation

```bash
# Generate protobuf files
make proto

# Generate templ templates (for web UI)
cd web && templ generate
```

### Database Migrations

```bash
# Apply migrations
make migrate-up

# Check migration status
make db-status
```

### Linting

```bash
make lint
```

### Testing `scripts/install.sh` in a container

You can iterate on the installer locally without pushing to GitHub by running it inside a disposable container.

**Using Docker:**

```bash
cd control-plane

docker run --rm -it --privileged \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$PWD:/repo:ro" \
  docker:24-dind sh -c "apk add curl bash openssl && bash /repo/scripts/install.sh"
```

**Using Podman:**

```bash
cd control-plane

podman run --rm -it --privileged \
  -v /var/run/podman/podman.sock:/var/run/docker.sock \
  -v "$PWD:/repo:ro" \
  docker.io/library/docker:24-cli sh -c "apk add curl bash openssl && bash /repo/scripts/install.sh"
```

The installer only requires Docker/Podman and curl - it pulls pre-built container images rather than building from source.

## Supported Databases

Narvana can provision managed database services:

| Type | Supported Versions | Default Port |
|------|-------------------|--------------|
| PostgreSQL | 14, 15, 16 | 5432 |
| MySQL | 8.0 | 3306 |
| MariaDB | 10, 11 | 3306 |
| MongoDB | 6.0, 7.0 | 27017 |
| Redis | 6, 7 | 6379 |
| SQLite | 3 | N/A |

## Resource Specifications

Services can specify resource limits directly:

```json
{
  "resources": {
    "cpu": "0.5",
    "memory": "512Mi"
  }
}
```

> **Note**: The predefined resource tiers (`nano`, `small`, `medium`, `large`, `xlarge`) are deprecated and will be removed in a future version. Use direct `resources` specification instead.

## Cleanup and Maintenance

Narvana includes automatic cleanup services:

```bash
# Manual cleanup endpoints (admin only)
curl -X POST http://localhost:8080/v1/admin/cleanup/containers \
  -H "Authorization: Bearer $TOKEN"

curl -X POST http://localhost:8080/v1/admin/cleanup/images \
  -H "Authorization: Bearer $TOKEN"

curl -X POST http://localhost:8080/v1/admin/cleanup/nix-gc \
  -H "Authorization: Bearer $TOKEN"

curl -X POST http://localhost:8080/v1/admin/cleanup/deployments \
  -H "Authorization: Bearer $TOKEN"
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

---

<p align="center">
  Built with ❄️ Nix and 💙 Go
</p>
