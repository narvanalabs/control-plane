-- Migration: github_accounts
-- Created at: 2025-12-30

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
