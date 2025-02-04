package users

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ditto-assistant/backend/pkg/utils/numfmt"
	"github.com/ditto-assistant/backend/types/rp"
	"github.com/ditto-assistant/backend/types/rq"
)

//go:generate stringer -type=Platform

type Platform int

const (
	PlatformWeb Platform = iota
	PlatformAndroid
	PlatformiOS
	PlatformLinux
	PlatformMacOS
	PlatformUnknown
	PlatformWindows
)

// GetBalance manages the entire balance check flow including airdrops
func GetBalance(r *http.Request, d *sql.DB, req rq.BalanceV1) (rp.BalanceV1, error) {
	ctx := r.Context()
	if req.DeviceID != "" {
		if err := handleDeviceID(r, d, req); err != nil {
			return rp.BalanceV1{}, err
		}
	}

	// Then handle the balance check
	if req.Email == "" {
		return getBalance(ctx, d, WithUserID(req.UserID))
	}
	res, err := handleAirdrop(ctx, d, req)
	if err != nil {
		return rp.BalanceV1{}, fmt.Errorf("failed to handle airdrop: %w", err)
	}
	balance, err := getBalance(ctx, d, WithID(res.ID))
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

func handleDeviceID(r *http.Request, d *sql.DB, req rq.BalanceV1) error {
	ctx := r.Context()
	userAgent := r.Header.Get("User-Agent")
	acceptLanguage := r.Header.Get("Accept-Language")
	device := UserDevice{
		DeviceUID: req.DeviceID,
		Version:   req.Version,
		Platform:  Platform(req.Platform),
		UserAgent: sql.NullString{
			String: userAgent,
			Valid:  userAgent != "",
		},
		AcceptLanguage: sql.NullString{
			String: acceptLanguage,
			Valid:  acceptLanguage != "",
		},
	}
	slog := slog.With(
		"device", device.DeviceUID,
		"version", device.Version,
		"platform", device.Platform,
		"userID", req.UserID,
		"email", req.Email,
	)

	// Try to get existing device
	err := device.Get(ctx, d)
	if err == sql.ErrNoRows {
		slog.Debug("new device")
		// New device, create it
		var user User
		user.UID = req.UserID
		if err := user.Get(ctx, d); err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		device.UserID = user.ID
		if err := device.Insert(ctx, d); err != nil {
			return fmt.Errorf("failed to insert device: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get device: %w", err)
	} else if device.Version != req.Version {
		slog.Debug("user updated device", "new_version", req.Version)
		// Version changed, create new device record
		var user User
		user.UID = req.UserID
		if err := user.Get(ctx, d); err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		device.UserID = user.ID
		if err := device.Insert(ctx, d); err != nil {
			return fmt.Errorf("failed to insert device: %w", err)
		}
	} else {
		if err := device.UpdateLastSignIn(ctx, d); err != nil {
			return fmt.Errorf("failed to update last sign in: %w", err)
		}
		slog.Debug("device online")
	}
	return nil
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

func getBalance(ctx context.Context, d *sql.DB, opts ...requestGetUserBalanceOption) (rp.BalanceV1, error) {
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
		err = d.QueryRowContext(ctx, mainQuery+" WHERE id = ?", req.ID).
			Scan(&q.Balance, &q.TotalAirdropped, &q.Dollars, &q.Images, &q.Searches)
	} else if req.UserID != nil {
		err = d.QueryRowContext(ctx, mainQuery+" WHERE uid = ?", req.UserID).
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
