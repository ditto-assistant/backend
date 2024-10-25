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

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/pkg/img"
	"github.com/ditto-assistant/backend/pkg/llm"
	"github.com/ditto-assistant/backend/types/rq"
	"github.com/ditto-assistant/backend/types/ty"
	"golang.org/x/oauth2/google"
)

const baseURL = "https://us-east5-aiplatform.googleapis.com/v1/projects/%s/locations/us-east5/publishers/anthropic/models/%s:streamRawPredict"
const Model = llm.ModelClaude35SonnetV2
const Version = "20241022"
const TaggedModel = llm.ModelClaude35SonnetV2 + "@" + Version

var requestUrl string

type Message struct {
	Role    string    `json:"role"`
	Content []Content `json:"content"`
}

type Content struct {
	Type   string            `json:"type"`
	Text   string            `json:"text,omitempty"`
	Source map[string]string `json:"source,omitempty"`
}

type Request struct {
	Messages         []Message `json:"messages"`
	MaxTokens        int       `json:"max_tokens"`
	Stream           bool      `json:"stream"`
	AnthropicVersion string    `json:"anthropic_version"`
	System           string    `json:"system,omitempty"`
}

// event: message_start
// data: {"type":"message_start","message":{"id":"msg_vrtx_01Amq7sdjChu5CgLm6hRXZjD","type":"message","role":"assistant","model":"claude-3-5-sonnet-20240620","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":17,"output_tokens":1}}              }

// event: ping
// data: {"type": "ping"}

// event: content_block_start
// data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}       }

// event: content_block_delta
// data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Q"}               }

// event: content_block_stop
// data: {"type":"content_block_stop","index":0             }

// event: message_delta
// data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":4}    }

// event: message_stop
// data: {"type":"message_stop" }

func init() {
	err := envs.Load()
	if err != nil {
		log.Fatalf("Error loading environment variables: %v", err)
	}
	requestUrl = fmt.Sprintf(baseURL, envs.GCLOUD_PROJECT, TaggedModel)
}

type Token ty.Result[string]

type Response struct {
	Text         <-chan Token
	InputTokens  int
	OutputTokens int
}

type EvMsgStart struct {
	Type    string `json:"type"`
	Message struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type EvContentBlockDelta struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

type EvMsgDelta struct {
	Type  string `json:"type"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// TODO: Add Prompt options, such as message array, last message role is assistant, etc.

func (rsp *Response) Prompt(ctx context.Context, prompt rq.PromptV1) error {
	messages := make([]Message, 0, 1)
	userContentCount := 1
	if prompt.ImageURL != "" {
		userContentCount++
	}
	userMessage := Message{Role: "user", Content: make([]Content, 0, userContentCount)}
	if prompt.ImageURL != "" {
		base64Image, err := img.GetBase64(ctx, prompt.ImageURL)
		if err != nil {
			return fmt.Errorf("error getting base64 image: %w", err)
		}
		userMessage.Content = append(userMessage.Content, Content{
			Type: "image",
			Source: map[string]string{
				"type": "base64",
				// TODO: DETECT IMAGE TYPE
				"media_type": "image/png", // Adjust this if needed based on the actual image type
				"data":       base64Image,
			},
		})
	}
	userMessage.Content = append(userMessage.Content, Content{
		Type: "text",
		Text: prompt.UserPrompt,
	})
	messages = append(messages, userMessage)
	req := Request{
		Messages:         messages,
		MaxTokens:        8192,
		Stream:           true,
		AnthropicVersion: "vertex-2023-10-16",
		System:           prompt.SystemPrompt,
	}
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(req)
	if err != nil {
		return fmt.Errorf("error encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", requestUrl, &buf)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")

	// Get the access token
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

	tokenChan := make(chan Token)
	rsp.Text = tokenChan

	go func() {
		defer resp.Body.Close()
		defer close(tokenChan)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if !bytes.HasPrefix(line, eventPrefix) {
				continue
			}

			eventType := string(bytes.TrimPrefix(line, eventPrefix))
			switch eventType {
			case "message_start", "content_block_delta", "message_delta":
				if !scanner.Scan() {
					tokenChan <- Token{Err: fmt.Errorf("unexpected end of stream after event: %s", eventType)}
					return
				}
				data := scanner.Bytes()
				if !bytes.HasPrefix(data, dataPrefix) {
					tokenChan <- Token{Err: fmt.Errorf("expected data line after event, got: %s", data)}
					return
				}
				data = bytes.TrimPrefix(data, dataPrefix)

				switch eventType {
				case "message_start":
					var msgStart EvMsgStart
					if err := json.Unmarshal(data, &msgStart); err != nil {
						tokenChan <- Token{Err: fmt.Errorf("error parsing message_start event: %w", err)}
						return
					}
					rsp.InputTokens += msgStart.Message.Usage.InputTokens
					rsp.OutputTokens += msgStart.Message.Usage.OutputTokens

				case "content_block_delta":
					var contentDelta EvContentBlockDelta
					if err := json.Unmarshal(data, &contentDelta); err != nil {
						tokenChan <- Token{Err: fmt.Errorf("error parsing content_block_delta event: %w", err)}
						return
					}
					tokenChan <- Token{Ok: contentDelta.Delta.Text}

				case "message_delta":
					var msgDelta EvMsgDelta
					if err := json.Unmarshal(data, &msgDelta); err != nil {
						tokenChan <- Token{Err: fmt.Errorf("error parsing message_delta event: %w", err)}
						return
					}
					rsp.OutputTokens += msgDelta.Usage.OutputTokens
				}

			default:
				// For events we don't care about, just continue to the next line
				continue
			}
		}

		if err := scanner.Err(); err != nil {
			tokenChan <- Token{Err: fmt.Errorf("error reading stream: %w", err)}
		}
	}()

	return nil
}

var dataPrefix = []byte("data: ")
var eventPrefix = []byte("event: ")
