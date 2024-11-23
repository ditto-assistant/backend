-- Add total_tokens_airdropped if it doesn't exist
SELECT CASE 
    WHEN NOT EXISTS(SELECT 1 FROM pragma_table_info('users') WHERE name='total_tokens_airdropped') 
    THEN 'ALTER TABLE users ADD COLUMN total_tokens_airdropped INTEGER DEFAULT 0;'
END AS sql_statement
WHERE sql_statement IS NOT NULL;

-- Add last_airdrop_at if it doesn't exist
SELECT CASE 
    WHEN NOT EXISTS(SELECT 1 FROM pragma_table_info('users') WHERE name='last_airdrop_at') 
    THEN 'ALTER TABLE users ADD COLUMN last_airdrop_at TIMESTAMP;'
END AS sql_statement
WHERE sql_statement IS NOT NULL;