package encryption

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ditto-assistant/backend/pkg/services/db/users"
)

// Key represents an encryption key in our system
type Key struct {
	ID           int64
	UserID       int64
	KeyID        string
	EncryptedKey string
	CreatedAt    time.Time
	LastUsedAt   time.Time
	IsActive     bool
	KeyVersion   int

	// Passkey-related fields
	CredentialID        string
	CredentialRPID      string
	CredentialCreatedAt time.Time
	KeyDerivationMethod string
	PasskeyName         string
}

// Service provides encryption key management functionality
type Service struct {
	DB *sql.DB
}

// NewService creates a new encryption service
func NewService(db *sql.DB) *Service {
	return &Service{
		DB: db,
	}
}

// RegisterKey stores a new encryption key for a user
// This version supports both regular keys and passkey-derived keys
func (s *Service) RegisterKey(
	ctx context.Context,
	userUID,
	keyID,
	encryptedKey string,
	credentialID string,
	credentialRPID string,
	keyDerivationMethod string,
	passkeyName string,
) error {
	// Get user
	user := users.User{UID: userUID}
	if err := user.GetByUID(ctx, s.DB); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("user not found: %w", err)
		}
		return fmt.Errorf("error getting user: %w", err)
	}

	// Begin transaction
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if key ID already exists for this user
	var count int
	err = tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM encryption_keys WHERE user_id = ? AND key_id = ?",
		user.ID, keyID).Scan(&count)
	if err != nil {
		return fmt.Errorf("error checking existing key: %w", err)
	}

	if count > 0 {
		// Update existing key
		_, err = tx.ExecContext(ctx,
			`UPDATE encryption_keys SET 
			encrypted_key = ?, 
			last_used_at = CURRENT_TIMESTAMP, 
			is_active = TRUE,
			credential_id = ?,
			credential_rp_id = ?,
			credential_created_at = CURRENT_TIMESTAMP,
			key_derivation_method = ?
			WHERE user_id = ? AND key_id = ?`,
			encryptedKey, credentialID, credentialRPID,
			keyDerivationMethod, user.ID, keyID)
		if err != nil {
			return fmt.Errorf("error updating key: %w", err)
		}
	} else {
		// Insert new key
		_, err = tx.ExecContext(ctx,
			`INSERT INTO encryption_keys (
				user_id, 
				key_id, 
				encrypted_key, 
				credential_id, 
				credential_rp_id, 
				credential_created_at,
				key_derivation_method) 
			VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)`,
			user.ID, keyID, encryptedKey, credentialID, credentialRPID, keyDerivationMethod)
		if err != nil {
			return fmt.Errorf("error inserting key: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	slog.Debug("registered encryption key",
		"user_id", user.ID,
		"key_id", keyID,
		"credential_id", credentialID,
		"key_derivation_method", keyDerivationMethod)
	return nil
}

// GetKey retrieves an encryption key for a user
func (s *Service) GetKey(ctx context.Context, userID int64, keyID string) (Key, error) {
	var key Key

	// Initialize null-able fields with pointers
	var credentialID, credentialRPID, keyDerivationMethod sql.NullString
	var credentialCreatedAt sql.NullTime

	// Get key
	err := s.DB.QueryRowContext(ctx,
		`SELECT 
            id, key_id, encrypted_key, created_at, last_used_at, is_active, key_version,
            credential_id,  credential_rp_id, credential_created_at,
            key_derivation_method
         FROM encryption_keys 
         WHERE user_id = ? AND key_id = ? AND is_active = TRUE`,
		userID, keyID).Scan(
		&key.ID,
		&key.KeyID,
		&key.EncryptedKey,
		&key.CreatedAt,
		&key.LastUsedAt,
		&key.IsActive,
		&key.KeyVersion,
		&credentialID,
		&credentialRPID,
		&credentialCreatedAt,
		&keyDerivationMethod,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Key{}, fmt.Errorf("key not found: %w", err)
		}
		return Key{}, fmt.Errorf("error getting key: %w", err)
	}

	// Set key.UserID
	key.UserID = userID

	// Assign nullable values
	if credentialID.Valid {
		key.CredentialID = credentialID.String
	}
	if credentialRPID.Valid {
		key.CredentialRPID = credentialRPID.String
	}
	if credentialCreatedAt.Valid {
		key.CredentialCreatedAt = credentialCreatedAt.Time
	}
	if keyDerivationMethod.Valid {
		key.KeyDerivationMethod = keyDerivationMethod.String
	} else {
		key.KeyDerivationMethod = "direct" // Default for backward compatibility
	}

	// Update last used timestamp
	if err := s.UpdateKeyLastUsed(ctx, userID, keyID); err != nil {
		slog.Error("error updating last used timestamp", "error", err)
		// Continue anyway, this is not critical
	}

	return key, nil
}

// UpdateKeyLastUsed updates the last used timestamp for a key
func (s *Service) UpdateKeyLastUsed(ctx context.Context, userID int64, keyID string) error {
	_, err := s.DB.ExecContext(ctx,
		"UPDATE encryption_keys SET last_used_at = CURRENT_TIMESTAMP WHERE user_id = ? AND key_id = ?",
		userID, keyID)
	if err != nil {
		return fmt.Errorf("error updating last used timestamp: %w", err)
	}
	return nil
}

