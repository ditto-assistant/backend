CREATE TABLE IF NOT EXISTS user_devices (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  device_uid TEXT NOT NULL,
  last_sign_in DATETIME DEFAULT CURRENT_TIMESTAMP,
  user_agent TEXT,
  version TEXT NOT NULL,
  -- 0: web, 1: android, 2: ios, 3: linux, 4: macos, 5: unknown, 6: windows
  platform INTEGER NOT NULL,
  accept_language TEXT,
  -- for Ditto team
  comment TEXT,
  FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_user_devices_user_id ON user_devices(user_id);
