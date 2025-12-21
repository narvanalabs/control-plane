-- Migration: 006_remove_app_build_type.sql
-- Remove the build_type column from apps table
-- Build type is now determined per-service via source_type in the services JSONB

ALTER TABLE apps DROP COLUMN IF EXISTS build_type;
