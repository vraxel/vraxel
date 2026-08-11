package socialauth

import (
	"net/url"
	"strings"
	"testing"
)

func TestGitHubAuthCodeURL(t *testing.T) {
	g := NewGitHub("cid", "secret", []string{"read:user", "user:email"})
	raw := g.AuthCodeURL("st8", "https://app.example/oidc/social/github/callback")
	if !strings.HasPrefix(raw, githubAuthorizeURL+"?") {
		t.Fatalf("unexpected base: %s", raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	if q.Get("client_id") != "cid" || q.Get("state") != "st8" {
		t.Fatalf("bad params: %v", q)
	}
	if q.Get("scope") != "read:user user:email" {
		t.Fatalf("bad scope: %q", q.Get("scope"))
	}
	if q.Get("redirect_uri") != "https://app.example/oidc/social/github/callback" {
		t.Fatalf("bad redirect_uri: %q", q.Get("redirect_uri"))
	}
}

func TestGoogleAuthCodeURL(t *testing.T) {
	g := NewGoogle("cid", "secret", nil) // nil -> default scopes
	u, err := url.Parse(g.AuthCodeURL("xyz", "https://app.example/oidc/social/google/callback"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	if q.Get("response_type") != "code" {
		t.Fatalf("missing response_type: %v", q)
	}
	if q.Get("scope") != "openid email profile" {
		t.Fatalf("bad default scope: %q", q.Get("scope"))
	}
}

func TestNames(t *testing.T) {
	if NewGitHub("", "", nil).Name() != "github" {
		t.Fatal("github name")
	}
	if NewGoogle("", "", nil).Name() != "google" {
		t.Fatal("google name")
	}
}
