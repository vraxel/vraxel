//go:build linux

package hostinfo

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// statsSampleWindow is how long the live collector waits between its two
// /proc reads. CPU time is a counter, so a percentage needs two samples
// and a known gap; 250ms is long enough that the jiffy counter (100 Hz)
// has resolution to give and short enough that nobody notices it in a
// page load.
const statsSampleWindow = 250 * time.Millisecond

// Processes collects the machine's workload.
//
// Two passes over /proc, in this order because the second needs the
// first: read the listening sockets to get their inode numbers, then walk
// the processes, and while walking match each one's open sockets against
// that set. There is no reverse index in the kernel's text interfaces --
// a socket knows its inode and a process knows its file descriptors, and
// /proc/pid/fd is the only place the two meet.
func Processes() agenttypes.HostProcesses {
	return collectProcesses(nil)
}

// ProcessesLive is Processes plus what each workload is USING right now.
//
// Separate entry point rather than a flag on Processes, because the two
// answers have opposite contracts: this one is measured on demand while
// somebody watches, and the other is an inventory compared against the
// last one sent. A single function returning both would put a
// per-sample-varying number in the frame that must not vary per sample.
func ProcessesLive() agenttypes.HostProcesses {
	first := sampleJiffies()
	time.Sleep(statsSampleWindow)
	return collectProcesses(first)
}

// sampleJiffies reads every process's cpu counter, for the earlier half
// of the live measurement.
func sampleJiffies() map[int64]int64 {
	out := map[int64]int64{}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, e := range entries {
		pid, err := strconv.ParseInt(e.Name(), 10, 64)
		if err != nil {
			continue
		}
		if st, ok := parseProcStat(readFileBytes(filepath.Join("/proc", e.Name(), "stat"))); ok {
			out[pid] = st.jiffies
		}
	}
	return out
}

// collectProcesses walks /proc once. A non-nil prev turns on the live
// measurement: cpu time is the RISE since that earlier sample, which is
// the only way a counter becomes a percentage.
func collectProcesses(prev map[int64]int64) agenttypes.HostProcesses {
	byInode := listeningSockets()
	uids := uidNameTable()
	seen := map[groupKey]*agenttypes.ProcessGroup{}
	bootMs, hz := bootTimeMs(), clockTicks()
	elapsed := statsSampleWindow.Seconds()

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return agenttypes.HostProcesses{}
	}
	for _, e := range entries {
		// A numeric name is what makes an entry a process; /proc is also
		// full of self, net, sys and the rest.
		pid, err := strconv.ParseInt(e.Name(), 10, 64)
		if err != nil {
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
			g = &agenttypes.ProcessGroup{
				Name: name, User: user, Unit: unit, Container: container,
				// The binary, resolved. Unreadable for a process that
				// exited between the readdir and here, and for a kernel
				// thread -- neither of which reaches this line.
				Exe: linkTarget(dir, "exe"),
			}
			seen[k] = g
		}
		g.Count++
		addPorts(g, dir, byInode)

		st, ok := parseProcStat(readFileBytes(filepath.Join(dir, "stat")))
		if !ok {
			continue
		}
		// The OLDEST member dates the group: a prefork server replaces
		// workers continuously while the service has been up for months.
		if started := bootMs + st.startTicks*1000/hz; started > 0 && (g.StartedAtMs == 0 || started < g.StartedAtMs) {
			g.StartedAtMs = started
		}
		if prev == nil {
			continue
		}
		g.RSSBytes += st.rssPages * int64(os.Getpagesize())
		// A process that did not exist at the first sample has no rise to
		// measure; counting its whole lifetime as if it fell in the window
		// would report hundreds of percent.
		if before, seenBefore := prev[pid]; seenBefore && st.jiffies >= before {
			g.CPUPct += float64(st.jiffies-before) / float64(hz) / elapsed * 100
		}
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

// bootTimeMs is when the machine booted, in unix milliseconds, from
// /proc/stat's btime.
//
// Start times in /proc/pid/stat are counted in clock ticks SINCE BOOT, so
// turning one into a wall-clock moment needs this. Read per collection
// rather than cached: it is one small file, and a cached value would
// survive a suspend/resume that moved the machine's idea of when it
// booted.
func bootTimeMs() int64 {
	for _, line := range strings.Split(readFileString("/proc/stat"), "\n") {
		v, ok := strings.CutPrefix(line, "btime ")
		if !ok {
			continue
		}
		sec, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0
		}
		return sec * 1000
	}
	return 0
}

// clockTicks is USER_HZ, the unit /proc/pid/stat counts cpu time and
// start time in.
//
// Hardcoded at 100 rather than read through sysconf(_SC_CLK_TCK), which
// needs cgo. The kernel has defined USER_HZ as 100 on every architecture
// Linux supports for the whole time this interface has existed -- it is
// part of the ABI, not the tick rate the kernel actually runs at.
func clockTicks() int64 { return 100 }

// linkTarget resolves a /proc symlink to its target, empty when it cannot
// be read.
func linkTarget(dir, name string) string {
	target, err := os.Readlink(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return target
}
