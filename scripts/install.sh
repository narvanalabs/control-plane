#!/usr/bin/env bash
set -euo pipefail

# Narvana Control Plane Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/narvanalabs/control-plane/master/scripts/install.sh | bash

VERSION="${NARVANA_VERSION:-latest}"
INSTALL_DIR="${NARVANA_INSTALL_DIR:-/opt/narvana}"
GITHUB_RAW="https://raw.githubusercontent.com/narvanalabs/control-plane/master"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

info()    { echo -e "${BLUE}${BOLD}➜${NC} $1"; }
success() { echo -e "${GREEN}${BOLD}✓${NC} $1"; }
warn()    { echo -e "${YELLOW}${BOLD}!${NC} $1"; }
error()   { echo -e "${RED}${BOLD}✗${NC} $1"; exit 1; }

# -----------------------------------------------------------------------------
# Preflight checks
# -----------------------------------------------------------------------------

check_root() {
    if [[ $EUID -ne 0 ]]; then
        error "Please run as root: sudo bash -c \"\$(curl -fsSL $GITHUB_RAW/scripts/install.sh)\""
    fi
}

check_os() {
    if [[ "$OSTYPE" != "linux-gnu"* ]]; then
        error "Narvana only supports Linux"
    fi
    success "Linux detected"
}

check_container_runtime() {
    # Podman is preferred - check it first
    if command -v podman &>/dev/null; then
        RUNTIME="podman"
        if command -v podman-compose &>/dev/null; then
            COMPOSE_CMD="podman-compose"
        elif podman compose version &>/dev/null 2>&1; then
            COMPOSE_CMD="podman compose"
        else
            error "Podman found but podman-compose is missing. Install: sudo dnf install podman-compose (or pip3 install podman-compose)"
        fi
        success "Podman detected (using $COMPOSE_CMD)"
    elif command -v docker &>/dev/null && docker info &>/dev/null; then
        RUNTIME="docker"
        COMPOSE_CMD="docker compose"
        warn "Docker detected (Podman is recommended)"
    else
        error "Podman is required. Install: sudo dnf install podman podman-compose"
    fi
}

check_curl() {
    command -v curl &>/dev/null || error "curl is required"
}

# -----------------------------------------------------------------------------
# Installation
# -----------------------------------------------------------------------------

create_directories() {
    info "Creating directories..."
    mkdir -p "$INSTALL_DIR"
    cd "$INSTALL_DIR"
    success "Install directory: $INSTALL_DIR"
}

download_compose_file() {
    info "Downloading compose file..."
    curl -fsSL "$GITHUB_RAW/deploy/compose.yaml" -o compose.yaml
    success "compose.yaml downloaded"
}

generate_env() {
    if [[ -f .env ]]; then
        warn ".env already exists, preserving existing configuration"
        if ! grep -q "^NARVANA_VERSION=" .env; then
            echo "NARVANA_VERSION=$VERSION" >> .env
        fi
        return
    fi

    info "Generating configuration..."

    # Generate secrets
    POSTGRES_PASSWORD=$(openssl rand -hex 16)
    JWT_SECRET=$(openssl rand -hex 32)

    # Detect public IP
    PUBLIC_IP=$(curl -sf --max-time 5 https://ifconfig.me 2>/dev/null || \
                curl -sf --max-time 5 https://api.ipify.org 2>/dev/null || \
                hostname -I 2>/dev/null | awk '{print $1}' || \
                echo "localhost")

    cat > .env <<EOF
# Narvana Control Plane Configuration
# Generated: $(date -Iseconds)

# Version
NARVANA_VERSION=$VERSION

# Database (auto-generated, do not share)
POSTGRES_PASSWORD=$POSTGRES_PASSWORD

# Authentication (auto-generated, do not share)
JWT_SECRET=$JWT_SECRET

# Ports
API_PORT=8080
GRPC_PORT=9090
WEB_PORT=8090

# Worker
WORKER_MAX_CONCURRENCY=4
BUILD_TIMEOUT=30m
EOF

    chmod 600 .env
    success "Configuration generated (.env)"
}

pull_images() {
    info "Pulling container images..."
    $COMPOSE_CMD pull
    success "Images pulled"
}

start_services() {
    info "Starting services..."
    $COMPOSE_CMD up -d
    success "Services started"
}

