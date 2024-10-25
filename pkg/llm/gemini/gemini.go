package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/pkg/img"
	"github.com/ditto-assistant/backend/pkg/llm"
	"github.com/ditto-assistant/backend/types/rq"
)

type Model llm.ServiceName

const (
	ModelGemini15Flash = Model(llm.ModelGemini15Flash)
	ModelGemini15Pro   = Model(llm.ModelGemini15Pro)
)

func (m Model) PrettyStr() string {
	switch m {
	case ModelGemini15Flash:
		return "Gemini 1.5 Flash"
	case ModelGemini15Pro:
		return "Gemini 1.5 Pro"
	default:
		return "Unknown"
	}
}

const (
	baseURL = "https://us-central1-aiplatform.googleapis.com/v1/projects/%s/locations/us-central1/publishers/google/models/%s:streamGenerateContent"
)

var requestURL string

func init() {
	err := envs.Load()
	if err != nil {
		panic(fmt.Sprintf("Error loading environment variables: %v", err))
	}
}

type Part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *InlineData `json:"inlineData,omitempty"`
}

type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type Content struct {
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
}

type Request struct {
	Contents []Content `json:"contents"`
}

type Response struct {
	Candidates    []Candidate `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata,omitempty"`
	ModelVersion string `json:"modelVersion"`
}

type Candidate struct {
	Content struct {
		Role  string `json:"role"`
		Parts []Part `json:"parts"`
	} `json:"content"`
	FinishReason  string         `json:"finishReason,omitempty"`
	SafetyRatings []SafetyRating `json:"safetyRatings,omitempty"`
}

type SafetyRating struct {
	Category         string  `json:"category"`
	Probability      string  `json:"probability"`
	ProbabilityScore float64 `json:"probabilityScore"`
	Severity         string  `json:"severity"`
	SeverityScore    float64 `json:"severityScore"`
}

func (m Model) Prompt(ctx context.Context, prompt rq.PromptV1, rsp *llm.StreamResponse) error {
	requestURL = fmt.Sprintf(baseURL, envs.GCLOUD_PROJECT, m)
	contents := []Content{
		{Role: "user", Parts: []Part{{Text: prompt.UserPrompt}}},
	}

	if prompt.ImageURL != "" {
		imageData, err := img.GetImageData(ctx, prompt.ImageURL)
		if err != nil {
			return fmt.Errorf("error getting image data: %w", err)
		}
		contents[0].Parts = append(contents[0].Parts, Part{
			InlineData: &InlineData{
				MimeType: imageData.MimeType,
				Data:     imageData.Base64,
			},
		})
	}

	if prompt.SystemPrompt != "" {
		contents = append([]Content{{Role: "system", Parts: []Part{{Text: prompt.SystemPrompt}}}}, contents...)
	}

	req := Request{Contents: contents}
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

		decoder := json.NewDecoder(resp.Body)
		var totalInputTokens, totalOutputTokens int

		// Read the opening bracket of the JSON array
		_, err := decoder.Token()
		if err != nil {
			tokenChan <- llm.Token{Err: fmt.Errorf("error reading opening bracket: %w", err)}
			return
		}

		for decoder.More() {
			var response Response
			if err := decoder.Decode(&response); err != nil {
				tokenChan <- llm.Token{Err: fmt.Errorf("error decoding JSON: %w; raw_json: %s", err, debugGetRawJSON(decoder))}
				return
			}

			// slog.Debug("Decoded response", "response", response)

			for _, candidate := range response.Candidates {
				for _, part := range candidate.Content.Parts {
					if part.Text != "" {
						tokenChan <- llm.Token{Ok: part.Text}
					}
				}
			}

			if response.UsageMetadata.PromptTokenCount > 0 {
				totalInputTokens = response.UsageMetadata.PromptTokenCount
				totalOutputTokens = response.UsageMetadata.CandidatesTokenCount
			}
		}

		// Read the closing bracket of the JSON array
		_, err = decoder.Token()
		if err != nil {
			tokenChan <- llm.Token{Err: fmt.Errorf("error reading closing bracket: %w", err)}
			return
		}

		rsp.InputTokens = totalInputTokens
		rsp.OutputTokens = totalOutputTokens
	}()

	return nil
}

// Helper function to get raw JSON for debugging
func debugGetRawJSON(decoder *json.Decoder) string {
	var raw json.RawMessage
	err := decoder.Decode(&raw)
	if err != nil {
		return fmt.Sprintf("Error getting raw JSON: %v", err)
	}
	return string(raw)
}
