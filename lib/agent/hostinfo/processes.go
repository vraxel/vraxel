package hostinfo

import (
	"bufio"
	"bytes"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// listenState is the TCP state procfs prints for a listening socket.
// /proc/net/tcp writes states as hex; 0A is TCP_LISTEN.
const listenState = "0A"

// procSocket is one listening socket as procfs describes it, before it
// has been matched to the process holding it.
type procSocket struct {
	proto string
	addr  string
	port  int32
	inode uint64
}

// parseNetSockets reads one of /proc/net/{tcp,tcp6,udp,udp6}.
//
// listening selects which rows to keep and differs by protocol: TCP has a
// state column and only 0A is a server, while UDP has no listen state at
// all -- a bound UDP socket is a server the moment it exists, so the
// state filter would reject every one of them.
func parseNetSockets(data []byte, proto string, listening bool) []procSocket {
	var out []procSocket
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		// sl local_address rem_address st tx:rx uid:timeout inode ...
		if len(f) < 10 || f[0] == "sl" {
			continue
		}
		if listening && f[3] != listenState {
			continue
		}
		addr, port, ok := parseHexAddr(f[1])
		if !ok {
			continue
		}
		inode, err := strconv.ParseUint(f[9], 10, 64)
		// Inode 0 is a socket with no open file behind it (a TIME_WAIT
		// remnant); nothing owns it, so there is no process to name.
		if err != nil || inode == 0 {
			continue
		}
		out = append(out, procSocket{proto: proto, addr: addr, port: port, inode: inode})
	}
	return out
}

// parseHexAddr decodes procfs's "ADDRESS:PORT" column.
//
// The address is the kernel's in-memory words printed as hex, so on a
// little-endian machine every four bytes are reversed -- 0100007F is
// 127.0.0.1. IPv6 is the same rule applied to four consecutive words,
// which is why it is decoded in 8-character chunks rather than as one
// number.
func parseHexAddr(s string) (addr string, port int32, ok bool) {
	h, p, found := strings.Cut(s, ":")
	if !found {
		return "", 0, false
	}
	n, err := strconv.ParseUint(p, 16, 32)
	if err != nil {
		return "", 0, false
	}
	port = int32(n)

	switch len(h) {
	case 8:
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return "", 0, false
		}
		return decodeRouteAddr32(uint32(v)), port, true
	case 32:
		// netip rather than net.IP: net.IP.String renders a v4-mapped
		// address as a dotted quad, which would print "::ffff:0.0.0.0"
		// and "0.0.0.0" identically while they are different bindings.
		// netip keeps them apart and collapses zero runs correctly, so
		// there is nothing here worth hand-rolling.
		var b [16]byte
		for i := range 4 {
			w, err := strconv.ParseUint(h[i*8:i*8+8], 16, 32)
			if err != nil {
				return "", 0, false
			}
			// Same little-endian word as v4, four times over.
			b[i*4+0] = byte(w)
			b[i*4+1] = byte(w >> 8)
			b[i*4+2] = byte(w >> 16)
			b[i*4+3] = byte(w >> 24)
		}
		return netip.AddrFrom16(b).String(), port, true
	}
	return "", 0, false
}

// decodeRouteAddr32 renders a little-endian IPv4 word as a dotted quad.
func decodeRouteAddr32(v uint32) string {
	return strconv.FormatUint(uint64(v&0xff), 10) + "." +
		strconv.FormatUint(uint64(v>>8&0xff), 10) + "." +
		strconv.FormatUint(uint64(v>>16&0xff), 10) + "." +
		strconv.FormatUint(uint64(v>>24&0xff), 10)
}

