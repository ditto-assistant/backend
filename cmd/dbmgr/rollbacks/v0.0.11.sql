DROP TABLE IF EXISTS encryption_keys;

-- Drop WebAuthn challenges table
DROP TABLE IF EXISTS webauthn_challenges;

-- Drop added columns from encryption_keys table
-- Note: SQLite doesn't support DROP COLUMN directly in older versions, 
-- so we need to recreate the table without those columns

-- First, create a temporary table with the original schema
CREATE TABLE encryption_keys_temp (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  key_id TEXT NOT NULL,
  encrypted_key TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  last_used_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  is_active BOOLEAN DEFAULT TRUE,
  key_version INTEGER DEFAULT 1,
  FOREIGN KEY (user_id) REFERENCES users(id),
  UNIQUE (user_id, key_id)
);

-- Copy data to the temporary table (only columns that exist in both tables)
INSERT INTO encryption_keys_temp 
SELECT id, user_id, key_id, encrypted_key, created_at, last_used_at, is_active, key_version
FROM encryption_keys;

-- Drop the original table
DROP TABLE encryption_keys;

-- Rename the temporary table to the original name
ALTER TABLE encryption_keys_temp RENAME TO encryption_keys;

-- Recreate the original indexes
CREATE INDEX IF NOT EXISTS idx_encryption_keys_user_id ON encryption_keys(user_id);

CREATE INDEX IF NOT EXISTS idx_encryption_keys_key_id ON encryption_keys(key_id);