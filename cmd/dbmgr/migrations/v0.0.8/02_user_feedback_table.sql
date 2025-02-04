CREATE TABLE IF NOT EXISTS user_feedback (
  id INTEGER PRIMARY KEY,
  device_id INTEGER NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  -- feedback-type: bug, feature-request, other
  type NOT NULL,
  feedback TEXT NOT NULL,
  -- for Ditto team
  comment TEXT,
  FOREIGN KEY (device_id) REFERENCES user_devices(id)
);
