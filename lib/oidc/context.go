package oidc

import (
	"context"
	"net/http"
)

type contextKey int

const (
	userIDKey contextKey = iota
	usernameKey
)

// WithUserID returns a new request with the user ID stored in context.
func WithUserID(r *http.Request, id int64) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userIDKey, id))
}

// UserIDFromContext extracts the authenticated user ID from context.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey).(int64)
	return id, ok
}

// WithUsername returns a new request with the username stored in context.
func WithUsername(r *http.Request, username string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), usernameKey, username))
}

// UsernameFromContext extracts the authenticated username from context.
func UsernameFromContext(ctx context.Context) string {
	s, _ := ctx.Value(usernameKey).(string)
	return s
}
