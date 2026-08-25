package types

import (
	"sort"
	"strings"
)

// HostFacts is the machine's inventory: what it IS, as opposed to what it
// is currently doing. Reported on the host.facts frame.
//
// Split in two on purpose, and the split is load-bearing for storage:
// the scalars are single-valued and comparable across a fleet, so they
// become columns somebody can filter and sort on ("every VMware guest",
// "every kernel older than X"). The lists are per-machine detail nobody
// filters a fleet by, so they become one JSONB row read only when a
// detail page is open. Putting the lists in columns would mean nic1_mac /
// nic2_mac; putting the scalars in JSONB would mean an index per query.
//
// Every field is optional. A container has no DMI, a cloud image may have
// no serial, and a machine that cannot answer must still register.
type HostFacts struct {
	// --- scalars ---
	// Virtualization is one of the Virt* constants: what runs this
	// machine. The first thing a CMDB asks about a host and the one
	// classification the platform currently cannot answer at all.
	Virtualization string `json:"virtualization,omitempty"`
	CPUModel       string `json:"cpuModel,omitempty"`
	// The topology behind HostSpec.CPUCores, which is a count of LOGICAL
	// CPUs and says nothing about their shape. A hypervisor routinely
	// presents 4 sockets of 1 core where the hardware has 1 socket of 4:
	// same logical count, different licensing, different NUMA behaviour,
	// and indistinguishable without these three.
	CPUSockets        int32 `json:"cpuSockets,omitempty"`
	CPUCoresPerSocket int32 `json:"cpuCoresPerSocket,omitempty"`
	CPUThreadsPerCore int32 `json:"cpuThreadsPerCore,omitempty"`
	// KernelVersion is uname -r. Distinct from HostSpec.OS ("Debian 13")
	// and far more useful: patch compliance is decided by the kernel, not
	// by the release name.
	KernelVersion string `json:"kernelVersion,omitempty"`
	SystemVendor  string `json:"systemVendor,omitempty"`
	ProductName   string `json:"productName,omitempty"`
	BIOSVersion   string `json:"biosVersion,omitempty"`
	// SerialNumber is the DMI product serial. Meaningful on physical
	// hardware; on a guest it is a restatement of the SMBIOS UUID, which
	// is why the UI shows it only when Virtualization is VirtPhysical.
	SerialNumber string `json:"serialNumber,omitempty"`
	Timezone     string `json:"timezone,omitempty"`

	// No uptime or boot time here, though the detail page shows one. It
	// already arrives on every hello inside MachineFingerprint, where the
	// server dates it against its OWN clock and stores host_agents.boot_at
	// -- and the whole value of THIS struct is that its content sits
	// still, so the agent can compare it against the last one it sent and
	// stay silent. A field that changes every sample would make every
	// resample look like a hardware change.

	// --- lists ---
	NICs         []NIC         `json:"nics,omitempty"`
	Filesystems  []Filesystem  `json:"filesystems,omitempty"`
	BlockDevices []BlockDevice `json:"blockDevices,omitempty"`
}

// Virtualization values. Anything unrecognised is reported as VirtUnknown
// rather than guessed at: "we could not tell" and "bare metal" are
// different answers and an operator acts differently on each.
const (
	VirtPhysical   = "physical"
	VirtVMware     = "vmware"
	VirtKVM        = "kvm"
	VirtQEMU       = "qemu"
	VirtXen        = "xen"
	VirtHyperV     = "hyperv"
	VirtVirtualBox = "virtualbox"
	VirtParallels  = "parallels"
	VirtBhyve      = "bhyve"
	VirtUnknown    = "unknown"
)

// NIC is one network interface that belongs to the machine. Virtual
// plumbing (veth, docker0, bridges) is filtered out at collection --
// see RealNetDevice.
type NIC struct {
	Name string `json:"name"`
	MAC  string `json:"mac,omitempty"`
	// IPv4 / IPv6 are in CIDR form: an address without its prefix length
	// cannot be placed on a network, which is most of why anyone reads
	// this list.
	IPv4 []string `json:"ipv4,omitempty"`
	IPv6 []string `json:"ipv6,omitempty"`
	// SpeedMbps is 0 when the link is down or the driver does not report
	// it (every virtio and many virtual NICs).
	SpeedMbps int32  `json:"speedMbps,omitempty"`
	MTU       int32  `json:"mtu,omitempty"`
	State     string `json:"state,omitempty"`
}

