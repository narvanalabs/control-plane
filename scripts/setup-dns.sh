#!/usr/bin/env bash
# Setup script for wildcard DNS using dnsmasq
# Supports Linux and macOS

set -euo pipefail

DOMAIN="narvana.local"
DNSMASQ_CONF="/etc/dnsmasq.conf"
RESOLVER_DIR="/etc/resolver"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() {
    echo -e "${GREEN}ℹ${NC} $1"
}

warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
}

success() {
    echo -e "${GREEN}✓${NC} $1"
}

# Detect OS
detect_os() {
    if [[ -f /etc/os-release ]]; then
        if grep -q "ID=nixos" /etc/os-release 2>/dev/null; then
            echo "nixos"
            return
        fi
    fi
    
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        echo "linux"
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        echo "macos"
    else
        error "Unsupported OS: $OSTYPE"
        exit 1
    fi
}

# Check if running as root/sudo
check_root() {
    if [[ $EUID -ne 0 ]]; then
        error "This script must be run as root (use sudo)"
        exit 1
    fi
}

# Check if dnsmasq is installed
check_dnsmasq() {
    local os=$1
    
    # On NixOS, check if the service is running instead of the command
    if [[ "$os" == "nixos" ]]; then
        if systemctl is-active --quiet dnsmasq 2>/dev/null || systemctl is-enabled --quiet dnsmasq 2>/dev/null; then
            return 0
        fi
        return 1
    fi
    
    # For other OSes, check if command exists
    if ! command -v dnsmasq &> /dev/null; then
        return 1
    fi
    return 0
}

# Install dnsmasq
install_dnsmasq() {
    local os=$1
    info "Installing dnsmasq..."
    
    if [[ "$os" == "nixos" ]]; then
        error "On NixOS, dnsmasq must be configured through your NixOS configuration."
        echo ""
        info "Add this to your NixOS configuration (e.g., configuration.nix or flake.nix):"
        echo ""
        echo "  services.dnsmasq = {"
        echo "    enable = true;"
        echo "    settings = {"
        echo "      address = \"/.${DOMAIN}/127.0.0.1\";"
        echo "    };"
        echo "  };"
        echo ""
        info "Then run: sudo nixos-rebuild switch"
        echo ""
        exit 1
    elif [[ "$os" == "macos" ]]; then
        if command -v brew &> /dev/null; then
            brew install dnsmasq
        else
            error "Homebrew not found. Please install Homebrew first: https://brew.sh"
            exit 1
        fi
    elif [[ "$os" == "linux" ]]; then
        if command -v apt-get &> /dev/null; then
            apt-get update && apt-get install -y dnsmasq
        elif command -v yum &> /dev/null; then
            yum install -y dnsmasq
        elif command -v dnf &> /dev/null; then
            dnf install -y dnsmasq
        else
            error "Unsupported package manager. Please install dnsmasq manually."
            exit 1
        fi
    fi
    
    success "dnsmasq installed"
}

# Configure dnsmasq for Linux
configure_linux() {
    local os=$1
    
    # Check if this is NixOS
    if [[ "$os" == "nixos" ]]; then
        configure_nixos
        return
    fi
    
    info "Configuring dnsmasq for Linux..."
    
    # Backup original config
    if [[ -f "$DNSMASQ_CONF" ]] && ! grep -q "# Narvana wildcard DNS" "$DNSMASQ_CONF"; then
        cp "$DNSMASQ_CONF" "${DNSMASQ_CONF}.backup.$(date +%Y%m%d_%H%M%S)"
        info "Backed up original config to ${DNSMASQ_CONF}.backup.*"
    fi
    
    # Add configuration if not already present
    if ! grep -q "# Narvana wildcard DNS" "$DNSMASQ_CONF" 2>/dev/null; then
        echo "" >> "$DNSMASQ_CONF"
        echo "# Narvana wildcard DNS" >> "$DNSMASQ_CONF"
        echo "address=/.${DOMAIN}/127.0.0.1" >> "$DNSMASQ_CONF"
        success "Added wildcard DNS configuration to $DNSMASQ_CONF"
    else
        warn "Wildcard DNS configuration already exists in $DNSMASQ_CONF"
    fi
    
    # Configure systemd-resolved to use dnsmasq (if systemd is available)
    if systemctl is-active --quiet systemd-resolved 2>/dev/null; then
        info "Configuring systemd-resolved to use dnsmasq..."
        
        RESOLVED_CONF="/etc/systemd/resolved.conf.d/narvana.conf"
        mkdir -p "$(dirname "$RESOLVED_CONF")"
        
        if [[ ! -f "$RESOLVED_CONF" ]]; then
            cat > "$RESOLVED_CONF" <<EOF
[Resolve]
DNS=127.0.0.1
Domains=${DOMAIN}
EOF
            success "Created systemd-resolved config: $RESOLVED_CONF"
        else
            warn "systemd-resolved config already exists: $RESOLVED_CONF"
        fi
    fi
    
    # Restart services (skip on NixOS - handled by configuration)
    if systemctl list-unit-files | grep -q "dnsmasq.service" 2>/dev/null; then
        if systemctl is-active --quiet dnsmasq 2>/dev/null; then
            systemctl restart dnsmasq
            success "Restarted dnsmasq service"
        else
            systemctl enable dnsmasq
            systemctl start dnsmasq
            success "Started and enabled dnsmasq service"
        fi
    else
        warn "dnsmasq.service not found in systemd. If you're on NixOS, configure it through your NixOS config."
    fi
    
    if systemctl is-active --quiet systemd-resolved 2>/dev/null; then
        systemctl restart systemd-resolved
        success "Restarted systemd-resolved service"
    fi
}

