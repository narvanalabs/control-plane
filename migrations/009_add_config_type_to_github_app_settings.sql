-- Migration: add_config_type_to_github_app_settings
-- Created at: 2025-12-30

ALTER TABLE github_app_settings ADD COLUMN IF NOT EXISTS config_type TEXT NOT NULL DEFAULT 'app';
ALTER TABLE github_app_settings ALTER COLUMN app_id DROP NOT NULL;
ALTER TABLE github_app_settings ALTER COLUMN private_key DROP NOT NULL;
ALTER TABLE github_app_settings ALTER COLUMN slug DROP NOT NULL;
