package users

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

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
func (u *User) Insert(ctx context.Context, d *sql.DB) error {
	res, err := d.ExecContext(ctx,
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

// GetByUID gets a user id, balance, and email by their UID.
// If the user does not exist, it creates a new user.
func (u *User) GetByUID(ctx context.Context, d *sql.DB) (err error) {
	*u, err = userCache.Get(u.UID, func() (User, error) {
		usr := User{UID: u.UID}
		err := d.QueryRowContext(ctx,
			"SELECT id, balance, email FROM users WHERE uid = ?", u.UID).
			Scan(&usr.ID, &usr.Balance, &usr.Email)
		if err != nil {
			return usr, err
		}
		slog.Debug("got user", "uid", usr.UID, "id", usr.ID, "balance", usr.Balance, "email", usr.Email.String)
		return usr, nil
	})
	return err
}

// Init sets the user's balance to the given value.
// If the user does not exist, it creates a new user.
func (u *User) InitBalance(ctx context.Context, d *sql.DB) error {
	err := d.QueryRowContext(ctx, "SELECT id FROM users WHERE uid = ?", u.UID).Scan(&u.ID)
	if err == sql.ErrNoRows { // User doesn't exist, create a new one
		return u.Insert(ctx, d)
	} else if err != nil {
		return err
	}
	// User exists, update the balance
	_, err = d.ExecContext(ctx, "UPDATE users SET balance = ? WHERE id = ?", u.Balance, u.ID)
	if err != nil {
		return err
	}
	return nil
}
