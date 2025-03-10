package passkeys

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/ditto-assistant/backend/pkg/middleware"
	"github.com/ditto-assistant/backend/pkg/services/db/users"
	"github.com/ditto-assistant/backend/pkg/services/encryption"
	"github.com/ditto-assistant/backend/pkg/services/webauthn"
	"github.com/ditto-assistant/backend/types/rp"
	"github.com/ditto-assistant/backend/types/rq"
	"github.com/go-webauthn/webauthn/protocol"
	wauthn "github.com/go-webauthn/webauthn/webauthn"
)

// SetupWebAuthnRoutes sets up the WebAuthn API routes
func (cl *Service) Routes(router *http.ServeMux) {
	passkeysRouter := http.NewServeMux()
	passkeysRouter.HandleFunc("POST /action/registration/challenge", cl.GenerateRegistrationChallenge)
	passkeysRouter.HandleFunc("POST /action/register", cl.RegisterPasskey)
	passkeysRouter.HandleFunc("POST /action/authentication/challenge", cl.GenerateAuthenticationChallenge)
	passkeysRouter.HandleFunc("POST /action/authenticate", cl.AuthenticatePasskey)
	passkeysRouter.HandleFunc("GET /", cl.ListPasskeys)
	handler := http.StripPrefix("/api/v2/passkeys", passkeysRouter)
	router.Handle("/api/v2/passkeys/", handler)
}

// Service contains handlers for WebAuthn registration and authentication
type Service struct {
	WebAuthn   *webauthn.Service
	Encryption *encryption.Client
}

// NewService creates a new instance of WebAuthnHandlers
func NewService(webAuthnService *webauthn.Service, encryptionService *encryption.Client) *Service {
	return &Service{
		WebAuthn:   webAuthnService,
		Encryption: encryptionService,
	}
}

// MARK: - GenerateRegistrationChallenge

func (h *Service) GenerateRegistrationChallenge(w http.ResponseWriter, r *http.Request) {
	uid := middleware.GetUserID(r.Context())
	user := users.User{UID: uid}
	if err := user.GetByUID(r.Context(), h.WebAuthn.DB); err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving user: %v", err), http.StatusInternalServerError)
		return
	}
	var req rq.WebAuthnRegistrationChallengeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		eStr := fmt.Sprintf("Invalid request format: %v", err)
		slog.Error(eStr)
		http.Error(w, eStr, http.StatusBadRequest)
		return
	}
	displayName := req.UserDisplayName
	userName := user.Email.String
	if displayName == "" {
		displayName = userName
	}

	// Create WebAuthn user
	webAuthnUser := &webauthn.User{
		ID:          []byte(uid),
		Name:        userName,
		DisplayName: displayName,
	}

	// Generate registration options
	options, sessionData, err := h.WebAuthn.WebAuthn.BeginRegistration(webAuthnUser)
	if err != nil {
		slog.Error("Error generating registration options", "error", err)
		http.Error(w, "Failed to generate registration challenge", http.StatusInternalServerError)
		return
	}

	// Store session data in database
	challengeID, err := h.WebAuthn.SaveChallengeToDB(
		r.Context(),
		user.ID,
		sessionData.Challenge,
		h.WebAuthn.WebAuthn.Config.RPID,
		"registration",
	)
	if err != nil {
		slog.Error("Error saving challenge to database", "error", err)
		http.Error(w, "Failed to save registration challenge", http.StatusInternalServerError)
		return
	}

	// Prepare response
	response := rp.WebAuthnChallenge{
		ChallengeID:      challengeID,
		Challenge:        sessionData.Challenge,
		RPID:             h.WebAuthn.WebAuthn.Config.RPID,
		RPName:           h.WebAuthn.WebAuthn.Config.RPDisplayName,
		UserVerification: string(options.Response.AuthenticatorSelection.UserVerification),
		Timeout:          options.Response.Timeout,
	}

	rp.RespondWithJSON(w, http.StatusOK, response)
}

// MARK: - RegisterPasskey

