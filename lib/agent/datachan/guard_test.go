package datachan

import (
	"net"
	"strings"
	"testing"
)

// TestResolveReturnsALiteralIP is the anti-rebinding contract: callers
// must dial what Resolve hands back, and what it hands back must not need
// resolving again.
func TestResolveReturnsALiteralIP(t *testing.T) {
	got, err := NewGuard(nil).Resolve("localhost:8848")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	host, port, err := net.SplitHostPort(got)
	if err != nil {
		t.Fatalf("Resolve returned %q, which is not host:port", got)
	}
	if port != "8848" {
		t.Fatalf("port = %s, want 8848", port)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		t.Fatalf("Resolve returned the name %q instead of an address; dialling it would resolve a second time", host)
	}
	if !ip.IsLoopback() {
		t.Fatalf("Resolve returned non-loopback %s", ip)
	}
}

func TestResolveRejectsNonLoopback(t *testing.T) {
	if _, err := NewGuard(nil).Resolve("10.1.1.10:5432"); err == nil {
		t.Fatal("a non-loopback target was accepted")
	}
}

func TestResolveRejectsMalformedTarget(t *testing.T) {
	g := NewGuard(nil)
	for _, target := range []string{"127.0.0.1", "127.0.0.1:0", "127.0.0.1:70000", "127.0.0.1:http"} {
		if _, err := g.Resolve(target); err == nil {
			t.Fatalf("%q was accepted", target)
		}
	}
}

// TestServerCannotWidenTheOperatorAllowlist is the point of keeping the
// two lists apart: the allowlist exists to limit a compromised server, so
// that server must not be able to unlock ports the operator excluded.
func TestServerCannotWidenTheOperatorAllowlist(t *testing.T) {
	g := NewGuard([]int{9100})

	// An empty push must not reset the guard to "any port".
	g.SetPorts(nil)
	if _, err := g.Resolve("127.0.0.1:5432"); err == nil {
		t.Fatal("an empty pushed list widened the operator's allowlist")
	}
	if _, err := g.Resolve("127.0.0.1:9100"); err != nil {
		t.Fatalf("the operator's own port was refused: %v", err)
	}

	// A push naming other ports narrows to the intersection, which here
	// is empty.
	g.SetPorts([]int{3306, 5432})
	if _, err := g.Resolve("127.0.0.1:3306"); err == nil {
		t.Fatal("a pushed port outside the operator's list was allowed")
	}
	if _, err := g.Resolve("127.0.0.1:9100"); err == nil {
		t.Fatal("a port the server dropped is still allowed")
	}

	// Overlap is what survives.
	g.SetPorts([]int{9100, 3306})
	if _, err := g.Resolve("127.0.0.1:9100"); err != nil {
		t.Fatalf("the overlapping port was refused: %v", err)
	}
	if ports := g.Ports(); len(ports) != 1 || ports[0] != 9100 {
		t.Fatalf("Ports() = %v, want [9100]", ports)
	}
}

// TestPushedAllowlistAppliesWithoutAnOperatorList covers the deployed
// default, where -allow-ports is unset.
func TestPushedAllowlistAppliesWithoutAnOperatorList(t *testing.T) {
	g := NewGuard(nil)
	if _, err := g.Resolve("127.0.0.1:5432"); err != nil {
		t.Fatalf("any loopback port should be reachable before a push: %v", err)
	}
	g.SetPorts([]int{9100})
	_, err := g.Resolve("127.0.0.1:5432")
	if err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("err = %v, want an allowlist rejection", err)
	}
}
