package webauthn

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// WebAuthnConfig holds the configuration for WebAuthn
type WebAuthnConfig struct {
	RPDisplayName string // Relying Party display name (e.g., "Ditto App")
	RPID          string // Relying Party ID (e.g., "ditto-app.example.com")
	RPOrigin      string // Relying Party origin (e.g., "https://ditto-app.example.com")
	Timeout       time.Duration
}

// Service is the WebAuthn service
type Service struct {
	DB       *sql.DB
	WebAuthn *webauthn.WebAuthn
}

// User represents a WebAuthn user
type User struct {
	ID          []byte
	Name        string
	DisplayName string
	Credentials []webauthn.Credential
}

// WebAuthnID implements the webauthn.User interface
func (u *User) WebAuthnID() []byte {
	return u.ID
}

// WebAuthnName implements the webauthn.User interface
func (u *User) WebAuthnName() string {
	return u.Name
}

// WebAuthnDisplayName implements the webauthn.User interface
func (u *User) WebAuthnDisplayName() string {
	return u.DisplayName
}

// WebAuthnCredentials implements the webauthn.User interface
func (u *User) WebAuthnCredentials() []webauthn.Credential {
	return u.Credentials
}

// NewService creates a new WebAuthn service
func NewService(ctx context.Context, db *sql.DB) (*Service, error) {
	// Create the configuration
	config := WebAuthnConfig{
		RPDisplayName: "Ditto App",
		RPID:          getDomain(),
		RPOrigin:      getOrigin(),
		Timeout:       60000, // 1 minute
	}

	// Create the WebAuthn instance
	w, err := webauthn.New(&webauthn.Config{
		RPDisplayName:         config.RPDisplayName,
		RPID:                  config.RPID,
		RPOrigins:             []string{config.RPOrigin},
		AttestationPreference: protocol.PreferDirectAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationPreferred,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login: webauthn.TimeoutConfig{
				Enforce: true,
				Timeout: config.Timeout,
			},
			Registration: webauthn.TimeoutConfig{
				Enforce: true,
				Timeout: config.Timeout,
			},
		},
		// PRF extension is enabled per registration/login, not in config
	})
	if err != nil {
		return nil, fmt.Errorf("error creating WebAuthn: %w", err)
	}

	return &Service{
		DB:       db,
		WebAuthn: w,
	}, nil
}

// SaveChallengeToDB saves a WebAuthn challenge to the database
func (s *Service) SaveChallengeToDB(ctx context.Context, userID int64, challenge string, rpID, sessionType string, extensions ...string) (int64, error) {
	var extensionsJSON string
	if len(extensions) > 0 && extensions[0] != "" {
		extensionsJSON = extensions[0]
	}

	// Save the challenge to the database
	result, err := s.DB.ExecContext(ctx,
		`INSERT INTO webauthn_challenges (user_id, challenge, rp_id, expires_at, type, extensions)
		 VALUES (?, ?, ?, datetime('now', '+5 minutes'), ?, ?)`,
		userID, challenge, rpID, sessionType, extensionsJSON)

	if err != nil {
		return 0, fmt.Errorf("error saving challenge to database: %w", err)
	}
	lastID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("error getting last insert ID: %w", err)
	}
	return lastID, nil
}

// GetChallengeFromDB retrieves a WebAuthn challenge from the database
func (s *Service) GetChallengeFromDB(ctx context.Context, challengeID int64) (string, string, error) {
	var challenge string
	var extensions sql.NullString
	err := s.DB.QueryRowContext(ctx,
		`SELECT challenge, extensions FROM webauthn_challenges 
		 WHERE id = ? AND expires_at > datetime('now')`,
		challengeID).Scan(&challenge, &extensions)

	if err != nil {
		return "", "", fmt.Errorf("error retrieving challenge: %w", err)
	}

	extensionsStr := ""
	if extensions.Valid {
		extensionsStr = extensions.String
	}

	return challenge, extensionsStr, nil
}

// DeleteChallengeFromDB deletes a WebAuthn challenge from the database
func (s *Service) DeleteChallengeFromDB(ctx context.Context, challengeID int64) error {
	_, err := s.DB.ExecContext(ctx,
		"DELETE FROM webauthn_challenges WHERE id = ?",
		challengeID)

	if err != nil {
		return fmt.Errorf("error deleting challenge: %w", err)
	}
	return nil
}

// CleanupExpiredChallenges removes expired challenges from the database
func (s *Service) CleanupExpiredChallenges(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx,
		"DELETE FROM webauthn_challenges WHERE expires_at <= datetime('now')")

	if err != nil {
		return fmt.Errorf("error cleaning up expired challenges: %w", err)
	}

	return nil
}

