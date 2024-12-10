-- Add Anthropic Claude models with their pricing
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
    -- Claude 3 Haiku models
    ('claude-3-haiku', 'Anthropic Claude 3 Haiku', '1.0', 'prompt', 'anthropic', 0.25, 1.25, 100.0, true, 'USD'),
    ('claude-3-haiku@20240307', 'Anthropic Claude 3 Haiku (2024-03-07)', '2024-03-07', 'prompt', 'anthropic', 0.25, 1.25, 100.0, true, 'USD'),
    -- Claude 3.5 Haiku models
    ('claude-3-5-haiku', 'Anthropic Claude 3.5 Haiku', '1.0', 'prompt', 'anthropic', 0.80, 4.00, 100.0, true, 'USD'),
    ('claude-3-5-haiku@20241022', 'Anthropic Claude 3.5 Haiku (2024-10-22)', '2024-10-22', 'prompt', 'anthropic', 0.80, 4.00, 100.0, true, 'USD'),
    -- Claude 3.5 Sonnet models
    ('claude-3-5-sonnet', 'Anthropic Claude 3.5 Sonnet', '1.0', 'prompt', 'anthropic', 3.00, 15.00, 100.0, true, 'USD'),
    ('claude-3-5-sonnet@20240620', 'Anthropic Claude 3.5 Sonnet (2024-06-20)', '2024-06-20', 'prompt', 'anthropic', 3.00, 15.00, 100.0, true, 'USD'),
    -- Claude 3.5 Sonnet V2 models
    ('claude-3-5-sonnet-v2', 'Anthropic Claude 3.5 Sonnet V2', '1.0', 'prompt', 'anthropic', 3.00, 15.00, 100.0, true, 'USD'),
    ('claude-3-5-sonnet-v2@20241022', 'Anthropic Claude 3.5 Sonnet V2 (2024-10-22)', '2024-10-22', 'prompt', 'anthropic', 3.00, 15.00, 100.0, true, 'USD');

-- Set common values for all Anthropic models
UPDATE services
SET 
    supports_batching = TRUE,
    peak_hours_multiplier = 1.5,
    volume_discount_threshold = 1000000,
    volume_discount_percentage = 10.0,
    minimum_profit_amount = 0.0001,
    rate_limit_per_minute = 60,
    rate_limit_per_day = 1000000
WHERE provider = 'anthropic' AND service_type = 'prompt';
