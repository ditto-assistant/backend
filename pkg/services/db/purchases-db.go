package db

import (
	"context"
	"database/sql"
	"time"
)

type Purchase struct {
	ID        int64
	PaymentID string
	UserID    int64
	Cents     int64
	Tokens    int64
	CreatedAt time.Time
}

func (p *Purchase) Insert(ctx context.Context, d *sql.DB, uid string) error {
	res, err := d.ExecContext(ctx, `
		WITH user_lookup AS (
			SELECT id FROM users WHERE uid = ?
		)
		INSERT INTO purchases (payment_id, user_id, cents, tokens) 
		SELECT ?, id, ?, ?
		FROM user_lookup`,
		uid,
		p.PaymentID,
		p.Cents,
		p.Tokens,
	)
	if err != nil {
		return err
	}
	p.ID, err = res.LastInsertId()
	return err
}
