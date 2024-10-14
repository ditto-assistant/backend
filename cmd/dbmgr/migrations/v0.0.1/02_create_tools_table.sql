CREATE TABLE IF NOT EXISTS tools (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  version TEXT NOT NULL,
  service_id INTEGER NOT NULL,
  FOREIGN KEY (service_id) REFERENCES services(id)
);