wait_for_postgres() {
    info "Waiting for PostgreSQL to be ready..."
    local max_wait=30
    local waited=0

    while [[ $waited -lt $max_wait ]]; do
        if $RUNTIME exec narvana-postgres pg_isready -U narvana -d narvana &>/dev/null; then
            success "PostgreSQL is ready"
            return 0
        fi
        sleep 2
        waited=$((waited + 2))
        echo -n "."
    done
    echo ""
    warn "PostgreSQL health check timed out"
    return 1
}

run_migrations() {
    info "Running database migrations..."
    
    # List of migration files in order
    local migrations=(
        "001_initial_schema.sql"
        "002_build_queue.sql"
        "003_deployment_depends_on.sql"
        "004_users.sql"
        "005_service_source_config.sql"
        "006_remove_app_build_type.sql"
        "007_flexible_build_strategies.sql"
        "008_github_app_settings.sql"
        "009_add_config_type_to_github_app_settings.sql"
        "010_github_accounts.sql"
        "011_settings.sql"
        "012_add_user_profile_fields.sql"
        "013_add_app_icon_url.sql"
        "014_add_auto_database_strategy.sql"
        "015_add_domains_table.sql"
        "016_organizations.sql"
        "017_user_roles.sql"
        "018_invitations.sql"
        "019_domain_wildcard_verified.sql"
        "020_app_version_optimistic_locking.sql"
        "021_buildjob_source_fields.sql"
        "022_node_disk_metrics.sql"
        "023_replace_resource_tier_with_resources.sql"
        "024_build_detection_result.sql"
    )
    
    for migration in "${migrations[@]}"; do
        curl -fsSL "$GITHUB_RAW/migrations/$migration" 2>/dev/null | \
            $RUNTIME exec -i narvana-postgres psql -U narvana -d narvana -q 2>/dev/null || true
    done
    
    success "Migrations completed"
}

wait_for_healthy() {
    info "Waiting for API to be healthy..."
    local max_wait=60
    local waited=0

    while [[ $waited -lt $max_wait ]]; do
        if curl -sf --max-time 2 http://localhost:8080/health &>/dev/null; then
            success "API is healthy"
            break
        fi
        sleep 2
        waited=$((waited + 2))
        echo -n "."
    done
    echo ""

    if [[ $waited -ge $max_wait ]]; then
        warn "Health check timed out. Services may still be starting."
        echo "  Check status: cd $INSTALL_DIR && $COMPOSE_CMD ps"
        echo "  View logs:    cd $INSTALL_DIR && $COMPOSE_CMD logs -f"
    fi
}

# -----------------------------------------------------------------------------
# Output
# -----------------------------------------------------------------------------

print_success() {
    # Try multiple services to get public IPv4 (use -4 to force IPv4)
    PUBLIC_IP=$(curl -4 -sf --max-time 3 https://ifconfig.me 2>/dev/null || \
                curl -4 -sf --max-time 3 https://api.ipify.org 2>/dev/null || \
                curl -4 -sf --max-time 3 https://icanhazip.com 2>/dev/null || \
                hostname -I 2>/dev/null | awk '{print $1}' || \
                echo "YOUR_SERVER_IP")

    echo ""
    echo -e "${GREEN}${BOLD}╔════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}${BOLD}║         Narvana Control Plane - Installation Complete          ║${NC}"
    echo -e "${GREEN}${BOLD}╚════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${BOLD}Dashboard:${NC}  http://${PUBLIC_IP}:8090"
    echo -e "${BOLD}API:${NC}        http://${PUBLIC_IP}:8080"
    echo ""
    echo -e "${BOLD}Next steps:${NC}"
    echo "  1. Open the dashboard URL above"
    echo "  2. Create your admin account"
    echo "  3. Connect your GitHub organization"
    echo ""
    echo -e "${BOLD}Useful commands:${NC}"
    echo "  cd $INSTALL_DIR"
    echo "  $COMPOSE_CMD ps          # Check status"
    echo "  $COMPOSE_CMD logs -f     # View logs"
    echo "  $COMPOSE_CMD down        # Stop services"
    echo "  $COMPOSE_CMD pull && $COMPOSE_CMD up -d  # Update"
    echo ""
}

# -----------------------------------------------------------------------------
# Main
# -----------------------------------------------------------------------------

main() {
    echo ""
    echo -e "${BLUE}${BOLD}Narvana Control Plane Installer${NC}"
    echo ""

    check_root
    check_os
    check_curl
    check_container_runtime

    echo ""
    create_directories
    download_compose_file
    generate_env
    pull_images
    start_services
    wait_for_postgres
    run_migrations
    wait_for_healthy

    print_success
}

main "$@"
