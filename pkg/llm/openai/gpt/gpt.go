package gpt

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/img"
	"github.com/ditto-assistant/backend/pkg/llm"
	"github.com/ditto-assistant/backend/types/rq"
)

const baseURL = "https://api.openai.com/v1/chat/completions"

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
	Content []Content `json:"content,omitempty"`
}

// Request represents a chat completion request to the OpenAI API
//
// https://platform.openai.com/docs/api-reference/chat/create
type Request struct {
	// Required: ID of the model to use
	Model string `json:"model"`

	// Required: List of messages comprising the conversation
	Messages []Message `json:"messages"`

	// Optional: Whether to stream partial message deltas (default: false)
	Stream bool `json:"stream,omitempty"`

	// Optional: Options for streaming response when stream is true
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`

	// Optional: What sampling temperature to use, between 0 and 2 (default: 1)
	// Higher values = more random, lower values = more focused
	Temperature *float64 `json:"temperature,omitempty"`

	// Optional: Upper bound on tokens to generate, including visible output and reasoning tokens
	MaxCompletionTokens int `json:"max_completion_tokens,omitempty"`

	// Optional: Alternative to temperature, called nucleus sampling (default: 1)
	// 0.1 means only tokens comprising top 10% probability mass are considered
	TopP float64 `json:"top_p,omitempty"`

	// Optional: Number of chat completion choices to generate (default: 1)
	// Higher values increase token usage and costs
	N int `json:"n,omitempty"`

	// Optional: Stop generating tokens at these sequences (max 4)
	Stop []string `json:"stop,omitempty"`

	// Optional: Number between -2.0 and 2.0 (default: 0)
	// Positive values penalize new tokens based on frequency
	FrequencyPenalty float64 `json:"frequency_penalty,omitempty"`

	// Optional: Number between -2.0 and 2.0 (default: 0)
	// Positive values penalize tokens based on whether they appear in text so far
	PresencePenalty float64 `json:"presence_penalty,omitempty"`

	// Optional: Whether to return log probabilities of output tokens
	LogProbs bool `json:"logprobs,omitempty"`

	// Optional: Number of most likely tokens to return (0-20)
	// Requires logprobs to be true
	TopLogProbs int `json:"top_logprobs,omitempty"`

	// Optional: Token bias mapping (-100 to 100)
	// Modifies likelihood of specified tokens appearing
	LogitBias map[string]float64 `json:"logit_bias,omitempty"`

	// Optional: Response format specification
	// Used for JSON mode or structured outputs
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`

	// Optional: Seed for deterministic sampling
	Seed *int `json:"seed,omitempty"`

	// Optional: Tools (like functions) the model may call
	Tools []Tool `json:"tools,omitempty"`

	// Optional: Controls which tool is called by the model
	ToolChoice *ToolChoice `json:"tool_choice,omitempty"`

	// Optional: Whether to enable parallel function calling during tool use
	ParallelToolCalls bool `json:"parallel_tool_calls,omitempty"`

	// Optional: Unique identifier representing end-user for abuse monitoring
	User string `json:"user,omitempty"`

	// Optional: Whether to store output for model distillation/evals
	Store bool `json:"store,omitempty"`

	// Optional: Developer-defined tags for filtering completions
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// StreamOptions configures streaming response behavior
type StreamOptions struct {
	// Whether to include token usage information in stream
	IncludeUsage bool `json:"include_usage"`
}

// ResponseFormat specifies the required output format
type ResponseFormat struct {
	// Type of format ("json_object" or "json_schema")
	Type string `json:"type"`
	// Optional JSON schema for structured output
	JSONSchema interface{} `json:"json_schema,omitempty"`
}

// Tool represents a function the model can call
type Tool struct {
	// Type of tool (currently only "function" is supported)
	Type string `json:"type"`
	// Function definition
	Function ToolFunction `json:"function"`
}

// ToolFunction defines a callable function
type ToolFunction struct {
	// Name of the function
	Name string `json:"name"`
	// Description of what the function does
	Description string `json:"description"`
	// Parameters the function accepts (in JSON Schema format)
	Parameters interface{} `json:"parameters"`
}

// ToolChoice controls tool usage
type ToolChoice struct {
	// Type of choice ("none", "auto", "function")
	Type string `json:"type"`
	// Optional specific function to call
	Function *ToolFunction `json:"function,omitempty"`
}

// StreamResponse represents a streamed chunk of a chat completion response
type StreamResponse struct {
	// A unique identifier for the chat completion
	ID string `json:"id"`

	// The object type, which is always chat.completion.chunk
	Object string `json:"object"`

	// The Unix timestamp (in seconds) of when the chat completion was created
	Created int64 `json:"created"`

	// The model used to generate the completion
	Model string `json:"model"`

	// The service tier used for processing the request (only if specified in request)
	ServiceTier *string `json:"service_tier,omitempty"`

	// Backend configuration fingerprint for determinism with seed parameter
	SystemFingerprint string `json:"system_fingerprint"`

	// List of chat completion choices
	Choices []struct {
		// The index of this choice among all choices
		Index int `json:"index"`

		// The delta content for this chunk
		Delta struct {
			// The text content of this chunk
			Content string `json:"content"`
		} `json:"delta"`

		// The reason the model stopped generating
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`

	// Token usage statistics (only present in final chunk with stream_options.include_usage)
	Usage *struct {
		// Number of tokens in the prompt
		PromptTokens int `json:"prompt_tokens"`
		// Number of tokens in the generated completion
		CompletionTokens int `json:"completion_tokens"`
		// Total number of tokens used
		TotalTokens int `json:"total_tokens"`

		// Detailed prompt token information
		PromptTokensDetails struct {
			// Number of cached tokens used
			CachedTokens int `json:"cached_tokens"`
			// Number of audio tokens used
			AudioTokens int `json:"audio_tokens"`
		} `json:"prompt_tokens_details"`

		// Detailed completion token information
		CompletionTokensDetails struct {
			// Number of tokens used for reasoning
			ReasoningTokens int `json:"reasoning_tokens"`
			// Number of audio tokens used
			AudioTokens int `json:"audio_tokens"`
			// Number of accepted prediction tokens
			AcceptedPredictionTokens int `json:"accepted_prediction_tokens"`
			// Number of rejected prediction tokens
			RejectedPredictionTokens int `json:"rejected_prediction_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage,omitempty"`
}

func init() {
	err := envs.Load()
	if err != nil {
		panic(fmt.Sprintf("Error loading environment variables: %v", err))
	}
}

func Prompt(ctx context.Context, prompt rq.PromptV1, rsp *llm.StreamResponse) error {
	if prompt.Model == "" {
		return fmt.Errorf("model is required")
	}
	messages := make([]Message, 0, 2)
	if prompt.SystemPrompt != "" {
		messages = append(messages, Message{
			Role: "system",
			Content: []Content{{
				Type: "text",
				Text: prompt.SystemPrompt,
			}},
		})
	}

	userContent := []Content{{
		Type: "text",
		Text: prompt.UserPrompt,
	}}

	if prompt.ImageURL != "" {
		// Only GPT-4o and GPT-4o-1120 support images
		if IsO1Model(prompt.Model) {
			return fmt.Errorf("image input not supported for model %s", prompt.Model)
		}

		imageData, err := img.GetImageData(ctx, prompt.ImageURL)
		if err != nil {
			return fmt.Errorf("error getting image data: %w", err)
		}

		// Create a data URL from the base64 data
		var b strings.Builder
		b.WriteString("data:")
		b.WriteString(imageData.MimeType)
		b.WriteString(";base64,")
		b.WriteString(imageData.Base64)
		dataURL := b.String()

		userContent = append([]Content{{
			Type: "image_url",
			ImageURL: &ImageURL{
				URL: dataURL,
			},
		}}, userContent...)
	}

	messages = append(messages, Message{
		Role:    "user",
		Content: userContent,
	})

	reqBody := Request{
		Model:               string(prompt.Model),
		Messages:            messages,
		Stream:              true,
		StreamOptions:       &StreamOptions{IncludeUsage: true},
		MaxCompletionTokens: 8192,
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(reqBody)
	if err != nil {
		return fmt.Errorf("error encoding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL, &buf)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("", secr.OPENAI_LLM_API_KEY.String())
	resp, err := llm.HttpClient.Do(req)
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
			}
			if streamResp.Usage != nil {
				rsp.InputTokens = streamResp.Usage.PromptTokens
				rsp.OutputTokens = streamResp.Usage.CompletionTokens
			}
		}

		if err := scanner.Err(); err != nil {
			tokenChan <- llm.Token{Err: fmt.Errorf("error reading stream: %w", err)}
		}
	}()

	return nil
}

func IsO1Model(m llm.ServiceName) bool {
	return m == llm.ModelO1Preview ||
		m == llm.ModelO1Mini ||
		m == llm.ModelO1Mini20240912 ||
		m == llm.ModelO1Preview20240912
}
