package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/ditto-assistant/backend/pkg/envs"
	"github.com/ditto-assistant/backend/pkg/llm"
	"golang.org/x/oauth2/google"
)

const baseURL = "https://us-east5-aiplatform.googleapis.com/v1/projects/%s/locations/us-east5/publishers/anthropic/models/%s:streamRawPredict"
const model llm.ModelName = "claude-3-5-sonnet@20240620"

type Message struct {
	Role    string    `json:"role"`
	Content []Content `json:"content"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Request struct {
	Messages         []Message `json:"messages"`
	MaxTokens        int       `json:"max_tokens"`
	Stream           bool      `json:"stream"`
	AnthropicVersion string    `json:"anthropic_version"`
}

type StreamResponse struct {
	Type  string `json:"type"`
	Delta struct {
		Text string `json:"text"`
	} `json:"delta"`
}

func init() {
	err := envs.Load()
	if err != nil {
		log.Fatalf("Error loading environment variables: %v", err)
	}
}

func Prompt(ctx context.Context, prompt string) (<-chan string, error) {
	url := fmt.Sprintf(baseURL, envs.GCLOUD_PROJECT, model)
	req := Request{
		Messages: []Message{
			{
				Role: "user",
				Content: []Content{
					{Type: "text", Text: prompt},
				},
			},
		},
		MaxTokens:        1024,
		Stream:           true,
		AnthropicVersion: "vertex-2023-10-16",
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(req)
	if err != nil {
		return nil, fmt.Errorf("error encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")

	// Get the access token
	tokenSource, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("error getting token source: %w", err)
	}
	token, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("error getting token: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := llm.HttpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("error response from API: status %d, body: %s", resp.StatusCode, string(body))
	}

	streamChan := make(chan string)

	go func() {
		defer resp.Body.Close()
		defer close(streamChan)

		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				streamChan <- fmt.Sprintf("Error reading stream: %v", err)
				return
			}

			line = bytes.TrimSpace(line)
			if len(line) == 0 || !bytes.HasPrefix(line, dataPrefix) {
				continue
			}

			data := bytes.TrimPrefix(line, dataPrefix)
			var streamResp StreamResponse
			if err := json.Unmarshal(data, &streamResp); err != nil {
				streamChan <- fmt.Sprintf("Error parsing JSON: %v", err)
				return
			}

			if streamResp.Type == "content_block_delta" {
				streamChan <- streamResp.Delta.Text
			}
		}
	}()

	return streamChan, nil
}

var dataPrefix = []byte("data: ")
