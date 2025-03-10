CREATE TABLE migrations (
			name TEXT PRIMARY KEY,
			date TEXT DEFAULT (datetime('now')),
			version TEXT NOT NULL
		);
CREATE TABLE services (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  version TEXT NOT NULL,
  service_type TEXT NOT NULL, -- e.g., 'prompt', 'embedding', 'search', 'image_generation'
  provider TEXT NOT NULL, -- e.g., 'openai', 'google', 'anthropic', 'brave'
  
  -- Base costs (updated to per million tokens)
  base_cost_per_call REAL,
  base_cost_per_million_tokens REAL,
  base_cost_per_million_input_tokens REAL,
  base_cost_per_million_output_tokens REAL,
  base_cost_per_image REAL,
  base_cost_per_search REAL,

  -- Token limits
  max_input_tokens INTEGER,
  max_output_tokens INTEGER,
  max_total_tokens INTEGER,

  -- Time-based costs
  base_cost_per_second REAL,
  
  -- Data volume costs
  base_cost_per_gb_processed REAL,
  base_cost_per_gb_stored REAL,

  -- Batch processing
  supports_batching BOOLEAN,
  batch_size_limit INTEGER,
  base_cost_per_batch REAL,

  -- API rate limiting
  rate_limit_per_minute INTEGER,
  rate_limit_per_day INTEGER,

  -- Ditto's profit margin factors
  profit_margin_percentage REAL, -- e.g., 20.0 for 20%
  minimum_profit_amount REAL, -- Minimum profit in currency units
  
  -- Dynamic pricing factors
  peak_hours_multiplier REAL, -- e.g., 1.5 for 50% increase during peak hours
  volume_discount_threshold INTEGER, -- Number of calls/tokens for discount
  volume_discount_percentage REAL, -- e.g., 10.0 for 10% discount

  -- Miscellaneous
  currency TEXT DEFAULT 'USD',
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER update_services_timestamp
AFTER UPDATE ON services
FOR EACH ROW
BEGIN
  UPDATE services SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;
CREATE TABLE tools (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  version TEXT NOT NULL,
  service_id INTEGER NOT NULL,
  FOREIGN KEY (service_id) REFERENCES services(id)
);
CREATE TABLE examples (
  id INTEGER PRIMARY KEY,
  tool_id INTEGER,
  prompt TEXT,
  response TEXT,
  em_prompt F32_BLOB(768), 
  em_prompt_response F32_BLOB(768),
  FOREIGN KEY (tool_id) REFERENCES tools(id)
);
CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  uid TEXT UNIQUE NOT NULL,
  balance INTEGER DEFAULT 0
, total_tokens_airdropped INTEGER DEFAULT 0, last_airdrop_at TIMESTAMP, email TEXT);
CREATE TABLE receipts (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  service_id INTEGER NOT NULL,
  timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
  
  -- Usage metrics
  input_tokens INTEGER,
  output_tokens INTEGER,
  total_tokens INTEGER,
  call_duration_seconds REAL,
  data_processed_bytes INTEGER,
  data_stored_bytes INTEGER,
  num_images INTEGER,
  num_searches INTEGER,
  num_api_calls INTEGER,

  -- Ditto Token cost: calculated by trigger
  ditto_token_cost INTEGER,

  metadata JSON,

  FOREIGN KEY (user_id) REFERENCES users(id),
  FOREIGN KEY (service_id) REFERENCES services(id)
);
CREATE INDEX idx_receipts_user_timestamp ON receipts(user_id, timestamp);
CREATE TABLE purchases (
  id INTEGER PRIMARY KEY,
  payment_id TEXT NOT NULL UNIQUE,
  user_id INTEGER NOT NULL,
  cents INTEGER NOT NULL,
  tokens INTEGER NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER after_insert_purchases
AFTER INSERT ON purchases
FOR EACH ROW
BEGIN
    UPDATE users
    SET balance = balance + NEW.tokens
    WHERE id = NEW.user_id;
END;
CREATE INDEX idx_services_name ON services(name);
CREATE INDEX idx_users_uid ON users(uid);
CREATE TABLE IF NOT EXISTS "tokens_per_unit" (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    count INTEGER NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER after_insert_receipts
AFTER INSERT ON receipts
FOR EACH ROW
BEGIN
    -- Calculate the ditto_token_cost and update the newly inserted row
    UPDATE receipts
    SET ditto_token_cost = (
        SELECT MAX(1, ROUND(
            (COALESCE(base_cost_per_call, 0) * tpu.count +
             COALESCE(base_cost_per_million_tokens * (NEW.total_tokens / 1000000.0), 0) * tpu.count +
             COALESCE(base_cost_per_million_input_tokens * (NEW.input_tokens / 1000000.0), 0) * tpu.count +
             COALESCE(base_cost_per_million_output_tokens * (NEW.output_tokens / 1000000.0), 0) * tpu.count +
             COALESCE(base_cost_per_image * NEW.num_images, 0) * tpu.count +
             COALESCE(base_cost_per_search * NEW.num_searches, 0) * tpu.count +
             COALESCE(base_cost_per_second * NEW.call_duration_seconds, 0) * tpu.count +
             COALESCE(base_cost_per_gb_processed * (NEW.data_processed_bytes / 1073741824.0), 0) * tpu.count +
             COALESCE(base_cost_per_gb_stored * (NEW.data_stored_bytes / 1073741824.0), 0) * tpu.count
            ) * (1 + profit_margin_percentage / 100.0)
        ))
        FROM services, tokens_per_unit AS tpu
        WHERE services.id = NEW.service_id AND tpu.name = 'dollar'
    )
    WHERE id = NEW.id;
    -- Update the user's balance
    UPDATE users
    SET balance = balance - (
        SELECT ditto_token_cost
        FROM receipts
        WHERE id = NEW.id
    )
    WHERE id = NEW.user_id;
END;
CREATE TRIGGER update_tokens_per_unit_timestamp
AFTER UPDATE ON tokens_per_unit
FOR EACH ROW
BEGIN
  UPDATE tokens_per_unit SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;
CREATE TRIGGER after_services_update_tokens_per_unit
AFTER UPDATE ON services
FOR EACH ROW
WHEN NEW.name IN ('dall-e-3', 'brave-search')
BEGIN
    -- Update image tokens only for DALL-E 3
    UPDATE tokens_per_unit 
    SET count = CASE
        WHEN NEW.name = 'dall-e-3' AND name = 'image' THEN
            ROUND((
                SELECT tpu.count * (NEW.base_cost_per_image + COALESCE(NEW.base_cost_per_call, 0)) * (1 + NEW.profit_margin_percentage / 100.0)
                FROM tokens_per_unit tpu
                WHERE tpu.name = 'dollar'
            ))
        ELSE count
    END,
    updated_at = CASE
        WHEN NEW.name = 'dall-e-3' AND name = 'image' THEN CURRENT_TIMESTAMP
        ELSE updated_at
    END
    WHERE name = 'image' AND NEW.name = 'dall-e-3';
    -- Update search tokens only for Brave Search
    UPDATE tokens_per_unit 
    SET count = CASE
        WHEN NEW.name = 'brave-search' AND name = 'search' THEN
            ROUND((
                SELECT tpu.count * (NEW.base_cost_per_search + COALESCE(NEW.base_cost_per_call, 0)) * (1 + NEW.profit_margin_percentage / 100.0)
                FROM tokens_per_unit tpu
                WHERE tpu.name = 'dollar'
            ))
        ELSE count
    END,
    updated_at = CASE
        WHEN NEW.name = 'brave-search' AND name = 'search' THEN CURRENT_TIMESTAMP
        ELSE updated_at
    END
    WHERE name = 'search' AND NEW.name = 'brave-search';
END;
CREATE INDEX idx_users_email ON users (email);
CREATE TABLE user_devices (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  device_uid TEXT NOT NULL,
  last_sign_in DATETIME DEFAULT CURRENT_TIMESTAMP,
  user_agent TEXT,
  version TEXT NOT NULL,
  -- 0: web, 1: android, 2: ios, 3: linux, 4: macos, 5: unknown, 6: windows
  platform INTEGER NOT NULL,
  accept_language TEXT,
  -- for Ditto team
  comment TEXT,
  FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE INDEX idx_user_devices_user_id ON user_devices(user_id);
CREATE TABLE user_feedback (
  id INTEGER PRIMARY KEY,
  device_id INTEGER NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  -- feedback-type: bug, feature-request, other
  type NOT NULL,
  feedback TEXT NOT NULL,
  -- for Ditto team
  comment TEXT,
  FOREIGN KEY (device_id) REFERENCES user_devices(id)
);
CREATE UNIQUE INDEX idx_user_devices_unique 
ON user_devices(user_id, device_uid, version);
CREATE TABLE encryption_keys (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  key_id TEXT NOT NULL,
  encrypted_key TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  last_used_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  is_active BOOLEAN DEFAULT TRUE,
  key_version INTEGER DEFAULT 1, credential_id TEXT NULL, credential_public_key TEXT NULL, credential_rp_id TEXT NULL, credential_created_at DATETIME NULL, key_derivation_method TEXT NOT NULL DEFAULT 'direct',
  FOREIGN KEY (user_id) REFERENCES users(id),
  UNIQUE (user_id, key_id)
);
CREATE INDEX idx_encryption_keys_user_id ON encryption_keys(user_id);
CREATE INDEX idx_encryption_keys_credential_id ON encryption_keys(credential_id);
CREATE TABLE webauthn_challenges (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  challenge TEXT NOT NULL,
  rp_id TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  expires_at DATETIME NOT NULL,
  type TEXT NOT NULL, -- 'registration' or 'authentication'
  FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE INDEX idx_webauthn_challenges_expires_at ON webauthn_challenges(expires_at);
