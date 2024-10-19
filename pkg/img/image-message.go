package img

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"

	"github.com/firebase/genkit/go/ai"
)

func NewPart(ctx context.Context, url string) (*ai.Part, error) {
	img, err := GetBase64(ctx, url)
	if err != nil {
		return nil, err
	}
	return ai.NewMediaPart("", "data:image/jpeg;base64,"+img), nil
}

func GetBase64(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	img, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(img), nil
}
