-- Migration: 023_replace_resource_tier_with_resources.sql
-- Replace resource_tier VARCHAR with resources JSONB for flexible resource specification

-- Add new resources column
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS resources JSONB;

-- Migrate existing resource_tier values to resources JSONB
UPDATE deployments SET resources = 
    CASE resource_tier
        WHEN 'nano' THEN '{"cpu": "0.25", "memory": "256Mi"}'::jsonb
        WHEN 'small' THEN '{"cpu": "0.5", "memory": "512Mi"}'::jsonb
        WHEN 'medium' THEN '{"cpu": "1", "memory": "1Gi"}'::jsonb
        WHEN 'large' THEN '{"cpu": "2", "memory": "2Gi"}'::jsonb
        WHEN 'xlarge' THEN '{"cpu": "4", "memory": "4Gi"}'::jsonb
        ELSE '{"cpu": "0.5", "memory": "512Mi"}'::jsonb
    END
WHERE resources IS NULL;

-- Drop the old resource_tier column and its constraint
ALTER TABLE deployments DROP COLUMN IF EXISTS resource_tier;
