//go:build linux

package hostinfo

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// Processes collects the machine's workload.
//
// Two passes over /proc, in this order because the second needs the
// first: read the listening sockets to get their inode numbers, then walk
// the processes, and while walking match each one's open sockets against
// that set. There is no reverse index in the kernel's text interfaces --
// a socket knows its inode and a process knows its file descriptors, and
// /proc/pid/fd is the only place the two meet.
func Processes() agenttypes.HostProcesses {
	byInode := listeningSockets()
	uids := uidNameTable()
	seen := map[groupKey]*agenttypes.ProcessGroup{}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return agenttypes.HostProcesses{}
	}
	for _, e := range entries {
		// A numeric name is what makes an entry a process; /proc is also
		// full of self, net, sys and the rest.
		if _, err := strconv.ParseInt(e.Name(), 10, 64); err != nil {
			continue
		}
		dir := filepath.Join("/proc", e.Name())
		// An empty cmdline is a kernel thread. They are 209 of the 271
		// entries on an idle machine, their names and pids churn
		// constantly (kworker/u516:3-...), and not one of them is a
		// workload anybody deploys, monitors or asks about.
		cmdline, err := os.ReadFile(filepath.Join(dir, "cmdline"))
		if err != nil || len(cmdline) == 0 {
			continue
		}
		name := strings.TrimSpace(readFileString(filepath.Join(dir, "comm")))
		if name == "" {
			continue
		}
		unit, container := parseCgroupWorkload(readFileBytes(filepath.Join(dir, "cgroup")))
		user := ""
		if uid, ok := parseStatusUID(readFileBytes(filepath.Join(dir, "status"))); ok {
			user = userName(uids, uid)
		}

		k := groupKey{name: name, user: user, unit: unit, container: container}
		g := seen[k]
		if g == nil {
			g = &agenttypes.ProcessGroup{Name: name, User: user, Unit: unit, Container: container}
			seen[k] = g
		}
		g.Count++
		addPorts(g, dir, byInode)
	}
	return agenttypes.HostProcesses{Groups: groupProcesses(seen)}
}

// addPorts attaches the listening sockets this process holds.
//
// Deduped against what the group already has: on a machine where several
// workers inherit one listening socket from their parent (nginx, php-fpm,
// any prefork server), every one of them reports the same port, and the
// group would otherwise carry it once per worker.
func addPorts(g *agenttypes.ProcessGroup, procDir string, byInode map[uint64]procSocket) {
	if len(byInode) == 0 {
		return
	}
	fds, err := os.ReadDir(filepath.Join(procDir, "fd"))
	if err != nil {
		// A process that exited between the readdir above and here, or one
		// this agent may not inspect. Neither is worth a log line on a
		// loop that runs over every process on the machine.
		return
	}
	for _, fd := range fds {
		target, err := os.Readlink(filepath.Join(procDir, "fd", fd.Name()))
		if err != nil {
			continue
		}
		// Sockets are symlinks to the literal text "socket:[<inode>]".
		raw, ok := strings.CutPrefix(target, "socket:[")
		if !ok {
			continue
		}
		inode, err := strconv.ParseUint(strings.TrimSuffix(raw, "]"), 10, 64)
		if err != nil {
			continue
		}
		s, listening := byInode[inode]
		if !listening {
			continue
		}
		p := agenttypes.ListenPort{Proto: s.proto, Addr: s.addr, Port: s.port}
		if !hasPort(g.Ports, p) {
			g.Ports = append(g.Ports, p)
		}
	}
}

func hasPort(ports []agenttypes.ListenPort, p agenttypes.ListenPort) bool {
	for _, e := range ports {
		if e == p {
			return true
		}
	}
	return false
}

// listeningSockets indexes every server socket on the machine by the
// inode that /proc/pid/fd will name it with.
func listeningSockets() map[uint64]procSocket {
	out := map[uint64]procSocket{}
	for _, src := range []struct {
		path      string
		proto     string
		listening bool
	}{
		{"/proc/net/tcp", "tcp", true},
		{"/proc/net/tcp6", "tcp", true},
		// UDP has no listening state: a bound socket is already a server,
		// so filtering by state here would drop every one of them.
		{"/proc/net/udp", "udp", false},
		{"/proc/net/udp6", "udp", false},
	} {
		for _, s := range parseNetSockets(readFileBytes(src.path), src.proto, src.listening) {
			out[s.inode] = s
		}
	}
	return out
}

func readFileBytes(path string) []byte {
	b, _ := os.ReadFile(path)
	return b
}

func readFileString(path string) string {
	return string(readFileBytes(path))
}
