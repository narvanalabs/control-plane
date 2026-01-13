#!/usr/bin/env bash
set -euo pipefail

# Narvana Control Plane - One-Click Installer
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

# Download pre-built binaries from GitHub releases
download_binaries() {
    log_info "Downloading pre-built binaries..."
    
    mkdir -p "$BIN_DIR"
    local base_url="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}"
    local binaries=("narvana-api" "narvana-web" "narvana-worker")
    
    for binary in "${binaries[@]}"; do
        local url="${base_url}/${binary}-linux-${GOARCH}"
        local dest="${BIN_DIR}/${binary#narvana-}"
        
        log_info "Downloading ${binary}..."
        if curl -fsSL "$url" -o "$dest"; then
            chmod +x "$dest"
            log_success "Downloaded ${binary}"
        else
            log_error "Failed to download ${binary} from ${url}"
            log_warn "Falling back to building from source..."
            build_from_source
            return
        fi
    done
    
    log_success "All binaries downloaded"
}

# Fallback: Build from source if binary download fails
build_from_source() {
    log_info "Building from source (this may take a few minutes)..."
    
    # Check for Go
    if ! command -v go &> /dev/null; then
        install_go
    fi
    
    # Clone repository if needed
    if [[ ! -d "$INSTALL_DIR/.git" ]]; then
        clone_repo
    fi
    
    cd "$INSTALL_DIR"
    export PATH=$PATH:/usr/local/go/bin:$(go env GOPATH 2>/dev/null || echo "$HOME/go")/bin
    
    # Install templ
    if ! command -v templ &> /dev/null; then
        log_info "Installing templ..."
        go install github.com/a-h/templ/cmd/templ@latest
    fi
    
    # Generate UI components
    log_info "Generating UI components..."
    (cd web && templ generate) || log_warn "templ generate failed"
    
    # Build CSS (optional)
    if command -v tailwindcss &> /dev/null; then
        (cd web && tailwindcss -i ./assets/css/input.css -o ./assets/css/output.css --minify 2>/dev/null) || true
    fi
    
    # Build binaries
    mkdir -p "$BIN_DIR"
    go build -buildvcs=false -o "$BIN_DIR/api" ./cmd/api
    go build -buildvcs=false -o "$BIN_DIR/worker" ./cmd/worker
    go build -buildvcs=false -o "$BIN_DIR/web" ./cmd/web
    
    log_success "Binaries built from source"
}

# Install Go (only needed for source builds)
install_go() {
    local GO_VERSION="1.24.0"
    log_info "Installing Go ${GO_VERSION}..."
    
    local GO_TAR="go${GO_VERSION}.linux-${GOARCH}.tar.gz"
    local GO_URL="https://go.dev/dl/${GO_TAR}"
    
    # Use subshell to avoid changing current directory
    (
        cd /tmp
        curl -fsSL "$GO_URL" -o "$GO_TAR"
        rm -rf /usr/local/go
        tar -C /usr/local -xzf "$GO_TAR"
        rm "$GO_TAR"
    )
    
    export PATH=$PATH:/usr/local/go/bin
    if ! grep -q "/usr/local/go/bin" /etc/profile 2>/dev/null; then
        echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    fi
    
    log_success "Go installed"
}

# Clone repository (for migrations and service files)
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

