package users

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/omniaura/mapcache"
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

var userCache, _ = mapcache.New[string, User](mapcache.WithTTL(time.Minute))

// Get gets a user by their UID.
// If the user does not exist, it creates a new user.
func (u *User) Get(ctx context.Context) (err error) {
	*u, err = userCache.Get(u.UID, func() (User, error) {
		usr := User{UID: u.UID}
		err := db.D.QueryRowContext(ctx,
			"SELECT id, balance FROM users WHERE uid = ?", u.UID).
			Scan(&usr.ID, &usr.Balance)
		if err == sql.ErrNoRows {
			err = usr.Insert(ctx)
		}
		slog.Debug("got user", "uid", usr.UID, "id", usr.ID, "balance", usr.Balance)
		return usr, err
	})
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
