CREATE TABLE IF NOT EXISTS receipts (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  tool_id INTEGER NOT NULL,
  timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
  tokens REAL NOT NULL,
  usage_type TEXT NOT NULL,
  metadata TEXT,
  FOREIGN KEY (user_id) REFERENCES users(id),
  FOREIGN KEY (tool_id) REFERENCES tools(id)
);

CREATE TRIGGER IF NOT EXISTS update_balance AFTER INSERT ON receipts
BEGIN
  UPDATE users
  SET balance = balance - NEW.tokens
  WHERE rowid = NEW.user_id;
END;