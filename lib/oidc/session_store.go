package oidc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Session represents an authenticated browser session at the provider.
type Session struct {
	SessionID string
	UserID    int64
	AuthTime  time.Time
	ExpiresAt time.Time
}

// SessionStore persists provider sessions. The production implementation
// is PostgreSQL-backed (NewPGSessionStore); sessions must be shared
// across horizontally-scaled instances.
type SessionStore interface {
	Create(ctx context.Context, session *Session)
	Get(ctx context.Context, sessionID string) (*Session, error)
	Delete(ctx context.Context, sessionID string)
}

// GenerateSessionID returns a 64-hex-char cryptographically random id.
func GenerateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