# Generate web assets (templ templates and CSS)
generate_web_assets() {
    log_info "Generating web assets..."
    
    # Check if assets already exist (e.g., from a previous install)
    if [[ -f "$INSTALL_DIR/web/assets/css/output.css" ]] && [[ -f "$INSTALL_DIR/web/layouts/base_templ.go" ]]; then
        log_success "Web assets already exist"
        return
    fi
    
    # We need Go for templ - install it if not present
    if ! command -v go &> /dev/null; then
        install_go
    fi
    
    export PATH=$PATH:/usr/local/go/bin:$(go env GOPATH 2>/dev/null || echo "$HOME/go")/bin
    
    # Install and run templ
    if ! command -v templ &> /dev/null; then
        log_info "Installing templ..."
        go install github.com/a-h/templ/cmd/templ@latest
    fi
    
    log_info "Generating templ templates..."
    (cd "$INSTALL_DIR/web" && templ generate) || {
        log_error "templ generate failed"
        exit 1
    }
    
    # Install tailwindcss if not available
    if ! command -v tailwindcss &> /dev/null; then
        log_info "Installing tailwindcss..."
        local TAILWIND_ARCH="x64"
        if [[ "$GOARCH" == "arm64" ]]; then
            TAILWIND_ARCH="arm64"
        fi
        curl -sLO "https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-${TAILWIND_ARCH}"
        chmod +x "tailwindcss-linux-${TAILWIND_ARCH}"
        mv "tailwindcss-linux-${TAILWIND_ARCH}" /usr/local/bin/tailwindcss
    fi
    
    # Generate CSS
    log_info "Generating CSS..."
    (cd "$INSTALL_DIR/web" && tailwindcss -i ./assets/css/input.css -o ./assets/css/output.css --minify) || {
        log_error "CSS generation failed"
        exit 1
    }
    
    log_success "Web assets generated"
}

# Install system dependencies
install_dependencies() {
    log_info "Installing system dependencies..."
    
    $PKG_UPDATE || true
    $PKG_INSTALL curl wget git openssl postgresql postgresql-contrib 2>/dev/null || \
    $PKG_INSTALL curl wget git openssl postgresql postgresql-server 2>/dev/null || {
        log_error "Failed to install dependencies"
        exit 1
    }
    
    log_success "Dependencies installed"
}

# Install Podman
install_podman() {
    if command -v podman &> /dev/null; then
        log_success "Podman already installed"
        return
    fi
    
    log_info "Installing Podman..."
    $PKG_INSTALL podman || log_warn "Failed to install Podman - install manually if needed"
    
    # Enable podman socket
    systemctl enable --now podman.socket 2>/dev/null || true
}

# Setup PostgreSQL
setup_postgresql() {
    log_info "Setting up PostgreSQL..."
    
    # Start PostgreSQL
    if command -v systemctl &> /dev/null; then
        systemctl enable postgresql 2>/dev/null || true
        systemctl start postgresql 2>/dev/null || true
    fi
    
    # Get or generate password
    if [[ -f "$ENV_FILE" ]] && grep -q "DATABASE_URL=" "$ENV_FILE"; then
        DB_PASSWORD=$(grep "^DATABASE_URL=" "$ENV_FILE" | sed -n 's|.*//[^:]*:\([^@]*\)@.*|\1|p')
    fi
    
    if [[ -z "${DB_PASSWORD:-}" ]] || [[ "$DB_PASSWORD" == *":"* ]] || [[ "$DB_PASSWORD" == *"/"* ]]; then
        DB_PASSWORD=$(openssl rand -hex 16)
    fi
    
    # Create user and database
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
    
    # Setup subuid/subgid for rootless Podman
    if ! grep -q "^narvana:" /etc/subuid 2>/dev/null; then
        echo "narvana:165536:65536" >> /etc/subuid
    fi
    if ! grep -q "^narvana:" /etc/subgid 2>/dev/null; then
        echo "narvana:165536:65536" >> /etc/subgid
    fi
    
    # Create all required directories
    mkdir -p /opt/narvana /opt/narvana/.config/containers /var/log/narvana /var/lib/narvana/builds /etc/narvana
    chown -R narvana:narvana /opt/narvana /var/log/narvana /var/lib/narvana
    chmod 755 /var/lib/narvana /var/lib/narvana/builds
    
    log_success "User configured"
}

