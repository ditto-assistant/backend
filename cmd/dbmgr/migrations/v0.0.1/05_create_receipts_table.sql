CREATE TABLE IF NOT EXISTS receipts (
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

-- Index for faster queries on user_id and timestamp
CREATE INDEX idx_receipts_user_timestamp ON receipts(user_id, timestamp);