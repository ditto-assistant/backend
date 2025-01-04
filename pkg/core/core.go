package core

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ditto-assistant/backend/pkg/core/firestoremem"
	"github.com/omniaura/mapcache"
)

type Service struct {
	app       *firebase.App
	Auth      *auth.Client
	Firestore *firestore.Client
	s3        *s3.S3
	urlCache  *mapcache.MapCache[string, string]
	Memories  *firestoremem.Client
}

const presignTTL = 24 * time.Hour

// NewService returns a new Firebase app.
func NewService(ctx context.Context) (*Service, error) {
	urlCache, err := mapcache.New[string, string](
		mapcache.WithTTL(presignTTL/2),
		mapcache.WithCleanup(ctx, presignTTL),
	)
	if err != nil {
		panic(err)
	}
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
	return &Service{
		app:       app,
		Auth:      auth,
		Firestore: firestore,
		urlCache:  urlCache,
		Memories:  firestoremem.NewClient(firestore),
	}, nil
}
