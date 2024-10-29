package fbase

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"firebase.google.com/go/v4/auth"
	"github.com/ditto-assistant/backend/types/rq"
)

type Auth struct {
	client *auth.Client
}

func NewAuth(ctx context.Context) (Auth, error) {
	app, err := App(ctx)
	if err != nil {
		return Auth{}, err
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return Auth{}, err
	}
	return Auth{client: client}, nil
}

type AuthToken auth.Token

func (a *Auth) VerifyToken(r *http.Request) (*AuthToken, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, errors.New("authorization header is required but not provided")
	}
	const bearerPrefix = "bearer "
	if !strings.HasPrefix(strings.ToLower(authHeader), bearerPrefix) {
		return nil, errors.New("invalid authorization header format")
	}
	token := authHeader[len(bearerPrefix):]
	authToken, err := a.client.VerifyIDToken(r.Context(), token)
	if err != nil {
		return nil, fmt.Errorf("error verifying ID token: %v", err)
	}
	return (*AuthToken)(authToken), nil
}

func (r *AuthToken) Check(body rq.HasUserID) error {
	if r.UID != body.GetUserID() {
		return errors.New("user ID does not match")
	}
	return nil
}
