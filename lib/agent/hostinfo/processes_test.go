package hostinfo

import (
	"reflect"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// procfs prints the kernel's in-memory address words as hex, so every
// four bytes come out reversed on a little-endian machine.
func TestParseHexAddr(t *testing.T) {
	for in, want := range map[string]struct {
		addr string
		port int32
	}{
		"0100007F:0035":                         {"127.0.0.1", 53},
		"00000000:1F90":                         {"0.0.0.0", 8080},
		"0A01010A:0016":                         {"10.1.1.10", 22},
		"00000000000000000000000000000000:1F90": {"::", 8080},
		"00000000000000000000000001000000:0016": {"::1", 22},
	} {
		addr, port, ok := parseHexAddr(in)
		if !ok || addr != want.addr || port != want.port {
			t.Errorf("parseHexAddr(%q) = (%q, %d, %v), want (%q, %d, true)",
				in, addr, port, ok, want.addr, want.port)
		}
	}
	for _, bad := range []string{"", "nope", "0100007F", "ZZ:0035"} {
		if _, _, ok := parseHexAddr(bad); ok {
			t.Errorf("parseHexAddr(%q) accepted", bad)
		}
	}
}

func TestParseNetSockets(t *testing.T) {
	// Column 3 is the TCP state; 0A is LISTEN and 01 is ESTABLISHED. An
	// established connection is not a service this machine offers.
	const tcp = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1538 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 24379 1 0000 100 0 0 10 0
   1: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 31337 1 0000 100 0 0 10 0
   2: 0A01010A:1538 0A01010A:C350 01 00000000:00000000 00:00000000 00000000     0        0 44444 1 0000 100 0 0 10 0
`
	got := parseNetSockets([]byte(tcp), "tcp", true)
	if len(got) != 2 {
		t.Fatalf("got %d sockets %+v, want 2 listeners", len(got), got)
	}
	if got[0].addr != "127.0.0.1" || got[0].port != 5432 || got[0].inode != 24379 {
		t.Errorf("socket 0 = %+v", got[0])
	}
	if got[1].addr != "0.0.0.0" || got[1].port != 8080 {
		t.Errorf("socket 1 = %+v", got[1])
	}
}

// UDP has no listening state, so a state filter would reject every bound
// UDP socket on the machine -- chronyd and every DNS resolver included.
func TestParseNetSocketsUDPHasNoListenState(t *testing.T) {
	const udp = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:0143 00000000:0000 07 00000000:00000000 00:00000000 00000000   123        0 20001 2 0000 0
`
	if got := parseNetSockets([]byte(udp), "udp", false); len(got) != 1 || got[0].port != 323 {
		t.Fatalf("got %+v, want the bound udp socket", got)
	}
	if got := parseNetSockets([]byte(udp), "udp", true); len(got) != 0 {
		t.Fatalf("state filter kept %+v; udp state 07 is not 0A and never will be", got)
	}
}

// Inode 0 is a socket with no open file behind it, so no process holds it
// and there is nothing to attribute it to.
func TestParseNetSocketsSkipsInodeZero(t *testing.T) {
	const tcp = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 0 1 0000 100 0 0 10 0
`
	if got := parseNetSockets([]byte(tcp), "tcp", true); len(got) != 0 {
		t.Errorf("got %+v, want nothing", got)
	}
}

func TestParseCgroupWorkload(t *testing.T) {
	for name, tc := range map[string]struct {
		data      string
		unit      string
		container bool
	}{
		"systemd service": {"0::/system.slice/nginx.service\n", "nginx.service", false},
		// The docker DAEMON is a unit; a container it started is a scope
		// whose name is the container id. Only the leaf tells them apart.
		"docker daemon":    {"0::/system.slice/docker.service\n", "docker.service", false},
		"docker container": {"0::/system.slice/docker-80ed70367abde163b79fe99f1d04e30cdfaa88852c6ced088652098ba33d7d53.scope\n", "", true},
		"containerd":       {"0::/system.slice/containerd.service\n", "containerd.service", false},
		"crio container":   {"0::/system.slice/crio-1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef.scope\n", "", true},
		// A unit whose name merely starts like a container scope. Without
		// the hex-id check this would be reported as a container.
		"unit named docker-something": {"0::/system.slice/docker-cleanup.service\n", "docker-cleanup.service", false},
		// The session number is fresh per login, so reporting it would
		// put an ephemeral id in the group key and make every ssh login
		// look like a workload change.
		"interactive login": {"0::/user.slice/user-1000.slice/session-3.scope\n", "", false},
		// A user manager IS stable -- one per uid, not one per login.
		"user manager": {"0::/user.slice/user-1000.slice/user@1000.service\n", "user@1000.service", false},
		"root cgroup":  {"0::/\n", "", false},
		// cgroup v1 repeats the path once per controller; only the systemd
		// hierarchy is the one that names units.
		"cgroup v1": {"12:pids:/system.slice/sshd.service\n1:name=systemd:/system.slice/sshd.service\n", "sshd.service", false},
		"empty":     {"", "", false},
	} {
		t.Run(name, func(t *testing.T) {
			unit, container := parseCgroupWorkload([]byte(tc.data))
			if unit != tc.unit || container != tc.container {
				t.Errorf("got (%q, %v), want (%q, %v)", unit, container, tc.unit, tc.container)
			}
		})
	}
}

// The Uid line is "real effective saved fs". Effective is the answer to
// "who is this running as": a server that bound port 80 as root and then
// dropped privileges is running as the unprivileged user.
func TestParseStatusUID(t *testing.T) {
	const status = "Name:\tnginx\nState:\tS (sleeping)\nUid:\t0\t33\t33\t33\nGid:\t0\t33\t33\t33\n"
	uid, ok := parseStatusUID([]byte(status))
	if !ok || uid != 33 {
		t.Errorf("got (%d, %v), want (33, true)", uid, ok)
	}
	if _, ok := parseStatusUID([]byte("Name:\tx\n")); ok {
		t.Error("accepted a status file with no Uid line")
	}
}

// The ordering is what makes the send-only-when-changed gate work: map
// iteration order alone would make every sample look like a change.
func TestGroupProcessesIsDeterministic(t *testing.T) {
	build := func() []agenttypes.ProcessGroup {
		seen := map[groupKey]*agenttypes.ProcessGroup{}
		for _, g := range []agenttypes.ProcessGroup{
			{Name: "postgres", User: "postgres", Count: 3},
			{Name: "nginx", User: "www-data", Unit: "nginx.service", Count: 4, Ports: []agenttypes.ListenPort{
				{Proto: "tcp", Addr: "::", Port: 443},
				{Proto: "tcp", Addr: "0.0.0.0", Port: 80},
				{Proto: "tcp", Addr: "0.0.0.0", Port: 443},
			}},
			{Name: "nginx", User: "root", Unit: "nginx.service", Count: 1},
			{Name: "cron", User: "root", Unit: "cron.service", Count: 1},
		} {
			seen[groupKey{name: g.Name, user: g.User, unit: g.Unit}] = &g
		}
		return groupProcesses(seen)
	}
	first := build()
	names := make([]string, len(first))
	for i, g := range first {
		names[i] = g.Name + "/" + g.User
	}
	want := []string{"cron/root", "nginx/root", "nginx/www-data", "postgres/postgres"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("order = %q, want %q", names, want)
	}
	// Ports sort by port then proto then address, so two samples of one
	// unchanged machine encode identically.
	ports := first[2].Ports
	if ports[0].Port != 80 || ports[1].Addr != "0.0.0.0" || ports[2].Addr != "::" {
		t.Errorf("ports = %+v", ports)
	}
	for range 20 {
		if got := build(); !reflect.DeepEqual(got, first) {
			t.Fatal("two builds of the same input differ; the dedup gate would never hold")
		}
	}
}

// The trap this parser exists for: field 2 of /proc/pid/stat is the
// executable name IN PARENTHESES, and it may contain spaces and
// parentheses of its own. Splitting the whole line on whitespace shifts
// every later field on exactly the processes whose names are worst.
func TestParseProcStat(t *testing.T) {
	// An ordinary one first: pid 1, comm "systemd", state S.
	const plain = "1 (systemd) S 0 1 1 0 -1 4194560 20876 445 91 0 " +
		"431 1874 51 47 20 0 1 0 34 175882240 4621 18446744073709551615 rest ignored\n"
	got, ok := parseProcStat([]byte(plain))
	if !ok {
		t.Fatal("rejected a well-formed line")
	}
	// fields 14+15 = 431+1874, field 22 = 34, field 24 = 4621
	if got.jiffies != 431+1874 || got.startTicks != 34 || got.rssPages != 4621 {
		t.Errorf("got %+v", got)
	}

	// The same numbers behind a name containing a space AND a closing
	// paren. Firefox really does name a process "(Web Content)".
	const nasty = "42 (Web Content (x)) S 0 1 1 0 -1 4194560 20876 445 91 0 " +
		"431 1874 51 47 20 0 1 0 34 175882240 4621 18446744073709551615 rest ignored\n"
	got2, ok := parseProcStat([]byte(nasty))
	if !ok {
		t.Fatal("rejected a line whose comm contains a paren")
	}
	if got2 != got {
		t.Errorf("a parenthesised name shifted the fields: %+v vs %+v", got2, got)
	}

	for _, bad := range []string{"", "no parens here", "1 (x) S"} {
		if _, ok := parseProcStat([]byte(bad)); ok {
			t.Errorf("accepted %q", bad)
		}
	}
}

// A workload's start time is dated from boot, so it must survive the
// machine's own clock being wrong -- the same property BootAt has.
func TestParseProcStatStartIsRelativeToBoot(t *testing.T) {
	const line = "1 (systemd) S 0 1 1 0 -1 0 0 0 0 0 " +
		"0 0 0 0 20 0 1 0 500 0 0 rest\n"
	got, ok := parseProcStat([]byte(line))
	if !ok {
		t.Fatal("parse failed")
	}
	// 500 ticks at 100 Hz is 5 seconds after boot, whatever the wall
	// clock says.
	if ms := got.startTicks * 1000 / 100; ms != 5000 {
		t.Errorf("start offset = %dms, want 5000", ms)
	}
}

func TestCPUPercent(t *testing.T) {
	// 100 ticks of 10ms is one full second of cpu; over a one-second
	// window that is one core, 100%.
	got := cpuPercent(
		map[int64]int64{1: 0, 2: 500, 3: 10, 4: 7, 6: 900},
		map[int64]int64{1: 100, 2: 550, 3: 10, 5: 42, 6: 100},
		1.0,
	)
	want := map[int64]float64{1: 100, 2: 50, 3: 0}
	for pid, w := range want {
		if g, ok := got[pid]; !ok || g != w {
			t.Errorf("pid %d = %v (present %v), want %v", pid, g, ok, w)
		}
	}
	// 4 vanished before the second reading, 5 appeared after the first,
	// and 6's counter went backwards because the kernel reused the pid.
	// None of the three has a rise this window can attribute.
	for _, pid := range []int64{4, 5, 6} {
		if _, ok := got[pid]; ok {
			t.Errorf("pid %d reported a rise it cannot have", pid)
		}
	}
}

// The window length is a resolution decision, and getting it wrong is
// invisible: the column still renders, it just quantises every reading to
// a multiple of 10ms/window. At the 250ms this shipped with, that was 4%
// -- every workload under 4% of a core read exactly 0.0%.
func TestCPUPercentResolution(t *testing.T) {
	oneTick := func(window float64) float64 {
		return cpuPercent(map[int64]int64{1: 0}, map[int64]int64{1: 1}, window)[1]
	}
	if got := oneTick(statsSampleWindow.Seconds()); got > 1.0 {
		t.Errorf("smallest non-zero cpu reading is %.1f%%, want <= 1%%", got)
	}
}
