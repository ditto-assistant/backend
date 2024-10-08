CREATE TABLE IF NOT EXISTS migrations (
  migration_name TEXT,
  migration_date TEXT DEFAULT (datetime('now'))
);