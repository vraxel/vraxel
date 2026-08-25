package hostinfo

import (
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// Four sockets of one core, which is what VMware presents for a 4-vCPU
// guest. The physical ids are 0/2/4/6 -- non-contiguous, so counting
// lines or taking the maximum both give the wrong answer.
const vmwareCPUInfo = `processor	: 0
model name	: Intel(R) Core(TM) i7-8850H CPU @ 2.60GHz
physical id	: 0
siblings	: 1
cpu cores	: 1
flags		: fpu vme de pse tsc msr hypervisor lahf_lm

processor	: 1
model name	: Intel(R) Core(TM) i7-8850H CPU @ 2.60GHz
physical id	: 2
siblings	: 1
cpu cores	: 1
flags		: fpu vme de pse tsc msr hypervisor lahf_lm

processor	: 2
model name	: Intel(R) Core(TM) i7-8850H CPU @ 2.60GHz
physical id	: 4
siblings	: 1
cpu cores	: 1
flags		: fpu vme de pse tsc msr hypervisor lahf_lm

processor	: 3
model name	: Intel(R) Core(TM) i7-8850H CPU @ 2.60GHz
physical id	: 6
siblings	: 1
cpu cores	: 1
flags		: fpu vme de pse tsc msr hypervisor lahf_lm
`

// One socket, 8 cores, hyperthreaded to 16 -- and no hypervisor flag.
const bareMetalCPUInfo = `processor	: 0
model name	: AMD EPYC 7302P 16-Core Processor
physical id	: 0
siblings	: 16
cpu cores	: 8
flags		: fpu vme de pse tsc msr sse4_2 avx2

processor	: 1
model name	: AMD EPYC 7302P 16-Core Processor
physical id	: 0
siblings	: 16
cpu cores	: 8
flags		: fpu vme de pse tsc msr sse4_2 avx2
`

func TestParseCPUInfoCountsDistinctSockets(t *testing.T) {
	model, sockets, cores, threads := parseCPUInfo([]byte(vmwareCPUInfo))
	if model != "Intel(R) Core(TM) i7-8850H CPU @ 2.60GHz" {
		t.Errorf("model = %q", model)
	}
	if sockets != 4 {
		t.Errorf("sockets = %d, want 4 (ids 0/2/4/6 are four sockets)", sockets)
	}
	if cores != 1 {
		t.Errorf("coresPerSocket = %d, want 1", cores)
	}
	if threads != 1 {
		t.Errorf("threadsPerCore = %d, want 1", threads)
	}
}

func TestParseCPUInfoHyperthreaded(t *testing.T) {
	_, sockets, cores, threads := parseCPUInfo([]byte(bareMetalCPUInfo))
	if sockets != 1 || cores != 8 || threads != 2 {
		t.Errorf("got %d sockets / %d cores / %d threads, want 1/8/2", sockets, cores, threads)
	}
}

// A kernel with no "physical id" at all still has processors, and the
// truthful reading is one socket rather than zero.
func TestParseCPUInfoWithoutPhysicalID(t *testing.T) {
	const arm = "processor\t: 0\nprocessor\t: 1\nprocessor\t: 2\nprocessor\t: 3\n"
	_, sockets, cores, _ := parseCPUInfo([]byte(arm))
	if sockets != 1 {
		t.Errorf("sockets = %d, want 1", sockets)
	}
	if cores != 4 {
		t.Errorf("coresPerSocket = %d, want 4 (all logical CPUs on the one socket)", cores)
	}
}

func TestDetectVirt(t *testing.T) {
	tests := []struct {
		name    string
		cpuinfo string
		vendor  string
		product string
		want    string
	}{
		{"vmware guest", vmwareCPUInfo, "VMware, Inc.", "VMware Virtual Platform", agenttypes.VirtVMware},
		{"bare metal", bareMetalCPUInfo, "Dell Inc.", "PowerEdge R6515", agenttypes.VirtPhysical},
		{"kvm guest", vmwareCPUInfo, "QEMU", "KVM Virtual Machine", agenttypes.VirtKVM},
		{"hyperv guest", vmwareCPUInfo, "Microsoft Corporation", "Virtual Machine", agenttypes.VirtHyperV},
		{"scrubbed dmi", vmwareCPUInfo, "", "", agenttypes.VirtUnknown},
		// The needle "xen" appears in no physical vendor string, but the
		// point of this case is the guard: without the hypervisor flag
		// nothing in DMI may promote a machine to "guest".
		{"physical board naming a hypervisor", bareMetalCPUInfo, "Xen-like Systems", "", agenttypes.VirtPhysical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectVirt([]byte(tt.cpuinfo), tt.vendor, tt.product); got != tt.want {
				t.Errorf("detectVirt = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseMounts(t *testing.T) {
	const mounts = `sysfs /sys sysfs rw,nosuid 0 0
proc /proc proc rw,nosuid 0 0
udev /dev devtmpfs rw,nosuid 0 0
/dev/mapper/debian--vg-root / ext4 rw,relatime,errors=remount-ro 0 0
tmpfs /run tmpfs rw,nosuid 0 0
/dev/sda1 /boot ext4 rw,relatime 0 0
server:/export /mnt/nfs nfs4 rw,relatime 0 0
//fileserver/share /mnt/smb cifs rw 0 0
tank/data /srv/tank zfs rw,relatime 0 0
/dev/sdb1 /mnt/data\040dir ext4 rw,relatime 0 0
`
	got := parseMounts([]byte(mounts))
	// zfs names a pool, not a device path, and is kept: it is this
	// machine's disk. nfs4 and cifs are somebody else's, and are not.
	want := []mountEntry{
		{device: "/dev/mapper/debian--vg-root", mount: "/", fstype: "ext4"},
		{device: "/dev/sda1", mount: "/boot", fstype: "ext4"},
		{device: "tank/data", mount: "/srv/tank", fstype: "zfs"},
		{device: "/dev/sdb1", mount: "/mnt/data dir", fstype: "ext4"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d mounts %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("mount %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// The second mountpoint of an already-counted device is dropped, so a
// btrfs subvolume cannot make a host look like it has twice the storage.
func TestParseMountsDedupesByDevice(t *testing.T) {
	const mounts = `/dev/sda2 /home btrfs rw 0 0
/dev/sda2 /home/user/snapshots btrfs rw 0 0
`
	if got := parseMounts([]byte(mounts)); len(got) != 1 || got[0].mount != "/home" {
		t.Errorf("got %+v, want only /home", got)
	}
}

func TestRealFSTypeAndNetDevice(t *testing.T) {
	for _, fs := range []string{"ext4", "xfs", "btrfs", "zfs", "ext3", "vfat"} {
		if !agenttypes.RealFSType(fs) {
			t.Errorf("RealFSType(%q) = false", fs)
		}
	}
	// The network types matter as much as the pseudo ones: node_exporter's
	// filesystem collector reports them by default, so leaving them in
	// would count the file server's disk toward every client that mounts
	// it -- in the gauge and in the table alike.
	for _, fs := range []string{"tmpfs", "proc", "sysfs", "overlay", "nfs4", "cifs", "fuse.sshfs"} {
		if agenttypes.RealFSType(fs) {
			t.Errorf("RealFSType(%q) = true", fs)
		}
	}
	for _, d := range []string{"ens33", "eth0", "eno1", "bond0"} {
		if !agenttypes.RealNetDevice(d) {
			t.Errorf("RealNetDevice(%q) = false", d)
		}
	}
	for _, d := range []string{"lo", "docker0", "br-9a6bf6a1ebb7", "vethc15cc89", "virbr0"} {
		if agenttypes.RealNetDevice(d) {
			t.Errorf("RealNetDevice(%q) = true", d)
		}
	}
}

// The trap this parser exists for: ext4's DEFAULT mount options contain
// the string "ro" inside errors=remount-ro, so a substring test would
// report every healthy root filesystem as faulted.
func TestParseMountsReadOnly(t *testing.T) {
	const mounts = `/dev/sda1 / ext4 rw,relatime,errors=remount-ro 0 0
/dev/sda2 /var ext4 ro,relatime 0 0
/dev/sda3 /srv xfs rw,nosuid,errors=remount-ro,discard 0 0
`
	got := parseMounts([]byte(mounts))
	want := map[string]bool{"/": false, "/var": true, "/srv": false}
	if len(got) != len(want) {
		t.Fatalf("got %d mounts, want %d", len(got), len(want))
	}
	for _, m := range got {
		if m.readOnly != want[m.mount] {
			t.Errorf("%s readOnly = %v, want %v", m.mount, m.readOnly, want[m.mount])
		}
	}
}

func TestChassisType(t *testing.T) {
	for code, want := range map[int64]string{
		3:  agenttypes.ChassisDesktop,
		7:  agenttypes.ChassisTower,
		10: agenttypes.ChassisLaptop,
		17: agenttypes.ChassisServer,
		23: agenttypes.ChassisRack,
		28: agenttypes.ChassisBlade,
		// 1 is "Other", which is what every hypervisor reports and what an
		// unreadable field decodes to. Naming it would put a made-up
		// enclosure on every VM in the fleet.
		1: "",
		2: "",
		// sysInt returns -1 when there is nothing to read.
		-1: "",
	} {
		if got := chassisType(code); got != want {
			t.Errorf("chassisType(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestParseRouteGateway(t *testing.T) {
	// 0201010A is 10.1.1.2 read back to front, which is how the kernel
	// prints a little-endian machine's in-memory address word.
	const route = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
ens33	00000000	0201010A	0003	0	0	100	00000000	0	0	0
ens33	0001010A	00000000	0001	0	0	0	00FFFFFF	0	0	0
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
`
	if got := parseRouteGateway([]byte(route)); got != "10.1.1.2" {
		t.Errorf("gateway = %q, want 10.1.1.2", got)
	}
}

// Two uplinks means two default routes, and the kernel uses the cheaper
// one. Reporting whichever came first would name the standby half the
// time, and the file's order is not stable across reboots.
func TestParseRouteGatewayPrefersLowestMetric(t *testing.T) {
	const route = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth1	00000000	FE01010A	0003	0	0	200	00000000	0	0	0
eth0	00000000	0201010A	0003	0	0	100	00000000	0	0	0
`
	if got := parseRouteGateway([]byte(route)); got != "10.1.1.2" {
		t.Errorf("gateway = %q, want 10.1.1.2 (metric 100)", got)
	}
}

// A default route with no RTF_GATEWAY bit is an on-link route out an
// interface. It is a valid way to reach the internet and it has no next
// hop, so there is nothing to report.
func TestParseRouteGatewayIgnoresOnLinkAndAbsent(t *testing.T) {
	const onlink = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	00000000	0001	0	0	0	00000000	0	0	0
`
	if got := parseRouteGateway([]byte(onlink)); got != "" {
		t.Errorf("on-link default gave %q, want empty", got)
	}
	if got := parseRouteGateway(nil); got != "" {
		t.Errorf("empty route table gave %q, want empty", got)
	}
}

func TestParseOSRelease(t *testing.T) {
	const osRelease = `PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
NAME="Debian GNU/Linux"
VERSION_ID="13"
VERSION="13 (trixie)"
ID=debian
HOME_URL="https://www.debian.org/"
`
	id, version := parseOSRelease([]byte(osRelease))
	if id != "debian" || version != "13" {
		t.Errorf("got (%q, %q), want (debian, 13)", id, version)
	}
	// ID_LIKE starts with "ID" and is a different field; a prefix match
	// would return "rhel fedora" as the distribution's own id.
	id, _ = parseOSRelease([]byte("ID_LIKE=\"rhel fedora\"\nID=\"rocky\"\n"))
	if id != "rocky" {
		t.Errorf("id = %q, want rocky", id)
	}
}

// The SCSI INQUIRY vendor field is 8 bytes, so a longer name arrives cut
// off wherever byte 8 lands -- "VMware, Inc." as "VMware, ". Only the
// dangling separator goes; a name that fits is untouched.
func TestDiskVendor(t *testing.T) {
	for in, want := range map[string]string{
		"VMware, ": "VMware",
		"VMware,":  "VMware",
		"ATA     ": "ATA",
		"SEAGATE":  "SEAGATE",
		"DELL":     "DELL",
		"":         "",
	} {
		if got := diskVendor(in); got != want {
			t.Errorf("diskVendor(%q) = %q, want %q", in, got, want)
		}
	}
}
