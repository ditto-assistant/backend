CREATE TABLE IF NOT EXISTS user_devices (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  device_uid TEXT NOT NULL,
  last_sign_in DATETIME DEFAULT CURRENT_TIMESTAMP,
  user_agent TEXT,
  version TEXT NOT NULL,
  -- mobile-browser, desktop-browser, mobile-app, desktop-app, unknown
  platform TEXT NOT NULL,
  accept_language TEXT,
  -- for Ditto team
  comment TEXT,
  FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_user_devices_user_id ON user_devices(user_id);
