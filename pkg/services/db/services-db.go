package db

import (
	"context"
	"fmt"
)

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
func (s *Service) GetByName(ctx context.Context) error {
	err := D.QueryRowContext(ctx, `
		SELECT id, description, version, service_type, provider,
			base_cost_per_call, base_cost_per_million_tokens, base_cost_per_million_input_tokens, base_cost_per_million_output_tokens,
			base_cost_per_image, base_cost_per_search, max_input_tokens, max_output_tokens, max_total_tokens,
			base_cost_per_second, base_cost_per_gb_processed, base_cost_per_gb_stored,
			supports_batching, batch_size_limit, base_cost_per_batch,
			rate_limit_per_minute, rate_limit_per_day,
			profit_margin_percentage, minimum_profit_amount,
			peak_hours_multiplier, volume_discount_threshold, volume_discount_percentage,
			currency, is_active
		FROM services WHERE name = ?`, s.Name).Scan(
		&s.ID, &s.Description, &s.Version, &s.ServiceType, &s.Provider,
		&s.BaseCostPerCall, &s.BaseCostPerMillionTokens, &s.BaseCostPerMillionInputTokens, &s.BaseCostPerMillionOutputTokens,
		&s.BaseCostPerImage, &s.BaseCostPerSearch, &s.MaxInputTokens, &s.MaxOutputTokens, &s.MaxTotalTokens,
		&s.BaseCostPerSecond, &s.BaseCostPerGBProcessed, &s.BaseCostPerGBStored,
		&s.SupportsBatching, &s.BatchSizeLimit, &s.BaseCostPerBatch,
		&s.RateLimitPerMinute, &s.RateLimitPerDay,
		&s.ProfitMarginPercentage, &s.MinimumProfitAmount,
		&s.PeakHoursMultiplier, &s.VolumeDiscountThreshold, &s.VolumeDiscountPercentage,
		&s.Currency, &s.IsActive)
	if err != nil {
		return fmt.Errorf("failed to get service by name: %w", err)
	}
	return nil
}
