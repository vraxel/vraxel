//go:build !linux

package hostinfo

// DefaultRoute is a no-op off Linux: /proc/net/route does not exist.
// Dev builds on macOS therefore report an empty primary IP, which the
// server stores as "" -- the same value an agent on a host with no
// default route would produce.
func DefaultRoute() (iface, ip string) { return "", "" }
