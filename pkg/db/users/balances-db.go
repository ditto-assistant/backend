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

// GetBalance manages the entire balance check flow including airdrops
func GetBalance(ctx context.Context, req rq.BalanceV1) (rp.BalanceV1, error) {
	if req.Email == "" {
		return getBalance(ctx, WithUserID(req.UserID))
	}
	res, err := handleAirdrop(ctx, req)
	if err != nil {
		return rp.BalanceV1{}, fmt.Errorf("failed to handle airdrop: %w", err)
	}
	balance, err := getBalance(ctx, WithID(res.ID))
	if err != nil {
		return rp.BalanceV1{}, fmt.Errorf("failed to get user balance: %w", err)
	}
	if res.DropAmount > 0 {
		slog.Info("airdropped tokens", "uid", req.UserID, "tokens", res.DropAmount)
		balance.DropAmountRaw = res.DropAmount
		balance.DropAmount = numfmt.LargeNumber(res.DropAmount)
	}
	return balance, nil
}

type requestGetUserBalance struct {
	ID     *int64
	UserID *string
}

type requestGetUserBalanceOption func(*requestGetUserBalance)

func WithID(id int64) requestGetUserBalanceOption {
	return func(req *requestGetUserBalance) {
		req.ID = &id
	}
}

func WithUserID(userID string) requestGetUserBalanceOption {
	return func(req *requestGetUserBalance) {
		req.UserID = &userID
	}
}

func getBalance(ctx context.Context, opts ...requestGetUserBalanceOption) (rp.BalanceV1, error) {
	var req requestGetUserBalance
	for _, opt := range opts {
		opt(&req)
	}
	var q struct {
		Balance         int64
		TotalAirdropped int64
		Images          float64
		Searches        float64
		Dollars         float64
	}
	const mainQuery = `
		SELECT users.balance,
			   users.total_tokens_airdropped,
			   CAST(users.balance AS FLOAT) / (SELECT CAST(count AS FLOAT) FROM tokens_per_unit WHERE name = 'dollar'),
			   (users.balance / (SELECT count FROM tokens_per_unit WHERE name = 'image')),
			   (users.balance / (SELECT count FROM tokens_per_unit WHERE name = 'search'))
		FROM users`
	var err error
	if req.ID != nil {
		err = db.D.QueryRowContext(ctx, mainQuery+" WHERE id = ?", req.ID).
			Scan(&q.Balance, &q.TotalAirdropped, &q.Dollars, &q.Images, &q.Searches)
	} else if req.UserID != nil {
		err = db.D.QueryRowContext(ctx, mainQuery+" WHERE uid = ?", req.UserID).
			Scan(&q.Balance, &q.TotalAirdropped, &q.Dollars, &q.Images, &q.Searches)
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
	if q.TotalAirdropped > 0 {
		rsp.TotalAirdroppedRaw = q.TotalAirdropped
		rsp.TotalAirdropped = numfmt.LargeNumber(q.TotalAirdropped)
	}
	return rsp, nil
}
