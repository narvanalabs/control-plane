-- Narvana Control Plane Initial Schema
-- Migration: 001_initial_schema.sql

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Apps table: stores application definitions
CREATE TABLE apps (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id VARCHAR(255) NOT NULL,
    name VARCHAR(63) NOT NULL,
    description TEXT,
    build_type VARCHAR(20) NOT NULL CHECK (build_type IN ('oci', 'pure-nix')),
    services JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Unique index on owner_id + name for non-deleted apps (replacement for constraint)
CREATE UNIQUE INDEX apps_owner_name_unique ON apps(owner_id, name) WHERE deleted_at IS NULL;

-- Indexes for apps
CREATE INDEX idx_apps_owner_id ON apps(owner_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_apps_name ON apps(name) WHERE deleted_at IS NULL;

-- Deployments table: stores deployment instances
CREATE TABLE deployments (
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

-- Indexes for deployments
CREATE INDEX idx_deployments_app_id ON deployments(app_id);
CREATE INDEX idx_deployments_status ON deployments(status);
CREATE INDEX idx_deployments_node_id ON deployments(node_id) WHERE node_id IS NOT NULL;
CREATE INDEX idx_deployments_created_at ON deployments(created_at DESC);


-- Nodes table: stores compute node information
CREATE TABLE nodes (
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
CREATE INDEX idx_nodes_healthy ON nodes(healthy);
CREATE INDEX idx_nodes_last_heartbeat ON nodes(last_heartbeat);

-- Add foreign key constraint for deployments.node_id after nodes table exists
ALTER TABLE deployments 
    ADD CONSTRAINT fk_deployments_node_id 
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE SET NULL;

-- Builds table: stores build job information
CREATE TABLE builds (
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

-- Indexes for builds
CREATE INDEX idx_builds_deployment_id ON builds(deployment_id);
CREATE INDEX idx_builds_app_id ON builds(app_id);
CREATE INDEX idx_builds_status ON builds(status);

-- Secrets table: stores encrypted application secrets
CREATE TABLE secrets (
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
CREATE INDEX idx_secrets_app_id ON secrets(app_id);

-- Logs table: stores build and runtime logs
CREATE TABLE logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    source VARCHAR(20) NOT NULL CHECK (source IN ('build', 'runtime')),
    level VARCHAR(20) NOT NULL,
    message TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for logs
CREATE INDEX idx_logs_deployment_id ON logs(deployment_id);
CREATE INDEX idx_logs_timestamp ON logs(timestamp DESC);
CREATE INDEX idx_logs_source ON logs(source);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers for updated_at
CREATE TRIGGER update_apps_updated_at
    BEFORE UPDATE ON apps
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_deployments_updated_at
    BEFORE UPDATE ON deployments
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_secrets_updated_at
    BEFORE UPDATE ON secrets
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
