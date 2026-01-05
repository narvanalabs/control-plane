-- Add disk metrics columns to nodes table for detailed disk monitoring
-- **Validates: Requirements 20.1**

-- Add columns for nix store disk metrics
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS nix_store_total BIGINT DEFAULT 0;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS nix_store_used BIGINT DEFAULT 0;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS nix_store_available BIGINT DEFAULT 0;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS nix_store_usage_percent DOUBLE PRECISION DEFAULT 0;

-- Add columns for container storage disk metrics
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS container_storage_total BIGINT DEFAULT 0;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS container_storage_used BIGINT DEFAULT 0;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS container_storage_available BIGINT DEFAULT 0;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS container_storage_usage_percent DOUBLE PRECISION DEFAULT 0;
