package encryptionapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/ditto-assistant/backend/pkg/middleware"
	"github.com/ditto-assistant/backend/pkg/services/db/users"
	"github.com/ditto-assistant/backend/pkg/services/encryption"
	"github.com/ditto-assistant/backend/pkg/services/firestoremem"
	"github.com/ditto-assistant/backend/types/rp"
	"github.com/ditto-assistant/backend/types/rq"
)

// Service handles encryption API endpoints
type Service struct {
	EncryptionService *encryption.Client
	MemoriesService   *firestoremem.Client
}

// NewService creates a new encryption API service
func NewService(encSvc *encryption.Client, memSvc *firestoremem.Client) *Service {
	return &Service{
		EncryptionService: encSvc,
		MemoriesService:   memSvc,
	}
}

// Routes registers the encryption API routes
func (s *Service) Routes(mux *http.ServeMux) {
	router := http.NewServeMux()

	// Register the migration endpoint
	router.HandleFunc("POST /migrate-conversations", s.MigrateConversations)

	// Mount the router at the encryption API prefix
	handler := http.StripPrefix("/api/v2/encryption", router)
	mux.Handle("/api/v2/encryption/", handler)

	slog.Info("Registered encryption API routes")
}

// MigrateConversations handles the batch migration of conversations to encrypted format
func (s *Service) MigrateConversations(w http.ResponseWriter, r *http.Request) {
	// Get user from auth context
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse request
	var apiReq rq.MigrateConversationsV2
	if err := json.NewDecoder(r.Body).Decode(&apiReq); err != nil {
		slog.Error("Failed to parse migration request", "error", err)
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Validate request
	if apiReq.EncryptionKeyID == "" {
		http.Error(w, "EncryptionKeyID is required", http.StatusBadRequest)
		return
	}

	if len(apiReq.Conversations) == 0 {
		http.Error(w, "No conversations to migrate", http.StatusBadRequest)
		return
	}

	// Get user from database
	user := users.User{UID: userID}
	if err := user.GetByUID(r.Context(), s.EncryptionService.DB); err != nil {
		slog.Error("Failed to get user", "error", err)
		http.Error(w, "Failed to get user", http.StatusInternalServerError)
		return
	}

	// Verify the encryption key exists and is active
	var keyExists bool
	err := s.EncryptionService.DB.QueryRowContext(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM encryption_keys WHERE user_id = ? AND key_id = ? AND is_active = TRUE)",
		user.ID, apiReq.EncryptionKeyID).Scan(&keyExists)
	if err != nil {
		slog.Error("Failed to check encryption key", "error", err)
		http.Error(w, "Failed to verify encryption key", http.StatusInternalServerError)
		return
	}
	if !keyExists {
		http.Error(w, "Encryption key not found or not active", http.StatusBadRequest)
		return
	}

	// Perform the migration
	response, err := s.MemoriesService.MigrateConversations(r.Context(), userID, &apiReq)
	if err != nil {
		slog.Error("Failed to migrate conversations", "error", err)
		http.Error(w, "Failed to migrate conversations", http.StatusInternalServerError)
		return
	}

	slog.Info("Migrated conversations to encrypted format",
		"userID", userID,
		"count", response.MigratedCount,
		"migrationTime", response.MigrationDuration)

	rp.RespondWithJSON(w, http.StatusOK, response)
}
