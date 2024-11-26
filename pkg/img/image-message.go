package img

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
)

type ImageData struct {
	Base64    string
	MimeType  string
	MediaType string
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
