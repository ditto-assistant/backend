CREATE TABLE IF NOT EXISTS encryption_keys (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  key_id TEXT NOT NULL,
  encrypted_key TEXT NOT NULL,
  credential_id TEXT NULL,
  credential_rp_id TEXT NULL,
  credential_created_at DATETIME NULL,
  key_derivation_method TEXT NOT NULL DEFAULT 'direct',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  last_used_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  is_active BOOLEAN DEFAULT TRUE,
  key_version INTEGER DEFAULT 1,
  FOREIGN KEY (user_id) REFERENCES users(id),
  UNIQUE (user_id, key_id)
);

CREATE INDEX IF NOT EXISTS idx_encryption_keys_user_id ON encryption_keys(user_id);

CREATE INDEX IF NOT EXISTS idx_encryption_keys_key_id ON encryption_keys(key_id);

CREATE INDEX IF NOT EXISTS idx_encryption_keys_credential_id ON encryption_keys(credential_id);

-- Create a table for WebAuthn challenges
CREATE TABLE IF NOT EXISTS webauthn_challenges (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  challenge TEXT NOT NULL,
  rp_id TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  expires_at DATETIME NOT NULL,
  type TEXT NOT NULL, -- 'registration' or 'authentication'
  FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Add index for challenge expiration for cleanup
CREATE INDEX IF NOT EXISTS idx_webauthn_challenges_expires_at ON webauthn_challenges(expires_at);