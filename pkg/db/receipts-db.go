package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ditto-assistant/backend/pkg/llm"
)

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
