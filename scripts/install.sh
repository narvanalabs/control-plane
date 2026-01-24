#!/usr/bin/env bash
set -euo pipefail

# Narvana Control Plane - One-Click Installer (Nix-first)
# Usage: curl -fsSL https://raw.githubusercontent.com/narvanalabs/control-plane/master/scripts/install.sh | bash

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# Configuration
GITHUB_REPO="narvanalabs/control-plane"
INSTALL_DIR="/opt/narvana/control-plane"
BIN_DIR="/opt/narvana/control-plane/bin"
ENV_FILE="/etc/narvana/control-plane.env"
VERSION="${NARVANA_VERSION:-latest}"

# Helper functions
log_info() { echo -e "${BLUE}${BOLD}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}${BOLD}[✓]${NC} $1"; }
log_warn() { echo -e "${YELLOW}${BOLD}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}${BOLD}[ERROR]${NC} $1"; }

# Detect if systemd is running as PID 1 (not the case in most containers)
has_systemd() {
    if command -v systemctl &> /dev/null && [[ -d /run/systemd/system ]]; then
        return 0
    fi
    return 1
}

# Check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

# Detect OS and architecture
detect_system() {
    if [[ "$OSTYPE" != "linux-gnu"* ]]; then
        log_error "Narvana currently only supports Linux"
        exit 1
    fi

    ARCH=$(uname -m)
    case $ARCH in
        x86_64) GOARCH="amd64" ;;
        aarch64|arm64) GOARCH="arm64" ;;
        *) log_error "Unsupported architecture: $ARCH"; exit 1 ;;
    esac

    if command -v apt-get &> /dev/null; then
        PKG_MANAGER="apt-get"
        PKG_INSTALL="apt-get install -y"
        PKG_UPDATE="apt-get update -y"
    elif command -v dnf &> /dev/null; then
        PKG_MANAGER="dnf"
        PKG_INSTALL="dnf install -y"
        PKG_UPDATE="dnf update -y"
    elif command -v yum &> /dev/null; then
        PKG_MANAGER="yum"
        PKG_INSTALL="yum install -y"
        PKG_UPDATE="yum update -y"
    else
        log_error "Unsupported package manager. Please install dependencies manually."
        exit 1
    fi

    log_success "Detected: Linux $GOARCH, package manager: $PKG_MANAGER"
}

# Get latest release version from GitHub
get_latest_version() {
    if [[ "$VERSION" == "latest" ]]; then
        VERSION=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/' || echo "")
        if [[ -z "$VERSION" ]]; then
            log_error "Failed to fetch latest version from GitHub"
            exit 1
        fi
    fi
    log_info "Installing Narvana v${VERSION}"
}

# Install minimal system dependencies (only what Nix can't handle)
install_system_dependencies() {
    log_info "Installing system dependencies..."
    
    $PKG_UPDATE || true
    # Only install: curl (for this script), git (for repo), postgresql, and openssl (for secrets)
    $PKG_INSTALL curl wget git openssl postgresql postgresql-contrib python3 2>/dev/null || \
    $PKG_INSTALL curl wget git openssl postgresql postgresql-server python3 2>/dev/null || {
        log_error "Failed to install dependencies"
        exit 1
    }
    
    log_success "System dependencies installed"
}

# Install Podman (still via system package manager)
install_podman() {
    if command -v podman &> /dev/null; then
        log_success "Podman already installed"
        return
    fi
    
    log_info "Installing Podman..."
    $PKG_INSTALL podman || log_warn "Failed to install Podman - install manually if needed"
    
    systemctl enable --now podman.socket 2>/dev/null || true
}

# Setup PostgreSQL
setup_postgresql() {
    log_info "Setting up PostgreSQL..."
    
    if command -v systemctl &> /dev/null; then
        systemctl enable postgresql 2>/dev/null || true
        systemctl start postgresql 2>/dev/null || true
    fi
    
    if [[ -f "$ENV_FILE" ]] && grep -q "DATABASE_URL=" "$ENV_FILE"; then
        DB_PASSWORD=$(grep "^DATABASE_URL=" "$ENV_FILE" | sed -n 's|.*//[^:]*:\([^@]*\)@.*|\1|p')
    fi
    
    if [[ -z "${DB_PASSWORD:-}" ]] || [[ "$DB_PASSWORD" == *":"* ]] || [[ "$DB_PASSWORD" == *"/"* ]]; then
        DB_PASSWORD=$(openssl rand -hex 16)
    fi
    
    sudo -u postgres psql -c "CREATE USER narvana WITH PASSWORD '${DB_PASSWORD}';" 2>/dev/null || \
    sudo -u postgres psql -c "ALTER USER narvana WITH PASSWORD '${DB_PASSWORD}';" 2>/dev/null || true
    sudo -u postgres psql -c "CREATE DATABASE narvana OWNER narvana;" 2>/dev/null || true
    sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE narvana TO narvana;" 2>/dev/null || true
    
    log_success "PostgreSQL configured"
}

