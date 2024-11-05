package llama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/pkg/img"
	"github.com/ditto-assistant/backend/pkg/llm"
	"github.com/ditto-assistant/backend/types/rq"
)

type Model llm.ServiceName

const (
	ModelLlama32 = Model(llm.ModelLlama32)
)

func (m Model) PrettyStr() string {
	switch m {
	case ModelLlama32:
		return "Llama 3.2 Vision"
	default:
		return "Unknown Llama Model"
	}
}

const (
	region  = "us-central1"
	baseURL = "https://%s-aiplatform.googleapis.com/v1beta1/projects/%s/locations/%s/endpoints/openapi/chat/completions"
)

var requestURL string

type Content struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

type Message struct {
	Role    string    `json:"role"`
	Content []Content `json:"content"`
}

type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
	TopK        int       `json:"top_k"`
	TopP        float64   `json:"top_p"`
	N           int       `json:"n"`
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

func init() {
	err := envs.Load()
	if err != nil {
		panic(fmt.Sprintf("Error loading environment variables: %v", err))
	}
	requestURL = fmt.Sprintf(baseURL, region, envs.GCLOUD_PROJECT, region)
}

func (m Model) Prompt(ctx context.Context, prompt rq.PromptV1, rsp *llm.StreamResponse) error {
	contents := []Content{{
		Type: "text",
		Text: prompt.UserPrompt,
	}}

	if prompt.ImageURL != "" {
		imageData, err := img.GetImageData(ctx, prompt.ImageURL)
		if err != nil {
			return fmt.Errorf("error getting image data: %w", err)
		}

		// Create a data URL from the base64 data
		var b strings.Builder
		b.Grow(len(imageData.Base64) + len(imageData.MimeType) + 13)
		b.WriteString("data:")
		b.WriteString(imageData.MimeType)
		b.WriteString(";base64,")
		b.WriteString(imageData.Base64)
		dataURL := b.String()

		contents = append([]Content{{
			Type: "image_url",
			ImageURL: &ImageURL{
				URL: dataURL,
			},
		}}, contents...)
	}

	messages := []Message{{
		Role:    "user",
		Content: contents,
	}}

	if prompt.SystemPrompt != "" {
		messages = append([]Message{{
			Role: "system",
			Content: []Content{{
				Type: "text",
				Text: prompt.SystemPrompt,
			}},
		}}, messages...)
	}

	req := Request{
		Model:       "meta/llama-3.2-90b-vision-instruct-maas",
		Messages:    messages,
		Stream:      true,
		MaxTokens:   8192,
		Temperature: 0.7,
		TopK:        10,
		TopP:        0.95,
		N:           1,
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(req)
	if err != nil {
		return fmt.Errorf("error encoding request: %w", err)
	}

	resp, err := llm.SendRequest(ctx, requestURL, &buf)
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
