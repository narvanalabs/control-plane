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

# Create narvana user
create_user() {
    if ! id "narvana" &>/dev/null; then
        log_info "Creating narvana user..."
        useradd -r -m -s /bin/bash -d /opt/narvana narvana || true
    fi
    
    mkdir -p /opt/narvana /var/log/narvana /tmp/narvana-builds /etc/narvana
    chown -R narvana:narvana /opt/narvana /var/log/narvana /tmp/narvana-builds
    
    log_success "User configured"
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
WORKER_WORKDIR=/tmp/narvana-builds
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
    
    cd "$INSTALL_DIR"
    cp deploy/narvana-api.service /etc/systemd/system/
    cp deploy/narvana-worker.service /etc/systemd/system/
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
    setup_postgresql
    clone_repo
    generate_web_assets
    download_binaries
    create_user
    setup_environment
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