# Install Nix package manager (required for builds)
install_nix() {
    if command -v nix &> /dev/null; then
        log_success "Nix already installed"
        return
    fi
    
    log_info "Installing Nix package manager..."
    
    # Create nix build users group if it doesn't exist
    if ! getent group nixbld > /dev/null 2>&1; then
        groupadd -r nixbld
    fi
    
    # Create nix build users
    for i in $(seq 1 10); do
        if ! id "nixbld$i" &>/dev/null; then
            useradd -r -g nixbld -G nixbld -d /var/empty -s /sbin/nologin "nixbld$i" 2>/dev/null || true
        fi
    done
    
    # Install Nix in multi-user mode
    sh <(curl -L https://nixos.org/nix/install) --daemon --yes || {
        log_error "Nix installation failed"
        exit 1
    }
    
    # Source nix profile for current session
    if [[ -f /etc/profile.d/nix.sh ]]; then
        . /etc/profile.d/nix.sh
    fi
    
    # Enable and start nix-daemon
    systemctl enable nix-daemon 2>/dev/null || true
    systemctl start nix-daemon 2>/dev/null || true
    
    # Verify installation
    if command -v nix &> /dev/null; then
        log_success "Nix installed: $(nix --version)"
    else
        log_warn "Nix installed but not in PATH - may need shell restart"
    fi
}

# Install Attic binary cache server
install_attic() {
    log_info "Setting up Attic binary cache server..."
    
    # Source nix if available
    if [[ -f /etc/profile.d/nix.sh ]]; then
        . /etc/profile.d/nix.sh
    fi
    
    # Install attic (both server and client) using nix
    if ! command -v atticd &> /dev/null; then
        log_info "Installing Attic server and client..."
        nix profile install --extra-experimental-features "nix-command flakes" nixpkgs#attic-server nixpkgs#attic-client || {
            log_warn "Failed to install Attic via nix profile, trying nix-env..."
            nix-env -iA nixpkgs.attic-server nixpkgs.attic-client || {
                log_error "Failed to install Attic"
                return 1
            }
        }
    fi
    
    # Find atticd binary path
    ATTICD_PATH=$(which atticd 2>/dev/null || echo "/root/.nix-profile/bin/atticd")
    if [[ ! -x "$ATTICD_PATH" ]]; then
        for path in /root/.nix-profile/bin/atticd /nix/var/nix/profiles/default/bin/atticd; do
            if [[ -x "$path" ]]; then
                ATTICD_PATH="$path"
                break
            fi
        done
    fi
    log_info "Using atticd at: $ATTICD_PATH"
    
    # Find attic client binary path
    ATTIC_PATH=$(which attic 2>/dev/null || echo "/root/.nix-profile/bin/attic")
    
    # Create Attic data directories
    mkdir -p /var/lib/narvana/attic/storage
    mkdir -p /opt/narvana/.config/attic
    chown -R root:root /var/lib/narvana/attic
    chown -R narvana:narvana /opt/narvana/.config
    
    # Generate JWT secret for Attic (base64 encoded, min 32 bytes)
    ATTIC_JWT_SECRET=$(openssl rand -base64 32)
    
    # Create Attic server config
    cat > /etc/narvana/attic.toml << EOF
# Attic server configuration for Narvana
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

[garbage-collection]
default-retention-period = "30 days"

[jwt.signing]
token-hs256-secret-base64 = "${ATTIC_JWT_SECRET}"
EOF
    
    chmod 600 /etc/narvana/attic.toml
    
    # Create systemd service for Attic
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

Environment=HOME=/root
Environment=RUST_LOG=info