# Test DNS resolution using multiple methods
# Uses Nix-provided DNS tools (dig, host, nslookup) via environment variables
test_dns_resolution() {
    local test_domain="test.${DOMAIN}"
    local resolved=false
    
    # Use Nix-provided tools if available (set by Nix wrapper)
    local DIG="${DIG:-dig}"
    local HOST="${HOST:-host}"
    local NSLOOKUP="${NSLOOKUP:-nslookup}"
    local GETENT="${GETENT:-getent}"
    
    # Try dig first (most reliable)
    if command -v "$DIG" &> /dev/null || [[ -n "${DIG_PATH:-}" ]]; then
        local dig_cmd="${DIG_PATH:-$DIG}"
        if "$dig_cmd" +short "${test_domain}" @127.0.0.1 2>/dev/null | grep -q "127.0.0.1"; then
            resolved=true
        fi
    # Try host as fallback
    elif command -v "$HOST" &> /dev/null || [[ -n "${HOST_PATH:-}" ]]; then
        local host_cmd="${HOST_PATH:-$HOST}"
        if "$host_cmd" "${test_domain}" 127.0.0.1 2>/dev/null | grep -q "127.0.0.1"; then
            resolved=true
        fi
    # Try nslookup as fallback
    elif command -v "$NSLOOKUP" &> /dev/null || [[ -n "${NSLOOKUP_PATH:-}" ]]; then
        local nslookup_cmd="${NSLOOKUP_PATH:-$NSLOOKUP}"
        if "$nslookup_cmd" "${test_domain}" 127.0.0.1 2>/dev/null | grep -q "127.0.0.1"; then
            resolved=true
        fi
    # Try getent as last resort (may not work with dnsmasq)
    elif command -v "$GETENT" &> /dev/null || [[ -n "${GETENT_PATH:-}" ]]; then
        local getent_cmd="${GETENT_PATH:-$GETENT}"
        if "$getent_cmd" hosts "${test_domain}" 2>/dev/null | grep -q "127.0.0.1"; then
            resolved=true
        fi
    fi
    
    echo "$resolved"
}

# Configure dnsmasq for NixOS
configure_nixos() {
    info "Configuring dnsmasq for NixOS..."
    echo ""
    
    # Check if dnsmasq is already configured and running
    if systemctl is-active --quiet dnsmasq 2>/dev/null; then
        success "dnsmasq service is running"
        
        # Test if the wildcard DNS is configured correctly
        info "Testing DNS configuration..."
        if [[ "$(test_dns_resolution)" == "true" ]]; then
            success "Wildcard DNS is configured correctly! test.${DOMAIN} resolves to 127.0.0.1"
        else
            warn "dnsmasq is running but DNS test failed (DNS tools may not be in PATH)."
            info "To test manually, run:"
            echo "  nix-shell -p bind.dnsutils --run 'dig test.${DOMAIN} @127.0.0.1'"
            echo ""
            info "If the test fails, make sure your NixOS configuration includes:"
            echo ""
            echo "  services.dnsmasq = {"
            echo "    enable = true;"
            echo "    settings = {"
            echo "      address = \"/.${DOMAIN}/127.0.0.1\";"
            echo "    };"
            echo "  };"
            echo ""
            info "Then run: sudo nixos-rebuild switch"
        fi
    else
        warn "dnsmasq service is not running. Please add it to your NixOS configuration."
        echo ""
        info "Add this to your NixOS configuration (e.g., configuration.nix or flake.nix):"
        echo ""
        echo "  services.dnsmasq = {"
        echo "    enable = true;"
        echo "    settings = {"
        echo "      address = \"/.${DOMAIN}/127.0.0.1\";"
        echo "    };"
        echo "  };"
        echo ""
        info "See deploy/nixos-dnsmasq.nix for a complete example."
        echo ""
        info "After updating your configuration, run:"
        echo "  sudo nixos-rebuild switch"
    fi
}

