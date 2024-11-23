package users

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/types/rq"
)

// Aidrop tokens every 24 hours when the user logs in
const airdropTokens = 20_000_000

type AirdropConfig struct {
	Period time.Duration // How often airdrops can occur
	Amount int64         // How many tokens to airdrop
}

type AirdropOption func(*AirdropConfig)

func DefaultAirdropConfig() AirdropConfig {
	return AirdropConfig{
		Period: 24 * time.Hour,
		Amount: airdropTokens,
	}
}

func WithPeriod(period time.Duration) AirdropOption {
	return func(cfg *AirdropConfig) {
		cfg.Period = period
	}
}

func WithAmount(amount int64) AirdropOption {
	return func(cfg *AirdropConfig) {
		cfg.Amount = amount
	}
}

// HandleGetAirdrop manages user creation/updates and airdrops
// Returns (ok, error) where:
// - ok: true if airdrop was given
// - error: any error that occurred
func HandleGetAirdrop(ctx context.Context, req rq.BalanceV1, opts ...AirdropOption) (bool, error) {
	cfg := DefaultAirdropConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	var q struct {
		ID            int64
		Email         sql.NullString
		LastAirdropAt sql.NullTime
		Balance       int64
	}

	// First try to find user by UID
	err := db.D.QueryRowContext(ctx, `
		SELECT id, email, last_airdrop_at, balance 
		FROM users 
		WHERE uid = ?`, req.UserID).
		Scan(&q.ID, &q.Email, &q.LastAirdropAt, &q.Balance)

	if err == sql.ErrNoRows && req.Email != "" {
		// User not found by UID, check if email exists
		err = db.D.QueryRowContext(ctx, `
			SELECT id, email, last_airdrop_at, balance 
			FROM users 
			WHERE email = ?`, req.Email).
			Scan(&q.ID, &q.Email, &q.LastAirdropAt, &q.Balance)

		if err == sql.ErrNoRows {
			// New user - create account with airdrop
			user := User{
				UID:                req.UserID,
				Email:              sql.NullString{String: req.Email, Valid: true},
				Balance:            cfg.Amount,
				TotalTokensAirdrop: cfg.Amount,
				LastAirdropAt:      sql.NullTime{Time: time.Now(), Valid: true},
			}
			if err := user.Insert(ctx); err != nil {
				return false, fmt.Errorf("failed to insert new user: %w", err)
			}
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("failed to check existing email: %w", err)
		}

		// Email exists but UID is different - update the UID
		_, err = db.D.ExecContext(ctx, `
			UPDATE users 
			SET uid = ? 
			WHERE id = ?`, req.UserID, q.ID)
		if err != nil {
			return false, fmt.Errorf("failed to update user UID: %w", err)
		}
	} else if err != nil {
		return false, fmt.Errorf("failed to get user: %w", err)
	}

	// Check if eligible for airdrop
	if !q.LastAirdropAt.Valid || time.Since(q.LastAirdropAt.Time) > cfg.Period {
		_, err = db.D.ExecContext(ctx, `
			UPDATE users SET
				balance = balance + ?,
				total_tokens_airdropped = total_tokens_airdropped + ?,
				last_airdrop_at = CURRENT_TIMESTAMP
			WHERE id = ?`, cfg.Amount, cfg.Amount, q.ID)
		if err != nil {
			return false, fmt.Errorf("failed to airdrop tokens: %w", err)
		}
		return true, nil
	}

	return false, nil
}