func (h *Service) RegisterPasskey(w http.ResponseWriter, r *http.Request) {
	uid := middleware.GetUserID(r.Context())
	user := users.User{UID: uid}
	if err := user.GetByUID(r.Context(), h.WebAuthn.DB); err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving user: %v", err), http.StatusInternalServerError)
		return
	}
	query := r.URL.Query()
	challengeID, err := strconv.ParseInt(query.Get("challengeID"), 10, 64)
	if err != nil {
		slog.Error("Error converting challengeID to int", "error", err)
		http.Error(w, "Invalid challengeID", http.StatusBadRequest)
		return
	}
	passkeyName := query.Get("passkeyName")
	challenge, err := h.WebAuthn.GetChallengeFromDB(r.Context(), challengeID)
	if err != nil {
		slog.Error("Error retrieving challenge", "error", err)
		http.Error(w, "Invalid or expired challenge", http.StatusBadRequest)
		return
	}

	webAuthnUser := &webauthn.User{
		ID:          []byte(uid),
		Name:        user.Email.String,
		DisplayName: user.Email.String,
	}

	// Create session data
	sessionData := wauthn.SessionData{
		Challenge:        challenge,
		UserID:           []byte(uid),
		UserVerification: protocol.VerificationPreferred,
		RelyingPartyID:   h.WebAuthn.WebAuthn.Config.RPID,
	}

	// Verify registration
	credential, err := h.WebAuthn.WebAuthn.FinishRegistration(webAuthnUser, sessionData, r)
	if err != nil {
		slog.Error("Error verifying registration", "error", err)
		http.Error(w, fmt.Sprintf("Registration verification failed: %v", err), http.StatusBadRequest)
		return
	}

	// Registration successful - now register the encryption key
	keyID := fmt.Sprintf("passkey-%s", base64.StdEncoding.EncodeToString(credential.ID))

	// Store the credential data in a format that can be loaded back for WebAuthn
	credentialID := base64.StdEncoding.EncodeToString(credential.ID)
	credentialPublicKey := base64.StdEncoding.EncodeToString(credential.PublicKey)

	// Register the key with encryption service
	err = h.Encryption.RegisterKey(
		r.Context(),
		uid,
		keyID,
		credentialPublicKey, // This is used as the encryption key
		credentialID,        // Store the credential ID
		h.WebAuthn.WebAuthn.Config.RPID,
		"passkey-derived", // Key derivation method
		passkeyName,       // User-friendly name
	)

	if err != nil {
		slog.Error("Error registering encryption key", "error", err)
		http.Error(w, "Failed to register encryption key", http.StatusInternalServerError)
		return
	}

	// Clean up the challenge
	if err := h.WebAuthn.DeleteChallengeFromDB(r.Context(), challengeID); err != nil {
		slog.Warn("Failed to delete challenge from database", "error", err)
		// Continue anyway - this is just cleanup
	}

	// Return success
	resp := rp.WebAuthnRegistrationResponse{
		Success:      true,
		CredentialID: base64.StdEncoding.EncodeToString(credential.ID),
		Message:      "Passkey registered successfully",
	}
	rp.RespondWithJSON(w, http.StatusOK, resp)
}

// MARK: - GenerateAuthenticationChallenge

func (h *Service) GenerateAuthenticationChallenge(w http.ResponseWriter, r *http.Request) {
	uid := middleware.GetUserID(r.Context())
	user := users.User{UID: uid}
	if err := user.GetByUID(r.Context(), h.WebAuthn.DB); err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving user: %v", err), http.StatusInternalServerError)
		return
	}
	var req rq.WebAuthnAuthenticationChallengeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		eStr := fmt.Sprintf("Invalid request format: %v", err)
		slog.Error(eStr)
		http.Error(w, eStr, http.StatusBadRequest)
		return
	}

	// Get WebAuthn user with credentials
	webAuthnUser, err := h.WebAuthn.GetUserByID(r.Context(), user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving user credentials: %v", err), http.StatusInternalServerError)
		return
	}

	// Generate authentication options
	options, sessionData, err := h.WebAuthn.WebAuthn.BeginLogin(webAuthnUser)
	if err != nil {
		slog.Error("Error generating authentication options", "error", err)
		http.Error(w, "Failed to generate authentication challenge", http.StatusInternalServerError)
		return
	}

	// Store session data in database
	challengeID, err := h.WebAuthn.SaveChallengeToDB(
		r.Context(),
		user.ID,
		sessionData.Challenge,
		h.WebAuthn.WebAuthn.Config.RPID,
		"authentication",
	)
	if err != nil {
		slog.Error("Error saving challenge to database", "error", err)
		http.Error(w, "Failed to save authentication challenge", http.StatusInternalServerError)
		return
	}
	// Prepare credential list for the response
	var allowCredentials []string
	for _, cred := range options.Response.AllowedCredentials {
		allowCredentials = append(allowCredentials, base64.StdEncoding.EncodeToString(cred.CredentialID))
	}

	// Prepare response
	response := rp.WebAuthnChallenge{
		ChallengeID:      challengeID,
		Challenge:        sessionData.Challenge,
		RPID:             h.WebAuthn.WebAuthn.Config.RPID,
		RPName:           h.WebAuthn.WebAuthn.Config.RPDisplayName,
		UserVerification: string(options.Response.UserVerification),
		Timeout:          options.Response.Timeout,
		AllowCredentials: allowCredentials,
	}

	rp.RespondWithJSON(w, http.StatusOK, response)
}

