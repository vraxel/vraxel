package oidc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
)

// Sentinel errors returned by AuthCodeStore.Consume.
var (
	ErrCodeNotFound    = errors.New("authorization code not found")
	ErrCodeExpired     = errors.New("authorization code expired")
	ErrCodeAlreadyUsed = errors.New("authorization code already used")
)

// AuthCodeStore persists single-use authorization codes. The production
// implementation is PostgreSQL-backed (NewPGAuthCodeStore); codes must
// be consumable exactly once across horizontally-scaled instances.
type AuthCodeStore interface {
	Store(ctx context.Context, code *AuthorizationCode)
	Consume(ctx context.Context, codeStr string) (*AuthorizationCode, error)
}

// GenerateCode returns a 64-hex-char cryptographically random code.
func GenerateCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
