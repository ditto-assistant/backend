-- Text Models
INSERT INTO services (name, description, version, service_type, provider, base_cost_per_million_input_tokens, base_cost_per_million_output_tokens, max_total_tokens, profit_margin_percentage)
VALUES 
  ('mistral-large', 'Mistral''s Large model', '1.0', 'prompt', 'mistral', 2.0, 6.0, 128000, 100.0);
ON CONFLICT DO NOTHING;