// MARK: - AuthenticatePasskey

func (h *Service) AuthenticatePasskey(w http.ResponseWriter, r *http.Request) {
	uid := middleware.GetUserID(r.Context())
	user := users.User{UID: uid}
	if err := user.GetByUID(r.Context(), h.WebAuthn.DB); err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving user: %v", err), http.StatusInternalServerError)
		return
	}
	query := r.URL.Query()
	challengeID, err := strconv.ParseInt(query.Get("challengeID"), 10, 64)
	if err != nil {
		slog.Error("Error converting challengeID to int", "error", err)
		http.Error(w, "Invalid challengeID", http.StatusBadRequest)
		return
	}

	challenge, err := h.WebAuthn.GetChallengeFromDB(r.Context(), challengeID)
	if err != nil {
		slog.Error("Error retrieving challenge", "error", err)
		http.Error(w, "Invalid or expired challenge", http.StatusBadRequest)
		return
	}

	// Get WebAuthn user with credentials
	webAuthnUser, err := h.WebAuthn.GetUserByID(r.Context(), user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving user credentials: %v", err), http.StatusInternalServerError)
		return
	}

	// Create session data
	sessionData := wauthn.SessionData{
		Challenge:        challenge,
		UserID:           []byte(uid),
		UserVerification: protocol.VerificationPreferred,
		RelyingPartyID:   h.WebAuthn.WebAuthn.Config.RPID,
	}

	// Verify authentication
	credential, err := h.WebAuthn.WebAuthn.FinishLogin(webAuthnUser, sessionData, r)
	if err != nil {
		slog.Error("Error verifying authentication", "error", err)
		http.Error(w, fmt.Sprintf("Authentication verification failed: %v", err), http.StatusBadRequest)
		return
	}

	// Authentication successful - update last used timestamp for the key
	keyID := fmt.Sprintf("passkey-%s", base64.StdEncoding.EncodeToString(credential.ID))
	if err := h.Encryption.UpdateKeyLastUsed(r.Context(), user.ID, keyID); err != nil {
		slog.Warn("Failed to update key last used timestamp", "error", err)
		// Continue anyway - this is just a timestamp update
	}

	// Clean up the challenge
	if err := h.WebAuthn.DeleteChallengeFromDB(r.Context(), challengeID); err != nil {
		slog.Warn("Failed to delete challenge from database", "error", err)
		// Continue anyway - this is just cleanup
	}

	// Return success
	resp := rp.WebAuthnAuthenticationResponse{
		Success: true,
		Message: "Authentication successful",
	}
	rp.RespondWithJSON(w, http.StatusOK, resp)
}

// MARK: - ListPasskeys

func (h *Service) ListPasskeys(w http.ResponseWriter, r *http.Request) {
	// Get the user ID from context (set by middleware)
	uid := middleware.GetUserID(r.Context())
	if uid == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get encryption keys from database
	keys, err := h.Encryption.ListKeys(r.Context(), uid)
	if err != nil {
		slog.Error("Error listing encryption keys", "error", err)
		http.Error(w, "Failed to list passkeys", http.StatusInternalServerError)
		return
	}

	// Filter keys that are passkey-derived and convert to response format
	var keyItems []rp.KeyListItem
	for _, key := range keys {
		// Only include passkey-derived keys
		if key.KeyDerivationMethod == "passkey-derived" && key.CredentialID != "" {
			keyItem := rp.KeyListItem{
				KeyID:               key.KeyID,
				CreatedAt:           key.CreatedAt,
				LastUsedAt:          key.LastUsedAt,
				IsActive:            key.IsActive,
				Version:             key.KeyVersion,
				CredentialID:        key.CredentialID,
				KeyDerivationMethod: key.KeyDerivationMethod,
				// Note: We don't currently store passkey names separately
				// We could extract this from the key.KeyID or add a new field in the database
				PasskeyName: fmt.Sprintf("Passkey %d", key.KeyVersion),
			}
			keyItems = append(keyItems, keyItem)
		}
	}

	// Return the response
	response := rp.ListKeysResponse{
		Keys: keyItems,
	}
	rp.RespondWithJSON(w, http.StatusOK, response)
}
