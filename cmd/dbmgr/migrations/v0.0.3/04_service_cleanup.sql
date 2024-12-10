UPDATE services
SET 
    version = '2024-06-20',
    name = 'claude-3-5-sonnet',
    description = 'Anthropic Claude 3.5 Sonnet'
WHERE name = 'claude-3-5-sonnet';

UPDATE services
SET 
    version = '2024-10-22',
    name = 'claude-3-5-sonnet-v2',
    description = 'Anthropic Claude 3.5 Sonnet V2'
WHERE name = 'claude-3-5-sonnet-v2';

-- Update descriptions to use standard apostrophes
UPDATE services
SET description = 'Google Gemini 1.5 Flash model'
WHERE name = 'gemini-1.5-flash';

UPDATE services
SET description = 'Google Gemini 1.5 Pro model'
WHERE name = 'gemini-1.5-pro';

UPDATE services
SET description = 'Google text embedding model'
WHERE name = 'text-embedding-004';

UPDATE services
SET description = 'OpenAI text embedding model'
WHERE name = 'text-embedding-3-small';

UPDATE services
SET description = 'Brave search engine'
WHERE name = 'brave-search';

UPDATE services
SET description = 'Google search engine'
WHERE name = 'google-search';

UPDATE services
SET description = 'Meta Llama 3.2 model'
WHERE name = 'llama-3-2';

UPDATE services
SET description = 'Mistral Nemo model'
WHERE name = 'mistral-nemo';

UPDATE services
SET description = 'Mistral Large model'
WHERE name = 'mistral-large';

-- Clean up duplicate services
DELETE FROM services 
WHERE id NOT IN (
    SELECT MIN(id)
    FROM services 
    GROUP BY name
);

