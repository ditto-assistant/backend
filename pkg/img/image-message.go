package img

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"

	"github.com/firebase/genkit/go/ai"
)

type ImageData struct {
	Base64    string
	MimeType  string
	MediaType string
}

func NewPart(ctx context.Context, url string) (*ai.Part, error) {
	img, err := GetImageData(ctx, url)
	if err != nil {
		return nil, err
	}
	return ai.NewMediaPart("", "data:"+img.MimeType+";base64,"+img.Base64), nil
}

func GetImageData(ctx context.Context, url string) (*ImageData, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	img, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Detect content type
	contentType := http.DetectContentType(img)

	// Determine media type (e.g., "image/jpeg" -> "jpeg")
	mediaType := contentType[6:] // Remove "image/" prefix

	return &ImageData{
		Base64:    base64.StdEncoding.EncodeToString(img),
		MimeType:  contentType,
		MediaType: mediaType,
	}, nil
}

// GetBase64 is kept for backward compatibility
func GetBase64(ctx context.Context, url string) (string, error) {
	imgData, err := GetImageData(ctx, url)
	if err != nil {
		return "", err
	}
	return imgData.Base64, nil
}
