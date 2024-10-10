CREATE TABLE IF NOT EXISTS tools (
  id INTEGER PRIMARY KEY,
  name TEXT,
  description TEXT,
  version TEXT,
  cost_per_call REAL NOT NULL,
  cost_multiplier REAL NOT NULL,
  base_tokens INTEGER NOT NULL
);