// RotateKey creates a new version of an encryption key
// Also supports passkey-derived keys
func (s *Service) RotateKey(
	ctx context.Context,
	userUID,
	keyID,
	newEncryptedKey string,
	newCredentialID string,
	newCredentialRPID string,
	keyDerivationMethod string,
) error {
	// Get user
	user := users.User{UID: userUID}
	if err := user.GetByUID(ctx, s.DB); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("user not found: %w", err)
		}
		return fmt.Errorf("error getting user: %w", err)
	}

	// Begin transaction
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Get current key version
	var currentVersion int
	err = tx.QueryRowContext(ctx,
		"SELECT key_version FROM encryption_keys WHERE user_id = ? AND key_id = ? AND is_active = TRUE",
		user.ID, keyID).Scan(&currentVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("key not found: %w", err)
		}
		return fmt.Errorf("error getting current key version: %w", err)
	}

	// Deactivate old key
	_, err = tx.ExecContext(ctx,
		"UPDATE encryption_keys SET is_active = FALSE WHERE user_id = ? AND key_id = ?",
		user.ID, keyID)
	if err != nil {
		return fmt.Errorf("error deactivating old key: %w", err)
	}

	// Set derivation method if not provided (backward compatibility)
	if keyDerivationMethod == "" {
		keyDerivationMethod = "direct"
	}

	// Insert new key version
	_, err = tx.ExecContext(ctx,
		`INSERT INTO encryption_keys (
            user_id, 
            key_id, 
            encrypted_key, 
            key_version,
            credential_id, 
            credential_rp_id, 
            credential_created_at,
            key_derivation_method
        ) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)`,
		user.ID, keyID, newEncryptedKey, currentVersion+1,
		newCredentialID, newCredentialRPID, keyDerivationMethod)
	if err != nil {
		return fmt.Errorf("error inserting new key version: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	slog.Debug("rotated encryption key",
		"user_id", user.ID,
		"key_id", keyID,
		"new_version", currentVersion+1,
		"credential_id", newCredentialID,
		"key_derivation_method", keyDerivationMethod)
	return nil
}

// ListKeys returns all active encryption keys for a user
func (s *Service) ListKeys(ctx context.Context, userUID string) ([]Key, error) {
	// Get user
	user := users.User{UID: userUID}
	if err := user.GetByUID(ctx, s.DB); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		return nil, fmt.Errorf("error getting user: %w", err)
	}

	// Query for keys
	rows, err := s.DB.QueryContext(ctx,
		`SELECT 
            id, key_id, encrypted_key, created_at, last_used_at, is_active, key_version,
            credential_id, credential_rp_id, credential_created_at,
            key_derivation_method
        FROM encryption_keys 
        WHERE user_id = ? 
        ORDER BY created_at DESC`,
		user.ID)
	if err != nil {
		return nil, fmt.Errorf("error querying keys: %w", err)
	}
	defer rows.Close()

	var keys []Key
	for rows.Next() {
		var key Key
		key.UserID = user.ID

		// Initialize null-able fields with pointers
		var credentialID, credentialRPID, keyDerivationMethod sql.NullString
		var credentialCreatedAt sql.NullTime

		if err := rows.Scan(
			&key.ID,
			&key.KeyID,
			&key.EncryptedKey,
			&key.CreatedAt,
			&key.LastUsedAt,
			&key.IsActive,
			&key.KeyVersion,
			&credentialID,
			&credentialRPID,
			&credentialCreatedAt,
			&keyDerivationMethod,
		); err != nil {
			return nil, fmt.Errorf("error scanning key row: %w", err)
		}

		// Assign nullable values
		if credentialID.Valid {
			key.CredentialID = credentialID.String
		}
		if credentialRPID.Valid {
			key.CredentialRPID = credentialRPID.String
		}
		if credentialCreatedAt.Valid {
			key.CredentialCreatedAt = credentialCreatedAt.Time
		}
		if keyDerivationMethod.Valid {
			key.KeyDerivationMethod = keyDerivationMethod.String
		} else {
			key.KeyDerivationMethod = "direct" // Default for backward compatibility
		}

		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating key rows: %w", err)
	}

	return keys, nil
}

// DeactivateKey marks a key as inactive
func (s *Service) DeactivateKey(ctx context.Context, userUID, keyID string) error {
	// Get user
	user := users.User{UID: userUID}
	if err := user.GetByUID(ctx, s.DB); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("user not found: %w", err)
		}
		return nil
	}

	// Deactivate key
	result, err := s.DB.ExecContext(ctx,
		"UPDATE encryption_keys SET is_active = FALSE WHERE user_id = ? AND key_id = ?",
		user.ID, keyID)
	if err != nil {
		return fmt.Errorf("error deactivating key: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("key not found")
	}

	slog.Debug("deactivated encryption key", "user_id", user.ID, "key_id", keyID)
	return nil
}
