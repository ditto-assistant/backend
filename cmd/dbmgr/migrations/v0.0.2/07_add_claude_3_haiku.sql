-- Text Models
INSERT INTO services (name, description, version, service_type, provider, base_cost_per_million_input_tokens, base_cost_per_million_output_tokens, max_total_tokens, profit_margin_percentage)
VALUES 
  ('claude-3-haiku@20240307', 'Anthropic''s Claude 3 Haiku model', '1.0', 'prompt', 'anthropic', 0.25, 1.25, 128000, 100.0);
ON CONFLICT DO NOTHING;
