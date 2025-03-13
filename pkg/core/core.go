package core

import (
	"context"

	firebase "firebase.google.com/go/v4"
	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/services/authfirebase"
	"github.com/ditto-assistant/backend/pkg/services/filestorage"
	"github.com/ditto-assistant/backend/pkg/services/firestoremem"
	"github.com/ditto-assistant/backend/pkg/services/llm/googai"
)

type Client struct {
	Secr        *secr.Client
	Auth        *authfirebase.Client
	Memories    *firestoremem.Client
	FileStorage *filestorage.Client
	Embedder    *googai.Client
}

func NewClient(ctx context.Context) (*Client, error) {
	secrClient, err := secr.Setup(ctx)
	if err != nil {
		return nil, err
	}
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return nil, err
	}
	auth, err := app.Auth(ctx)
	if err != nil {
		return nil, err
	}
	fbAuth := authfirebase.NewClient(auth)
	firestore, err := app.Firestore(ctx)
	if err != nil {
		return nil, err
	}
	fsClient, err := filestorage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	embedder, err := googai.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &Client{
		Secr:        secrClient,
		Auth:        fbAuth,
		Memories:    firestoremem.NewClient(firestore, fsClient),
		FileStorage: fsClient,
		Embedder:    embedder,
	}, nil
}
