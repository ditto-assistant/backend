package db_test

import (
	"context"
	"sync"
	"testing"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserBalanceUpdateTrigger(t *testing.T) {
	// Set up the test environment
	var shutdown sync.WaitGroup
	defer shutdown.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	envs.DITTO_ENV = envs.EnvLocal
	envs.Load()
	err := db.Setup(ctx, &shutdown)
	require.NoError(t, err, "Failed to set up database")

	// Create a new user
	user, err := db.GetOrCreateUser(ctx, "test@example.com")
	require.NoError(t, err, "Failed to create user")

	// Set the user's balance to 10,000
	_, err = db.D.ExecContext(ctx, "UPDATE users SET balance = ? WHERE id = ?", 10000, user.ID)
	require.NoError(t, err, "Failed to set user balance")

	// Insert a receipt
	_, err = db.D.ExecContext(ctx, `
		INSERT INTO receipts (user_id, ditto_tokens, usage_type)
		VALUES (?, ?, ?)
	`, user.ID, 500, "test")
	require.NoError(t, err, "Failed to insert receipt")

	// Verify the trigger fired and updated the user's balance
	var newBalance int
	err = db.D.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = ?", user.ID).Scan(&newBalance)
	require.NoError(t, err, "Failed to fetch updated user balance")

	assert.Equal(t, 9500, newBalance, "User balance was not updated correctly")
}