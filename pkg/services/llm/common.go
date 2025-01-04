package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/ditto-assistant/backend/types/ty"
	"golang.org/x/oauth2/google"
)

type StreamResponse struct {
	Text         <-chan Token
	InputTokens  int
	OutputTokens int
}

type Token = ty.Result[string]

func GetAccessToken(ctx context.Context) (string, error) {
	tokenSource, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("error getting token source: %w", err)
	}
	token, err := tokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("error getting token: %w", err)
	}
	return token.AccessToken, nil
}

func SendRequest(ctx context.Context, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	accessToken, err := GetAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("error response from API: status %d, body: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}
