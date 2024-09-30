package secr

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"

	"golang.org/x/sync/errgroup"
	"google.golang.org/api/secretmanager/v1"
)

var (
	BRAVE_SEARCH_API_KEY string
	SEARCH_API_KEY       string
	OPENAI_DALLE_API_KEY string
)

func fetchSecret(
	ctx context.Context,
	sm *secretmanager.Service,
	secName string,
	secPtr *string,
	group *errgroup.Group,
) {
	group.Go(func() error {
		var sb strings.Builder
		sb.WriteString("projects/22790208601/secrets/")
		sb.WriteString(secName)
		sb.WriteString("/versions/latest")
		sid := sb.String()
		s, err := sm.Projects.Secrets.Versions.Access(sid).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("failed to get secret %s: %w", sid, err)
		}
		decoded, err := base64.StdEncoding.DecodeString(s.Payload.Data)
		if err != nil {
			return fmt.Errorf("failed to decode secret %s: %w", sid, err)
		}
		*secPtr = string(decoded)
		slog.Debug("fetched secret", "id", sid)
		return nil
	})
}

func Setup(ctx context.Context) error {
	sm, err := secretmanager.NewService(ctx)
	if err != nil {
		return err
	}
	group, ctx := errgroup.WithContext(ctx)
	fetchSecret(ctx, sm, "BRAVE_SEARCH_API_KEY", &BRAVE_SEARCH_API_KEY, group)
	fetchSecret(ctx, sm, "SEARCH_API_KEY", &SEARCH_API_KEY, group)
	fetchSecret(ctx, sm, "OPENAI_DALLE_API_KEY", &OPENAI_DALLE_API_KEY, group)
	if err := group.Wait(); err != nil {
		return err
	}
	return nil
}
