CREATE TABLE IF NOT EXISTS purchases (
  id INTEGER PRIMARY KEY,
  payment_id TEXT NOT NULL UNIQUE,
  user_id INTEGER NOT NULL,
  cents INTEGER NOT NULL,
  tokens INTEGER NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER IF NOT EXISTS after_insert_purchases
AFTER INSERT ON purchases
FOR EACH ROW
BEGIN
    UPDATE users
    SET balance = balance + NEW.tokens
    WHERE id = NEW.user_id;
END;
