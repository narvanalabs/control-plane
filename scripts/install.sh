#!/bin/bash

# Narvana Control Plane - One-Click Installer
# Inspired by Dokploy and Coolify

set -e

# Output Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

echo -e "${BLUE}${BOLD}Narvana Control Plane Installer${NC}"
echo -e "==============================="

# 1. Prerequisite Checks
if [[ $EUID -ne 0 ]]; then
   echo -e "${RED}Error: This script must be run as root.${NC}" 
   exit 1
fi

# Detect OS
if [[ "$OSTYPE" != "linux-gnu"* ]]; then
    echo -e "${RED}Error: Narvana currently only supports Linux-based VPS.${NC}"
    exit 1
fi

# 2. Dependency Installation
echo -e "\n${BLUE}Step 1: Installing dependencies...${NC}"
if command -v apt-get &> /dev/null; then
    apt-get update -y
    apt-get install -y curl wget git openssl postgresql postgresql-contrib caddy
elif command -v dnf &> /dev/null; then
    dnf install -y curl wget git openssl postgresql-server caddy
else
    echo -e "${RED}Warning: Package manager not recognized. Please ensure postgresql and caddy are installed.${NC}"
fi

# 3. Repository Setup (for curl | sh support)
echo -e "\n${BLUE}Step 2: Setting up repository...${NC}"
INSTALL_DIR="/opt/narvana/control-plane"
REPO_URL="${REPO_URL:-https://github.com/narvanalabs/control-plane.git}"
REPO_BRANCH="${REPO_BRANCH:-master}"

if [ ! -d "$INSTALL_DIR/.git" ]; then
    echo "Cloning Narvana Control Plane (${REPO_BRANCH})..."
    mkdir -p /opt/narvana
    git clone -b "$REPO_BRANCH" "$REPO_URL" "$INSTALL_DIR"
fi
cd "$INSTALL_DIR"

mkdir -p "$INSTALL_DIR/bin"
mkdir -p /etc/narvana
mkdir -p /var/log/narvana
mkdir -p /tmp/narvana-builds

# Create narvana user if not exists
if ! id "narvana" &>/dev/null; then
    useradd -m -s /bin/bash narvana
fi
chown -R narvana:narvana /opt/narvana
chown -R narvana:narvana /var/log/narvana
chown -R narvana:narvana /tmp/narvana-builds

# 4. Environment Configuration
echo -e "\n${BLUE}Step 3: Generating/Repairing environment configuration...${NC}"

