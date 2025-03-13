package users

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/ditto-assistant/backend/types/rq"
)

// Aidrop tokens every 24 hours when the user logs in
const airdropTokens = 20_000_000
const initialAirdropTokens = 250_000_000

type airdropConfig struct {
	Period        time.Duration // How often airdrops can occur
	Amount        int64         // How many tokens to airdrop
	InitialAmount int64         // How many tokens to airdrop when the user is created
}

type airdropOption func(*airdropConfig)

func defaultAirdropConfig() airdropConfig {
	return airdropConfig{
		Period:        time.Hour * 24,
		Amount:        airdropTokens,
		InitialAmount: initialAirdropTokens,
	}
}

func withPeriod(period time.Duration) airdropOption {
	return func(cfg *airdropConfig) {
		cfg.Period = period
	}
}

func withAmount(amount int64) airdropOption {
	return func(cfg *airdropConfig) {
		cfg.Amount = amount
	}
}

type resultAirdrop struct {
	ID         int64
	DropAmount int64
}

func handleAirdrop(
	ctx context.Context,
	d *sql.DB,
	req rq.BalanceV1,
	opts ...airdropOption,
) (*resultAirdrop, error) {
	cfg := defaultAirdropConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	var q User
	err := d.QueryRowContext(ctx, `
		SELECT id, uid, email, last_airdrop_at FROM users WHERE email = ? OR uid = ?`, req.Email, req.UserID).
		Scan(&q.ID, &q.UID, &q.Email, &q.LastAirdropAt)
	if err == sql.ErrNoRows {
		// New user - create account with airdrop
		amt := cfg.InitialAmount
		if req.Email == "" {
			amt = 0
		}
		user := User{
			UID:                   req.UserID,
			Email:                 sql.NullString{String: req.Email, Valid: req.Email != ""},
			Balance:               amt,
			TotalTokensAirdropped: amt,
		}
		if err := user.Insert(ctx, d); err != nil {
			return nil, fmt.Errorf("failed to insert new user: %w", err)
		}
		return &resultAirdrop{ID: user.ID, DropAmount: amt}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if !q.Email.Valid && req.Email == "" {
		return &resultAirdrop{ID: q.ID}, nil
	}

	if q.Email.Valid &&
		q.Email.String == req.Email &&
		q.UID != req.UserID { // Email is the same but userID is different
		_, err = d.ExecContext(ctx, `
			UPDATE users SET uid = ? WHERE id = ?`, req.UserID, q.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to update user UID: %w", err)
		}
		slog.Info("user deleted and recreated account", "uid", req.UserID, "email", req.Email)
	} else if q.UID == req.UserID &&
		(!q.Email.Valid || q.Email.String != req.Email) { // Email changed or not set
		_, err = d.ExecContext(ctx, `
			UPDATE users SET email = ? WHERE id = ?`, req.Email, q.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to update user email: %w", err)
		}
		slog.Info("updated user email", "uid", req.UserID, "email", req.Email)
	}

	if q.LastAirdropAt.Valid && time.Since(q.LastAirdropAt.Time) < cfg.Period {
		return &resultAirdrop{ID: q.ID}, nil
	}
	_, err = d.ExecContext(ctx, `
			UPDATE users SET
				balance = balance + ?,
				total_tokens_airdropped = total_tokens_airdropped + ?,
				last_airdrop_at = CURRENT_TIMESTAMP
			WHERE id = ?`, cfg.Amount, cfg.Amount, q.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to airdrop tokens: %w", err)
	}
	return &resultAirdrop{ID: q.ID, DropAmount: cfg.Amount}, nil
}
