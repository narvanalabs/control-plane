-- Migration: 016_organizations.sql
-- Add organizations and org_memberships tables

-- Organizations table: stores organization definitions
CREATE TABLE IF NOT EXISTS organizations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(63) NOT NULL,
    slug VARCHAR(63) NOT NULL UNIQUE,
    description TEXT,
    icon_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for organizations
CREATE INDEX IF NOT EXISTS idx_organizations_slug ON organizations(slug);
CREATE INDEX IF NOT EXISTS idx_organizations_created_at ON organizations(created_at);

-- Organization memberships table: links users to organizations
CREATE TABLE IF NOT EXISTS org_memberships (
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL CHECK (role IN ('owner', 'member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (org_id, user_id)
);

-- Indexes for org_memberships
CREATE INDEX IF NOT EXISTS idx_org_memberships_user_id ON org_memberships(user_id);
CREATE INDEX IF NOT EXISTS idx_org_memberships_org_id ON org_memberships(org_id);

-- Add org_id column to apps table (nullable initially for migration)
ALTER TABLE apps ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

-- Create index for org_id on apps
CREATE INDEX IF NOT EXISTS idx_apps_org_id ON apps(org_id) WHERE deleted_at IS NULL;

-- Trigger for updated_at on organizations
DROP TRIGGER IF EXISTS update_organizations_updated_at ON organizations;
CREATE TRIGGER update_organizations_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Create default organization (only if none exists)
INSERT INTO organizations (id, name, slug, description, created_at, updated_at)
SELECT uuid_generate_v4(), 'Default Organization', 'default', 'Default organization created during platform setup', NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM organizations WHERE slug = 'default');

-- Associate existing apps with the default organization
UPDATE apps 
SET org_id = (SELECT id FROM organizations WHERE slug = 'default' LIMIT 1)
WHERE org_id IS NULL;

-- Make org_id NOT NULL after migration (all apps now have an org)
-- Note: This is commented out to allow for gradual migration
-- ALTER TABLE apps ALTER COLUMN org_id SET NOT NULL;