# Create narvana user and directories
create_user() {
    if ! id "narvana" &>/dev/null; then
        log_info "Creating narvana user..."
        useradd -r -m -s /bin/bash -d /opt/narvana narvana || true
    fi
    
    if ! grep -q "^narvana:" /etc/subuid 2>/dev/null; then
        echo "narvana:165536:65536" >> /etc/subuid
    fi
    if ! grep -q "^narvana:" /etc/subgid 2>/dev/null; then
        echo "narvana:165536:65536" >> /etc/subgid
    fi
    
    mkdir -p /opt/narvana /opt/narvana/.config/containers /var/log/narvana /var/lib/narvana/builds /etc/narvana
    chown -R narvana:narvana /opt/narvana /var/log/narvana /var/lib/narvana
    chmod 755 /var/lib/narvana /var/lib/narvana/builds
    
    log_success "User configured"
}

# Install Nix package manager
install_nix() {
    if command -v nix &> /dev/null; then
        log_success "Nix already installed"
        return
    fi

    # Allow skipping Nix when testing the installer in constrained environments
    if [[ "${NARVANA_SKIP_NIX_INSTALL:-0}" == "1" ]]; then
        log_warn "Skipping Nix installation because NARVANA_SKIP_NIX_INSTALL=1"
        return
    fi

    log_info "Installing Nix package manager..."

    # If a nixbld group already exists (e.g. from a previous Nix install),
    # tell the installer to reuse its GID instead of failing.
    if getent group nixbld > /dev/null 2>&1; then
        NIX_BUILD_GROUP_ID="$(getent group nixbld | cut -d: -f3 || true)"
        if [[ -n "${NIX_BUILD_GROUP_ID:-}" ]]; then
            export NIX_BUILD_GROUP_ID
            log_info "Using existing nixbld group (GID ${NIX_BUILD_GROUP_ID}) for Nix installer"
        fi
    fi

    sh <(curl -L https://nixos.org/nix/install) --daemon --yes || {
        log_error "Nix installation failed"
        exit 1
    }
    
    if [[ -f /etc/profile.d/nix.sh ]]; then
        . /etc/profile.d/nix.sh
    fi
    
    systemctl enable nix-daemon 2>/dev/null || true
    systemctl start nix-daemon 2>/dev/null || true
    
    if command -v nix &> /dev/null; then
        log_success "Nix installed: $(nix --version)"
    else
        log_warn "Nix installed but not in PATH - sourcing profile"
        export PATH="/nix/var/nix/profiles/default/bin:$PATH"
    fi
}

# Source Nix environment
source_nix() {
    if [[ -f /etc/profile.d/nix.sh ]]; then
        . /etc/profile.d/nix.sh
    fi
    
    # Ensure nix is in PATH
    if ! command -v nix &> /dev/null; then
        export PATH="/nix/var/nix/profiles/default/bin:$PATH"
    fi
    
    # Enable flakes and nix-command
    export NIX_CONFIG="experimental-features = nix-command flakes"
}

# Clone repository
clone_repo() {
    log_info "Cloning repository..."
    
    local REPO_URL="https://github.com/${GITHUB_REPO}.git"
    local REPO_BRANCH="${NARVANA_BRANCH:-master}"
    
    mkdir -p "$(dirname "$INSTALL_DIR")"
    
    if [[ -d "$INSTALL_DIR/.git" ]]; then
        cd "$INSTALL_DIR"
        git fetch origin
        git checkout "$REPO_BRANCH" 2>/dev/null || git checkout -b "$REPO_BRANCH" "origin/$REPO_BRANCH"
        git pull origin "$REPO_BRANCH" || true
    else
        git clone -b "$REPO_BRANCH" "$REPO_URL" "$INSTALL_DIR"
    fi
    
    log_success "Repository ready"
}

