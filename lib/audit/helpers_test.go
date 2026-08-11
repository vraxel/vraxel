package audit

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func req(remote, xff, xri string) *http.Request {
	r := httptest.NewRequest("POST", "/oidc/login", nil)
	r.RemoteAddr = remote
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	if xri != "" {
		r.Header.Set("X-Real-IP", xri)
	}
	return r
}

func mustPrefixes(t *testing.T, cidrs ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		p, err := netip.ParsePrefix(c)
		if err != nil {
			t.Fatalf("parse %q: %v", c, err)
		}
		out = append(out, p)
	}
	return out
}

// The security-critical case: with no trusted proxies, a forged header
// must not change the result, or every per-IP control (login throttle)
// becomes bypassable by sending a random X-Forwarded-For.
func TestClientIPIgnoresForgedHeadersByDefault(t *testing.T) {
	SetTrustedProxies(nil)
	t.Cleanup(func() { SetTrustedProxies(nil) })

	if got := ClientIP(req("203.0.113.9:5555", "1.2.3.4", "5.6.7.8")); got != "203.0.113.9" {
		t.Fatalf("got %q, want the peer address", got)
	}
}

func TestClientIPHonoursTrustedProxy(t *testing.T) {
	SetTrustedProxies(mustPrefixes(t, "10.0.0.0/8"))
	t.Cleanup(func() { SetTrustedProxies(nil) })

	cases := []struct {
		name, remote, xff, xri, want string
	}{
		{"single hop", "10.1.1.5:1234", "198.51.100.7", "", "198.51.100.7"},
		// Rightmost untrusted hop: the attacker prepends 1.2.3.4, but the
		// proxy appended the address it actually saw.
		{"forged prefix in chain", "10.1.1.5:1234", "1.2.3.4, 198.51.100.7", "", "198.51.100.7"},
		{"chained trusted proxies", "10.1.1.5:1234", "198.51.100.7, 10.9.9.9", "", "198.51.100.7"},
		{"x-real-ip fallback", "10.1.1.5:1234", "", "198.51.100.7", "198.51.100.7"},
		{"untrusted peer ignores header", "203.0.113.9:1234", "198.51.100.7", "", "203.0.113.9"},
		{"all hops trusted falls back to peer", "10.1.1.5:1234", "10.2.2.2", "", "10.1.1.5"},
	}
	for _, c := range cases {
		if got := ClientIP(req(c.remote, c.xff, c.xri)); got != c.want {
			t.Fatalf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
