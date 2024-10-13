package db

import (
	"context"
	"database/sql"
	"encoding/json"
)

type User struct {
	ID      int64
	UID     string
	Name    string
	Balance int64
}

// Insert inserts a new user into the database.
// It updates the User's ID with the ID from the database.
func (u *User) Insert(ctx context.Context) error {
	res, err := D.ExecContext(ctx, "INSERT INTO users (uid, name, balance) VALUES (?, ?, ?)", u.UID, u.Name, u.Balance)
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

// GetOrCreateUser gets a user by their UID.
// If the user does not exist, it creates a new user.
func GetOrCreateUser(ctx context.Context, uid string) (User, error) {
	var u User
	err := D.QueryRowContext(ctx, "SELECT id, uid, name, balance FROM users WHERE uid = ?", uid).Scan(&u.ID, &u.UID, &u.Name, &u.Balance)
	if err == sql.ErrNoRows {
		u = User{UID: uid}
		err = u.Insert(ctx)
	}
	return u, err
}

type Receipt struct {
	ID        int64
	UserID    int64
	ToolID    int64
	Tokens    int64
	UsageType string
	Model     string
	Metadata  json.RawMessage
}

// Insert inserts a new receipt into the database.
// It updates the Receipt's ID with the ID from the database.
func (r *Receipt) Insert(ctx context.Context) error {
	res, err := D.ExecContext(ctx,
		"INSERT INTO receipts (user_id, tool_id, tokens, usage_type, model, metadata) VALUES (?, ?, ?, ?, ?, ?)",
		r.UserID, r.ToolID, r.Tokens, r.UsageType, r.Model, r.Metadata,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	r.ID = id
	return nil
}
