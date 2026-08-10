package oidc

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// Cookie names for the BFF (HttpOnly-cookie) flow.
const (
	CookieAccessToken  = "vraxel_at"
	CookieRefreshToken = "vraxel_rt"
	CookieCSRFToken    = "vraxel_csrf"
	RefreshCookiePath  = "/oidc/token"
)

// GenerateCSRFToken returns a fresh, random, URL-safe CSRF token.
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// IsSecureRequest reports whether the request was received over TLS or a
// TLS-terminating proxy indicated so via X-Forwarded-Proto.
func IsSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return r.Header.Get("X-Forwarded-Proto") == "https"
}
