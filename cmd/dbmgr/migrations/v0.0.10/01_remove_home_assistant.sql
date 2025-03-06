-- First, delete all examples related to the Home Assistant tool
DELETE FROM examples
WHERE tool_id IN (
    SELECT id FROM tools WHERE name = 'Home Assistant'
);

-- Then, delete the Home Assistant tool itself
DELETE FROM tools
WHERE name = 'Home Assistant'; 