// Filesystem is one mounted real filesystem.
type Filesystem struct {
	Mount     string `json:"mount"`
	Device    string `json:"device,omitempty"`
	FSType    string `json:"fstype,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	UsedBytes int64  `json:"usedBytes,omitempty"`
}

// BlockDevice is one whole disk, partitions excluded.
type BlockDevice struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	// Rotational separates spinning rust from SSD/NVMe. A hypervisor lies
	// about this as often as not, which is worth knowing in itself.
	Rotational bool   `json:"rotational,omitempty"`
	Model      string `json:"model,omitempty"`
}

// notLocalStorage are the filesystem types that are not this machine's
// disk, in two groups.
//
// The kernel's bookkeeping and memory-backed filesystems: a full /run or
// /dev/shm is a memory problem wearing a filesystem's clothes, and it
// would pin the disk gauge at a number no operator can act on.
//
// And the network ones. An NFS share is real storage, but it is the
// FILE SERVER's disk: counting it toward this host's capacity inflates
// the fleet's total by however many hosts mount the same export, and a
// filling share would raise the alert on every client instead of on the
// machine that owns it. node_exporter's filesystem collector reports
// them (its default exclusion list covers only the pseudo group), so
// they have to be excluded here or they are silently counted.
//
// This predicate and RealNetDevice live in the wire contract because
// they define what the fields around them MEAN. The host list's disk
// gauge (nodemetrics' summary) and the detail page's filesystem table
// are two views of one machine, and they have to agree on what counts --
// a table whose rows do not add up to the number printed beside them is
// worse than either view alone.
var notLocalStorage = map[string]struct{}{
	// pseudo / memory-backed
	"tmpfs": {}, "ramfs": {}, "devtmpfs": {},
	"proc": {}, "sysfs": {}, "devpts": {}, "cgroup": {}, "cgroup2": {},
	"securityfs": {}, "pstore": {}, "bpf": {}, "autofs": {}, "hugetlbfs": {},
	"mqueue": {}, "debugfs": {}, "tracefs": {}, "fusectl": {}, "configfs": {},
	"binfmt_misc": {}, "rpc_pipefs": {}, "nsfs": {}, "squashfs": {},
	"overlay": {}, "fuse.gvfsd-fuse": {}, "efivarfs": {}, "iso9660": {},
	"erofs": {}, "selinuxfs": {}, "procfs": {},
	// network / remote
	"nfs": {}, "nfs4": {}, "cifs": {}, "smbfs": {}, "smb3": {},
	"afs": {}, "ceph": {}, "glusterfs": {}, "lustre": {}, "beegfs": {},
	"9p": {}, "ncpfs": {}, "coda": {}, "gpfs": {},
	"fuse.sshfs": {}, "fuse.s3fs": {}, "fuse.rclone": {}, "fuse.glusterfs": {},
}

// RealFSType reports whether a filesystem type describes storage
// attached to this machine.
func RealFSType(fstype string) bool {
	_, skip := notLocalStorage[fstype]
	return !skip
}

// NonLocalFSTypes lists what RealFSType rejects, sorted.
//
// Exported so the VictoriaMetrics backend can build the equivalent
// PromQL label matcher from this same set. The two metric backends
// promise to answer identically for the same window, and the only way to
// keep two hand-written exclusion lists agreeing is to not have two.
func NonLocalFSTypes() []string {
	out := make([]string, 0, len(notLocalStorage))
	for k := range notLocalStorage {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// virtualNetPrefixes are the per-container and per-bridge interface
// families. A packet crossing a bridge is counted again on every veth it
// traverses, and an operator reading a NIC list wants the machine's
// interfaces, not its container plumbing.
var virtualNetPrefixes = []string{
	"veth", "docker", "br-", "virbr", "cni", "flannel", "cali", "tunl",
	"nodelocaldns", "kube-ipvs", "dummy", "lxc", "tap",
}

// VirtualNetPrefixes lists the interface-name prefixes RealNetDevice
// rejects, plus the loopback it rejects by exact name. Exported for the
// same reason as NonLocalFSTypes: one membership rule, two query
// languages.
func VirtualNetPrefixes() (prefixes []string, exact []string) {
	return append([]string(nil), virtualNetPrefixes...), []string{"lo"}
}

// RealNetDevice reports whether an interface belongs to the machine
// rather than to something running on it.
func RealNetDevice(name string) bool {
	if name == "lo" {
		return false
	}
	for _, p := range virtualNetPrefixes {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	return true
}
