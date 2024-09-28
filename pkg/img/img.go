package img

import (
	"encoding/base64"
	"io"
	"net/http"

	"github.com/firebase/genkit/go/ai"
)

func NewPart(url string) (*ai.Part, error) {
	img, err := getBase64(url)
	if err != nil {
		return nil, err
	}
	return ai.NewMediaPart("", "data:image/jpeg;base64,"+img), nil
}

func getBase64(url string) (string, error) {
	resp, err := http.Get(url)
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
