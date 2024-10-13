CREATE TABLE IF NOT EXISTS receipts (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  tool_id INTEGER,
  timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
  tokens INTEGER NOT NULL,
  usage_type TEXT NOT NULL,
  model TEXT,
  metadata JSON,
  FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TRIGGER IF NOT EXISTS update_balance AFTER INSERT ON receipts
BEGIN
  UPDATE users
  SET balance = balance - NEW.tokens
  WHERE rowid = NEW.user_id;
END;