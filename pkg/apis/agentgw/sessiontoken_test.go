package agentgw

import (
	"testing"
	"time"

	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

func TestSessionTokenRoundtrip(t *testing.T) {
	s := NewSessionTokenSigner([]byte("master-key"))
	tok, err := s.Issue(42, "inst-a", 1700000000000000)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := s.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.HostID != 42 || claims.Instance != "inst-a" || claims.ConnEpoch != 1700000000000000 {
		t.Fatalf("claims roundtrip mismatch: %+v", claims)
	}
}

func TestSessionTokenRejectsTamperAndForeignKey(t *testing.T) {
	s := NewSessionTokenSigner([]byte("master-key"))
	tok, _ := s.Issue(42, "inst-a", 1)

	if _, err := s.Parse(tok + "x"); err == nil {
		t.Fatal("tampered signature accepted")
	}
	// A different master key must not validate: session tokens are keyed
	// off the master key under their own domain separator.
	other := NewSessionTokenSigner([]byte("different-key"))
	if _, err := other.Parse(tok); err == nil {
		t.Fatal("token validated under a different key")
	}
	// An agent token (different prefix, different domain) must not parse
	// as a session token.
	at, _ := NewTokenSigner([]byte("master-key")).Issue(AgentClaims{AgentID: "a", HostID: 42, TokenVersion: 1})
	if _, err := s.Parse(at); err == nil {
		t.Fatal("agent token accepted as session token")
	}
}

func TestSessionTokenExpiry(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	s := NewSessionTokenSigner([]byte("master-key"))
	s.now = func() time.Time { return base }
	tok, _ := s.Issue(42, "inst-a", 1)

	// Just before expiry: still valid.
	s.now = func() time.Time { return base.Add(sessionTokenTTL - time.Second) }
	if _, err := s.Parse(tok); err != nil {
		t.Fatalf("token rejected before expiry: %v", err)
	}
	// After expiry: rejected.
	s.now = func() time.Time { return base.Add(sessionTokenTTL + time.Second) }
	if _, err := s.Parse(tok); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestSessionRowValid(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 123456000) // microsecond-precision
	claims := &SessionClaims{HostID: 42, ConnEpoch: epoch.UnixMicro()}

	online := &gwstore.AgentRow{Status: "online", ConnectedAt: &epoch}
	if !sessionRowValid(claims, online) {
		t.Fatal("valid online row with matching epoch rejected")
	}

	offline := &gwstore.AgentRow{Status: "offline", ConnectedAt: &epoch}
	if sessionRowValid(claims, offline) {
		t.Fatal("offline row accepted (channel is gone)")
	}

	reconnected := epoch.Add(time.Millisecond)
	stale := &gwstore.AgentRow{Status: "online", ConnectedAt: &reconnected}
	if sessionRowValid(claims, stale) {
		t.Fatal("token from a previous connection accepted after reconnect")
	}

	nilConn := &gwstore.AgentRow{Status: "online", ConnectedAt: nil}
	if sessionRowValid(claims, nilConn) {
		t.Fatal("row with nil connected_at accepted")
	}
}
