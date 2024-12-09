INSERT INTO services (
    name, 
    description, 
    version, 
    service_type, 
    provider, 
    base_cost_per_image,
    profit_margin_percentage
) VALUES 
    ('flux-1-1-pro-ultra', 'BFL FLUX 1.1 Pro Ultra image generation model', '1.1', 'image_generation', 'bfl', 0.06, 100.0),
    ('flux-1-1-pro', 'BFL FLUX 1.1 Pro image generation model', '1.1', 'image_generation', 'bfl', 0.04, 100.0),
    ('flux-1-pro', 'BFL FLUX.1 Pro image generation model', '1.0', 'image_generation', 'bfl', 0.05, 100.0),
    ('flux-1-dev', 'BFL FLUX.1 Dev image generation model', '1.0', 'image_generation', 'bfl', 0.025, 100.0);