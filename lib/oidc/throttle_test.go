package oidc

import (
	"testing"
	"time"
)

func TestThrottleDecision(t *testing.T) {
	now := time.Now()
	window := 15 * time.Minute

	cases := []struct {
		name        string
		count       int
		windowStart time.Time
		locked      bool
	}{
		{"under limit", 4, now, false},
		{"at limit, fresh window", 5, now.Add(-time.Minute), true},
		{"at limit, window elapsed", 5, now.Add(-16 * time.Minute), false},
		{"over limit, mid window", 9, now.Add(-14 * time.Minute), true},
		{"zero", 0, time.Time{}, false},
	}
	for _, c := range cases {
		retry, locked := throttleDecision(c.count, c.windowStart, 5, window, now)
		if locked != c.locked {
			t.Fatalf("%s: locked = %v, want %v", c.name, locked, c.locked)
		}
		if locked && (retry <= 0 || retry > window) {
			t.Fatalf("%s: retryAfter %v out of (0, %v]", c.name, retry, window)
		}
	}
}

// A provider without a throttle store must not throttle (unit-test
// construction path) and must not panic on the note calls.
func TestThrottleNilStoreIsOpen(t *testing.T) {
	p := &Provider{}
	if _, locked := p.LoginLocked(t.Context(), "admin", "1.2.3.4"); locked {
		t.Fatal("nil store must fail open")
	}
	p.NoteLoginFailure(t.Context(), "admin", "1.2.3.4")
	p.NoteLoginSuccess(t.Context(), "admin")
}
