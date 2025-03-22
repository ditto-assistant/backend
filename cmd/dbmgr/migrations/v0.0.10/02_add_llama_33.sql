-- Text Models
INSERT INTO services (name, description, version, service_type, provider, base_cost_per_million_input_tokens, base_cost_per_million_output_tokens, max_total_tokens, profit_margin_percentage)
VALUES 
  ('meta/llama-3.3-70b-instruct-maas', 'Meta''s Llama 3.3 70B Instruct model', '1.0', 'prompt', 'meta', 0.0, 0.0, 1000000, 100.0);
ON CONFLICT DO NOTHING;