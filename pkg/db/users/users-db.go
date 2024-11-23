package users

import (
	"context"
	"database/sql"

	"github.com/ditto-assistant/backend/pkg/db"
)

type User struct {
	ID                    int64
	UID                   string
	Email                 sql.NullString
	Balance               int64
	LastAirdropAt         sql.NullTime
	TotalTokensAirdropped int64
}

// Insert inserts a new user into the database.
// It updates the User's ID with the ID from the database.
func (u *User) Insert(ctx context.Context) error {
	res, err := db.D.ExecContext(ctx,
		"INSERT INTO users (uid, email, balance, total_tokens_airdropped, last_airdrop_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)",
		u.UID, u.Email, u.Balance, u.TotalTokensAirdropped)
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

// Get gets a user by their UID.
// If the user does not exist, it creates a new user.
func (u *User) Get(ctx context.Context) error {
	err := db.D.QueryRowContext(ctx,
		"SELECT id, balance FROM users WHERE uid = ?", u.UID).
		Scan(&u.ID, &u.Balance)
	if err == sql.ErrNoRows {
		err = u.Insert(ctx)
	}
	return err
}

// Init sets the user's balance to the given value.
// If the user does not exist, it creates a new user.
func (u *User) InitBalance(ctx context.Context) error {
	err := db.D.QueryRowContext(ctx, "SELECT id FROM users WHERE uid = ?", u.UID).Scan(&u.ID)
	if err == sql.ErrNoRows { // User doesn't exist, create a new one
		return u.Insert(ctx)
	} else if err != nil {
		return err
	}
	// User exists, update the balance
	_, err = db.D.ExecContext(ctx, "UPDATE users SET balance = ? WHERE id = ?", u.Balance, u.ID)
	if err != nil {
		return err
	}
	return nil
}
