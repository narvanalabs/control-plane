-- Migration: 021_buildjob_source_fields.sql
-- Add explicit source type and git ref fields to builds table for clarity

-- Add source_type column to builds table
-- This explicitly indicates whether the source is git, flake, or database
ALTER TABLE builds ADD COLUMN IF NOT EXISTS source_type VARCHAR(20);

-- Add service_name column to builds table if not exists
-- This links the build to a specific service within the app
ALTER TABLE builds ADD COLUMN IF NOT EXISTS service_name VARCHAR(255);

-- Add comments documenting the new columns
COMMENT ON COLUMN builds.source_type IS 'Explicit source type: git (git repo), flake (direct flake URI), database (internal database)';
COMMENT ON COLUMN builds.service_name IS 'Name of the service within the app that this build is for';

-- Create index for filtering builds by source type
CREATE INDEX IF NOT EXISTS idx_builds_source_type ON builds(source_type);
