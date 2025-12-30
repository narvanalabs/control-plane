-- Migration: 005_service_source_config.sql
-- Add source configuration fields to services in the apps.services JSONB column
-- This migration updates existing services to use the new source type model

-- Update existing services in the apps table to have proper source_type values
-- Services are stored as JSONB array in the apps.services column

-- For each app, update the services array to:
-- 1. Set source_type based on which source field is populated
-- 2. Apply defaults for git_ref ("main") and flake_output ("packages.x86_64-linux.default")

-- Update services that have git_repo set to use source_type = 'git'
UPDATE apps
SET services = (
    SELECT COALESCE(
        jsonb_agg(
            CASE
                -- Service has git_repo: set source_type to 'git' and apply defaults
                WHEN service->>'git_repo' IS NOT NULL AND service->>'git_repo' != '' THEN
                    service || jsonb_build_object(
                        'source_type', 'git',
                        'git_ref', COALESCE(NULLIF(service->>'git_ref', ''), 'main'),
                        'flake_output', COALESCE(NULLIF(service->>'flake_output', ''), 'packages.x86_64-linux.default')
                    )
                -- Service has flake_uri: set source_type to 'flake'
                WHEN service->>'flake_uri' IS NOT NULL AND service->>'flake_uri' != '' THEN
                    service || jsonb_build_object('source_type', 'flake')
                -- Service has image: set source_type to 'image'
                WHEN service->>'image' IS NOT NULL AND service->>'image' != '' THEN
                    service || jsonb_build_object('source_type', 'image')
                -- Service has no source configured yet: leave as-is (will need source on next update)
                ELSE
                    service
            END
        ),
        '[]'::jsonb
    )
    FROM jsonb_array_elements(apps.services) AS service
)
WHERE jsonb_typeof(services) = 'array';

-- Add comment documenting the service source fields
COMMENT ON COLUMN apps.services IS 'JSONB array of ServiceConfig objects. Each service has source_type (git/flake/image) with corresponding fields: git_repo+git_ref+flake_output for git, flake_uri for flake, or image for OCI images.';

