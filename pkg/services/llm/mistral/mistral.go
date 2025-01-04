package mistral

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/ditto-assistant/backend/types/rq"
	"golang.org/x/oauth2/google"
)

type Model llm.ServiceName

const (
	ModelMistralNemo  = Model(llm.ModelMistralNemo)
	ModelMistralLarge = Model(llm.ModelMistralLarge)
)

func (m Model) PrettyStr() string {
	switch m {
	case ModelMistralNemo:
		return "Mistral Nemo"
	default:
		return "Unknown Mistral Model"
	}
}

const (
	region  = "europe-west4"
	baseURL = "https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/mistralai/models/%s@%s:streamRawPredict"
	Version = "2407"
)

var requestURL string

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	Stream      bool      `json:"stream"`
}

type StreamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func init() {
	err := envs.Load()
	if err != nil {
		panic(fmt.Sprintf("Error loading environment variables: %v", err))
	}
}

func Prompt(ctx context.Context, prompt rq.PromptV1, rsp *llm.StreamResponse) error {
	requestURL = fmt.Sprintf(baseURL, region, envs.GCLOUD_PROJECT, region, prompt.Model, Version)
	messages := make([]Message, 0, 2)
	if prompt.SystemPrompt != "" {
		messages = append([]Message{{Role: "system", Content: prompt.SystemPrompt}}, messages...)
	}
	messages = append(messages, Message{Role: "user", Content: prompt.UserPrompt})

	if prompt.ImageURL != "" {
		return fmt.Errorf("image not supported")
	}

	req := Request{
		Model:       string(prompt.Model),
		Messages:    messages,
		Temperature: 0.7,
		Stream:      true,
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(req)
	if err != nil {
		return fmt.Errorf("error encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", requestURL, &buf)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")

	tokenSource, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return fmt.Errorf("error getting token source: %w", err)
	}
	token, err := tokenSource.Token()
	if err != nil {
		return fmt.Errorf("error getting token: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := llm.HttpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return fmt.Errorf("error response from API: status %d, body: %s", resp.StatusCode, string(body))
	}

	tokenChan := make(chan llm.Token)
	rsp.Text = tokenChan

	go func() {
		defer resp.Body.Close()
		defer close(tokenChan)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var streamResp StreamResponse
			if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
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
