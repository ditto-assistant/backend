CREATE TABLE IF NOT EXISTS tokens_per_dollar (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    count INTEGER NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO tokens_per_dollar (name, count) VALUES ('ditto', 1000000000);

CREATE TRIGGER IF NOT EXISTS update_tokens_per_dollar_timestamp
AFTER UPDATE ON tokens_per_dollar
FOR EACH ROW
BEGIN
  UPDATE tokens_per_dollar SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;