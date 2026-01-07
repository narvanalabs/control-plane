#!/bin/bash
# Initialize Attic cache for Narvana development
# Run this once after starting Attic server

set -e

ATTIC_URL="${ATTIC_URL:-http://localhost:5000}"
CACHE_NAME="${CACHE_NAME:-narvana}"

echo "=== Initializing Attic Cache ==="
echo "Attic URL: $ATTIC_URL"
echo "Cache name: $CACHE_NAME"
echo ""

# Wait for Attic to be ready
echo "Waiting for Attic server..."
for i in {1..30}; do
    if curl -s "$ATTIC_URL" > /dev/null 2>&1; then
        echo "Attic is ready!"
        break
    fi
    sleep 1
done

# Login to Attic (creates config)
echo "Logging in to Attic..."
attic login narvana "$ATTIC_URL"

# Create cache (ignore if already exists)
echo "Creating cache '$CACHE_NAME'..."
attic cache create "$CACHE_NAME" 2>/dev/null || echo "Cache already exists or creation failed (this is OK for dev)"

# Configure cache as public for easy access
echo "Configuring cache..."
attic cache configure "$CACHE_NAME" --public 2>/dev/null || echo "Cache configure failed (this is OK for dev)"

echo ""
echo "=== Attic initialized successfully ==="
echo "Cache URL: $ATTIC_URL/$CACHE_NAME"
echo ""
echo "Node agents should use: AGENT_ATTIC_URL=$ATTIC_URL/$CACHE_NAME"



