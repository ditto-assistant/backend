INSERT INTO services (
    name,
    description,
    version,
    service_type,
    provider,
    base_cost_per_million_tokens,
    max_total_tokens,
    profit_margin_percentage,
    is_active,
    currency
) VALUES 
    ('text-embedding-005', 'Google text embedding model', '1.0', 'embedding', 'google', 0.025, 3072, 100.0, true, 'USD');

UPDATE services
SET 
    supports_batching = TRUE,
    batch_size_limit = 100,
    peak_hours_multiplier = 1.5,
    volume_discount_threshold = 1000000,
    volume_discount_percentage = 20.0,
    minimum_profit_amount = 0.0001,
    rate_limit_per_minute = 60,
    rate_limit_per_day = 1000000
WHERE name = 'text-embedding-005';