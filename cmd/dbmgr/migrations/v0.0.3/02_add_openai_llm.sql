-- Add OpenAI GPT models with their pricing
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
    -- GPT-4o models
    ('gpt-4o', 'OpenAI GPT-4 Turbo', '1.0', 'prompt', 'openai', 2.50, 10.00, 100.0, true, 'USD'),
    ('gpt-4o-2024-11-20', 'OpenAI GPT-4 Turbo (2024-11-20)', '2024-11-20', 'prompt', 'openai', 2.50, 10.00, 100.0, true, 'USD'),
    -- GPT-4o Mini models
    ('gpt-4o-mini', 'OpenAI GPT-4 Turbo Mini', '1.0', 'prompt', 'openai', 0.15, 0.60, 100.0, true, 'USD'),
    ('gpt-4o-mini-2024-07-18', 'OpenAI GPT-4 Turbo Mini (2024-07-18)', '2024-07-18', 'prompt', 'openai', 0.15, 0.60, 100.0, true, 'USD'),
    -- O1 Preview models
    ('o1-preview', 'OpenAI O1 Preview', '1.0', 'prompt', 'openai', 15.00, 60.00, 100.0, true, 'USD'),
    ('o1-preview-2024-09-12', 'OpenAI O1 Preview (2024-09-12)', '2024-09-12', 'prompt', 'openai', 15.00, 60.00, 100.0, true, 'USD'),
    -- O1 Mini models
    ('o1-mini', 'OpenAI O1 Mini', '1.0', 'prompt', 'openai', 3.00, 12.00, 100.0, true, 'USD'),
    ('o1-mini-2024-09-12', 'OpenAI O1 Mini (2024-09-12)', '2024-09-12', 'prompt', 'openai', 3.00, 12.00, 100.0, true, 'USD');

-- Set common values for all OpenAI models
UPDATE services
SET 
    supports_batching = FALSE,
    peak_hours_multiplier = 1.5,
    volume_discount_threshold = 1000000,
    volume_discount_percentage = 10.0,
    minimum_profit_amount = 0.0001,
    rate_limit_per_minute = 60,
    rate_limit_per_day = 1000000
WHERE provider = 'openai' AND service_type = 'prompt';
