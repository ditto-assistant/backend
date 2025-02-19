package cerebras

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/ditto-assistant/backend/types/rq"
	"github.com/ditto-assistant/backend/types/ty"
)

type Service struct {
	mu     sync.RWMutex
	apiKey string
	secr   *secr.Client
	sd     *ty.ShutdownContext
}

func NewService(sd *ty.ShutdownContext, secr *secr.Client) *Service {
	return &Service{sd: sd, secr: secr}
}

const baseURL = "https://api.cerebras.ai/v1/chat/completions"
const ApiKeySuffic = "CEREBRAS_API_KEY"

func (s *Service) setupKey(ctx context.Context) (string, error) {
	s.mu.RLock()
	key := s.apiKey
	s.mu.RUnlock()
	if key != "" {
		return key, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.apiKey != "" {
		return s.apiKey, nil
	}
	var err error
	s.apiKey, err = s.secr.FetchEnv(ctx, ApiKeySuffic)
	if err != nil {
		return "", fmt.Errorf("failed to fetch api key: %w", err)
	}
	return s.apiKey, nil
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	TopP        float64   `json:"top_p,omitempty"`
}

type StreamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (s *Service) Prompt(ctx context.Context, prompt rq.PromptV1, rsp *llm.StreamResponse) error {
	if prompt.ImageURL != "" {
		return errors.New("image input not supported for Cerebras models")
	}
	key, err := s.setupKey(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup api key: %w", err)
	}
	messages := make([]Message, 0, 2)
	if prompt.SystemPrompt != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: prompt.SystemPrompt,
		})
	}
	messages = append(messages, Message{
		Role:    "user",
		Content: prompt.UserPrompt,
	})
	req := Request{
		Model:       string(prompt.Model),
		Messages:    messages,
		Stream:      true,
		MaxTokens:   1024,
		Temperature: 0.2,
		TopP:        1.0,
	}
	var buf bytes.Buffer
	err = json.NewEncoder(&buf).Encode(req)
	if err != nil {
		return fmt.Errorf("error encoding request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL, &buf)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+key)
	resp, err := llm.HttpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	tokenChan := make(chan llm.Token)
	rsp.Text = tokenChan
	go func() {
		defer resp.Body.Close()
		defer close(tokenChan)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if !bytes.HasPrefix(line, llm.PrefixData) {
				continue
			}
			data := bytes.TrimPrefix(line, llm.PrefixData)
			if bytes.Equal(data, llm.TokenDone) {
				break
			}
			var streamResp StreamResponse
			if err := json.Unmarshal(data, &streamResp); err != nil {
				tokenChan <- llm.Token{Err: fmt.Errorf("error parsing stream response: %w", err)}
				return
			}
			for _, choice := range streamResp.Choices {
				if choice.Delta.Content != "" {
					tokenChan <- llm.Token{Ok: choice.Delta.Content}
				}
				if choice.FinishReason == "stop" {
					rsp.InputTokens = streamResp.Usage.PromptTokens
					rsp.OutputTokens = streamResp.Usage.CompletionTokens
				}
			}
		}
		if err := scanner.Err(); err != nil {
			tokenChan <- llm.Token{Err: fmt.Errorf("error reading stream: %w", err)}
		}
	}()
	return nil
}
