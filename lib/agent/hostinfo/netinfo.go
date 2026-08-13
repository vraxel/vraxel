package hostinfo

import (
	"bufio"
	"bytes"
	"net"
	"strconv"
	"strings"
)

// /proc/net/route column indexes. The file is tab-separated with a
// header line: Iface Destination Gateway Flags RefCnt Use Metric Mask ...
const (
	routeColIface  = 0
	routeColDest   = 1
	routeColFlags  = 3
	routeColMetric = 6
	routeMinCols   = 8
)

// Linux route flags: the default route must be up and via a gateway.
const (
	rtfUp      = 0x1
	rtfGateway = 0x2
)

// parseDefaultRouteIface finds the interface carrying the default route.
//
// When several default routes exist (multi-homed hosts, VPNs) the lowest
// metric wins -- the same rule the kernel applies when choosing an
// outbound path, so the address this ends up reporting is the one the
// host actually originates traffic from.
//
// Pure over the file bytes, so it unit-tests on a non-Linux dev machine.
func parseDefaultRouteIface(data []byte) string {
	best := ""
	bestMetric := int64(1<<62 - 1)
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < routeMinCols || f[routeColIface] == "Iface" {
			continue
		}
		if f[routeColDest] != "00000000" {
			continue
		}
		// Flags is printed as %04X by the kernel (Metric, just below, is
		// decimal). Parsing it as base 10 silently skipped every route
		// whose flags contain a-f -- e.g. 0013 (UP|GATEWAY|DYNAMIC) parsed
		// as thirteen, losing RTF_GATEWAY, so the host reported no IP at
		// all. The common 0003 happens to read the same in both bases,
		// which is why this survived testing.
		flags, err := strconv.ParseInt(f[routeColFlags], 16, 64)
		if err != nil || flags&rtfUp == 0 || flags&rtfGateway == 0 {
			continue
		}
		metric, err := strconv.ParseInt(f[routeColMetric], 10, 64)
		if err != nil {
			metric = 0
		}
		if metric < bestMetric {
			best, bestMetric = f[routeColIface], metric
		}
	}
	return best
}

// firstIPv4 returns the first non-loopback IPv4 configured on iface.
func firstIPv4(iface string) string {
	ni, err := net.InterfaceByName(iface)
	if err != nil {
		return ""
	}
	addrs, err := ni.Addrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if ip4 := ipnet.IP.To4(); ip4 != nil && !ip4.IsLoopback() {
			return ip4.String()
		}
	}
	return ""
}
