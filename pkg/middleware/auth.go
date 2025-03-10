package middleware

import (
	"context"
	"net/http"

	"github.com/ditto-assistant/backend/pkg/services/authfirebase"
)

type contextKey string

const contextKeyUserID contextKey = "userID"

// AuthMiddleware provides authentication middleware
type AuthMiddleware struct {
	auth *authfirebase.Client
}

// NewAuth creates a new authentication middleware
func NewAuth(auth *authfirebase.Client) *AuthMiddleware {
	return &AuthMiddleware{
		auth: auth,
	}
}

// Handler wraps an HTTP handler with authentication middleware
func (m *AuthMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Firebase token
		token, err := m.auth.VerifyToken(r)
		if err != nil {
			http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}

		// Store user ID in context
		ctx := context.WithValue(r.Context(), contextKeyUserID, token.UID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserIDFromContext extracts the user ID from the context.
// It panics if the user ID is not found in the context.
func GetUserIDFromContext(ctx context.Context) string {
	userID, ok := ctx.Value(contextKeyUserID).(string)
	if !ok {
		panic("user ID not found in context")
	}
	return userID
}