[Install]
WantedBy=multi-user.target
EOF
    
    systemctl daemon-reload
    systemctl enable narvana-attic
    systemctl restart narvana-attic
    
    # Wait for Attic server to start
    log_info "Waiting for Attic server to start..."
    for i in {1..10}; do
        if curl -sf http://localhost:5000/ > /dev/null 2>&1; then
            log_success "Attic server is running"
            break
        fi
        sleep 1
    done
    
    # Generate a proper JWT token for the worker
    # The token needs: sub (subject), exp (expiry), and the cache permissions
    # Using a long expiry (10 years) for the service token
    EXPIRY=$(($(date +%s) + 315360000))  # 10 years from now
    
    # Create JWT header and payload
    JWT_HEADER=$(echo -n '{"alg":"HS256","typ":"JWT"}' | base64 -w0 | tr '+/' '-_' | tr -d '=')
    JWT_PAYLOAD=$(echo -n "{\"sub\":\"narvana-worker\",\"exp\":${EXPIRY},\"https://jwt.attic.rs/v1\":{\"caches\":{\"narvana\":{\"r\":1,\"w\":1,\"cc\":1},\"*\":{\"r\":1,\"w\":1,\"cc\":1}}}}" | base64 -w0 | tr '+/' '-_' | tr -d '=')
    
    # Sign the token
    JWT_SIGNATURE=$(echo -n "${JWT_HEADER}.${JWT_PAYLOAD}" | openssl dgst -sha256 -hmac "$(echo -n ${ATTIC_JWT_SECRET} | base64 -d)" -binary | base64 -w0 | tr '+/' '-_' | tr -d '=')
    ATTIC_TOKEN="${JWT_HEADER}.${JWT_PAYLOAD}.${JWT_SIGNATURE}"
    
    # Configure attic client for the narvana user
    mkdir -p /opt/narvana/.config/attic
    cat > /opt/narvana/.config/attic/config.toml << EOF
# Attic client configuration
default-server = "narvana"

[servers.narvana]
endpoint = "http://localhost:5000"
token = "${ATTIC_TOKEN}"
EOF
    
    chown -R narvana:narvana /opt/narvana/.config/attic
    chmod 600 /opt/narvana/.config/attic/config.toml
    
    # Also configure for root (used during builds in containers)
    mkdir -p /root/.config/attic
    cat > /root/.config/attic/config.toml << EOF
# Attic client configuration
default-server = "narvana"

[servers.narvana]
endpoint = "http://localhost:5000"
token = "${ATTIC_TOKEN}"
EOF
    
    chmod 600 /root/.config/attic/config.toml
    
    # Create the narvana cache
    log_info "Creating narvana cache..."
    ${ATTIC_PATH} cache create narvana 2>/dev/null || log_info "Cache 'narvana' may already exist"
    
    # Verify the setup works
    if ${ATTIC_PATH} cache info narvana > /dev/null 2>&1; then
        log_success "Attic cache 'narvana' is configured and accessible"
    else
        log_warn "Could not verify Attic cache - check: journalctl -u narvana-attic"
    fi
    
    # Store the token in the environment file for the worker
    if ! grep -q "ATTIC_TOKEN=" /etc/narvana/control-plane.env 2>/dev/null; then
        echo "" >> /etc/narvana/control-plane.env
        echo "# Attic binary cache token" >> /etc/narvana/control-plane.env
        echo "ATTIC_TOKEN=${ATTIC_TOKEN}" >> /etc/narvana/control-plane.env
    fi
    
    log_success "Attic binary cache configured with authentication"
}

# Setup environment
setup_environment() {
    log_info "Configuring environment..."
    
    # Detect public IP
    PUBLIC_IP=$(curl -s --max-time 5 https://ifconfig.me || \
                curl -s --max-time 5 https://ifconfig.io || \
                curl -s --max-time 5 https://icanhazip.com || \
                hostname -I | awk '{print $1}' || \
                echo "your-server-ip")
    
    # Generate secrets if needed
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
    
    # Get narvana user's UID for XDG_RUNTIME_DIR
    local NARVANA_UID=$(id -u narvana)
    
    # Create XDG_RUNTIME_DIR for rootless Podman
    mkdir -p "/run/user/${NARVANA_UID}"
    chown narvana:narvana "/run/user/${NARVANA_UID}"
    chmod 700 "/run/user/${NARVANA_UID}"
    
    cd "$INSTALL_DIR"
    
    # Update XDG_RUNTIME_DIR in service file with actual UID
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
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    
    check_root
    detect_system
    get_latest_version
    install_dependencies
    install_podman
    install_nix
    setup_postgresql
    create_user
    clone_repo
    generate_web_assets
    download_binaries
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
