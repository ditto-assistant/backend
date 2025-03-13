package rq

import (
	"github.com/go-webauthn/webauthn/protocol"
)

// GetKeyRequest represents a request to retrieve an encryption key
type GetKeyRequest struct {
	KeyID string `json:"keyId"`
}

// DeactivateKeyRequest represents a request to deactivate an encryption key
type DeactivateKeyRequest struct {
	KeyID string `json:"keyId"`
}

// WebAuthn Registration Challenge Request
type WebAuthnRegistrationChallengeRequest struct {
	UserDisplayName string `json:"userDisplayName,omitempty"` // Optional display name
}

// WebAuthn Registration Request
type WebAuthnRegistrationRequest struct {
	ChallengeID int64  `json:"challengeId"`
	PasskeyName string `json:"passkeyName"`
	protocol.CredentialCreationResponse
}

// WebAuthn Authentication Challenge Request
type WebAuthnAuthenticationChallengeRequest struct {
	CredentialID string `json:"credentialId,omitempty"` // Optional specific credential ID
}

// WebAuthn Authentication Request
type WebAuthnAuthenticationRequest struct {
	ChallengeID int64 `json:"challengeId"`
	protocol.CredentialAssertionResponse
}

// Updated RegisterKeyRequest for WebAuthn support
type RegisterKeyRequest struct {
	// Original fields
	KeyID     string `json:"keyId"`
	PublicKey string `json:"encryptedKey"`

	// New fields for passkey support
	CredentialID        string `json:"credentialId,omitempty"`
	CredentialPublicKey string `json:"credentialPublicKey,omitempty"`
	CredentialRPID      string `json:"credentialRpId,omitempty"`
	KeyDerivationMethod string `json:"keyDerivationMethod,omitempty"` // "direct" or "passkey-derived"
}

// Updated RotateKeyRequest for WebAuthn support
type RotateKeyRequest struct {
	// Original fields
	KeyID        string `json:"keyId"`
	NewPublicKey string `json:"newPublicKey"`

	// New fields for passkey support
	NewCredentialID        string `json:"newCredentialId,omitempty"`
	NewCredentialPublicKey string `json:"newCredentialPublicKey,omitempty"`
	NewCredentialRPID      string `json:"newCredentialRpId,omitempty"`
	KeyDerivationMethod    string `json:"keyDerivationMethod,omitempty"`
}