# Create shell.nix for the build environment
create_nix_shell() {
    log_info "Creating Nix shell environment..."
    
    cat > "$INSTALL_DIR/shell.nix" << 'EOF'
{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  buildInputs = with pkgs; [
    go_1_24
    templ
    tailwindcss
    nodejs
    git
  ];

  shellHook = ''
    export GOPATH=$HOME/go
    export PATH=$GOPATH/bin:$PATH
    echo "Narvana build environment ready"
    echo "Go version: $(go version)"
  '';
}
EOF
    
    log_success "Nix shell environment created"
}

# Build everything using Nix
build_with_nix() {
    log_info "Building Narvana using Nix environment..."
    
    source_nix
    cd "$INSTALL_DIR"
    
    # Enter nix-shell and build
    nix-shell --run '
        set -e
        
        # Generate templ templates
        echo "Generating templ templates..."
        cd web
        templ generate || exit 1
        cd ..
        
        # Generate CSS
        echo "Generating CSS..."
        cd web
        tailwindcss -i ./assets/css/input.css -o ./assets/css/output.css --minify || exit 1
        cd ..
        
        # Build binaries
        echo "Building binaries..."
        mkdir -p '"$BIN_DIR"'
        go build -buildvcs=false -o '"$BIN_DIR"'/api ./cmd/api || exit 1
        go build -buildvcs=false -o '"$BIN_DIR"'/worker ./cmd/worker || exit 1
        go build -buildvcs=false -o '"$BIN_DIR"'/web ./cmd/web || exit 1
        
        echo "Build completed successfully"
    ' || {
        log_error "Nix build failed"
        exit 1
    }
    
    log_success "Binaries built with Nix"
}

# Download pre-built binaries (fallback)
download_binaries() {
    log_info "Attempting to download pre-built binaries..."
    
    mkdir -p "$BIN_DIR"
    local base_url="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"
    local binaries=("narvana-api" "narvana-web" "narvana-worker")
    local download_success=true
    
    for binary in "${binaries[@]}"; do
        local url="${base_url}/${binary}-linux-${GOARCH}"
        local dest="${BIN_DIR}/${binary#narvana-}"
        
        log_info "Downloading ${binary}..."
        if curl -fsSL "$url" -o "$dest" 2>/dev/null; then
            chmod +x "$dest"
            log_success "Downloaded ${binary}"
        else
            log_warn "Failed to download ${binary}"
            download_success=false
            break
        fi
    done
    
    if [[ "$download_success" == "false" ]]; then
        log_info "Falling back to Nix build..."
        create_nix_shell
        build_with_nix
    else
        log_success "All binaries downloaded"
    fi
}

# Install Attic using Nix
install_attic() {
    log_info "Setting up Attic binary cache server..."
    
    source_nix
    
    # Install attic using Nix
    if ! command -v atticd &> /dev/null; then
        log_info "Installing Attic via Nix..."
        nix profile install nixpkgs#attic-server nixpkgs#attic-client || {
            log_error "Failed to install Attic"
            return 1
        }
    fi
    
    ATTICD_PATH=$(which atticd 2>/dev/null || echo "/root/.nix-profile/bin/atticd")
    ATTIC_PATH=$(which attic 2>/dev/null || echo "/root/.nix-profile/bin/attic")
    
    log_info "Using atticd at: $ATTICD_PATH"
    log_info "Using attic at: $ATTIC_PATH"
    
    # Create Attic data directories
    mkdir -p /var/lib/narvana/attic/storage
    mkdir -p /opt/narvana/.config/attic
    mkdir -p /root/.config/attic
    chown -R root:root /var/lib/narvana/attic
    chown -R narvana:narvana /opt/narvana/.config
    
    # Generate a strong JWT secret (must be base64 encoded and >= 32 bytes when decoded)
    ATTIC_JWT_SECRET=$(openssl rand -base64 48)
    
    # Create Attic server config with simplified settings
    cat > /etc/narvana/attic.toml << EOF
# Attic server configuration
listen = "[::]:5000"

[database]
url = "sqlite:///var/lib/narvana/attic/server.db?mode=rwc"

[storage]
type = "local"
path = "/var/lib/narvana/attic/storage"

