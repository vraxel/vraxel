package audit

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
)

// trustedProxies holds the peer ranges allowed to set forwarding
// headers. Nil/empty means "trust nobody", which is the safe default.
var trustedProxies atomic.Pointer[[]netip.Prefix]

// SetTrustedProxies declares which immediate peers may set
// X-Forwarded-For / X-Real-IP. Called once at boot from the server
// config.
//
// Empty (the default) makes ClientIP ignore those headers entirely.
// This matters beyond audit accuracy: ClientIP keys per-IP security
// controls such as the login brute-force throttle, and any client can
// forge a header. Honouring them unconditionally would let an attacker
// rotate their bucket on every request and neutralise the control.
func SetTrustedProxies(prefixes []netip.Prefix) {
	cp := append([]netip.Prefix(nil), prefixes...)
	trustedProxies.Store(&cp)
}

func isTrusted(addr netip.Addr) bool {
	p := trustedProxies.Load()
	if p == nil {
		return false
	}
	for _, prefix := range *p {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// ClientIP returns the effective client IP.
//
// When the immediate peer is a configured trusted proxy, the
// X-Forwarded-For chain is walked right-to-left and the first address
// that is NOT itself trusted is returned -- the last hop the trusted
// infrastructure actually observed, and the furthest left an attacker
// cannot control. Otherwise the peer address is returned and forwarding
// headers are ignored.
func ClientIP(r *http.Request) string {
	peer := remoteAddrHost(r)
	addr, err := netip.ParseAddr(peer)
	if err != nil || !isTrusted(addr) {
		return peer
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			hop, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
			if err != nil {
				continue
			}
			if !isTrusted(hop) {
				return hop.String()
			}
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if hop, err := netip.ParseAddr(xri); err == nil {
			return hop.String()
		}
	}
	return peer
}

func remoteAddrHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
