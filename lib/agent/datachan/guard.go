package datachan

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync"

	"vraxel.io/vraxel/lib/agent/loopback"
)

// Guard decides which targets a tcp stream may reach.
//
// Two rules, in order of strength:
//
//  1. The target must be a loopback address. This is unconditional and
//     not configurable. It is the property the whole agent design rests
//     on (design §4.3): an attacker who owns the server still cannot use
//     an agent to reach anything else in the customer's network, which is
//     what distinguishes this from a VPN or an overlay.
//
//  2. The port must be in the allowlist, when one has been set. The
//     server derives that list from what it knows about the host (ssh
//     port, agent port, the service ports the platform itself deployed there,
//     6443 on a k8s node) and pushes it; until it does, any loopback port
//     is reachable. Rule 1 already bounds the blast radius to processes
//     on the host itself, so this is defence in depth, not the barrier.
type Guard struct {
	mu sync.RWMutex
	// operator is the allowlist from this agent's own command line, and
	// server is the one pushed over the control channel. They are kept
	// apart so a pushed list can only narrow what the operator set: the
	// allowlist exists to limit a compromised server, so letting that
	// server widen it -- which an empty pushed list would do -- inverts
	// the point of having it.
	operator map[int]struct{}
	server   map[int]struct{}
}

// NewGuard builds a guard with the operator's allowlist. A nil or empty
// slice means "any loopback port".
func NewGuard(ports []int) *Guard {
	return &Guard{operator: portSet(ports)}
}

// SetPorts records the server-pushed allowlist. It never widens the
// effective set beyond the operator's.
func (g *Guard) SetPorts(ports []int) {
	set := portSet(ports)
	g.mu.Lock()
	g.server = set
	g.mu.Unlock()
}

// Ports reports the effective allowlist, sorted. Empty means any port.
func (g *Guard) Ports() []int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]int, 0)
	for p := range g.effectiveLocked() {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

// effectiveLocked is the intersection of whichever lists are set. Nil
// means unrestricted.
func (g *Guard) effectiveLocked() map[int]struct{} {
	switch {
	case g.operator == nil:
		return g.server
	case g.server == nil:
		return g.operator
	default:
		out := map[int]struct{}{}
		for p := range g.server {
			if _, ok := g.operator[p]; ok {
				out[p] = struct{}{}
			}
		}
		return out
	}
}

// Resolve validates a host:port target and returns the address a stream
// may dial: the same port, and a verified loopback IP.
//
// It returns the resolved address rather than approving the caller's
// string because the caller must not resolve the name a second time; see
// loopback.Resolve.
func (g *Guard) Resolve(target string) (string, error) {
	_, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return "", fmt.Errorf("target %q is not host:port", target)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("target %q has an invalid port", target)
	}

	g.mu.RLock()
	allowed := g.effectiveLocked()
	g.mu.RUnlock()
	if allowed != nil {
		if _, ok := allowed[port]; !ok {
			return "", fmt.Errorf("port %d is not in the allowlist", port)
		}
	}
	return loopback.Resolve(target)
}

func portSet(ports []int) map[int]struct{} {
	if len(ports) == 0 {
		return nil
	}
	set := make(map[int]struct{}, len(ports))
	for _, p := range ports {
		set[p] = struct{}{}
	}
	return set
}