[chunking]
nar-size-threshold = 65536
min-size = 16384
avg-size = 65536
max-size = 262144

[compression]
type = "zstd"

[garbage-collection]
default-retention-period = "30 days"

# JWT token configuration
[jwt.signing]
token-hs256-secret-base64 = "${ATTIC_JWT_SECRET}"
EOF
    
    chmod 600 /etc/narvana/attic.toml
    
    # Create systemd service
    cat > /etc/systemd/system/narvana-attic.service << EOF
[Unit]
Description=Narvana Attic Binary Cache Server
After=network.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/var/lib/narvana/attic
ExecStart=${ATTICD_PATH} --config /etc/narvana/attic.toml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

Environment=HOME=/root
Environment=RUST_LOG=info,attic=debug

[Install]
WantedBy=multi-user.target
EOF
    
    # Start Attic server (use systemd when available, otherwise run directly)
    if has_systemd; then
        systemctl daemon-reload
        systemctl enable narvana-attic
        systemctl restart narvana-attic
    else
        log_warn "systemd not available (container/test environment); starting atticd directly"
        "${ATTICD_PATH}" --config /etc/narvana/attic.toml >/var/log/narvana/atticd.log 2>&1 &
    fi
    
    # Wait for server with better checking
    log_info "Waiting for Attic server to start..."
    local max_attempts=20
    local attempt=0
    while [ $attempt -lt $max_attempts ]; do
        if curl -sf http://localhost:5000/_health > /dev/null 2>&1; then
            log_success "Attic server is running"
            break
        fi
        attempt=$((attempt + 1))
        sleep 1
    done
    
    if [ $attempt -eq $max_attempts ]; then
        if has_systemd; then
            log_error "Attic server failed to start. Checking logs..."
            journalctl -u narvana-attic -n 50 --no-pager
            return 1
        else
            log_warn "Attic server failed to start in non-systemd environment; continuing without binary cache"
            return 0
        fi
    fi
    
    # Generate token using atticd itself (most reliable method)
    log_info "Generating authentication token..."
    
    # Create a token with 10-year expiry using atticd's built-in command
    ATTIC_TOKEN=$(cd /var/lib/narvana/attic && ${ATTICD_PATH} --config /etc/narvana/attic.toml \
        make-token --sub "narvana-worker" \
        --validity "87600h" \
        --pull "*" \
        --push "*" \
        --delete "*" \
        --create-cache "*" \
        --configure-cache "*" \
        --destroy-cache "*" 2>/dev/null) || {
        
        log_warn "atticd make-token failed, using Python fallback..."
        
        # Fallback: Use Python to generate proper JWT
        ATTIC_TOKEN=$(python3 << 'PYTHON_EOF'
import json
import base64
import hmac
import hashlib
import time
import sys

# Read the secret from the config file
with open('/etc/narvana/attic.toml', 'r') as f:
    for line in f:
        if 'token-hs256-secret-base64' in line:
            secret_b64 = line.split('"')[1]
            break

secret = base64.b64decode(secret_b64)

# Create header
header = {
    "alg": "HS256",
    "typ": "JWT"
}

# Create payload with proper structure
payload = {
    "sub": "narvana-worker",
    "exp": int(time.time()) + (87600 * 3600),  # 10 years
    "https://jwt.attic.rs/v1": {
        "caches": {
            "*": {
                "r": 1,
                "w": 1,
                "cc": 1,
                "cd": 1,
                "dc": 1
            }
        }
    }
}

# Encode header and payload
def b64_encode(data):
    return base64.urlsafe_b64encode(json.dumps(data).encode()).decode().rstrip('=')

header_b64 = b64_encode(header)
payload_b64 = b64_encode(payload)

# Create signature
message = f"{header_b64}.{payload_b64}".encode()
signature = hmac.new(secret, message, hashlib.sha256).digest()
signature_b64 = base64.urlsafe_b64encode(signature).decode().rstrip('=')

# Create final token
token = f"{header_b64}.{payload_b64}.{signature_b64}"
print(token)
PYTHON_EOF
)
    }
    
    if [ -z "$ATTIC_TOKEN" ]; then
        log_error "Failed to generate Attic token"
        return 1
    fi
    
    log_success "Token generated successfully"
    
    # Configure attic client for root user
    cat > /root/.config/attic/config.toml << EOF
# Attic client configuration for root
default-server = "narvana"

