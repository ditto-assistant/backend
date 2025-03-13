-- Delete duplicate devices
DELETE FROM user_devices
WHERE id NOT IN (
    SELECT MIN(id) FROM user_devices
    GROUP BY user_id, device_uid, version
);


CREATE UNIQUE INDEX IF NOT EXISTS idx_user_devices_unique 
ON user_devices(user_id, device_uid, version);