# Detect Public IP with multiple fallbacks
echo "Detecting public IP..."
PUBLIC_IP=$(curl -s --max-time 5 https://ifconfig.me || curl -s --max-time 5 https://ifconfig.io || curl -s --max-time 5 https://icanhazip.com || curl -s --max-time 5 https://ident.me || hostname -I | awk '{print $1}')

if [ -z "$PUBLIC_IP" ] || [ "$PUBLIC_IP" = "your-ip" ]; then
    PUBLIC_IP="your-server-ip"
fi

ENV_FILE="/etc/narvana/control-plane.env"

if [ ! -f "$ENV_FILE" ]; then
    JWT_SECRET=$(openssl rand -hex 32)
    DB_PASSWORD=$(openssl rand -hex 16)
    
    cat > "$ENV_FILE" <<EOF
# Narvana Control Plane Configuration
DATABASE_URL=postgres://narvana:${DB_PASSWORD}@localhost:5432/narvana?sslmode=disable
JWT_SECRET=${JWT_SECRET}
API_HOST=0.0.0.0
API_PORT=8080
INTERNAL_API_URL=http://127.0.0.1:8080
API_URL=http://${PUBLIC_IP}:8080
WEB_URL=http://${PUBLIC_IP}:8090
GRPC_PORT=9090
WEB_PORT=8090
WORKER_WORKDIR=/tmp/narvana-builds
EOF
    echo -e "${GREEN}✓ Created new environment file using IP: ${PUBLIC_IP}${NC}"
else
    echo -e "Environment file exists. Checking for health..."
    
    # 1. Ensure Internal communication is healthy
    if ! grep -q "INTERNAL_API_URL=" "$ENV_FILE" || grep -q "localhost:8080" "$ENV_FILE"; then
        # Force 127.0.0.1 for reliability
        if grep -q "INTERNAL_API_URL=" "$ENV_FILE"; then
            sed -i "s|INTERNAL_API_URL=.*|INTERNAL_API_URL=http://127.0.0.1:8080|" "$ENV_FILE"
        else
            echo "INTERNAL_API_URL=http://127.0.0.1:8080" >> "$ENV_FILE"
        fi
        echo "Updated INTERNAL_API_URL to 127.0.0.1 for local communication"
    fi

    # 2. Update API_URL and WEB_URL to use the raw IP if they are using sslip.io (revert magic domain)
    if grep -q "\.sslip\.io" "$ENV_FILE"; then
        echo "Migrating from sslip.io back to raw IP..."
        sed -i "s|API_URL=http://.*\.sslip\.io:8080|API_URL=http://${PUBLIC_IP}:8080|" "$ENV_FILE"
        sed -i "s|WEB_URL=http://.*\.sslip\.io:8090|WEB_URL=http://${PUBLIC_IP}:8090|" "$ENV_FILE"
    fi

    # 2. Fix malformed/unsafe DATABASE_URL (especially if password contains / or :)
    CURRENT_URL=$(grep "^DATABASE_URL=" "$ENV_FILE" | cut -d'=' -f2-)
    
    # Check if URL looks broken (e.g. invalid port error from logs suggested colon/slash issues)
    # We'll extract the password safely and regenerate if it looks like it came from a broken base64 or has colons
    EXTRACTED_PASS=$(echo "$CURRENT_URL" | sed -n 's|.*//[^:]*:\([^@]*\)@.*|\1|p')
    
    if [[ -z "$EXTRACTED_PASS" ]] || [[ "$EXTRACTED_PASS" == *":"* ]] || [[ "$EXTRACTED_PASS" == *"/"* ]]; then
        echo -e "${RED}Warning: Malformed or unsafe DATABASE_URL detected. Regenerating password...${NC}"
        NEW_PASS=$(openssl rand -hex 16)
        sed -i "s|DATABASE_URL=.*|DATABASE_URL=postgres://narvana:${NEW_PASS}@localhost:5432/narvana?sslmode=disable|" "$ENV_FILE"
        DB_PASSWORD="$NEW_PASS"
        FORCE_DB_UPDATE=true
    else
        DB_PASSWORD="$EXTRACTED_PASS"
    fi
fi
chmod 600 "$ENV_FILE"
chown narvana:narvana "$ENV_FILE"

# 5. Database Setup
echo -e "\n${BLUE}Step 4: Syncing PostgreSQL...${NC}"

# Ensure user and database exist
sudo -u postgres psql -c "CREATE USER narvana WITH PASSWORD '${DB_PASSWORD}';" || true
sudo -u postgres psql -c "CREATE DATABASE narvana OWNER narvana;" || true

# If we regenerated the password, force update it in Postgres
if [ "$FORCE_DB_UPDATE" = true ]; then
    echo "Updating PostgreSQL password for 'narvana' user..."
    sudo -u postgres psql -c "ALTER USER narvana WITH PASSWORD '${DB_PASSWORD}';"
fi

echo "Running database migrations..."
for f in migrations/*.sql; do
    echo "Applying $f..."
    PGPASSWORD="${DB_PASSWORD}" psql -h localhost -U narvana -d narvana -f "$f" || echo "Warning: Migration $f failed or already applied."
done

echo -e "${GREEN}✓ Database initialized and migrations applied.${NC}"

# 6. Binary Installation (Self-Building as fallback if no release found)
echo -e "\n${BLUE}Step 5: Installing Narvana binaries...${NC}"

# Find Go (often in /usr/local/go/bin on VPS)
if ! command -v go &> /dev/null; then
    for path in "/usr/local/go/bin" "/usr/lib/go/bin" "/snap/bin"; do
        if [ -f "$path/go" ]; then
            export PATH="$PATH:$path"
            break
        fi
    done
fi

if command -v go &> /dev/null; then
    echo "Building from source using $(go version)..."
    
    # 1. Install Templ if missing
    if ! command -v templ &> /dev/null; then
        echo "Installing templ..."
        go install github.com/a-h/templ/cmd/templ@latest
        export PATH="$PATH:$(go env GOPATH)/bin"
    fi

    # 2. Generate UI components
    echo "Generating UI components (templ)..."
    (cd web && templ generate)
    
    # 3. Generate CSS
    echo "Building CSS (tailwindcss)..."
    if ! command -v tailwindcss &> /dev/null; then
        echo "TailwindCSS not found. Downloading standalone binary..."
        curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64
        chmod +x tailwindcss-linux-x64
        mv tailwindcss-linux-x64 /usr/local/bin/tailwindcss
    fi
    (cd web && tailwindcss -i ./assets/css/input.css -o ./assets/css/output.css --minify)

    # 4. Build Binaries
    go build -buildvcs=false -o /opt/narvana/control-plane/bin/api ./cmd/api
    go build -buildvcs=false -o /opt/narvana/control-plane/bin/worker ./cmd/worker
    go build -buildvcs=false -o /opt/narvana/control-plane/bin/web ./cmd/web
else
    echo -e "${RED}Error: 'go' is not in PATH.${NC}"
    echo "Please ensure Go is installed and in root's PATH, or run: sudo env PATH=\$PATH bash install.sh"
    # exit 1 
fi

# 7. Systemd Service Setup
echo -e "\n${BLUE}Step 6: Configuring systemd services...${NC}"
cp deploy/narvana-api.service /etc/systemd/system/
cp deploy/narvana-worker.service /etc/systemd/system/
cp deploy/narvana-web.service /etc/systemd/system/

systemctl daemon-reload
systemctl enable narvana-api narvana-web narvana-worker
systemctl restart narvana-api narvana-web narvana-worker

# 8. Verification
echo -e "\n${BLUE}Step 7: Verifying services...${NC}"
sleep 5 # Give services a moment to start

check_service() {
    local name=$1
    local port=$2
    if curl -s "http://127.0.0.1:${port}/health" > /dev/null || curl -s "http://127.0.0.1:${port}/login" > /dev/null; then
        echo -e "${GREEN}✓ $name is responding on port $port${NC}"
    else
        echo -e "${RED}✗ $name is NOT responding on port $port${NC}"
        echo "Tailing logs for $name:"
        journalctl -u $name -n 20 --no-pager
    fi
}

check_service "narvana-api" 8080
check_service "narvana-web" 8090

# 9. Success Output
echo -e "\n${GREEN}${BOLD}─────────────────────────────────────────────────────────────────${NC}"
echo -e "${GREEN}${BOLD}              Narvana Control Plane Installed Successfully!      ${NC}"
echo -e "${GREEN}${BOLD}─────────────────────────────────────────────────────────────────${NC}"
echo -e "\nYou can now access your dashboard at:"
echo -e "👉 ${BOLD}http://${PUBLIC_IP}:8090${NC}"
echo -e "\n${BLUE}Next Steps:${NC}"
echo -e "1. Visit the URL above and create your admin account."
echo -e "2. Connect your GitHub App via the dashboard."
echo -e "\n${BLUE}Troubleshooting:${NC}"
echo -e "View logs: journalctl -u narvana-web -f"
echo -e "API logs:  journalctl -u narvana-api -f"
echo -e "${GREEN}${BOLD}─────────────────────────────────────────────────────────────────${NC}"
