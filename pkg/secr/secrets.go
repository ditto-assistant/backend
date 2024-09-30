package secr

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"

	"google.golang.org/api/secretmanager/v1"
)

var (
	BRAVE_SEARCH_API_KEY string
	SEARCH_API_KEY       string
	OPENAI_DALLE_API_KEY string
)

func Setup(ctx context.Context) error {
	sm, err := secretmanager.NewService(ctx)
	if err != nil {
		return err
	}

	braveSearch, err := sm.Projects.Locations.Secrets.Versions.
		Access("projects/22790208601/secrets/BRAVE_SEARCH_API_KEY/versions/latest").Do()
	if err != nil {
		return err
	}
	decodedBraveSearch, err := base64.StdEncoding.DecodeString(braveSearch.Payload.Data)
	if err != nil {
		return fmt.Errorf("failed to decode BRAVE_SEARCH_API_KEY: %w", err)
	}
	BRAVE_SEARCH_API_KEY = string(decodedBraveSearch)

	search, err := sm.Projects.Locations.Secrets.Versions.
		Access("projects/22790208601/secrets/SEARCH_API_KEY/versions/latest").Do()
	if err != nil {
		return err
	}
	decodedSearch, err := base64.StdEncoding.DecodeString(search.Payload.Data)
	if err != nil {
		return fmt.Errorf("failed to decode SEARCH_API_KEY: %w", err)
	}
	SEARCH_API_KEY = string(decodedSearch)

	openaiDalle, err := sm.Projects.Locations.Secrets.Versions.
		Access("projects/22790208601/secrets/OPENAI_DALLE_API_KEY/versions/latest").Do()
	if err != nil {
		return err
	}
	decodedOpenaiDalle, err := base64.StdEncoding.DecodeString(openaiDalle.Payload.Data)
	if err != nil {
		return fmt.Errorf("failed to decode OPENAI_DALLE_API_KEY: %w", err)
	}
	OPENAI_DALLE_API_KEY = string(decodedOpenaiDalle)

	slog.Debug("loaded secrets", "ids", []string{braveSearch.Name, search.Name, openaiDalle.Name})
	return nil
}
