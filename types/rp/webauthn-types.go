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
	KeyID        string    `json:"keyId"`
	EncryptedKey string    `json:"encryptedKey"`
	CreatedAt    time.Time `json:"createdAt"`
	LastUsedAt   time.Time `json:"lastUsedAt,omitempty"`
	IsActive     bool      `json:"isActive"`
	Version      int       `json:"version"`

	// New fields for passkey support
	CredentialID        string `json:"credentialId,omitempty"`
	CredentialRPID      string `json:"credentialRpId,omitempty"`
	KeyDerivationMethod string `json:"keyDerivationMethod,omitempty"`
}

// Updated KeyListItem for WebAuthn metadata
type KeyListItem struct {
	KeyID      string    `json:"keyId"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
	IsActive   bool      `json:"isActive"`
	Version    int       `json:"version"`

	// New fields for passkey support
	CredentialID        string `json:"credentialId,omitempty"`
	KeyDerivationMethod string `json:"keyDerivationMethod,omitempty"`
	PasskeyName         string `json:"passkeyName,omitempty"`
}

// ListKeysResponse represents the response for listing encryption keys
type ListKeysResponse struct {
	Keys []KeyListItem `json:"keys"`
}
