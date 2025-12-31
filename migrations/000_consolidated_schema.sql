-- Narvana Control Plane Consolidated Schema
-- This is a single idempotent schema file combining all migrations
-- Safe to run multiple times - uses IF NOT EXISTS / IF EXISTS throughout

-- ============================================================================
-- Extensions
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================================
-- Functions
-- ============================================================================

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- ============================================================================
-- Tables
-- ============================================================================

-- Users table for authentication
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    is_admin BOOLEAN DEFAULT FALSE,
    created_at BIGINT NOT NULL,
    name TEXT,
    avatar_url TEXT
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- Apps table: stores application definitions
CREATE TABLE IF NOT EXISTS apps (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id VARCHAR(255) NOT NULL,
    name VARCHAR(63) NOT NULL,
    description TEXT,
    services JSONB NOT NULL DEFAULT '[]',
    icon_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Unique index on owner_id + name for non-deleted apps
CREATE UNIQUE INDEX IF NOT EXISTS apps_owner_name_unique ON apps(owner_id, name) WHERE deleted_at IS NULL;

-- Indexes for apps
CREATE INDEX IF NOT EXISTS idx_apps_owner_id ON apps(owner_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_apps_name ON apps(name) WHERE deleted_at IS NULL;

-- Nodes table: stores compute node information
CREATE TABLE IF NOT EXISTS nodes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    hostname VARCHAR(255) NOT NULL,
    address VARCHAR(255) NOT NULL,
    grpc_port INTEGER NOT NULL DEFAULT 9090,
    healthy BOOLEAN NOT NULL DEFAULT true,
    cpu_total DOUBLE PRECISION NOT NULL DEFAULT 0,
    cpu_available DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_total BIGINT NOT NULL DEFAULT 0,
    memory_available BIGINT NOT NULL DEFAULT 0,
    disk_total BIGINT NOT NULL DEFAULT 0,
    disk_available BIGINT NOT NULL DEFAULT 0,
    cached_paths TEXT[] DEFAULT '{}',
    last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for nodes
CREATE INDEX IF NOT EXISTS idx_nodes_healthy ON nodes(healthy);
CREATE INDEX IF NOT EXISTS idx_nodes_last_heartbeat ON nodes(last_heartbeat);

-- Deployments table: stores deployment instances
CREATE TABLE IF NOT EXISTS deployments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    app_id UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    service_name VARCHAR(255) NOT NULL,
    version INTEGER NOT NULL,
    git_ref VARCHAR(255) NOT NULL,
    git_commit VARCHAR(40),
    build_type VARCHAR(20) NOT NULL CHECK (build_type IN ('oci', 'pure-nix')),
    artifact TEXT,
    status VARCHAR(20) NOT NULL CHECK (status IN (
        'pending', 'building', 'built', 'scheduled', 
        'starting', 'running', 'stopping', 'stopped', 'failed'
    )),
    node_id UUID,
    resource_tier VARCHAR(20) NOT NULL CHECK (resource_tier IN (
        'nano', 'small', 'medium', 'large', 'xlarge'
    )),
    config JSONB,
    depends_on JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

-- Indexes for deployments
CREATE INDEX IF NOT EXISTS idx_deployments_app_id ON deployments(app_id);
CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
CREATE INDEX IF NOT EXISTS idx_deployments_node_id ON deployments(node_id) WHERE node_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_deployments_created_at ON deployments(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_deployments_depends_on ON deployments USING GIN (depends_on);

-- Add foreign key constraint for deployments.node_id (only if it doesn't exist)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_deployments_node_id'
    ) THEN
        ALTER TABLE deployments 
            ADD CONSTRAINT fk_deployments_node_id 
            FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE SET NULL;
    END IF;
END $$;

-- Builds table: stores build job information
CREATE TABLE IF NOT EXISTS builds (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    app_id UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    git_url TEXT NOT NULL,
    git_ref VARCHAR(255) NOT NULL,
    flake_output VARCHAR(255) NOT NULL,
    build_type VARCHAR(20) NOT NULL CHECK (build_type IN ('oci', 'pure-nix')),
    status VARCHAR(20) NOT NULL CHECK (status IN (
        'queued', 'running', 'succeeded', 'failed'
    )),
    build_strategy VARCHAR(20) DEFAULT 'flake',
    build_config JSONB,
    generated_flake TEXT,
    flake_lock TEXT,
    vendor_hash VARCHAR(255),
    retry_count INTEGER NOT NULL DEFAULT 0,
    retry_as_oci BOOLEAN NOT NULL DEFAULT false,
    timeout_seconds INTEGER NOT NULL DEFAULT 1800,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

-- Add build_strategy check constraint (drop and recreate to ensure it includes auto-database)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'builds_build_strategy_check'
    ) THEN
        ALTER TABLE builds DROP CONSTRAINT builds_build_strategy_check;
    END IF;
    ALTER TABLE builds
        ADD CONSTRAINT builds_build_strategy_check
        CHECK (build_strategy IN ('flake', 'auto-go', 'auto-rust', 'auto-node', 'auto-python', 'auto-database', 'dockerfile', 'nixpacks', 'auto'));
END $$;

-- Indexes for builds
CREATE INDEX IF NOT EXISTS idx_builds_deployment_id ON builds(deployment_id);
CREATE INDEX IF NOT EXISTS idx_builds_app_id ON builds(app_id);
CREATE INDEX IF NOT EXISTS idx_builds_status ON builds(status);
CREATE INDEX IF NOT EXISTS idx_builds_build_strategy ON builds(build_strategy);
CREATE INDEX IF NOT EXISTS idx_builds_retry_as_oci ON builds(retry_as_oci) WHERE retry_as_oci = true;

-- Build queue table: stores pending build jobs for worker processing
CREATE TABLE IF NOT EXISTS build_queue (
    id UUID PRIMARY KEY,
    job_data JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing')),
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ
);

-- Indexes for build_queue
CREATE INDEX IF NOT EXISTS idx_build_queue_status ON build_queue(status);
CREATE INDEX IF NOT EXISTS idx_build_queue_created_at ON build_queue(created_at ASC);
CREATE INDEX IF NOT EXISTS idx_build_queue_pending ON build_queue(created_at ASC) WHERE status = 'pending';

-- Secrets table: stores encrypted application secrets
CREATE TABLE IF NOT EXISTS secrets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    app_id UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    key VARCHAR(255) NOT NULL,
    encrypted_value BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Unique constraint on app_id + key
    CONSTRAINT secrets_app_key_unique UNIQUE (app_id, key)
);

-- Indexes for secrets
CREATE INDEX IF NOT EXISTS idx_secrets_app_id ON secrets(app_id);

-- Logs table: stores build and runtime logs
CREATE TABLE IF NOT EXISTS logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    source VARCHAR(20) NOT NULL CHECK (source IN ('build', 'runtime')),
    level VARCHAR(20) NOT NULL,
    message TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for logs
CREATE INDEX IF NOT EXISTS idx_logs_deployment_id ON logs(deployment_id);
CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON logs(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_logs_source ON logs(source);

-- GitHub App Settings table
CREATE TABLE IF NOT EXISTS github_app_settings (
    id TEXT PRIMARY KEY, -- "default" or app ID
    app_id BIGINT,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    webhook_secret TEXT,
    private_key TEXT,
    slug TEXT,
    config_type TEXT NOT NULL DEFAULT 'app',
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

-- GitHub Installations table
CREATE TABLE IF NOT EXISTS github_installations (
    id BIGINT PRIMARY KEY, -- GitHub Installation ID
    account_id BIGINT NOT NULL,
    account_login TEXT NOT NULL,
    account_type TEXT NOT NULL, -- "User" or "Organization"
    access_tokens_url TEXT NOT NULL,
    repositories_url TEXT NOT NULL,
    html_url TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_github_installations_user_id ON github_installations(user_id);

-- GitHub Accounts table
CREATE TABLE IF NOT EXISTS github_accounts (
    id BIGINT PRIMARY KEY, -- GitHub User ID
    login TEXT NOT NULL,
    name TEXT,
    email TEXT,
    avatar_url TEXT,
    access_token TEXT NOT NULL,
    refresh_token TEXT,
    expiry BIGINT,
    token_type TEXT,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_github_accounts_user_id ON github_accounts(user_id);

-- Settings table for global configuration
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- ============================================================================
-- Triggers
-- ============================================================================

-- Triggers for updated_at
DROP TRIGGER IF EXISTS update_apps_updated_at ON apps;
CREATE TRIGGER update_apps_updated_at
    BEFORE UPDATE ON apps
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_deployments_updated_at ON deployments;
CREATE TRIGGER update_deployments_updated_at
    BEFORE UPDATE ON deployments
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_secrets_updated_at ON secrets;
CREATE TRIGGER update_secrets_updated_at
    BEFORE UPDATE ON secrets
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- Data Migrations (idempotent updates to apps.services JSONB)
-- ============================================================================

-- Update services to have source_type based on which source field is populated
-- This is idempotent - services that already have source_type will be left unchanged
UPDATE apps
SET services = (
    SELECT COALESCE(
        jsonb_agg(
            CASE
                -- Service already has source_type: leave as-is
                WHEN service->>'source_type' IS NOT NULL AND service->>'source_type' != '' THEN
                    service
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
                -- Service has database: set source_type to 'database'
                WHEN service->>'database' IS NOT NULL THEN
                    service || jsonb_build_object('source_type', 'database')
                -- Service has no source configured yet: leave as-is
                ELSE
                    service
            END
        ),
        '[]'::jsonb
    )
    FROM jsonb_array_elements(apps.services) AS service
)
WHERE jsonb_typeof(services) = 'array'
  AND EXISTS (
      SELECT 1 FROM jsonb_array_elements(apps.services) AS s
      WHERE s->>'source_type' IS NULL OR s->>'source_type' = ''
  );

-- Update services to have build_strategy = 'flake' as default (if not already set)
-- This is idempotent - services that already have build_strategy will be left unchanged
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
                    service || jsonb_build_object('build_strategy', 'flake')
                -- Service has flake_uri (flake source): default to 'flake' strategy
                WHEN service->>'flake_uri' IS NOT NULL AND service->>'flake_uri' != '' THEN
                    service || jsonb_build_object('build_strategy', 'flake')
                -- Service has database (database source): default to 'auto-database' strategy
                WHEN service->>'database' IS NOT NULL THEN
                    service || jsonb_build_object('build_strategy', 'auto-database')
                -- Service has image (image source): no build strategy needed
                ELSE
                    service
            END
        ),
        '[]'::jsonb
    )
    FROM jsonb_array_elements(apps.services) AS service
)
WHERE jsonb_typeof(services) = 'array'
  AND EXISTS (
      SELECT 1 FROM jsonb_array_elements(apps.services) AS s
      WHERE (s->>'build_strategy' IS NULL OR s->>'build_strategy' = '')
        AND (s->>'git_repo' IS NOT NULL OR s->>'flake_uri' IS NOT NULL OR s->>'database' IS NOT NULL)
  );

-- Seed initial settings if they don't exist
INSERT INTO settings (key, value) VALUES ('server_domain', 'localhost') ON CONFLICT DO NOTHING;
INSERT INTO settings (key, value) VALUES ('public_ip', '') ON CONFLICT DO NOTHING;

-- ============================================================================
-- Comments
-- ============================================================================

COMMENT ON COLUMN apps.services IS 'JSONB array of ServiceConfig objects. Each service has source_type (git/flake/image/database), build_strategy (flake/auto-go/auto-rust/auto-node/auto-python/auto-database/dockerfile/nixpacks/auto), and optional build_config for strategy-specific options.';
COMMENT ON COLUMN builds.build_strategy IS 'Build strategy: flake (existing flake.nix), auto-go/rust/node/python/database (generate flake), dockerfile (use Dockerfile), nixpacks (use Nixpacks), auto (auto-detect)';
COMMENT ON COLUMN builds.build_config IS 'JSONB containing strategy-specific configuration options (go_version, node_version, build_command, etc.)';
COMMENT ON COLUMN builds.generated_flake IS 'Auto-generated flake.nix content for auto-* strategies';
COMMENT ON COLUMN builds.flake_lock IS 'Flake lock file content for reproducible builds';
COMMENT ON COLUMN builds.vendor_hash IS 'Calculated vendor hash for dependency reproducibility (Go vendor hash, npm hash, Cargo hash)';
COMMENT ON COLUMN builds.retry_count IS 'Number of retry attempts for this build';
COMMENT ON COLUMN builds.retry_as_oci IS 'Whether this build was retried as OCI after pure-nix failure';
COMMENT ON COLUMN builds.timeout_seconds IS 'Build timeout in seconds (default: 1800 = 30 minutes)';