# Configure dnsmasq for macOS
configure_macos() {
    info "Configuring dnsmasq for macOS..."
    
    # Backup original config
    if [[ -f "$DNSMASQ_CONF" ]] && ! grep -q "# Narvana wildcard DNS" "$DNSMASQ_CONF"; then
        cp "$DNSMASQ_CONF" "${DNSMASQ_CONF}.backup.$(date +%Y%m%d_%H%M%S)"
        info "Backed up original config to ${DNSMASQ_CONF}.backup.*"
    fi
    
    # Add configuration if not already present
    if ! grep -q "# Narvana wildcard DNS" "$DNSMASQ_CONF" 2>/dev/null; then
        echo "" >> "$DNSMASQ_CONF"
        echo "# Narvana wildcard DNS" >> "$DNSMASQ_CONF"
        echo "address=/.${DOMAIN}/127.0.0.1" >> "$DNSMASQ_CONF"
        success "Added wildcard DNS configuration to $DNSMASQ_CONF"
    else
        warn "Wildcard DNS configuration already exists in $DNSMASQ_CONF"
    fi
    
    # Create resolver directory and config
    mkdir -p "$RESOLVER_DIR"
    RESOLVER_FILE="${RESOLVER_DIR}/${DOMAIN}"
    
    if [[ ! -f "$RESOLVER_FILE" ]]; then
        cat > "$RESOLVER_FILE" <<EOF
nameserver 127.0.0.1
port 5353
EOF
        chmod 644 "$RESOLVER_FILE"
        success "Created resolver config: $RESOLVER_FILE"
    else
        warn "Resolver config already exists: $RESOLVER_FILE"
    fi
    
    # Restart dnsmasq via Homebrew
    if brew services list | grep -q "dnsmasq.*started"; then
        brew services restart dnsmasq
        success "Restarted dnsmasq service"
    else
        brew services start dnsmasq
        success "Started dnsmasq service"
    fi
}

# Test DNS resolution
test_dns() {
    info "Testing DNS resolution..."
    
    # Wait a moment for DNS to propagate
    sleep 2
    
    if [[ "$(test_dns_resolution)" == "true" ]]; then
        success "DNS resolution working! test.${DOMAIN} resolves to 127.0.0.1"
    else
        warn "DNS test failed or DNS tools not available."
        info "To test manually, install a DNS tool and run:"
        echo "  nix-shell -p bind.dnsutils --run 'dig test.${DOMAIN} @127.0.0.1'"
        echo ""
        warn "You may also need to flush your DNS cache:"
        if [[ "$(detect_os)" == "macos" ]]; then
            echo "  sudo dscacheutil -flushcache; sudo killall -HUP mDNSResponder"
        else
            echo "  sudo systemd-resolve --flush-caches || sudo resolvectl flush-caches"
        fi
    fi
}

# Main execution
main() {
    echo "=========================================="
    echo "  Narvana Wildcard DNS Setup"
    echo "=========================================="
    echo ""
    
    check_root
    
    local os=$(detect_os)
    info "Detected OS: $os"
    
    if ! check_dnsmasq "$os"; then
        warn "dnsmasq not found"
        install_dnsmasq "$os"
    else
        if [[ "$os" == "nixos" ]]; then
            success "dnsmasq service is configured/running"
        else
            success "dnsmasq is already installed"
        fi
    fi
    
    if [[ "$os" == "macos" ]]; then
        configure_macos
    elif [[ "$os" == "nixos" ]]; then
        configure_nixos
    else
        configure_linux "$os"
    fi
    
    test_dns
    
    echo ""
    success "Setup complete!"
    echo ""
    info "You can now access services via: http://{app}-{service}.${DOMAIN}:8088"
    echo ""
    warn "Note: You may need to flush your DNS cache if resolution doesn't work immediately:"
    if [[ "$os" == "macos" ]]; then
        echo "  sudo dscacheutil -flushcache; sudo killall -HUP mDNSResponder"
    else
        echo "  sudo systemd-resolve --flush-caches"
    fi
}

main "$@"

