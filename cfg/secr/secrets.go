package secr

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ditto-assistant/backend/cfg/envs"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/secretmanager/v1"
)

// Secrets
var (
	BRAVE_SEARCH_API_KEY      string
	SEARCH_API_KEY            string
	OPENAI_DALLE_API_KEY      string
	OPENAI_EMBEDDINGS_API_KEY string
	LIBSQL_ENCRYPTION_KEY     string
	TURSO_AUTH_TOKEN          string
)

func fetchSecret(
	ctx context.Context,
	group *errgroup.Group,
	sm *secretmanager.Service,
	secName string,
	secPtr *string,
) {
	group.Go(func() error {
		var sb strings.Builder
		sb.WriteString("projects/")
		sb.WriteString(envs.PROJECT_ID)
		sb.WriteString("/secrets/")
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
		return fmt.Errorf("failed to create secret manager: %w", err)
	}
	if err := envs.Load(); err != nil {
		return fmt.Errorf("failed to load environment variables: %w", err)
	}
	group, ctx := errgroup.WithContext(ctx)
	fetchSecret(ctx, group, sm, "BRAVE_SEARCH_API_KEY", &BRAVE_SEARCH_API_KEY)
	fetchSecret(ctx, group, sm, "SEARCH_API_KEY", &SEARCH_API_KEY)
	fetchSecret(ctx, group, sm, "OPENAI_DALLE_API_KEY", &OPENAI_DALLE_API_KEY)
	fetchSecret(ctx, group, sm, "LIBSQL_ENCRYPTION_KEY", &LIBSQL_ENCRYPTION_KEY)
	fetchSecret(ctx, group, sm, "OPENAI_EMBEDDINGS_API_KEY", &OPENAI_EMBEDDINGS_API_KEY)
	switch envs.DITTO_ENV {
	case envs.EnvStaging:
		fetchSecret(ctx, group, sm, "STAGING_TURSO_AUTH_TOKEN", &TURSO_AUTH_TOKEN)
	case envs.EnvProd:
		fetchSecret(ctx, group, sm, "PROD_TURSO_AUTH_TOKEN", &TURSO_AUTH_TOKEN)
	}
	if err := group.Wait(); err != nil {
		return fmt.Errorf("failed to fetch secrets: %w", err)
	}
	return nil
}