// parseCgroupWorkload extracts the supervisor's name from a
// /proc/pid/cgroup file: the systemd unit, or a flag saying the process
// lives in a container.
//
// Only the cgroup v2 line (hierarchy id 0) and the systemd v1 controller
// are read; the rest of a v1 file repeats the same path once per
// controller. The path's LAST element is the one that matters, because
// systemd nests -- /system.slice/docker.service is the docker daemon
// itself, while /system.slice/docker-<64hex>.scope is a container it
// started, and only the leaf distinguishes them.
func parseCgroupWorkload(data []byte) (unit string, container bool) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		// hierarchy-ID:controller-list:cgroup-path
		parts := strings.SplitN(sc.Text(), ":", 3)
		if len(parts) != 3 {
			continue
		}
		if parts[0] != "0" && !strings.Contains(parts[1], "name=systemd") {
			continue
		}
		leaf := parts[2]
		if i := strings.LastIndex(leaf, "/"); i >= 0 {
			leaf = leaf[i+1:]
		}
		switch {
		case leaf == "" || leaf == "-.slice":
			continue
		// An interactive login gets session-<n>.scope, and the number is
		// fresh per login. Reported as a unit it would put an ephemeral id
		// in the group key, so every ssh session would move bash into a
		// new group and make the whole workload report look changed --
		// against a gate whose entire job is to notice real change. That
		// somebody is logged in is already in the count.
		case isSessionScope(leaf):
			return "", false
		case isContainerScope(leaf):
			return "", true
		case strings.HasSuffix(leaf, ".service"), strings.HasSuffix(leaf, ".socket"),
			strings.HasSuffix(leaf, ".mount"), strings.HasSuffix(leaf, ".scope"):
			return leaf, false
		}
	}
	return "", false
}

// isSessionScope reports whether a cgroup leaf names one interactive
// login session.
func isSessionScope(leaf string) bool {
	n, ok := strings.CutPrefix(strings.TrimSuffix(leaf, ".scope"), "session-")
	if !ok || n == "" {
		return false
	}
	for _, c := range n {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// containerScopePrefixes are how the container runtimes name their
// cgroups. The suffix is always the container's 64-hex id, which is why
// the NAME is not recoverable from here.
var containerScopePrefixes = []string{"docker-", "crio-", "libpod-", "containerd-", "cri-containerd-"}

// isContainerScope reports whether a cgroup leaf names a container.
//
// The prefix alone is not enough: "docker.service" starts with neither,
// but "docker-" would also match a systemd unit somebody named
// docker-cleanup.service. The 64-hex id is the part no unit name has.
func isContainerScope(leaf string) bool {
	name := strings.TrimSuffix(strings.TrimSuffix(leaf, ".scope"), ".slice")
	for _, p := range containerScopePrefixes {
		id, ok := strings.CutPrefix(name, p)
		if ok && isHex(id) && len(id) >= 32 {
			return true
		}
	}
	return false
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// parseStatusUID returns the EFFECTIVE uid from /proc/pid/status.
//
// Effective, not real: a process that dropped privileges after binding a
// port is running as the unprivileged user, and that is the answer to
// "who is this running as". The Uid line is "real effective saved fs".
func parseStatusUID(data []byte) (int64, bool) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		v, ok := strings.CutPrefix(sc.Text(), "Uid:")
		if !ok {
			continue
		}
		f := strings.Fields(v)
		if len(f) < 2 {
			return 0, false
		}
		uid, err := strconv.ParseInt(f[1], 10, 64)
		return uid, err == nil
	}
	return 0, false
}

// groupKey identifies one workload. Two processes sharing it are two
// instances of the same thing.
type groupKey struct {
	name      string
	user      string
	unit      string
	container bool
}

// groupProcesses folds per-process observations into the reported list.
//
// Sorted deterministically at the end, and that is not cosmetic: the
// agent decides whether to send by comparing this structure's JSON to the
// last one it sent, so map iteration order alone would make every sample
// look like a change and defeat the gate entirely.
func groupProcesses(seen map[groupKey]*agenttypes.ProcessGroup) []agenttypes.ProcessGroup {
	out := make([]agenttypes.ProcessGroup, 0, len(seen))
	for _, g := range seen {
		sort.Slice(g.Ports, func(i, j int) bool {
			a, b := g.Ports[i], g.Ports[j]
			if a.Port != b.Port {
				return a.Port < b.Port
			}
			if a.Proto != b.Proto {
				return a.Proto < b.Proto
			}
			return a.Addr < b.Addr
		})
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Unit != b.Unit {
			return a.Unit < b.Unit
		}
		return a.User < b.User
	})
	return out
}
