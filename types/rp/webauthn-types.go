package rp

import "time"

// WebAuthnChallenge represents a challenge for WebAuthn registration/authentication
type WebAuthnChallenge struct {
	ChallengeID      int64    `json:"challengeId"`
	Challenge        string   `json:"challenge"`                  // Base64 encoded challenge bytes
	RPID             string   `json:"rpId"`                       // Relying Party ID
	RPName           string   `json:"rpName"`                     // Relying Party display name
	UserVerification string   `json:"userVerification"`           // "required", "preferred", "discouraged"
	Timeout          int      `json:"timeout"`                    // Timeout in milliseconds
	AllowCredentials []string `json:"allowCredentials,omitempty"` // Allowed credential IDs for authentication
	PRFSalt          string   `json:"prfSalt,omitempty"`          // PRF Salt for key derivation (base64 encoded)
}

// WebAuthnRegistrationResponse represents the response to a registration request
type WebAuthnRegistrationResponse struct {
	Success      bool   `json:"success"`
	CredentialID string `json:"credentialId"`
	Message      string `json:"message,omitempty"`
}

// WebAuthnAuthenticationResponse represents the response to an authentication request
type WebAuthnAuthenticationResponse struct {
	Success      bool   `json:"success"`
	SessionToken string `json:"sessionToken,omitempty"` // Optional session token for encryption operations
	Message      string `json:"message,omitempty"`
}

// Updated response types for the GetKeyResponse
type GetKeyResponse struct {
	KeyID      string    `json:"keyId"`
	PublicKey  string    `json:"encryptedKey"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt,omitempty"`
	IsActive   bool      `json:"isActive"`
	Version    int       `json:"version"`

	// New fields for passkey support
	CredentialID        string `json:"credentialId,omitempty"`
	CredentialRPID      string `json:"credentialRpId,omitempty"`
	KeyDerivationMethod string `json:"keyDerivationMethod,omitempty"`
}

// ListKeysResponse represents the response for listing encryption keys
type ListKeysResponse struct {
	Keys []Key `json:"keys"`
}

// Key represents an encryption key in our system
type Key struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"userId"`
	KeyID      string    `json:"keyId"`
	PublicKey  string    `json:"publicKey"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
	IsActive   bool      `json:"isActive"`
	KeyVersion int       `json:"keyVersion"`

	// Passkey-related fields
	CredentialID        string    `json:"credentialId"`
	CredentialRPID      string    `json:"credentialRpId"`
	CredentialCreatedAt time.Time `json:"credentialCreatedAt"`
	KeyDerivationMethod string    `json:"keyDerivationMethod"`
	PasskeyName         string    `json:"passkeyName"`

	// PRF extension fields
	PRFSalt    string `json:"prfSalt"`    // Base64-encoded salt for PRF
	PRFEnabled bool   `json:"prfEnabled"` // Whether PRF extension was used
	PRFResult  string `json:"prfResult"`  // Base64-encoded PRF evaluation result
}
