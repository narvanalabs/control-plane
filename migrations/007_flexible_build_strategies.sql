-- Migration: 007_flexible_build_strategies.sql
-- Add flexible build strategy support to services and builds

-- ============================================================================
-- Part 1: Update services in apps.services JSONB to include build_strategy
-- ============================================================================

-- Update existing services to have build_strategy = 'flake' as default
-- Services are stored as JSONB array in the apps.services column
UPDATE apps
SET services = (
    SELECT COALESCE(
        jsonb_agg(
            CASE
                -- Service already has build_strategy: leave as-is
                WHEN service->>'build_strategy' IS NOT NULL AND service->>'build_strategy' != '' THEN
                    service
                -- Service has git_repo (git source): default to 'flake' strategy
                WHEN service->>'git_repo' IS NOT NULL AND service->>'git_repo' != '' THEN
                    service || jsonb_build_object(
                        'build_strategy', 'flake'
                    )
                -- Service has flake_uri (flake source): default to 'flake' strategy
                WHEN service->>'flake_uri' IS NOT NULL AND service->>'flake_uri' != '' THEN
                    service || jsonb_build_object(
                        'build_strategy', 'flake'
                    )
                -- Service has image (image source): no build strategy needed
                ELSE
                    service
            END
        ),
        '[]'::jsonb
    )
    FROM jsonb_array_elements(apps.services) AS service
)
WHERE jsonb_array_length(services) > 0;

-- ============================================================================
-- Part 2: Update builds table with new columns for flexible build strategies
-- ============================================================================

-- Add build_strategy column to builds table
ALTER TABLE builds
ADD COLUMN IF NOT EXISTS build_strategy VARCHAR(20) DEFAULT 'flake'
CHECK (build_strategy IN ('flake', 'auto-go', 'auto-rust', 'auto-node', 'auto-python', 'dockerfile', 'nixpacks', 'auto'));

-- Add build_config JSONB column for strategy-specific configuration
ALTER TABLE builds
ADD COLUMN IF NOT EXISTS build_config JSONB;

-- Add generated_flake column for storing auto-generated flake.nix content
ALTER TABLE builds
ADD COLUMN IF NOT EXISTS generated_flake TEXT;

-- Add flake_lock column for storing flake.lock content for reproducibility
ALTER TABLE builds
ADD COLUMN IF NOT EXISTS flake_lock TEXT;

-- Add vendor_hash column for storing calculated vendor hashes (Go, npm, Cargo)
ALTER TABLE builds
ADD COLUMN IF NOT EXISTS vendor_hash VARCHAR(255);

-- Add retry_count column for tracking build retry attempts
ALTER TABLE builds
ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0;

-- Add retry_as_oci column for tracking OCI fallback attempts
ALTER TABLE builds
ADD COLUMN IF NOT EXISTS retry_as_oci BOOLEAN NOT NULL DEFAULT false;

-- Add timeout_seconds column for configurable build timeouts
ALTER TABLE builds
ADD COLUMN IF NOT EXISTS timeout_seconds INTEGER NOT NULL DEFAULT 1800;

-- ============================================================================
-- Part 3: Add indexes for new columns
-- ============================================================================

-- Index for filtering builds by strategy
CREATE INDEX IF NOT EXISTS idx_builds_build_strategy ON builds(build_strategy);

-- Index for finding builds that retried as OCI
CREATE INDEX IF NOT EXISTS idx_builds_retry_as_oci ON builds(retry_as_oci) WHERE retry_as_oci = true;

-- ============================================================================
-- Part 4: Add comments documenting the new columns
-- ============================================================================

COMMENT ON COLUMN builds.build_strategy IS 'Build strategy: flake (existing flake.nix), auto-go/rust/node/python (generate flake), dockerfile (use Dockerfile), nixpacks (use Nixpacks), auto (auto-detect)';
COMMENT ON COLUMN builds.build_config IS 'JSONB containing strategy-specific configuration options (go_version, node_version, build_command, etc.)';
COMMENT ON COLUMN builds.generated_flake IS 'Auto-generated flake.nix content for auto-* strategies';
COMMENT ON COLUMN builds.flake_lock IS 'Flake lock file content for reproducible builds';
COMMENT ON COLUMN builds.vendor_hash IS 'Calculated vendor hash for dependency reproducibility (Go vendor hash, npm hash, Cargo hash)';
COMMENT ON COLUMN builds.retry_count IS 'Number of retry attempts for this build';
COMMENT ON COLUMN builds.retry_as_oci IS 'Whether this build was retried as OCI after pure-nix failure';
COMMENT ON COLUMN builds.timeout_seconds IS 'Build timeout in seconds (default: 1800 = 30 minutes)';

-- Update comment on apps.services to document build strategy fields
COMMENT ON COLUMN apps.services IS 'JSONB array of ServiceConfig objects. Each service has source_type (git/flake/image), build_strategy (flake/auto-go/auto-rust/auto-node/auto-python/dockerfile/nixpacks/auto), and optional build_config for strategy-specific options.';
