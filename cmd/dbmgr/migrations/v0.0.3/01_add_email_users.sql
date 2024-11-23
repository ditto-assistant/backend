ALTER TABLE users ADD COLUMN email TEXT;

CREATE INDEX idx_users_email ON users (email);
