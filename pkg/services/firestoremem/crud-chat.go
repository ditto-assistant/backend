package firestoremem

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
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
	EncryptedResponse        string             `firestore:"encrypted_response,omitempty"`
	EncryptionKeyID          string             `firestore:"encryption_key_id,omitempty"`
	EncryptionVersion        int                `firestore:"encryption_version,omitempty"`
	IsEncrypted              bool               `firestore:"is_encrypted,omitempty"`
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
