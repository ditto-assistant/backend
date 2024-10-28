package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ditto-assistant/backend/pkg/llm"
)

type User struct {
	ID                 int64
	UID                string
	Balance            int64
	LastAirdropAt      time.Time
	TotalTokensAirdrop int64
}

// Insert inserts a new user into the database.
// It updates the User's ID with the ID from the database.
func (u *User) Insert(ctx context.Context) error {
	res, err := D.ExecContext(ctx,
		"INSERT INTO users (uid, balance, total_tokens_airdropped, last_airdrop_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)",
		u.UID, u.Balance, u.TotalTokensAirdrop)
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

// GetByUID gets a user by their UID.
// If the user does not exist, it creates a new user.
func (u *User) GetByUID(ctx context.Context) error {
	err := D.QueryRowContext(ctx,
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
	err := D.QueryRowContext(ctx, "SELECT id FROM users WHERE uid = ?", u.UID).Scan(&u.ID)
	if err == sql.ErrNoRows { // User doesn't exist, create a new one
		return u.Insert(ctx)
	} else if err != nil {
		return err
	}
	// User exists, update the balance
	_, err = D.ExecContext(ctx, "UPDATE users SET balance = ? WHERE id = ?", u.Balance, u.ID)
	if err != nil {
		return err
	}
	return nil
}

type Receipt struct {
	ID     int64
	UserID int64
	// ServiceName is the name of the service used for this receipt.
	// It is not in this DB table, but is used for internal logic.
	ServiceName         llm.ServiceName
	ServiceID           int64
	Timestamp           time.Time
	InputTokens         int64
	OutputTokens        int64
	TotalTokens         int64
	CallDurationSeconds float64
	DataProcessedBytes  int64
	DataStoredBytes     int64
	NumImages           int64
	NumSearches         int64
	NumAPICalls         int64
	DittoTokenCost      int64
	Metadata            json.RawMessage
}

// Insert inserts a new receipt into the database.
// It updates the Receipt's ID with the ID from the database.
func (r *Receipt) Insert(ctx context.Context) error {
	// The service name, not the ID, is set by the caller.
	err := D.QueryRowContext(ctx, "SELECT id FROM services WHERE name = ?", r.ServiceName).Scan(&r.ServiceID)
	if err != nil {
		return fmt.Errorf("failed to get service id for %s: %w", r.ServiceName, err)
	}
	res, err := D.ExecContext(ctx,
		`INSERT INTO receipts (user_id, service_id, input_tokens, output_tokens, total_tokens,
			call_duration_seconds, data_processed_bytes, data_stored_bytes, num_images, num_searches,
			num_api_calls, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.UserID, r.ServiceID, r.InputTokens, r.OutputTokens, r.TotalTokens,
		r.CallDurationSeconds, r.DataProcessedBytes, r.DataStoredBytes, r.NumImages, r.NumSearches,
		r.NumAPICalls, r.Metadata,
	)
	if err != nil {
		return fmt.Errorf("failed to insert receipt: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	r.ID = id
	return nil
}