// GetUserByID retrieves a user by ID and their credentials
func (s *Service) GetUserByID(ctx context.Context, userID int64) (*User, error) {
	// Get user details
	var uid string
	var email string
	err := s.DB.QueryRowContext(ctx,
		"SELECT uid, email FROM users WHERE id = ?",
		userID).Scan(&uid, &email)

	if err != nil {
		return nil, fmt.Errorf("error retrieving user: %w", err)
	}

	// Get user credentials
	rows, err := s.DB.QueryContext(ctx,
		`SELECT credential_id, public_key FROM encryption_keys 
		 WHERE user_id = ? AND credential_id IS NOT NULL`,
		userID)

	if err != nil {
		return nil, fmt.Errorf("error retrieving user credentials: %w", err)
	}
	defer rows.Close()

	var credentials []webauthn.Credential
	for rows.Next() {
		var credentialID, encryptedKey string
		if err := rows.Scan(&credentialID, &encryptedKey); err != nil {
			return nil, fmt.Errorf("error scanning credential row: %w", err)
		}

		// Parse credential ID from base64
		credID, err := base64.StdEncoding.DecodeString(credentialID)
		if err != nil {
			slog.Warn("Error decoding credential ID, skipping", "error", err)
			continue
		}

		// Decode the public key from base64
		pubKey, err := base64.StdEncoding.DecodeString(encryptedKey)
		if err != nil {
			slog.Warn("Error decoding public key, skipping", "error", err)
			continue
		}

		// Create WebAuthn credential
		cred := webauthn.Credential{
			ID:              credID,
			PublicKey:       pubKey,
			AttestationType: "direct",
			Transport:       []protocol.AuthenticatorTransport{protocol.Internal},
		}

		credentials = append(credentials, cred)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating credential rows: %w", err)
	}

	// Create user
	user := &User{
		ID:          []byte(uid),
		Name:        email,
		DisplayName: email,
		Credentials: credentials,
	}

	return user, nil
}

// GeneratePRFSalt creates a random salt for PRF extension
func (s *Service) GeneratePRFSalt() (string, []byte, error) {
	// Generate a random 32-byte salt
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return "", nil, fmt.Errorf("error generating PRF salt: %w", err)
	}

	// Encode for storage
	saltBase64 := base64.StdEncoding.EncodeToString(salt)

	return saltBase64, salt, nil
}

// StorePRFResult saves the PRF result for a key
func (s *Service) StorePRFResult(ctx context.Context, keyID string, prfResult []byte) error {
	prfResultBase64 := base64.StdEncoding.EncodeToString(prfResult)

	_, err := s.DB.ExecContext(ctx,
		`UPDATE encryption_keys 
		 SET prf_result = ?, prf_enabled = TRUE
		 WHERE key_id = ?`,
		prfResultBase64, keyID)

	if err != nil {
		return fmt.Errorf("error storing PRF result: %w", err)
	}

	return nil
}

// GetPRFSaltForKey retrieves the PRF salt for a key
func (s *Service) GetPRFSaltForKey(ctx context.Context, keyID string) ([]byte, error) {
	var saltBase64 string
	err := s.DB.QueryRowContext(ctx,
		`SELECT prf_salt FROM encryption_keys 
		 WHERE key_id = ? AND prf_enabled = TRUE`,
		keyID).Scan(&saltBase64)

	if err != nil {
		return nil, fmt.Errorf("error retrieving PRF salt: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(saltBase64)
	if err != nil {
		return nil, fmt.Errorf("error decoding PRF salt: %w", err)
	}

	return salt, nil
}

// Helper functions to get domain and origin based on environment
func getDomain() string {
	// Use environment variable if available
	if envs.WEBAUTHN_RPID != "" {
		return envs.WEBAUTHN_RPID
	}

	// Default based on environment
	switch envs.DITTO_ENV {
	case envs.EnvProd:
		return "assistant.heyditto.ai"
	case envs.EnvStaging:
		return "staging.assistant.heyditto.ai"
	default:
		return "localhost"
	}
}

func getOrigin() string {
	// Use environment variable if available
	if envs.WEBAUTHN_ORIGIN != "" {
		return envs.WEBAUTHN_ORIGIN
	}

	// Default based on environment
	switch envs.DITTO_ENV {
	case envs.EnvProd:
		return "https://assistant.heyditto.ai"
	case envs.EnvStaging:
		return "https://staging.assistant.heyditto.ai"
	default:
		return "http://localhost:3000"
	}
}
