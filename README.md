# Narvana Control Plane

The control plane manages applications, services, deployments, and coordinates with node agents.

## Development Setup

### Prerequisites
- [Nix](https://nixos.org/download.html) with flakes enabled

### Quick Start

**Local Development (Nix):**
```bash
# Enter the development shell (starts PostgreSQL automatically)
nix develop

# Run migrations and start all services
make dev-all
```

**VPS Deployment (One-Click):**
Run this command on your Azure VPS to install everything (API, Worker, UI, Caddy, Postgres):
```bash
curl -sSL https://raw.githubusercontent.com/narvanalabs/control-plane/master/scripts/install.sh | sudo bash
```

That's it! The API server runs on `http://localhost:8080` and gRPC on port `9090`.

### DNS Setup (for Wildcard Domains)

To access deployed services via wildcard domains (e.g., `myapp-api.narvana.local:8088`), you need to configure local DNS:

```bash
# One-time setup (requires sudo)
sudo ./scripts/setup-dns.sh
```

This script will:
- Install dnsmasq (if not already installed)
- Configure wildcard DNS for `*.narvana.local` → `127.0.0.1`
- Set up resolver configuration for your OS (Linux/macOS)
- Start/restart the dnsmasq service

After setup, you can access services at: `http://{app-name}-{service-name}.narvana.local:8088`

**Note:** `/etc/hosts` does NOT support wildcards, so dnsmasq is required for automatic routing.

### Individual Services

If you prefer running services separately:

```bash
# Terminal 1: API server
make dev-api

# Terminal 2: Build worker
make dev-worker
```

### Available Commands

```bash
make help          # Show all commands
make dev-all       # Run all services (recommended)
make dev-api       # Run API server only
make dev-worker    # Run worker only
make dev-stop      # Stop all services
make test          # Run tests
make db-stop       # Stop PostgreSQL
```

## Production Deployment

### Using systemd

1. Build the binaries:
   ```bash
   make build
   ```

2. Copy binaries to `/opt/narvana/control-plane/bin/`

3. Copy and configure environment:
   ```bash
   sudo cp deploy/control-plane.env.example /etc/narvana/control-plane.env
   sudo chmod 600 /etc/narvana/control-plane.env
   # Edit with your production values
   ```

4. Install systemd services:
   ```bash
   sudo cp deploy/*.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable narvana-api narvana-worker
   sudo systemctl start narvana-api narvana-worker
   ```

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `JWT_SECRET` | Yes | - | JWT signing key (min 32 chars) |
| `API_PORT` | No | 8080 | HTTP API port |
| `GRPC_PORT` | No | 9090 | gRPC port for agents |

See `deploy/control-plane.env.example` for all options.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Control Plane                        │
├─────────────────────┬───────────────────────────────────┤
│     API Server      │         Build Worker              │
│  (HTTP + gRPC)      │    (processes build queue)        │
├─────────────────────┴───────────────────────────────────┤
│                     PostgreSQL                          │
│            (apps, services, deployments)                │
└─────────────────────────────────────────────────────────┘
```
