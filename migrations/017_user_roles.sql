-- Migration: 017_user_roles.sql
-- Add role and invited_by columns to users table for RBAC

-- Add role column with default 'member'
ALTER TABLE users ADD COLUMN IF NOT EXISTS role VARCHAR(20) DEFAULT 'member' CHECK (role IN ('owner', 'member'));

-- Add invited_by column to track who invited the user
ALTER TABLE users ADD COLUMN IF NOT EXISTS invited_by TEXT;

-- Update existing admin users to have 'owner' role
UPDATE users SET role = 'owner' WHERE is_admin = TRUE AND role IS NULL;

-- Update existing non-admin users to have 'member' role
UPDATE users SET role = 'member' WHERE is_admin = FALSE AND role IS NULL;

-- Create index for role lookups
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
