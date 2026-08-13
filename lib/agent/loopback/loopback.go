// Package loopback holds the one rule the agent's network safety rests
// on: a target the control plane names must be on this machine.
//
// It exists as its own package because two callers enforce it (the data
// channel's tcp streams and the metrics scraper) and a security
// invariant with two implementations eventually has two behaviours.
package loopback

import (
	"context"
	"fmt"
	"net"
)

// Resolve validates a host:port and returns an address that is safe to
// dial: the port as given, and a literal IP that has been checked.
//
// Returning the IP rather than the original string is the whole point.
// Checking a name and then dialling that same name resolves twice, and
// an attacker who controls DNS can answer 127.0.0.1 to the check and a
// remote address to the dial. Everything after this function must use
// the address it returns.
func Resolve(hostport string) (string, error) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return "", fmt.Errorf("target %q is not host:port", hostport)
	}
	ip, err := ResolveHost(host)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(ip, port), nil
}

// ResolveHost validates a host and returns the loopback IP to use.
//
// A name must resolve to loopback and to nothing else: accepting it
// because one of its answers is 127.0.0.1 would let a doctored
// /etc/hosts or a split DNS answer aim the connection at a neighbouring
// machine, which is exactly what this rule blocks.
func ResolveHost(host string) (string, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !ip.IsLoopback() {
			return "", fmt.Errorf("target %s is not a loopback address", host)
		}
		return ip.String(), nil
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		return "", fmt.Errorf("resolve target %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("target %s resolves to nothing", host)
	}
	for _, ip := range addrs {
		if !ip.IsLoopback() {
			return "", fmt.Errorf("target %s resolves to non-loopback %s", host, ip)
		}
	}
	return addrs[0].String(), nil
}

// DialContext is a net.Dialer.DialContext replacement that enforces the
// rule at the moment of dialling. Wiring it into an http.Transport
// closes the gap by construction: the address the check approved is the
// address the connection uses, with no second resolution in between.
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	safe, err := Resolve(addr)
	if err != nil {
		return nil, err
	}
	var d net.Dialer
	return d.DialContext(ctx, network, safe)
}