[servers.narvana]
endpoint = "http://localhost:5000"
token = "${ATTIC_TOKEN}"
EOF
    
    chmod 600 /root/.config/attic/config.toml
    
    # Configure attic client for narvana user
    cat > /opt/narvana/.config/attic/config.toml << EOF
# Attic client configuration for narvana user
default-server = "narvana"

[servers.narvana]
endpoint = "http://localhost:5000"
token = "${ATTIC_TOKEN}"
EOF
    
    chown -R narvana:narvana /opt/narvana/.config/attic
    chmod 600 /opt/narvana/.config/attic/config.toml
    
    # Create the cache using the token
    log_info "Creating 'narvana' cache..."
    
    # Export token for attic CLI
    export ATTIC_TOKEN="${ATTIC_TOKEN}"
    
    # Try to create cache
    if ${ATTIC_PATH} cache create narvana 2>&1 | tee /tmp/attic-create.log; then
        log_success "Cache 'narvana' created"
    elif grep -q "already exists" /tmp/attic-create.log; then
        log_info "Cache 'narvana' already exists"
    else
        log_warn "Could not create cache. Checking server status..."
        journalctl -u narvana-attic -n 20 --no-pager
    fi
    
    # Verify the setup
    log_info "Verifying Attic configuration..."
    if ${ATTIC_PATH} cache info narvana 2>&1; then
        log_success "Attic cache 'narvana' is configured and accessible"
    else
        if has_systemd; then
            log_error "Cannot access Attic cache. Troubleshooting info:"
            echo ""
            echo "1. Check if server is running:"
            echo "   systemctl status narvana-attic"
            echo ""
            echo "2. Check server logs:"
            echo "   journalctl -u narvana-attic -f"
            echo ""
            echo "3. Test server directly:"
            echo "   curl http://localhost:5000/_health"
            echo ""
            echo "4. Verify token manually:"
            echo "   attic cache list"
            echo ""
            return 1
        else
            log_warn "Attic cache verification failed in non-systemd environment; continuing without binary cache"
        fi
    fi
    
    # Store token in environment file for worker
    if ! grep -q "ATTIC_TOKEN=" /etc/narvana/control-plane.env 2>/dev/null; then
        echo "" >> /etc/narvana/control-plane.env
        echo "# Attic binary cache authentication" >> /etc/narvana/control-plane.env
        echo "ATTIC_TOKEN=${ATTIC_TOKEN}" >> /etc/narvana/control-plane.env
        echo "ATTIC_ENDPOINT=http://localhost:5000" >> /etc/narvana/control-plane.env
    fi
    
    log_success "Attic binary cache configured with authentication"
}

# Setup environment
setup_environment() {
    log_info "Configuring environment..."
    
    PUBLIC_IP=$(curl -s --max-time 5 https://ifconfig.me || \
                curl -s --max-time 5 https://ifconfig.io || \
                hostname -I | awk '{print $1}' || \
                echo "your-server-ip")
    
    if [[ ! -f "$ENV_FILE" ]] || ! grep -q "JWT_SECRET=" "$ENV_FILE"; then
        JWT_SECRET=$(openssl rand -hex 32)
    else
        JWT_SECRET=$(grep "^JWT_SECRET=" "$ENV_FILE" | cut -d'=' -f2-)
    fi
    
    cat > "$ENV_FILE" <<EOF
# Narvana Control Plane Configuration
# Generated on $(date)

DATABASE_URL=postgres://narvana:${DB_PASSWORD}@localhost:5432/narvana?sslmode=disable
JWT_SECRET=${JWT_SECRET}

# API Server
API_HOST=0.0.0.0
API_PORT=8080
GRPC_PORT=9090

# URLs
INTERNAL_API_URL=http://127.0.0.1:8080
API_URL=http://${PUBLIC_IP}:8080
WEB_URL=http://${PUBLIC_IP}:8090

# Worker
WORKER_WORKDIR=/var/lib/narvana/builds
WORKER_MAX_CONCURRENCY=4
BUILD_TIMEOUT=30m
PODMAN_SOCKET=unix:///run/podman/podman.sock

# Scheduler
SCHEDULER_HEALTH_THRESHOLD=30s
SCHEDULER_MAX_RETRIES=5
SCHEDULER_RETRY_BACKOFF=5s
EOF
    
    chmod 600 "$ENV_FILE"
    chown narvana:narvana "$ENV_FILE"
    
    log_success "Environment configured"
}

