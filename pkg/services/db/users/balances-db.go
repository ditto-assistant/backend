package users

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ditto-assistant/backend/pkg/utils/numfmt"
	"github.com/ditto-assistant/backend/types/rp"
	"github.com/ditto-assistant/backend/types/rq"
	"github.com/omniaura/mapcache"
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

var cacheBalance, _ = mapcache.New[rq.BalanceV1, rp.BalanceV1](mapcache.WithTTL(10 * time.Second))

// GetBalance manages the entire balance check flow including airdrops
func GetBalance(r *http.Request, d *sql.DB, req rq.BalanceV1) (rp.BalanceV1, error) {
	return cacheBalance.Get(req, func() (rp.BalanceV1, error) {
		ctx := r.Context()
		res, err := handleAirdrop(ctx, d, req)
		if err != nil {
			return rp.BalanceV1{}, fmt.Errorf("failed to handle airdrop: %w", err)
		}
		if err := handleDeviceID(r, d, req, res.ID); err != nil {
			return rp.BalanceV1{}, err
		}
		balance, err := getBalance(ctx, d, res)
		if err != nil {
			return rp.BalanceV1{}, fmt.Errorf("failed to get user balance: %w", err)
		}
		return balance, nil
	})
}

func handleDeviceID(r *http.Request, d *sql.DB, req rq.BalanceV1, id int64) error {
	if req.DeviceID == "" {
		return nil
	}
	ctx := r.Context()
	userAgent := r.Header.Get("User-Agent")
	acceptLanguage := r.Header.Get("Accept-Language")
	device := UserDevice{
		UserID:    id,
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
	exists, err := device.Exists(ctx, d)
	if err != nil {
		return fmt.Errorf("failed to check if device exists: %w", err)
	}
	if exists {
		slog.Debug("device online")
		return device.UpdateLastSignIn(ctx, d)
	}
	if device.Version != req.Version &&
		device.DeviceUID == req.DeviceID {
		slog.Debug("user updated device", "new_version", req.Version)
	} else {
		slog.Debug("new device")
	}
	if err := device.Insert(ctx, d); err != nil {
		return fmt.Errorf("failed to insert device: %w", err)
	}
	return nil
}

func getBalance(ctx context.Context, d *sql.DB, airdrop *resultAirdrop) (rp.BalanceV1, error) {
	var q struct {
		Balance, TotalAirdropped  int64
		Images, Searches, Dollars float64
		LastAirdropAt             sql.NullTime
	}
	err := d.QueryRowContext(ctx, `
		SELECT users.balance,
			   users.total_tokens_airdropped,
			   users.last_airdrop_at,
			   CAST(users.balance AS FLOAT) / (SELECT CAST(count AS FLOAT) FROM tokens_per_unit WHERE name = 'dollar'),
			   (users.balance / (SELECT count FROM tokens_per_unit WHERE name = 'image')),
			   (users.balance / (SELECT count FROM tokens_per_unit WHERE name = 'search'))
		FROM users WHERE id = ?`, airdrop.ID).
		Scan(&q.Balance, &q.TotalAirdropped, &q.LastAirdropAt, &q.Dollars, &q.Images, &q.Searches)
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
	if airdrop.DropAmount > 0 {
		slog.Info("airdropped tokens", "uid", airdrop.ID, "tokens", airdrop.DropAmount)
		rsp.DropAmountRaw = airdrop.DropAmount
		rsp.DropAmount = numfmt.LargeNumber(airdrop.DropAmount)
	}
	if q.LastAirdropAt.Valid {
		rsp.LastAirdropAt = &q.LastAirdropAt.Time
	}
	return rsp, nil
}
