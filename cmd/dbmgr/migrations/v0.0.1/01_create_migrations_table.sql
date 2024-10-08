CREATE TABLE IF NOT EXISTS migrations (
  name TEXT NOT NULL,
  date TEXT DEFAULT (datetime('now')),
  version TEXT NOT NULL
);
