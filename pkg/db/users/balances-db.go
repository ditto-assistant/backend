package users

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/pkg/numfmt"
	"github.com/ditto-assistant/backend/types/rp"
	"github.com/ditto-assistant/backend/types/rq"
)

// HandleGetBalance manages the entire balance check flow including airdrops
func HandleGetBalance(ctx context.Context, req rq.BalanceV1) (rp.BalanceV1, error) {
	if req.Email == "" {
		return GetUserBalance(ctx, WithUserID(req.UserID))
	}

	// Handle airdrop first
	ok, err := HandleGetAirdrop(ctx, req)
	if err != nil {
		return rp.BalanceV1{}, fmt.Errorf("failed to handle airdrop: %w", err)
	}
	if ok {
		slog.Info("airdropped tokens", "uid", req.UserID, "tokens", airdropTokens)
	}

	// Get user ID from either UID or email
	var userID int64
	err = db.D.QueryRowContext(ctx, `
		SELECT id FROM users WHERE uid = ? OR (email = ? AND email != '')
	`, req.UserID, req.Email).Scan(&userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return rp.BalanceV1{}.Zeroes(), nil
		}
		return rp.BalanceV1{}, fmt.Errorf("failed to get user ID: %w", err)
	}

	// Get balance
	balance, err := GetUserBalance(ctx, WithID(userID))
	if err != nil {
		return rp.BalanceV1{}, fmt.Errorf("failed to get user balance: %w", err)
	}

	return balance, nil
}

type RequestGetUserBalance struct {
	ID     *int64
	UserID *string
}

type RequestGetUserBalanceOption func(*RequestGetUserBalance)

func WithID(id int64) RequestGetUserBalanceOption {
	return func(req *RequestGetUserBalance) {
		req.ID = &id
	}
}

func WithUserID(userID string) RequestGetUserBalanceOption {
	return func(req *RequestGetUserBalance) {
		req.UserID = &userID
	}
}

func GetUserBalance(ctx context.Context, opts ...RequestGetUserBalanceOption) (rp.BalanceV1, error) {
	var req RequestGetUserBalance
	for _, opt := range opts {
		opt(&req)
	}
	var q struct {
		Balance  int64
		Images   float64
		Searches float64
		Dollars  float64
	}
	const mainQuery = `
		SELECT users.balance,
			   CAST(users.balance AS FLOAT) / (SELECT CAST(count AS FLOAT) FROM tokens_per_unit WHERE name = 'dollar'),
			   (users.balance / (SELECT count FROM tokens_per_unit WHERE name = 'image')),
			   (users.balance / (SELECT count FROM tokens_per_unit WHERE name = 'search'))
		FROM users`
	var err error
	if req.ID != nil {
		err = db.D.QueryRowContext(ctx, mainQuery+" WHERE id = ?", req.ID).
			Scan(&q.Balance, &q.Dollars, &q.Images, &q.Searches)
	} else if req.UserID != nil {
		err = db.D.QueryRowContext(ctx, mainQuery+" WHERE uid = ?", req.UserID).
			Scan(&q.Balance, &q.Dollars, &q.Images, &q.Searches)
	}
	if err != nil {
		if err == sql.ErrNoRows {
			return rp.BalanceV1{}.Zeroes(), nil
		}
		return rp.BalanceV1{}, err
	}
	var rsp rp.BalanceV1
	rsp.BalanceRaw = q.Balance
	rsp.Balance = numfmt.LargeNumber(q.Balance)
	rsp.USD = numfmt.USD(q.Dollars)
	rsp.ImagesRaw = int64(q.Images)
	rsp.Images = numfmt.LargeNumber(int64(q.Images))
	rsp.SearchesRaw = int64(q.Searches)
	rsp.Searches = numfmt.LargeNumber(int64(q.Searches))
	return rsp, nil
}
