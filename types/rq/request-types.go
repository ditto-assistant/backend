package rq

import (
	"errors"
	"net/http"

	"github.com/ditto-assistant/backend/pkg/llm"
)

type HasUserID interface {
	GetUserID() string
}

type ChatV2 struct {
	UserID string `json:"userID"`
}

func (c ChatV2) GetUserID() string { return c.UserID }

type PromptV1 struct {
	UserID         string          `json:"userID"`
	UserPrompt     string          `json:"userPrompt"`
	SystemPrompt   string          `json:"systemPrompt"`
	Model          llm.ServiceName `json:"model,omitempty"`
	ImageURL       string          `json:"imageURL,omitempty"`
	UsersOpenaiKey string          `json:"usersOpenaiKey,omitempty"`
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
}

func (g GenerateImageV1) GetUserID() string { return g.UserID }

type SearchExamplesV1 struct {
	UserID    string        `json:"userID"`
	Embedding llm.Embedding `json:"embedding"`
	K         int           `json:"k"`
}

func (s SearchExamplesV1) GetUserID() string { return s.UserID }

type BalanceV1 struct {
	UserID string `json:"userID" form:"userID"`
}

func (b BalanceV1) GetUserID() string { return b.UserID }

func (b *BalanceV1) FromQuery(r *http.Request) error {
	uid := r.URL.Query().Get("userID")
	if uid == "" {
		return errors.New("userID is required")
	}
	b.UserID = uid
	return nil
}