# Run database migrations
run_migrations() {
    log_info "Running database migrations..."
    
    cd "$INSTALL_DIR"
    for f in migrations/*.sql; do
        if [[ -f "$f" ]]; then
            PGPASSWORD="${DB_PASSWORD}" psql -h localhost -U narvana -d narvana -f "$f" 2>/dev/null || \
            log_warn "Migration $(basename "$f") may have already been applied"
        fi
    done
    
    log_success "Migrations completed"
}

# Setup systemd services
setup_services() {
    log_info "Configuring systemd services..."

    if ! has_systemd; then
        log_warn "systemd not available; skipping Narvana systemd service setup (expected in containers/test environments)"
        return 0
    fi
    local NARVANA_UID=$(id -u narvana)
    
    mkdir -p "/run/user/${NARVANA_UID}"
    chown narvana:narvana "/run/user/${NARVANA_UID}"
    chmod 700 "/run/user/${NARVANA_UID}"
    
    cd "$INSTALL_DIR"
    
    sed "s|/run/user/1001|/run/user/${NARVANA_UID}|g" deploy/narvana-worker.service > /etc/systemd/system/narvana-worker.service
    cp deploy/narvana-api.service /etc/systemd/system/
    cp deploy/narvana-web.service /etc/systemd/system/
    
    systemctl daemon-reload
    systemctl enable narvana-api narvana-web narvana-worker
    systemctl restart narvana-api narvana-web narvana-worker
    
    log_success "Services configured and started"
}

# Verify installation
verify_installation() {
    log_info "Verifying installation..."

    if ! has_systemd; then
        log_warn "Skipping service health verification because systemd is not available in this environment"
        return 0
    fi
    sleep 5
    
    local failed=0
    
    if curl -sf --max-time 10 "http://127.0.0.1:8080/health" > /dev/null; then
        log_success "API server is healthy"
    else
        log_error "API server is not responding"
        journalctl -u narvana-api -n 10 --no-pager 2>/dev/null || true
        failed=1
    fi
    
    if curl -sf --max-time 10 "http://127.0.0.1:8090/login" > /dev/null; then
        log_success "Web UI is accessible"
    else
        log_error "Web UI is not responding"
        journalctl -u narvana-web -n 10 --no-pager 2>/dev/null || true
        failed=1
    fi
    
    return $failed
}

# Print success message
print_success() {
    echo ""
    echo -e "${GREEN}${BOLD}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}${BOLD}║       Narvana Control Plane Installed Successfully!          ║${NC}"
    echo -e "${GREEN}${BOLD}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${BOLD}Access your dashboard at:${NC}"
    echo -e "  ${BLUE}http://${PUBLIC_IP}:8090${NC}"
    echo ""
    echo -e "${BOLD}Installed version:${NC} v${VERSION}"
    echo ""
    echo -e "${BOLD}Next steps:${NC}"
    echo "  1. Visit the URL above and create your admin account"
    echo "  2. Configure your domain and SSL (optional)"
    echo "  3. Connect your GitHub App via the dashboard"
    echo ""
    echo -e "${BOLD}Useful commands:${NC}"
    echo "  View logs:    journalctl -u narvana-* -f"
    echo "  Restart:      systemctl restart narvana-*"
    echo "  Status:       systemctl status narvana-*"
    echo "  Update:       curl -fsSL https://raw.githubusercontent.com/narvanalabs/control-plane/master/scripts/install.sh | bash"
    echo ""
}

# Main installation flow
main() {
    echo -e "${BLUE}${BOLD}"
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║        Narvana Control Plane - One-Click Installer           ║"
    echo "║                  (Nix-powered Build)                         ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    
    check_root
    detect_system
    get_latest_version
    install_system_dependencies
    install_podman
    install_nix
    source_nix
    setup_postgresql
    create_user
    clone_repo
    download_binaries  # Will fallback to Nix build if download fails
    setup_environment
    install_attic
    run_migrations
    setup_services
    
    if verify_installation; then
        print_success
    else
        echo ""
        log_warn "Installation completed with warnings. Check logs above."
        echo -e "  View logs: ${BOLD}journalctl -u narvana-* -f${NC}"
        exit 1
    fi
}

main "$@"
