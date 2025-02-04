package rq

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ditto-assistant/backend/pkg/services/llm"
)

type HasUserID interface {
	GetUserID() string
}

type ChatV2 struct {
	UserID string `json:"userID"`
}

func (c ChatV2) GetUserID() string { return c.UserID }

type PromptV1 struct {
	UserID       string          `json:"userID"`
	UserPrompt   string          `json:"userPrompt"`
	SystemPrompt string          `json:"systemPrompt"`
	Model        llm.ServiceName `json:"model,omitempty"`
	ImageURL     string          `json:"imageURL,omitempty"`
	Images       []string        `json:"images,omitempty"`
}

func (p PromptV1) GetUserID() string { return p.UserID }

type SearchV1 struct {
	UserID     string `json:"userID"`
	Query      string `json:"query"`
	NumResults int    `json:"numResults"`
}

func (s SearchV1) GetUserID() string { return s.UserID }

type EmbedV1 struct {
	UserID string          `json:"userID"`
	Text   string          `json:"text"`
	Model  llm.ServiceName `json:"model"`
}

func (e EmbedV1) GetUserID() string { return e.UserID }

type GenerateImageV1 struct {
	UserID string          `json:"userID"`
	Prompt string          `json:"prompt"`
	Model  llm.ServiceName `json:"model"`

	// DALL-E specific fields
	Size string `json:"size,omitempty"`

	// FLUX specific fields
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
	PromptUpsampling string `json:"promptUpsampling,omitempty"`
	Seed             int    `json:"seed,omitempty"`
	SafetyTolerance  int    `json:"safetyTolerance,omitempty"`
}

func (g GenerateImageV1) GetUserID() string { return g.UserID }

type SearchExamplesV1 struct {
	UserID    string        `json:"userID"`
	Embedding llm.Embedding `json:"embedding"`
	K         int           `json:"k"`
}

func (s SearchExamplesV1) GetUserID() string { return s.UserID }

type BalanceV1 struct {
	UserID   string `json:"userID"`
	Email    string `json:"email"`
	Version  string `json:"version"`
	Platform int    `json:"platform"`
	DeviceID string `json:"deviceId"`
}

func (b BalanceV1) GetUserID() string { return b.UserID }

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

func (p PresignedURLV1) GetUserID() string { return p.UserID }

type CreateUploadURLV1 struct {
	UserID string `json:"userID"`
}

func (c CreateUploadURLV1) GetUserID() string { return c.UserID }

type GetMemoriesV1 struct {
	UserID string    `json:"userID"`
	Vector []float32 `json:"vector"`
	K      int       `json:"k,omitempty"`
}

func (g GetMemoriesV1) GetUserID() string { return g.UserID }

type GetMemoriesV2 struct {
	UserID      string                     `json:"userID"`
	LongTerm    *ParamsLongTermMemoriesV2  `json:"longTerm"`
	ShortTerm   *ParamsShortTermMemoriesV2 `json:"shortTerm"`
	StripImages bool                       `json:"stripImages"`
}

type ParamsLongTermMemoriesV2 struct {
	Vector     []float32 `json:"vector"`
	NodeCounts []int     `json:"nodeCounts"`
}

type ParamsShortTermMemoriesV2 struct {
	K int `json:"k"`
}

func (g *GetMemoriesV2) GetUserID() string { return g.UserID }

type FeedbackV1 struct {
	UserID   string `json:"userID"`
	DeviceID string `json:"deviceId"`
	Type     string `json:"type"` // bug, feature-request, other
	Feedback string `json:"feedback"`
}

func (f FeedbackV1) GetUserID() string { return f.UserID }
