package rq

type HasUserID interface {
	GetUserID() string
}

var _, _ HasUserID = ChatV2{}, PromptV1{}

type ChatV2 struct {
	UserID string `json:"userID"`
}

func (c ChatV2) GetUserID() string { return c.UserID }

type PromptV1 struct {
	UserID         string `json:"userID"`
	UserPrompt     string `json:"userPrompt"`
	SystemPrompt   string `json:"systemPrompt"`
	Model          string `json:"model,omitempty"`
	ImageURL       string `json:"imageURL,omitempty"`
	UsersOpenaiKey string `json:"usersOpenaiKey,omitempty"`
}

func (p PromptV1) GetUserID() string { return p.UserID }

type SearchV1 struct {
	UserID     string `json:"userID"`
	Query      string `json:"query"`
	NumResults int    `json:"numResults"`
}

func (s SearchV1) GetUserID() string { return s.UserID }

type EmbedV1 struct {
	UserID string `json:"userID"`
	Text   string `json:"text"`
	Model  string `json:"model"`
}

func (e EmbedV1) GetUserID() string { return e.UserID }

type GenerateImageV1 struct {
	UserID string `json:"userID"`
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

func (g GenerateImageV1) GetUserID() string { return g.UserID }
