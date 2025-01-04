package core

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ditto-assistant/backend/pkg/core/filestorage"
	"github.com/ditto-assistant/backend/pkg/core/firestoremem"
)

type Client struct {
	app         *firebase.App
	Auth        *auth.Client
	Firestore   *firestore.Client
	S3          *s3.S3
	Memories    *firestoremem.Client
	FileStorage *filestorage.Client
}

const presignTTL = 24 * time.Hour

func NewClient(ctx context.Context) (*Client, error) {
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return nil, err
	}
	auth, err := app.Auth(ctx)
	if err != nil {
		return nil, err
	}
	firestore, err := app.Firestore(ctx)
	if err != nil {
		return nil, err
	}
	fsClient, err := filestorage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &Client{
		app:         app,
		Auth:        auth,
		Firestore:   firestore,
		Memories:    firestoremem.NewClient(firestore, fsClient),
		FileStorage: fsClient,
	}, nil
}
