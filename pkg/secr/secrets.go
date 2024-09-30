package secr

import (
	"context"
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
	braveSearch, err := sm.Projects.Secrets.Versions.
		Access("projects/22790208601/secrets/BRAVE_SEARCH_API_KEY/versions/latest").Do()
	if err != nil {
		return err
	}
	BRAVE_SEARCH_API_KEY = braveSearch.Payload.Data

	search, err := sm.Projects.Secrets.Versions.
		Access("projects/22790208601/secrets/SEARCH_API_KEY/versions/latest").Do()
	if err != nil {
		return err
	}
	SEARCH_API_KEY = search.Payload.Data

	openaiDalle, err := sm.Projects.Secrets.Versions.
		Access("projects/22790208601/secrets/OPENAI_DALLE_API_KEY/versions/latest").Do()
	if err != nil {
		return err
	}
	OPENAI_DALLE_API_KEY = openaiDalle.Payload.Data
	slog.Debug("loaded secrets", "ids", []string{braveSearch.Name, search.Name, openaiDalle.Name})
	return nil
}
