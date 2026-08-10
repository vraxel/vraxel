package config

import (
	"strings"
	"testing"
)

// Zero-config boot: no file, no env. Every security-relevant default
// must land so the server comes up with authentication ON.
func TestSetDefaultsZeroConfig(t *testing.T) {
	cfg := &Config{}
	SetDefaults(cfg)

	if cfg.Server.ExternalURL != "http://localhost:8088" {
		t.Fatalf("externalUrl default = %q", cfg.Server.ExternalURL)
	}
	if cfg.OIDC.Issuer != "http://localhost:8088" {
		t.Fatalf("issuer must derive from externalUrl, got %q", cfg.OIDC.Issuer)
	}
	if len(cfg.OIDC.Clients) != 1 || cfg.OIDC.Clients[0].ID != "vraxel-ui" {
		t.Fatalf("default client missing: %+v", cfg.OIDC.Clients)
	}
	uris := cfg.OIDC.Clients[0].RedirectURIs
	if len(uris) != 2 || uris[0] != "http://localhost:8088/auth/callback" {
		t.Fatalf("redirect URIs must be full URLs (exact-match validated), got %v", uris)
	}
	if err := cfg.Server.Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
}

// SetDefaults runs after the override layers; derived fields must see
// the effective externalUrl, and explicit values must never be touched.
func TestSetDefaultsDerivesFromOverriddenExternalURL(t *testing.T) {
	cfg := &Config{}
	cfg.Server.ExternalURL = "https://vraxel.example.com/" // e.g. via SERVER_EXTERNAL_URL
	SetDefaults(cfg)

	if cfg.OIDC.Issuer != "https://vraxel.example.com/" {
		t.Fatalf("issuer = %q, want the overridden externalUrl", cfg.OIDC.Issuer)
	}
	if got := cfg.OIDC.Clients[0].RedirectURIs[0]; got != "https://vraxel.example.com/auth/callback" {
		t.Fatalf("redirect URI = %q, trailing slash must not double", got)
	}

	explicit := &Config{}
	explicit.Server.ExternalURL = "https://vraxel.example.com"
	explicit.OIDC.Issuer = "https://sso.example.com"
	SetDefaults(explicit)
	if explicit.OIDC.Issuer != "https://sso.example.com" {
		t.Fatalf("explicit issuer overwritten: %q", explicit.OIDC.Issuer)
	}
}

func TestServerNameValidate(t *testing.T) {
	for _, bad := range []string{"", "has space", "läbel", strings.Repeat("x", 33)} {
		c := &ServerConfig{Name: bad}
		if err := c.Validate(); err == nil {
			t.Fatalf("name %q must be rejected", bad)
		}
	}
	c := &ServerConfig{Name: "vraxel-dev_1"}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid name rejected: %v", err)
	}
}
