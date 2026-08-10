package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// RefreshTokenData holds the data associated with a refresh token.
type RefreshTokenData struct {
	TokenHash string
	UserID    int64
	ClientID  string
	Scope     string
	ExpiresAt time.Time
}

// RefreshTokenStore manages refresh tokens (OIDC side interface).
type RefreshTokenStore interface {
	Store(ctx context.Context, rt *RefreshTokenData) error
	Consume(ctx context.Context, rawToken string) (*RefreshTokenData, error)
	RevokeByUser(ctx context.Context, userID int64) error
}

// GenerateRefreshToken generates a cryptographically random refresh token.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken computes SHA-256 hash of a raw token for storage.
func HashToken(rawToken string) string {
	h := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(h[:])
}
