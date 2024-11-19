UPDATE services 
SET base_cost_per_image = 0.04,  -- $0.040 base cost per image
    base_cost_per_call = 0.0     -- $0.002 API call cost
WHERE name = 'dall-e-3';