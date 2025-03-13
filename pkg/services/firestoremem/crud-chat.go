package firestoremem

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/ditto-assistant/backend/types/rp"
	"github.com/ditto-assistant/backend/types/rq"
)

// Using llm.ModelTextEmbedding005
const (
	ColumnEmbeddingPrompt5   = "embedding_prompt_5"
	ColumnEmbeddingResponse5 = "embedding_response_5"
)

type CreatePromptRequest struct {
	DeviceID          string             `firestore:"device_id"`
	EmbeddingPrompt5  firestore.Vector32 `firestore:"embedding_prompt_5"`
	Prompt            string             `firestore:"prompt"`
	EncryptedPrompt   string             `firestore:"encrypted_prompt,omitempty"`
	EncryptionKeyID   string             `firestore:"encryption_key_id,omitempty"`
	EncryptionVersion int                `firestore:"encryption_version,omitempty"`
	IsEncrypted       bool               `firestore:"is_encrypted,omitempty"`
	Timestamp         time.Time          `firestore:"timestamp,serverTimestamp"`
}

func (cl *Client) CreatePrompt(ctx context.Context, userID string, req *CreatePromptRequest) (string, error) {
	pair := cl.ConversationsRef(userID).NewDoc()
	_, err := pair.Create(ctx, req)
	if err != nil {
		return "", err
	}
	return pair.ID, nil
}

type SaveResponseRequest struct {
	UserID, PairID, Response string
	EmbeddingResponse5       firestore.Vector32
	EncryptedResponse        string `firestore:"encrypted_response,omitempty"`
	EncryptionKeyID          string `firestore:"encryption_key_id,omitempty"`
	EncryptionVersion        int    `firestore:"encryption_version,omitempty"`
	IsEncrypted              bool   `firestore:"is_encrypted,omitempty"`
}

func (cl *Client) SaveResponse(ctx context.Context, req *SaveResponseRequest) error {
	pair := cl.ConversationsRef(req.UserID).Doc(req.PairID)

	updates := []firestore.Update{
		{Path: "response", Value: req.Response},
		{Path: ColumnEmbeddingResponse5, Value: req.EmbeddingResponse5},
	}

	// Add encryption fields if this is an encrypted response
	if req.IsEncrypted {
		updates = append(updates,
			firestore.Update{Path: "encrypted_response", Value: req.EncryptedResponse},
			firestore.Update{Path: "encryption_key_id", Value: req.EncryptionKeyID},
			firestore.Update{Path: "encryption_version", Value: req.EncryptionVersion},
			firestore.Update{Path: "is_encrypted", Value: req.IsEncrypted},
		)
	}

	_, err := pair.Update(ctx, updates)
	if err != nil {
		return err
	}
	return nil
}

func (cl *Client) GetEmbeddingPrompt(ctx context.Context, userID, pairID string) (firestore.Vector32, error) {
	pair := cl.ConversationsRef(userID).Doc(pairID)
	doc, err := pair.Get(ctx)
	if err != nil {
		return nil, err
	}
	var embedding struct {
		EmbeddingPrompt5 firestore.Vector32 `firestore:"embedding_prompt_5"`
	}
	if err := doc.DataTo(&embedding); err != nil {
		return nil, err
	}
	return embedding.EmbeddingPrompt5, nil
}

// MigrateConversations handles the batch migration of conversations to encrypted format
func (cl *Client) MigrateConversations(ctx context.Context, userID string, req *rq.MigrateConversationsV2) (*rp.MigrateConversationsV2, error) {
	startTime := time.Now()
	batch := cl.firestore.BulkWriter(ctx)
	for _, conv := range req.Conversations {
		docRef := cl.ConversationsRef(userID).Doc(conv.DocID)
		_, err := batch.Update(docRef, []firestore.Update{
			{Path: "prompt", Value: ""},
			{Path: "response", Value: ""},
			{Path: "encrypted_prompt", Value: conv.EncryptedPrompt},
			{Path: "encrypted_response", Value: conv.EncryptedResponse},
			{Path: "encryption_key_id", Value: req.EncryptionKeyID},
			{Path: "encryption_version", Value: req.EncryptionVersion},
			{Path: "is_encrypted", Value: true},
			{Path: "migration_timestamp", Value: time.Now()},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update pairID: %s: %w", conv.DocID, err)
		}
	}
	batch.End()
	migratedCount := len(req.Conversations)
	migrationTime := time.Since(startTime)
	return &rp.MigrateConversationsV2{
		MigratedCount:     migratedCount,
		MigrationDuration: migrationTime,
		CompletedAt:       time.Now(),
	}, nil
}

// GetConversationsResult represents the result of a paginated conversation query
type GetConversationsResult struct {
	Messages   []rp.Memory
	NextCursor string
}

// GetConversations retrieves a paginated list of conversations for a user
func (cl *Client) GetConversations(ctx context.Context, userID string, limit int, cursor string) (*GetConversationsResult, error) {
	memoriesRef := cl.ConversationsRef(userID)
	query := memoriesRef.OrderBy("timestamp", firestore.Desc).Limit(limit + 1) // Get one extra to determine if there are more pages

	if cursor != "" {
		cursorDoc, err := memoriesRef.Doc(cursor).Get(ctx)
		if err != nil {
			return nil, err
		}
		query = query.StartAfter(cursorDoc)
	}

	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	hasNextPage := len(docs) > limit
	if hasNextPage {
		docs = docs[:limit] // Remove the extra document we fetched
	}

	messages := make([]rp.Memory, 0, len(docs))
	for _, doc := range docs {
		var mem rp.Memory
		if err := doc.DataTo(&mem); err != nil {
			continue
		}
		mem.ID = doc.Ref.ID
		mem.FormatResponse()
		messages = append(messages, mem)
	}

	nextCursor := ""
	if hasNextPage && len(messages) > 0 {
		nextCursor = messages[len(messages)-1].ID
	}

	return &GetConversationsResult{
		Messages:   messages,
		NextCursor: nextCursor,
	}, nil
}
