package dalle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ditto-assistant/backend/pkg/llm"
	"github.com/ditto-assistant/backend/types/rq"
)

// Client handles DALL-E API requests
type Client struct {
	apiKey string
	client *http.Client
}

// NewClient creates a new DALL-E client
func NewClient(apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		apiKey: apiKey,
		client: httpClient,
	}
}

// ReqDalle represents a request to generate an image using DALL-E models.
type ReqDalle struct {
	// Prompt is the text prompt for image generation.
	// Required field that guides the image generation process.
	Prompt string `json:"prompt"`

	// Model specifies which DALL-E model to use
	Model llm.ServiceName `json:"model"`

	// N specifies the number of images to generate
	N int `json:"n"`

	// Size specifies the square image size.
	//
	//	- Valid values: "1024x1024", "1792x1024", "1024x1792"
	//	- Cannot be used together with Width and Height
	Size string `json:"size,omitempty"`

	// Width of the generated image in pixels.
	//
	//	- Cannot be used together with Size
	//	- Must be provided together with Height
	Width int `json:"width,omitempty"`

	// Height of the generated image in pixels.
	//
	//	- Cannot be used together with Size
	//	- Must be provided together with Width
	Height int `json:"height,omitempty"`

	// Quality specifies the image generation quality level.
	//
	//	- Only applicable for DALL-E 3
	//	- Valid values: "standard" (default), "hd"
	//	- "hd" provides higher quality but increased cost and latency
	Quality string `json:"quality,omitempty"`

	// Style specifies the visual style of the generated image.
	//
	//	- Valid values: "vivid" (default), "natural"
	//	- "vivid" produces more vibrant and dramatic images
	//	- "natural" produces more subtle and realistic images
	Style string `json:"style,omitempty"`
}

// Prompt sends an image generation request to DALL-E
func (c *Client) Prompt(ctx context.Context, r *rq.GenerateImageV1) (string, error) {
	req := ReqDalle{
		Prompt:  r.Prompt,
		Model:   r.Model,
		Size:    r.Size,
		Width:   r.Width,
		Height:  r.Height,
		Quality: "hd",
	}
	if err := req.Validate(); err != nil {
		return "", fmt.Errorf("validation error: %w", err)
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return "", fmt.Errorf("failed to encode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/images/generations", &buf)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}
	var dalleResp struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dalleResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	if len(dalleResp.Data) == 0 {
		return "", errors.New("no image URL returned")
	}
	return dalleResp.Data[0].URL, nil
}

// Validate checks if the request parameters are valid.
// It also fills in default values for missing fields.
func (r *ReqDalle) Validate() error {
	if r.Prompt == "" {
		return errors.New("prompt is required")
	}
	// Check if both size and dimensions are provided
	if r.Size != "" && (r.Width != 0 || r.Height != 0) {
		return errors.New("cannot specify both size and dimensions (width/height)")
	}
	// Validate size if provided
	if r.Size != "" {
		if !isValidSize(r.Size) {
			return errors.New("invalid size: must be one of '1024x1024', '1792x1024', '1024x1792'")
		}
		return nil
	}
	// Validate width and height if provided
	if (r.Width == 0 && r.Height != 0) || (r.Width != 0 && r.Height == 0) {
		return errors.New("both width and height must be provided together")
	}
	if r.Width != 0 && r.Height != 0 && !isValidDimensions(r.Width, r.Height) {
		return errors.New("invalid dimensions: must be one of '1024x1024', '1792x1024', '1024x1792'")
	}
	if r.Size == "" {
		if r.Width != 0 && r.Height != 0 {
			r.Size = fmt.Sprintf("%dx%d", r.Width, r.Height)
		} else {
			r.Size = "1024x1024"
		}
	}
	r.Width, r.Height = 0, 0
	if r.Model == "" {
		r.Model = llm.ModelDalle3
	}
	if r.N == 0 {
		r.N = 1
	}
	if r.Quality == "" {
		r.Quality = "hd"
	}
	return nil
}

// isValidSize checks if the provided size string is valid.
func isValidSize(size string) bool {
	validSizes := map[string]bool{
		"1024x1024": true,
		"1792x1024": true,
		"1024x1792": true,
	}
	return validSizes[size]
}

// isValidDimensions checks if the provided width and height combination is valid.
func isValidDimensions(width, height int) bool {
	return (width == 1024 && height == 1024) ||
		(width == 1792 && height == 1024) ||
		(width == 1024 && height == 1792)
}
