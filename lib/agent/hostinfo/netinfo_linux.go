//go:build linux

package hostinfo

import "os"

// DefaultRoute returns the default-route interface and its first
// non-loopback IPv4.
//
// This address becomes hosts.reported_primary_ip. The platform never dials it
// (design §5.13) -- it is what the UI shows and what PaaS cluster members
// use to reach each other inside the customer network, which is precisely
// why "the address this host sends traffic from" is the right definition
// rather than "the first address on the first interface".
func DefaultRoute() (iface, ip string) {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return "", ""
	}
	iface = parseDefaultRouteIface(data)
	if iface == "" {
		return "", ""
	}
	return iface, firstIPv4(iface)
}
