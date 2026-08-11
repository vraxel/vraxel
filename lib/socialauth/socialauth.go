// Package socialauth implements the outbound OAuth2 "login with GitHub /
// Google" flow as a relying party. It is deliberately dependency-free (plain
// net/http), matching the way lib/oidc self-implements the provider side.
//
// A Provider turns an authorization code into a verified UserProfile. The
// vraxel-specific parts (state storage, user provisioning, session minting)
// live in the caller (pkg/apis/iam social handlers); this package only speaks
// to the external provider.
package socialauth

import (
	"context"
	"net/http"
	"time"
)

// UserProfile is the normalized identity returned by every provider. Email is
// always a provider-verified address; providers reject accounts without one.
type UserProfile struct {
	Subject   string
	Email     string
	Name      string
	AvatarURL string
}

// Provider is one external identity source (GitHub, Google, ...).
type Provider interface {
	// Name is the stable provider key stored in user_identities.provider.
	Name() string
	// AuthCodeURL builds the provider's authorization URL to redirect the
	// browser to. redirectURI must exactly match one registered with the
	// provider's OAuth app.
	AuthCodeURL(state, redirectURI string) string
	// Authenticate exchanges the callback code for tokens and returns the
	// verified user profile.
	Authenticate(ctx context.Context, code, redirectURI string) (*UserProfile, error)
}

// httpClient is the shared client for all outbound provider calls; a short
// timeout keeps a hung provider from pinning a request goroutine.
var httpClient = &http.Client{Timeout: 15 * time.Second}
