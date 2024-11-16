DELETE FROM services 
WHERE id NOT IN (
    SELECT MIN(id)
    FROM services 
    GROUP BY name
);

UPDATE services 
SET 
    description = 'DALL-E 3 Standard Quality (1024x1024)',
    base_cost_per_image = 0.04,
    base_cost_per_call = 0.0
WHERE name = 'dall-e-3';

INSERT INTO services (
    name,
    description,
    version,
    service_type,
    provider,
    base_cost_per_image,
    profit_margin_percentage
) VALUES 
    ('dall-e-3-wide', 'DALL-E 3 Standard Quality Wide (1024x1792 or 1792x1024)', '1.0', 'image_generation', 'openai', 0.08, 100.0),
    ('dall-e-3-hd', 'DALL-E 3 HD Quality (1024x1024)', '1.0', 'image_generation', 'openai', 0.08, 100.0),
    ('dall-e-3-hd-wide', 'DALL-E 3 HD Quality Wide (1024x1792 or 1792x1024)', '1.0', 'image_generation', 'openai', 0.12, 100.0),
    ('dall-e-2', 'DALL-E 2 (1024x1024)', '1.0', 'image_generation', 'openai', 0.02, 100.0),
    ('dall-e-2-small', 'DALL-E 2 Small (512x512)', '1.0', 'image_generation', 'openai', 0.018, 100.0),
    ('dall-e-2-tiny', 'DALL-E 2 Tiny (256x256)', '1.0', 'image_generation', 'openai', 0.016, 100.0);

DELETE FROM services 
WHERE id NOT IN (
    SELECT MIN(id)
    FROM services 
    GROUP BY name
);