package users

import (
	"context"
	"database/sql"
	"time"
)

type UserFeedback struct {
	ID        int64
	DeviceID  int64
	CreatedAt time.Time
	Type      string // bug, feature-request, other
	Feedback  string
	Comment   sql.NullString
}

// Insert inserts a new user feedback into the database.
// It updates the UserFeedback's ID with the ID from the database.
func (u *UserFeedback) Insert(ctx context.Context, d *sql.DB) error {
	res, err := d.ExecContext(ctx,
		`INSERT INTO user_feedback (
			device_id, type, feedback, comment
		) VALUES (?, ?, ?, ?)`,
		u.DeviceID, u.Type, u.Feedback, u.Comment)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	u.ID = id
	return nil
}

// Get gets a user feedback by its ID.
func (u *UserFeedback) Get(ctx context.Context, d *sql.DB) error {
	return d.QueryRowContext(ctx,
		`SELECT device_id, created_at, type, feedback, comment 
		 FROM user_feedback WHERE id = ?`, u.ID).
		Scan(&u.DeviceID, &u.CreatedAt, &u.Type, &u.Feedback, &u.Comment)
}

// GetByDeviceID gets all feedback for a device.
func GetFeedbackByDeviceID(ctx context.Context, d *sql.DB, deviceID int64) ([]UserFeedback, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, created_at, type, feedback, comment 
		 FROM user_feedback WHERE device_id = ?
		 ORDER BY created_at DESC`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feedbacks []UserFeedback
	for rows.Next() {
		var feedback UserFeedback
		feedback.DeviceID = deviceID
		err := rows.Scan(&feedback.ID, &feedback.CreatedAt,
			&feedback.Type, &feedback.Feedback, &feedback.Comment)
		if err != nil {
			return nil, err
		}
		feedbacks = append(feedbacks, feedback)
	}
	return feedbacks, rows.Err()
}
