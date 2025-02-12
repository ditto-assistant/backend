-- Add total_tokens_airdropped column
ALTER TABLE users ADD COLUMN total_tokens_airdropped INTEGER DEFAULT 0;

-- Add last_airdrop_at column
ALTER TABLE users ADD COLUMN last_airdrop_at TIMESTAMP;