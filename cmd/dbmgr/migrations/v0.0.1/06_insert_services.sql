-- Text Embeddings
INSERT INTO services (name, description, version, service_type, provider, base_cost_per_million_tokens, max_total_tokens, profit_margin_percentage)
VALUES 
  ('text-embedding-004', 'Google''s embedding model', '1.0', 'embedding', 'google', 0.025, 3072, 100.0),
  ('text-embedding-3-small', 'OpenAI''s embedding model', '1.0', 'embedding', 'openai', 0.02, 8192, 100.0);

-- Text Models
INSERT INTO services (name, description, version, service_type, provider, base_cost_per_million_input_tokens, base_cost_per_million_output_tokens, max_total_tokens, profit_margin_percentage)
VALUES 
  ('gemini-1.5-flash', 'Google''s Gemini 1.5 Flash model', '1.0', 'prompt', 'google', 0.15, 0.6, 1000000, 100.0),
  ('gemini-1.5-pro', 'Google''s Gemini 1.5 Pro model', '1.0', 'prompt', 'google', 0.625, 2.5, 2000000, 100.0),
  ('claude-3-5-sonnet', 'Anthropic''s Claude 3.5 Sonnet model', '20240620', 'prompt', 'anthropic', 3.0, 15.0, 200000, 100.0),
  ('dall-e-3', 'OpenAI''s DALL-E 3 model', '1.0', 'image_generation', 'openai', NULL, NULL, NULL, 100.0);

-- Update DALL-E 3 with image-specific pricing
UPDATE services 
SET base_cost_per_image = 0.08
WHERE name = 'dall-e-3';

-- Search Engines
INSERT INTO services (name, description, version, service_type, provider, base_cost_per_search, profit_margin_percentage)
VALUES 
  ('brave-search', 'Brave''s search engine', '1.0', 'search', 'brave', 0.009, 100.0),
  ('google-search', 'Google''s search engine', '1.0', 'search', 'google', 0.01, 100.0);

-- Set some common values for all services
UPDATE services
SET 
  is_active = TRUE,
  currency = 'USD',
  peak_hours_multiplier = 1.5,
  volume_discount_threshold = 1000000,
  volume_discount_percentage = 10.0,
  minimum_profit_amount = 0.0001,
  rate_limit_per_minute = 60,
  rate_limit_per_day = 1000000;

-- Set batch processing for embedding models
UPDATE services
SET 
  supports_batching = TRUE,
  batch_size_limit = 100
WHERE service_type = 'embedding';

-- Set image generation specifics
UPDATE services
SET
  base_cost_per_call = 0.02,
  supports_batching = FALSE
WHERE name = 'dall-e-3';

-- Set search engine specifics
UPDATE services
SET
  supports_batching = FALSE,
  base_cost_per_call = 0.001
WHERE service_type = 'search';
