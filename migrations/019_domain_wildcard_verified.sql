-- Add wildcard and verified columns to domains table
ALTER TABLE domains ADD COLUMN IF NOT EXISTS is_wildcard BOOLEAN DEFAULT FALSE;
ALTER TABLE domains ADD COLUMN IF NOT EXISTS verified BOOLEAN DEFAULT FALSE;
