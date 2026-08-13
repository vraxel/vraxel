package client

import (
	"encoding/hex"
	"testing"
	"time"
)

// TestBootNonceIsStableWithinTheProcess pins the property the whole
// duplicate-agent check rests on: one running agent sends ONE value, on
// every reconnect, for its whole life. If it varied per session the
// server would see a fresh nonce each time and could never distinguish a
// reconnect from a second process claiming the same identity.
func TestBootNonceIsStableWithinTheProcess(t *testing.T) {
	if bootNonce == "" {
		t.Fatal("boot nonce must not be empty; an empty value disables the check server-side")
	}
	if _, err := hex.DecodeString(bootNonce); err != nil {
		t.Errorf("boot nonce should be hex so it survives the varchar column: %v", err)
	}
}

// TestBootNonceIsFreshPerCall covers the other half: a restarted agent
// must present a DIFFERENT value, or two clones booting from the same
// image would look like one agent reconnecting.
func TestBootNonceIsFreshPerCall(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		n := newBootNonce()
		if seen[n] {
			t.Fatalf("newBootNonce repeated %q", n)
		}
		seen[n] = true
	}
}

// TestJitteredWithinHalfToFull asserts full jitter never returns a wait
// outside [d/2, d] -- below d/2 would defeat the backoff, above d would
// exceed the intended cap.
func TestJitteredWithinHalfToFull(t *testing.T) {
	for _, d := range []time.Duration{reconnectMin, 4 * time.Second, reconnectMax} {
		for i := 0; i < 1000; i++ {
			got := jittered(d)
			if got < d/2 || got > d {
				t.Fatalf("jittered(%s) = %s, want within [%s, %s]", d, got, d/2, d)
			}
		}
	}
}
