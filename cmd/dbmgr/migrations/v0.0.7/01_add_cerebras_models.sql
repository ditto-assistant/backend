-- Add Cerebras models with their pricing
INSERT INTO services (
    name,
    description,
    version,
    service_type,
    provider,
    base_cost_per_million_input_tokens,
    base_cost_per_million_output_tokens,
    profit_margin_percentage,
    is_active,
    currency
) VALUES 
    -- Llama 3.1 8B model
    ('llama3.1-8b', 'Cerebras Llama 3.1 8B', '1.0', 'prompt', 'cerebras', 0.10, 0.10, 100.0, true, 'USD'),
    -- Llama 3.3 70B model
    ('llama-3.3-70b', 'Cerebras Llama 3.3 70B', '1.0', 'prompt', 'cerebras', 0.85, 1.20, 100.0, true, 'USD');

-- Set common values for all Cerebras models
UPDATE services
SET 
    supports_batching = FALSE,
    peak_hours_multiplier = 1.5,
    volume_discount_threshold = 1000000,
    volume_discount_percentage = 10.0,
    minimum_profit_amount = 0.0001,
    rate_limit_per_minute = 60,
    rate_limit_per_day = 1000000
WHERE provider = 'cerebras' AND service_type = 'prompt'; 