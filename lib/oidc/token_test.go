package oidc

import (
	"testing"
	"time"
)

func TestTokenService_RoundTrip(t *testing.T) {
	algorithms := []string{"EdDSA", "ES256", "RS256"}
	for _, alg := range algorithms {
		t.Run(alg, func(t *testing.T) {
			ks, err := GenerateKeySet(alg)
			if err != nil {
				t.Fatalf("GenerateKeySet: %v", err)
			}
			ts := NewTokenService(ks, "https://test.example.com", time.Hour)

			token, err := ts.IssueAccessToken(42, "test-client", []string{"openid", "profile"})
			if err != nil {
				t.Fatalf("IssueAccessToken: %v", err)
			}
			if token == "" {
				t.Fatal("token is empty")
			}

			claims, err := ts.VerifyAccessToken(token)
			if err != nil {
				t.Fatalf("VerifyAccessToken: %v", err)
			}
			if claims.Subject != "42" {
				t.Errorf("Subject = %q, want 42", claims.Subject)
			}
			if claims.Scope != "openid profile" {
				t.Errorf("Scope = %q, want 'openid profile'", claims.Scope)
			}
		})
	}
}

func TestTokenService_IDToken(t *testing.T) {
	algorithms := []string{"EdDSA", "ES256", "RS256"}
	for _, alg := range algorithms {
		t.Run(alg, func(t *testing.T) {
			ks, _ := GenerateKeySet(alg)
			ts := NewTokenService(ks, "https://test.example.com", time.Hour)

			at, _ := ts.IssueAccessToken(1, "client", []string{"openid"})
			user := &UserInfo{Name: "Test", Email: "test@example.com"}
			idToken, err := ts.IssueIDToken(1, "client", "nonce123", time.Now(), at, user, []string{"openid", "profile", "email"})
			if err != nil {
				t.Fatalf("IssueIDToken: %v", err)
			}
			if idToken == "" {
				t.Fatal("idToken is empty")
			}
		})
	}
}

func TestTokenService_WrongKey(t *testing.T) {
	ks1, _ := GenerateKeySet("EdDSA")
	ks2, _ := GenerateKeySet("EdDSA")
	ts1 := NewTokenService(ks1, "https://test.example.com", time.Hour)
	ts2 := NewTokenService(ks2, "https://test.example.com", time.Hour)

	token, _ := ts1.IssueAccessToken(1, "client", nil)
	_, err := ts2.VerifyAccessToken(token)
	if err == nil {
		t.Error("expected error verifying with wrong key")
	}
}
