package client

import (
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
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

// The sshd summary rides the accounts frame, and its access lists are
// the only part of it a configuration can make arbitrarily long. An
// oversize frame is refused outright, so an uncapped AllowUsers would
// cost the host its whole account inventory.
func TestCapSSHDLists(t *testing.T) {
	many := make([]string, 2000)
	for i := range many {
		many[i] = "serviceaccount-with-a-long-name"
	}
	cfg := &agenttypes.SSHDConfig{
		Ports: []int32{22}, AllowUsers: many, AllowGroups: many,
		DenyUsers: many, DenyGroups: many,
	}
	capSSHDLists(cfg, nopLogger{})

	for name, got := range map[string][]string{
		"allowUsers": cfg.AllowUsers, "allowGroups": cfg.AllowGroups,
		"denyUsers": cfg.DenyUsers, "denyGroups": cfg.DenyGroups,
	} {
		if len(got) == 0 || len(got) >= len(many) {
			t.Errorf("%s = %d entries, want it trimmed but not emptied", name, len(got))
		}
		b, _ := json.Marshal(got)
		if len(b) > accountsSSHDListBudget {
			t.Errorf("%s encodes to %d bytes, over the %d budget", name, len(b), accountsSSHDListBudget)
		}
	}
	// Nil is what a host without sshd reports, and it must not panic.
	capSSHDLists(nil, nopLogger{})
}
