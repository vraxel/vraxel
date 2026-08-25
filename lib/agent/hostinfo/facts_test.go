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
/dev/mapper/debian--vg-root /var/lib/docker/btrfs btrfs rw,relatime 0 0
server:/export /mnt/nfs nfs4 rw,relatime 0 0
/dev/sdb1 /mnt/data\040dir ext4 rw,relatime 0 0
`
	got := parseMounts([]byte(mounts))
	want := []mountEntry{
		{device: "/dev/mapper/debian--vg-root", mount: "/", fstype: "ext4"},
		{device: "/dev/sda1", mount: "/boot", fstype: "ext4"},
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
	for _, fs := range []string{"ext4", "xfs", "btrfs", "zfs"} {
		if !agenttypes.RealFSType(fs) {
			t.Errorf("RealFSType(%q) = false", fs)
		}
	}
	for _, fs := range []string{"tmpfs", "proc", "sysfs", "overlay"} {
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
