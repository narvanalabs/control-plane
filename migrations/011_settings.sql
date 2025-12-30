-- Create settings table for global configuration
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Seed initial settings if they don't exist
INSERT INTO settings (key, value) VALUES ('server_domain', 'localhost') ON CONFLICT DO NOTHING;
INSERT INTO settings (key, value) VALUES ('public_ip', '') ON CONFLICT DO NOTHING;
