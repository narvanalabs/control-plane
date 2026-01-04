-- Migration: 020_app_version_optimistic_locking.sql
-- Add version field for optimistic locking and unique constraint on (org_id, name)

-- Add version column to apps table for optimistic locking
ALTER TABLE apps ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1;

-- Create unique index on (org_id, name) for non-deleted apps
-- This ensures app names are unique within an organization
-- Note: We drop the old owner-based unique index if it exists
DROP INDEX IF EXISTS apps_owner_name_unique;

-- Create the new org-based unique index
CREATE UNIQUE INDEX IF NOT EXISTS apps_org_name_unique ON apps(org_id, name) WHERE deleted_at IS NULL;

-- Add comment documenting the version field
COMMENT ON COLUMN apps.version IS 'Version number for optimistic locking. Incremented on each update to detect concurrent modifications.';
