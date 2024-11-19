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
		Prompt: r.Prompt,
		Model:  r.Model,
		Size:   r.Size,
		Width:  r.Width,
		Height: r.Height,
	}
	var err error
	r.Model, err = req.Build()
	if err != nil {
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

// Build checks if the request parameters are valid.
// It also fills in default values for missing fields.
func (r *ReqDalle) Build() (receiptModel llm.ServiceName, err error) {
	if r.Prompt == "" {
		err = errors.New("prompt is required")
		return
	}
	if r.Size != "" && (r.Width != 0 || r.Height != 0) {
		err = fmt.Errorf("cannot specify both size: %s and dimensions (width: %d, height: %d)", r.Size, r.Width, r.Height)
		return
	}
	sz, err := r.DimToSize()
	if err != nil {
		return "", err
	}
	if r.Size == "" {
		r.Size = sz
	}
	r.N = 1
	return r.ValidateSizeForModel()
}

func (r *ReqDalle) ValidateSizeForModel() (receiptModel llm.ServiceName, err error) {
	r.Width, r.Height = 0, 0
	switch r.Model {
	case llm.ModelDalle3HD:
		r.Model = llm.ModelDalle3
		r.Quality = "hd"
		switch r.Size {
		case "1024x1024":
			receiptModel = llm.ModelDalle3HD
		case "1792x1024":
			receiptModel = llm.ModelDalle3HDWide
		case "1024x1792":
			receiptModel = llm.ModelDalle3HDTall
		default:
			err = fmt.Errorf("invalid size: %s for model: %s", r.Size, r.Model)
			return
		}
	case llm.ModelDalle3:
		switch r.Size {
		case "1024x1024":
			receiptModel = llm.ModelDalle3
		case "1792x1024":
			receiptModel = llm.ModelDalle3Wide
		case "1024x1792":
			receiptModel = llm.ModelDalle3Tall
		default:
			err = fmt.Errorf("invalid size: %s for model: %s", r.Size, r.Model)
			return
		}
	case llm.ModelDalle2:
		switch r.Size {
		case "1024x1024":
			receiptModel = llm.ModelDalle2
		case "512x512":
			receiptModel = llm.ModelDalle2Small
		case "256x256":
			receiptModel = llm.ModelDalle2Tiny
		default:
			err = fmt.Errorf("invalid size: %s for model: %s", r.Size, r.Model)
			return
		}
	}
	return
}

func (r *ReqDalle) DimToSize() (string, error) {
	if r.Width == 0 && r.Height == 0 {
		return "", nil
	}
	if r.Width == 0 || r.Height == 0 {
		return "", fmt.Errorf("both width and height must be provided together (width: %d, height: %d)", r.Width, r.Height)
	}
	return fmt.Sprintf("%dx%d", r.Width, r.Height), nil
}
