package secr

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/ditto-assistant/backend/cfg/envs"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/secretmanager/v1"
)

type SecretID string

// Secrets
var (
	BACKBLAZE_API_KEY         SecretID
	OPENAI_DALLE_API_KEY      SecretID
	OPENAI_LLM_API_KEY        SecretID
	OPENAI_EMBEDDINGS_API_KEY SecretID
	LIBSQL_ENCRYPTION_KEY     SecretID
	TURSO_AUTH_TOKEN          SecretID
)

func (s SecretID) String() string { return string(s) }

// FetchEnv fetches a secret from the secret manager
// with the environment name prepended to the secret name
func (cl *Client) FetchEnv(
	ctx context.Context,
	secName string,
) (string, error) {
	secName = strings.ToUpper(envs.DITTO_ENV.String()) + "_" + secName
	return cl.Fetch(ctx, secName)
}

// Fetch fetches a secret from the secret manager
func (cl *Client) Fetch(
	ctx context.Context,
	secName string,
) (string, error) {
	var sb strings.Builder
	sb.WriteString("projects/")
	sb.WriteString(envs.PROJECT_ID)
	sb.WriteString("/secrets/")
	sb.WriteString(secName)
	sb.WriteString("/versions/latest")
	sid := sb.String()
	s, err := cl.sm.Projects.Secrets.Versions.Access(sid).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("failed to get secret: %s: %w", sid, err)
	}
	decoded, err := base64.StdEncoding.DecodeString(s.Payload.Data)
	if err != nil {
		return "", fmt.Errorf("failed to decode secret: %s: %w", sid, err)
	}
	return string(decoded), nil
}

func (secPtr *SecretID) fetch(
	ctx context.Context,
	group *errgroup.Group,
	cl *Client,
	secName string,
) {
	group.Go(func() error {
		sec, err := cl.Fetch(ctx, secName)
		if err != nil {
			return err
		}
		*secPtr = SecretID(sec)
		return nil
	})
}

type Client struct {
	sm *secretmanager.Service
}

func Setup(ctx context.Context) (*Client, error) {
	sm, err := secretmanager.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret manager: %w", err)
	}
	if err := envs.Load(); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}
	cl := &Client{sm: sm}
	group, ctx := errgroup.WithContext(ctx)
	BACKBLAZE_API_KEY.fetch(ctx, group, cl, "BACKBLAZE_API_KEY")
	OPENAI_DALLE_API_KEY.fetch(ctx, group, cl, "OPENAI_DALLE_API_KEY")
	LIBSQL_ENCRYPTION_KEY.fetch(ctx, group, cl, "LIBSQL_ENCRYPTION_KEY")
	OPENAI_EMBEDDINGS_API_KEY.fetch(ctx, group, cl, "OPENAI_EMBEDDINGS_API_KEY")
	OPENAI_LLM_API_KEY.fetch(ctx, group, cl, "OPENAI_LLM_API_KEY")
	switch envs.DITTO_ENV {
	case envs.EnvStaging:
		TURSO_AUTH_TOKEN.fetch(ctx, group, cl, "STAGING_TURSO_AUTH_TOKEN")
	case envs.EnvProd:
		TURSO_AUTH_TOKEN.fetch(ctx, group, cl, "PROD_TURSO_AUTH_TOKEN")
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return cl, nil
}
