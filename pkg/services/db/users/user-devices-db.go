package users

import (
	"context"
	"database/sql"
	"time"
)

type UserDevice struct {
	ID             int64
	UserID         int64
	DeviceUID      string
	LastSignIn     time.Time
	UserAgent      sql.NullString
	Version        string
	Platform       Platform
	AcceptLanguage sql.NullString
	Comment        sql.NullString
}

// Insert inserts a new user device into the database.
// It updates the UserDevice's ID with the ID from the database.
// If a device with the same user_id, device_uid and version exists,
// it will fail with a constraint error.
func (ud *UserDevice) Insert(ctx context.Context, d *sql.DB) error {
	res, err := d.ExecContext(ctx,
		`INSERT INTO user_devices (
			user_id, device_uid, user_agent, 
			version, platform, accept_language, last_sign_in
		) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		ud.UserID, ud.DeviceUID, ud.UserAgent, ud.Version, ud.Platform, ud.AcceptLanguage)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	ud.ID = id
	return nil
}

// Get gets a user device by its device_uid.
func (ud *UserDevice) Get(ctx context.Context, d *sql.DB) error {
	return d.QueryRowContext(ctx,
		`SELECT id, user_id, last_sign_in, user_agent, 
		        version, platform, accept_language, comment 
		 FROM user_devices WHERE device_uid = ?`, ud.DeviceUID).
		Scan(&ud.ID, &ud.UserID, &ud.LastSignIn, &ud.UserAgent,
			&ud.Version, &ud.Platform, &ud.AcceptLanguage, &ud.Comment)
}

// Exists checks if a device exists by its device_uid and version.
func (ud *UserDevice) Exists(ctx context.Context, d *sql.DB) (bool, error) {
	err := d.QueryRowContext(ctx,
		`SELECT id FROM user_devices 
		 WHERE device_uid = ? AND version = ? AND user_id = ?`,
		ud.DeviceUID, ud.Version, ud.UserID).
		Scan(&ud.ID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetByUserID gets all devices for a user.
func GetDevicesByUserID(ctx context.Context, d *sql.DB, userID int64) ([]UserDevice, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, device_uid, last_sign_in, user_agent, 
		        version, platform, accept_language, comment 
		 FROM user_devices WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []UserDevice
	for rows.Next() {
		var device UserDevice
		device.UserID = userID
		err := rows.Scan(&device.ID, &device.DeviceUID, &device.LastSignIn,
			&device.UserAgent, &device.Version, &device.Platform,
			&device.AcceptLanguage, &device.Comment)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

// UpdateLastSignIn updates the last_sign_in timestamp for a device
func (ud *UserDevice) UpdateLastSignIn(ctx context.Context, d *sql.DB) error {
	_, err := d.ExecContext(ctx,
		"UPDATE user_devices SET last_sign_in = CURRENT_TIMESTAMP WHERE id = ?", ud.ID)
	return err
}
