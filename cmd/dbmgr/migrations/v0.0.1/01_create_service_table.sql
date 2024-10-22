CREATE TABLE IF NOT EXISTS services (
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

-- Trigger to update the 'updated_at' column
CREATE TRIGGER update_services_timestamp
AFTER UPDATE ON services
FOR EACH ROW
BEGIN
  UPDATE services SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;
