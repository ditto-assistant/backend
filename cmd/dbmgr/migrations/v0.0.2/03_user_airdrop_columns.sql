ALTER TABLE users ADD COLUMN total_tokens_airdropped INTEGER DEFAULT 0;

ALTER TABLE users ADD COLUMN last_airdrop_at TIMESTAMP;
