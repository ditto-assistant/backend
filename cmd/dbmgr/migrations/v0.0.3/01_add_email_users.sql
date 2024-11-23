ALTER TABLE users ADD COLUMN email TEXT UNIQUE;

CREATE INDEX idx_users_email ON users (email);
