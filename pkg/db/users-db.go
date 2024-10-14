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
	ID      int64
	UID     string
	Name    string
	Balance int64
}

// Insert inserts a new user into the database.
// It updates the User's ID with the ID from the database.
func (u *User) Insert(ctx context.Context) error {
	res, err := D.ExecContext(ctx, "INSERT INTO users (uid, name, balance) VALUES (?, ?, ?)", u.UID, u.Name, u.Balance)
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

// GetOrCreateUser gets a user by their UID.
// If the user does not exist, it creates a new user.
func GetOrCreateUser(ctx context.Context, uid string) (User, error) {
	var u User
	err := D.QueryRowContext(ctx, "SELECT id, uid, name, balance FROM users WHERE uid = ?", uid).Scan(&u.ID, &u.UID, &u.Name, &u.Balance)
	if err == sql.ErrNoRows {
		u = User{UID: uid}
		err = u.Insert(ctx)
	}
	return u, err
}

type Service struct {
	ID                             int64
	Name                           string
	Description                    string
	Version                        string
	ServiceType                    string
	Provider                       string
	BaseCostPerCall                float64
	BaseCostPerMillionTokens       float64
	BaseCostPerMillionInputTokens  float64
	BaseCostPerMillionOutputTokens float64
	BaseCostPerImage               float64
	BaseCostPerSearch              float64
	MaxInputTokens                 int64
	MaxOutputTokens                int64
	MaxTotalTokens                 int64
	BaseCostPerSecond              float64
	BaseCostPerGBProcessed         float64
	BaseCostPerGBStored            float64
	SupportsBatching               bool
	BatchSizeLimit                 int64
	BaseCostPerBatch               float64
	RateLimitPerMinute             int64
	RateLimitPerDay                int64
	ProfitMarginPercentage         float64
	MinimumProfitAmount            float64
	PeakHoursMultiplier            float64
	VolumeDiscountThreshold        int64
	VolumeDiscountPercentage       float64
	Currency                       string
	IsActive                       bool
}

// GetServiceByName retrieves a service from the database by its name.
func GetServiceByName(ctx context.Context, name string) (*Service, error) {
	var s Service
	err := D.QueryRowContext(ctx, `
		SELECT id, name, description, version, service_type, provider,
			base_cost_per_call, base_cost_per_million_tokens, base_cost_per_million_input_tokens, base_cost_per_million_output_tokens,
			base_cost_per_image, base_cost_per_search, max_input_tokens, max_output_tokens, max_total_tokens,
			base_cost_per_second, base_cost_per_gb_processed, base_cost_per_gb_stored,
			supports_batching, batch_size_limit, base_cost_per_batch,
			rate_limit_per_minute, rate_limit_per_day,
			profit_margin_percentage, minimum_profit_amount,
			peak_hours_multiplier, volume_discount_threshold, volume_discount_percentage,
			currency, is_active
		FROM services WHERE name = ?`, name).Scan(
		&s.ID, &s.Name, &s.Description, &s.Version, &s.ServiceType, &s.Provider,
		&s.BaseCostPerCall, &s.BaseCostPerMillionTokens, &s.BaseCostPerMillionInputTokens, &s.BaseCostPerMillionOutputTokens,
		&s.BaseCostPerImage, &s.BaseCostPerSearch, &s.MaxInputTokens, &s.MaxOutputTokens, &s.MaxTotalTokens,
		&s.BaseCostPerSecond, &s.BaseCostPerGBProcessed, &s.BaseCostPerGBStored,
		&s.SupportsBatching, &s.BatchSizeLimit, &s.BaseCostPerBatch,
		&s.RateLimitPerMinute, &s.RateLimitPerDay,
		&s.ProfitMarginPercentage, &s.MinimumProfitAmount,
		&s.PeakHoursMultiplier, &s.VolumeDiscountThreshold, &s.VolumeDiscountPercentage,
		&s.Currency, &s.IsActive)
	if err != nil {
		return nil, fmt.Errorf("failed to get service by name: %w", err)
	}
	return &s, nil
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
