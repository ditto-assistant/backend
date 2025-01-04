package core

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"firebase.google.com/go/v4/auth"
	"github.com/ditto-assistant/backend/types/rq"
)

type AuthToken auth.Token

func (a *Service) VerifyToken(r *http.Request) (*AuthToken, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, errors.New("authorization header is required but not provided")
	}
	const bearerPrefix = "bearer "
	if !strings.HasPrefix(strings.ToLower(authHeader), bearerPrefix) {
		return nil, errors.New("invalid authorization header format")
	}
	token := authHeader[len(bearerPrefix):]
	authToken, err := a.Auth.VerifyIDToken(r.Context(), token)
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
