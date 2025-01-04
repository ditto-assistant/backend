package fbase

import (
	"context"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
)

type FirebaseApp struct {
	app       *firebase.App
	Auth      *auth.Client
	Firestore *firestore.Client
}

// NewApp returns a new Firebase app.
func NewApp(ctx context.Context) (*FirebaseApp, error) {
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
	return &FirebaseApp{app: app, Auth: auth, Firestore: firestore}, nil
}
