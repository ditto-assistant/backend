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

type SecretID string

// Secrets
var (
	BACKBLAZE_API_KEY         SecretID
	BRAVE_SEARCH_API_KEY      SecretID
	SEARCH_API_KEY            SecretID
	OPENAI_DALLE_API_KEY      SecretID
	OPENAI_EMBEDDINGS_API_KEY SecretID
	LIBSQL_ENCRYPTION_KEY     SecretID
	TURSO_AUTH_TOKEN          SecretID
	STRIPE_SECRET_KEY         SecretID
	STRIPE_WEBHOOK_SECRET     SecretID
)

func (s SecretID) String() string { return string(s) }

func (secPtr *SecretID) fetch(
	ctx context.Context,
	group *errgroup.Group,
	sm *secretmanager.Service,
	secName string,
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
			return fmt.Errorf("failed to get secret: %s: %w", sid, err)
		}
		decoded, err := base64.StdEncoding.DecodeString(s.Payload.Data)
		if err != nil {
			return fmt.Errorf("failed to decode secret: %s: %w", sid, err)
		}
		*secPtr = SecretID(decoded)
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
	BACKBLAZE_API_KEY.fetch(ctx, group, sm, "BACKBLAZE_API_KEY")
	BRAVE_SEARCH_API_KEY.fetch(ctx, group, sm, "BRAVE_SEARCH_API_KEY")
	SEARCH_API_KEY.fetch(ctx, group, sm, "SEARCH_API_KEY")
	OPENAI_DALLE_API_KEY.fetch(ctx, group, sm, "OPENAI_DALLE_API_KEY")
	LIBSQL_ENCRYPTION_KEY.fetch(ctx, group, sm, "LIBSQL_ENCRYPTION_KEY")
	OPENAI_EMBEDDINGS_API_KEY.fetch(ctx, group, sm, "OPENAI_EMBEDDINGS_API_KEY")
	switch envs.DITTO_ENV {
	case envs.EnvLocal:
		STRIPE_SECRET_KEY.fetch(ctx, group, sm, "LOCAL_STRIPE_SECRET_KEY")
		STRIPE_WEBHOOK_SECRET.fetch(ctx, group, sm, "LOCAL_STRIPE_WEBHOOK_SECRET")
	case envs.EnvStaging:
		STRIPE_SECRET_KEY.fetch(ctx, group, sm, "STAGING_STRIPE_SECRET_KEY")
		STRIPE_WEBHOOK_SECRET.fetch(ctx, group, sm, "STAGING_STRIPE_WEBHOOK_SECRET")
		TURSO_AUTH_TOKEN.fetch(ctx, group, sm, "STAGING_TURSO_AUTH_TOKEN")
	case envs.EnvProd:
		STRIPE_SECRET_KEY.fetch(ctx, group, sm, "PROD_STRIPE_SECRET_KEY")
		STRIPE_WEBHOOK_SECRET.fetch(ctx, group, sm, "PROD_STRIPE_WEBHOOK_SECRET")
		TURSO_AUTH_TOKEN.fetch(ctx, group, sm, "PROD_TURSO_AUTH_TOKEN")
	}
	if err := group.Wait(); err != nil {
		return err
	}
	return nil
}
