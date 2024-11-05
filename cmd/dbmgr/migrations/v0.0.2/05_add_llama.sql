-- Text Models
INSERT INTO services (name, description, version, service_type, provider, base_cost_per_million_input_tokens, base_cost_per_million_output_tokens, max_total_tokens, profit_margin_percentage)
VALUES 
  ('llama-3-2', 'Meta''s Llama 3.2 model', '1.0', 'prompt', 'meta', 0.0, 0.0, 1000000, 100.0),
  ('mistral-nemo', 'Mistral''s Nemo model', '1.0', 'prompt', 'mistral', 0.15, 0.15, 2000000, 100.0);
ON CONFLICT DO NOTHING;