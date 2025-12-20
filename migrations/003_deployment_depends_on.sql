-- Migration: 003_deployment_depends_on.sql
-- Add depends_on column to deployments table for multi-service dependency tracking

-- Add depends_on column to store service dependencies as JSONB array
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS depends_on JSONB DEFAULT '[]';

-- Create index for querying deployments by their dependencies
CREATE INDEX IF NOT EXISTS idx_deployments_depends_on ON deployments USING GIN (depends_on);
