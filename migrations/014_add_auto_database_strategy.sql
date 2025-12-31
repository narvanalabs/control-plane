-- Migration: 014_add_auto_database_strategy.sql
-- Add 'auto-database' to the allowed build strategies

-- Drop the existing check constraint
ALTER TABLE builds
DROP CONSTRAINT IF EXISTS builds_build_strategy_check;

-- Recreate the check constraint with 'auto-database' included
ALTER TABLE builds
ADD CONSTRAINT builds_build_strategy_check
CHECK (build_strategy IN ('flake', 'auto-go', 'auto-rust', 'auto-node', 'auto-python', 'auto-database', 'dockerfile', 'nixpacks', 'auto'));

-- Update the comment to include auto-database
COMMENT ON COLUMN builds.build_strategy IS 'Build strategy: flake (existing flake.nix), auto-go/rust/node/python/database (generate flake), dockerfile (use Dockerfile), nixpacks (use Nixpacks), auto (auto-detect)';




