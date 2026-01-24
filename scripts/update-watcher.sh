#!/bin/bash
# Narvana Update Watcher
# This script monitors for update requests and performs the actual container restart

set -euo pipefail

UPDATE_FLAG_FILE="${UPDATE_FLAG_FILE:-/var/lib/narvana/.update-requested}"
COMPOSE_FILE="${COMPOSE_FILE:-/opt/narvana/compose.yaml}"
ENV_FILE="${ENV_FILE:-/opt/narvana/.env}"
COMPOSE_CMD="${COMPOSE_CMD:-podman-compose}"

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

perform_update() {
    local version=$1
    log "Performing update to version $version..."
    
    # Update the version in .env
    if [[ -f "$ENV_FILE" ]]; then
        if grep -q "^NARVANA_VERSION=" "$ENV_FILE"; then
            sed -i "s/^NARVANA_VERSION=.*/NARVANA_VERSION=$version/" "$ENV_FILE"
        else
            echo "NARVANA_VERSION=$version" >> "$ENV_FILE"
        fi
        log "Updated .env with version $version"
    fi
    
    # Pull new images
    log "Pulling new container images..."
    cd "$(dirname "$COMPOSE_FILE")"
    $COMPOSE_CMD pull || {
        log "ERROR: Failed to pull images"
        return 1
    }
    
    # Restart services
    log "Restarting services..."
    $COMPOSE_CMD down || log "WARN: Failed to stop services cleanly"
    $COMPOSE_CMD up -d || {
        log "ERROR: Failed to start services"
        return 1
    }
    
    log "Update to $version completed successfully"
    
    # Remove the flag file
    rm -f "$UPDATE_FLAG_FILE"
}

# Main loop
log "Narvana Update Watcher started"
log "Monitoring: $UPDATE_FLAG_FILE"
log "Compose file: $COMPOSE_FILE"

while true; do
    if [[ -f "$UPDATE_FLAG_FILE" ]]; then
        log "Update request detected"
        
        # Read the requested version
        version=$(grep "^version=" "$UPDATE_FLAG_FILE" | cut -d= -f2)
        
        if [[ -z "$version" ]]; then
            log "ERROR: No version specified in update flag"
            rm -f "$UPDATE_FLAG_FILE"
        else
            perform_update "$version"
        fi
    fi
    
    sleep 10
done

