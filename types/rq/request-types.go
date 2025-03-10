package rq

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/ditto-assistant/backend/pkg/services/llm"
)

type ChatV2 struct {
	UserID string `json:"userID"`
}

type PromptV1 struct {
	UserID       string          `json:"userID"`
	UserPrompt   string          `json:"userPrompt"`
	SystemPrompt string          `json:"systemPrompt"`
	Model        llm.ServiceName `json:"model,omitempty"`
	ImageURL     string          `json:"imageURL,omitempty"`
	Images       []string        `json:"images,omitempty"`
}

type SearchV1 struct {
	UserID     string `json:"userID"`
	Query      string `json:"query"`
	NumResults int    `json:"numResults"`
}

type EmbedV1 struct {
	UserID string          `json:"userID"`
	Text   string          `json:"text"`
	Model  llm.ServiceName `json:"model"`
}

type CreatePromptV1 struct {
	UserID   string `json:"userID"`
	DeviceID string `json:"deviceID"`
	Prompt   string `json:"prompt"`
}

type GenerateImageV1 struct {
	UserID    string          `json:"userID"`
	Prompt    string          `json:"prompt"`
	Model     llm.ServiceName `json:"model"`
	DummyMode bool            `json:"dummyMode"`

	// DALL-E specific fields
	Size string `json:"size,omitempty"`

	// FLUX specific fields
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
	PromptUpsampling string `json:"promptUpsampling,omitempty"`
	Seed             int    `json:"seed,omitempty"`
	SafetyTolerance  int    `json:"safetyTolerance,omitempty"`
}

type SearchExamplesV1 struct {
	UserID    string        `json:"userID"`
	PairID    string        `json:"pairID"`
	Embedding llm.Embedding `json:"embedding"`
	K         int           `json:"k"`
}

type BalanceV1 struct {
	UserID   string `json:"userID"`
	Email    string `json:"email"`
	Version  string `json:"version"`
	Platform int    `json:"platform"`
	DeviceID string `json:"deviceId"`
}

func (b *BalanceV1) FromQuery(r *http.Request) error {
	uid := r.URL.Query().Get("userID")
	if uid == "" {
		return errors.New("userID is required")
	}
	b.UserID = uid
	b.Email = r.URL.Query().Get("email")
	b.Version = r.URL.Query().Get("version")
	b.Platform, _ = strconv.Atoi(r.URL.Query().Get("platform"))
	b.DeviceID = r.URL.Query().Get("deviceID")
	if b.DeviceID == "" {
		b.DeviceID = r.URL.Query().Get("deviceId")
	}
	return nil
}

type PresignedURLV1 struct {
	UserID string `json:"userID"`
	URL    string `json:"url"`
	Folder string `json:"folder"`
}

type CreateUploadURLV1 struct {
	UserID string `json:"userID"`
}

type GetMemoriesV1 struct {
	UserID string    `json:"userID"`
	Vector []float32 `json:"vector"`
	K      int       `json:"k,omitempty"`
}

type GetMemoriesV2 struct {
	UserID      string                     `json:"userID"`
	LongTerm    *ParamsLongTermMemoriesV2  `json:"longTerm"`
	ShortTerm   *ParamsShortTermMemoriesV2 `json:"shortTerm"`
	StripImages bool                       `json:"stripImages"`
}

func (req *GetMemoriesV2) TotalRequestedMemories() int {
	memoriesRequested := 0
	if req.ShortTerm != nil {
		memoriesRequested = req.ShortTerm.K
	}
	for _, nc := range req.LongTerm.NodeCounts {
		memoriesRequested += nc
	}
	return memoriesRequested
}

type ParamsLongTermMemoriesV2 struct {
	PairID         string             `json:"pairID"`
	Vector         firestore.Vector32 `json:"vector"`
	NodeCounts     []int              `json:"nodeCounts"`
	NodeThresholds []float64          `json:"nodeThresholds"`
	// SkipShortTermContext skips the normalized vector summation of short-term memories.
	SkipShortTermContext bool `json:"skipShortTermContext"`
}

type ParamsShortTermMemoriesV2 struct {
	K int `json:"k"`
}

type FeedbackV1 struct {
	UserID   string
	DeviceID string
	Version  string
	Type     string // bug, feature-request, other
	Feedback string
}

func (f *FeedbackV1) FromForm(r *http.Request) error {
	f.UserID = r.FormValue("userID")
	if f.UserID == "" {
		return errors.New("userID is required")
	}
	f.DeviceID = r.FormValue("deviceID")
	if f.DeviceID == "" {
		return errors.New("deviceID is required")
	}
	f.Version = r.FormValue("version")
	if f.Version == "" {
		return errors.New("version is required")
	}
	f.Type = r.FormValue("type")
	if f.Type == "" {
		return errors.New("type is required")
	}
	f.Feedback = r.FormValue("feedback")
	if f.Feedback == "" {
		return errors.New("feedback is required")
	}
	return nil
}

type SaveResponseV1 struct {
	UserID   string `json:"userID"`
	PairID   string `json:"pairID"`
	Response string `json:"response"`
}

// Encryption Request Types

// CreateEncryptedPromptV1 represents a request to create an encrypted prompt
type CreateEncryptedPromptV1 struct {
	UserID            string `json:"userId,omitempty"` // Can be derived from auth context
	EncryptedPrompt   string `json:"encryptedPrompt"`
	UnencryptedPrompt string `json:"unencryptedPrompt"` // For embeddings only, not stored
	EncryptionKeyID   string `json:"encryptionKeyId"`
	EncryptionVersion int    `json:"encryptionVersion"`
	Model             string `json:"model"`
	ConversationID    string `json:"conversationId,omitempty"`
}

// SaveEncryptedResponseV1 represents a request to save an encrypted response
type SaveEncryptedResponseV1 struct {
	UserID              string    `json:"userId,omitempty"` // Can be derived from auth context
	PromptID            string    `json:"promptId"`
	EncryptedResponse   string    `json:"encryptedResponse"`
	UnencryptedResponse string    `json:"unencryptedResponse"` // For embeddings only, not stored
	EncryptionKeyID     string    `json:"encryptionKeyId"`
	EncryptionVersion   int       `json:"encryptionVersion"`
	ResponseTimestamp   time.Time `json:"responseTimestamp"`
}

// EncryptedConversation represents a single conversation to be migrated to encryption
type EncryptedConversation struct {
	DocID             string `json:"docId"`
	EncryptedPrompt   string `json:"encryptedPrompt"`
	EncryptedResponse string `json:"encryptedResponse"`
}

// MigrateConversationsV2 represents a request to migrate a user's conversations to encrypted versions
type MigrateConversationsV2 struct {
	EncryptionKeyID   string                  `json:"encryptionKeyId"`
	EncryptionVersion int                     `json:"encryptionVersion"`
	Conversations     []EncryptedConversation `json:"conversations"`
}
