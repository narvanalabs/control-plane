-- Migration: github_app_settings
-- Created at: 2025-12-30

CREATE TABLE IF NOT EXISTS github_app_settings (
    id TEXT PRIMARY KEY, -- "default" or app ID
    app_id BIGINT NOT NULL,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    webhook_secret TEXT,
    private_key TEXT NOT NULL,
    slug TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

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
