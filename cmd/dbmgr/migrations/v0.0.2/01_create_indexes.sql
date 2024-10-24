CREATE INDEX IF NOT EXISTS idx_services_name ON services(name);

CREATE INDEX IF NOT EXISTS idx_users_uid ON users(uid);

CREATE INDEX IF NOT EXISTS idx_tokens_per_dollar_name ON tokens_per_dollar(name);