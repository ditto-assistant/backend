package authfirebase

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"firebase.google.com/go/v4/auth"
)

type Client struct {
	Auth *auth.Client
}

func NewClient(auth *auth.Client) *Client {
	return &Client{Auth: auth}
}

type AuthToken auth.Token

func (a *Client) VerifyTokenFromForm(r *http.Request) (*AuthToken, error) {
	err := r.ParseForm()
	if err != nil {
		return nil, fmt.Errorf("error parsing form: %w", err)
	}
	token := r.FormValue("authorization")
	return a.verifyToken(r.Context(), token)
}

func (a *Client) VerifyToken(r *http.Request) (*AuthToken, error) {
	token := r.Header.Get("Authorization")
	return a.verifyToken(r.Context(), token)
}

func (r *AuthToken) Check(uid string) error {
	if r.UID != uid {
		return fmt.Errorf("user ID does not match: header: %s != body: %s", r.UID, uid)
	}
	return nil
}

func (a *Client) verifyToken(ctx context.Context, idToken string) (*AuthToken, error) {
	if idToken == "" {
		return nil, errors.New("authorization header is required but not provided")
	}
	const bearerPrefix = "bearer "
	if !strings.HasPrefix(strings.ToLower(idToken), bearerPrefix) {
		return nil, errors.New("invalid authorization header format")
	}
	token := idToken[len(bearerPrefix):]
	authToken, err := a.Auth.VerifyIDToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("error verifying ID token: %v", err)
	}
	return (*AuthToken)(authToken), nil
}
