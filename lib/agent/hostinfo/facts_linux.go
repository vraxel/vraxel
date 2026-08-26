//go:build linux

package hostinfo

import (
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

const (
	dmiDir         = "/sys/class/dmi/id"
	sysNetDir      = "/sys/class/net"
	sysBlockDir    = "/sys/block"
	zoneinfoPrefix = "/usr/share/zoneinfo/"
)

// Facts collects the machine's inventory.
//
// Every read is best-effort and failure yields a zero value: this runs on
// customer machines with unknown kernels, in containers with half a /sys,
// and it must never be the reason an agent cannot report. An empty field
// renders as "-"; a refused collection renders as nothing at all.
func Facts() agenttypes.HostFacts {
	cpuinfo, _ := os.ReadFile("/proc/cpuinfo")
	model, sockets, coresPerSocket, threadsPerCore := parseCPUInfo(cpuinfo)
	vendor, product := dmiField("sys_vendor"), dmiField("product_name")
	osReleaseData, _ := os.ReadFile("/etc/os-release")
	osID, osVersionID := parseOSRelease(osReleaseData)
	route, _ := os.ReadFile("/proc/net/route")
	resolv, _ := os.ReadFile("/etc/resolv.conf")
	dnsServers, dnsSearch := parseResolvConf(resolv)
	swaps, _ := os.ReadFile("/proc/swaps")

	return agenttypes.HostFacts{
		Virtualization:    detectVirt(cpuinfo, vendor, product),
		CPUModel:          model,
		CPUSockets:        sockets,
		CPUCoresPerSocket: coresPerSocket,
		CPUThreadsPerCore: threadsPerCore,
		KernelVersion:     kernelVersion(),
		OSID:              osID,
		OSVersionID:       osVersionID,
		SystemVendor:      vendor,
		ProductName:       product,
		BIOSVersion:       dmiField("bios_version"),
		BIOSDate:          dmiField("bios_date"),
		BoardName:         dmiField("board_name"),
		BoardSerial:       dmiField("board_serial"),
		ChassisType:       chassisType(dmiInt("chassis_type")),
		SerialNumber:      dmiField("product_serial"),
		// The chassis tag is the one an operator burns in at racking; the
		// board tag is what a board vendor may have left. Chassis first,
		// board only when the chassis has nothing, because a replaced
		// board must not silently change a machine's asset identity.
		AssetTag:       firstNonEmpty(dmiField("chassis_asset_tag"), dmiField("board_asset_tag")),
		Timezone:       timezone(),
		DefaultGateway: parseRouteGateway(route),

		KernelCmdline: strings.TrimSpace(readFileString("/proc/cmdline")),
		ClockSync:     clockSync(),

		NICs:           nics(),
		Filesystems:    filesystems(),
		BlockDevices:   blockDevices(),
		Swaps:          parseSwaps(swaps),
		DNSServers:     dnsServers,
		DNSSearch:      dnsSearch,
		CPUMitigations: cpuMitigations(),
		SSHHostKeys:    sshHostKeys(),
	}
}

// cpuMitigationDir is where the kernel publishes its verdict on each
// hardware vulnerability it knows about, one file per name.
const cpuMitigationDir = "/sys/devices/system/cpu/vulnerabilities"

// cpuMitigations reads that directory. Sorted by name, because ReadDir
// order is not a promise and this rides a report compared byte for byte
// against the last one sent.
func cpuMitigations() []agenttypes.CPUMitigation {
	entries, err := os.ReadDir(cpuMitigationDir)
	if err != nil {
		// The directory is absent on architectures with nothing to report
		// and on kernels built without the reporting. Empty, not an
		// error: "no known vulnerabilities are tracked here" is the
		// truthful answer, and it is not the same as "not affected".
		return nil
	}
	out := make([]agenttypes.CPUMitigation, 0, len(entries))
	for _, e := range entries {
		status := strings.TrimSpace(readFileString(filepath.Join(cpuMitigationDir, e.Name())))
		if status == "" {
			continue
		}
		out = append(out, agenttypes.CPUMitigation{Name: e.Name(), Status: status})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// sshHostKeys fingerprints the machine's public host keys.
//
// Public halves only, by glob on the .pub files -- the private keys sit
// beside them under the same prefix and are never opened. What leaves the
// host is the algorithm and a SHA256 fingerprint, the same reduction the
// account inventory applies to authorized_keys.
func sshHostKeys() []agenttypes.SSHKey {
	paths, err := filepath.Glob("/etc/ssh/ssh_host_*_key.pub")
	if err != nil {
		return nil
	}
	sort.Strings(paths)
	var out []agenttypes.SSHKey
	for _, p := range paths {
		for _, k := range parseAuthorizedKeys(readFileBytes(p)) {
			// The comment on a host key is the hostname it was generated
			// on, which is stale on any machine that was ever renamed or
			// cloned from an image. Dropped rather than shown as fact.
			out = append(out, agenttypes.SSHKey{Type: k.Type, Fingerprint: k.Fingerprint})
		}
	}
	return out
}

// clockSync asks the kernel whether its clock is disciplined.
//
// adjtimex with no modes set is a pure query -- it cannot change the
// clock -- and needs no privileges. TIME_ERROR is the state the kernel
// reports when STA_UNSYNC is set, which is what timedatectl and ntpq are
// both reading underneath.
//
// A syscall rather than a file or a command because there is no portable
// file: this host runs chrony, which publishes nothing under /run, while
// the systemd-timesyncd flag file everyone reaches for first
// (/run/systemd/timesync/synchronized) does not exist here at all. The
// kernel's own view is the one answer every implementation feeds into.
func clockSync() string {
	var tx syscall.Timex
	state, err := syscall.Adjtimex(&tx)
	if err != nil {
		return ""
	}
	if state == timeError {
		return agenttypes.ClockUnsynced
	}
	return agenttypes.ClockSynced
}

// timeError is TIME_ERROR from <sys/timex.h>, the clock state meaning
// "not synchronised". Not in syscall, and one constant is cheaper than a
// dependency.
const timeError = 5

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// dmiField reads one SMBIOS string.
//
// Vendors ship placeholders where they have nothing to say, and a
// placeholder is worse than an empty field: "To Be Filled By O.E.M."
// looks like data, sorts, groups, and survives into a report. Filtered
// here so nothing downstream has to know the vocabulary.
func dmiField(name string) string {
	b, err := os.ReadFile(filepath.Join(dmiDir, name))
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(b))
	switch strings.ToLower(v) {
	case "", "unknown", "none", "n/a", "not specified", "not available",
		"no asset tag", "default string", "system serial number",
		"to be filled by o.e.m.", "to be filled by o.e.m", "0123456789":
		return ""
	}
	return v
}

// dmiInt reads one SMBIOS field that holds a number, -1 when there is
// none. Separate from dmiField because the placeholder filter there would
// have to know that "0" is a real chassis code and "0123456789" is not.
func dmiInt(name string) int64 {
	return sysInt(dmiDir, name)
}

// kernelVersion is uname -r, read from procfs rather than through
// syscall.Uname: the syscall returns a fixed [65]int8 that has to be
// hand-trimmed at the NUL, and this file holds exactly the same string.
func kernelVersion() string {
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// timezone is the IANA name, recovered from what /etc/localtime points
// at. /etc/timezone holds the name directly but is Debian-family only;
// the symlink is what systemd maintains everywhere.
func timezone() string {
	p, err := filepath.EvalSymlinks("/etc/localtime")
	if err != nil {
		return ""
	}
	if name, ok := strings.CutPrefix(p, zoneinfoPrefix); ok {
		return name
	}
	return ""
}

// nics lists the machine's own interfaces.
//
// Two filters, and they catch different things. RealNetDevice drops
// container plumbing by name, which is what an operator means by "not my
// NIC". nicKind then keeps what has hardware behind it OR is one of the
// three kernel devices that CARRY ADDRESSES -- bond, bridge, vlan -- and
// drops the rest (tunnels, wireguard, dummy).
//
// Keeping those three is the point. Enslave eth0 and eth1 to bond0 and
// the addresses move to bond0; filter bond0 out and the page shows two
// hardware ports with no address and no sign that the machine has one.
// The same happens to a hypervisor host whose address sits on br0 and to
// any tagged uplink. So the aggregate is listed alongside its members,
// and Master says which member belongs to which.
func nics() []agenttypes.NIC {
	entries, err := os.ReadDir(sysNetDir)
	if err != nil {
		return nil
	}
	var out []agenttypes.NIC
	for _, e := range entries {
		name := e.Name()
		if !agenttypes.RealNetDevice(name) {
			continue
		}
		dir := filepath.Join(sysNetDir, name)
		kind := nicKind(dir)
		if kind == "" {
			continue
		}
		n := agenttypes.NIC{
			Name:   name,
			MAC:    strings.ToLower(sysStr(dir, "address")),
			State:  sysStr(dir, "operstate"),
			Kind:   kind,
			Master: linkName(dir, "master"),
		}
		// A down link reports -1, and a driver that does not know reports
		// an error; both mean "no speed to show", not zero megabits. Same
		// shape for MTU, where an unreadable file must not become -1.
		if s := sysInt(dir, "speed"); s > 0 {
			n.SpeedMbps = int32(s)
		}
		if m := sysInt(dir, "mtu"); m > 0 {
			n.MTU = int32(m)
		}
		// duplex reads EINVAL on a down link, the same as speed above, so
		// a link that is not up simply has none. Matched against the two
		// values the kernel defines rather than passed through: it also
		// spells "unknown", which would render as a duplex mode.
		if d := sysStr(dir, "duplex"); d == "full" || d == "half" {
			n.Duplex = d
		}
		n.Driver = linkName(filepath.Join(dir, "device"), "driver")
		n.IPv4, n.IPv6 = addrsOf(name)
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// nicKind classifies one interface, empty for the ones not worth listing.
//
// DEVTYPE in uevent is how the kernel names its own device classes, and
// it is the only answer that does not involve guessing from the
// interface's name: a bond called "uplink" and a vlan called "storage"
// are both ordinary, and neither is recognisable by string matching.
// Hardware has no DEVTYPE at all, which is what the device/ symlink is
// left to answer.
func nicKind(dir string) string {
	switch devType(dir) {
	case "bond":
		return agenttypes.NICBond
	case "bridge":
		return agenttypes.NICBridge
	case "vlan":
		return agenttypes.NICVLAN
	}
	if _, err := os.Stat(filepath.Join(dir, "device")); err == nil {
		return agenttypes.NICPhysical
	}
	return ""
}

// devType reads DEVTYPE out of an interface's uevent file.
func devType(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "uevent"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "DEVTYPE="); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// linkName resolves a sysfs symlink to the last element of its target,
// which is the name of whatever it points at: the bond an interface is
// enslaved to, the driver module bound to a device.
func linkName(dir, name string) string {
	target, err := os.Readlink(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

// addrsOf splits an interface's addresses by family, in CIDR form.
func addrsOf(name string) (v4, v6 []string) {
	ni, err := net.InterfaceByName(name)
	if err != nil {
		return nil, nil
	}
	addrs, err := ni.Addrs()
	if err != nil {
		return nil, nil
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		// Link-local v6 is on every interface and identifies nothing;
		// listing it would bury the addresses that do.
		if ipnet.IP.IsLinkLocalUnicast() {
			continue
		}
		if ipnet.IP.To4() != nil {
			v4 = append(v4, ipnet.String())
			continue
		}
		v6 = append(v6, ipnet.String())
	}
	return v4, v6
}

// filesystems lists the real mounted filesystems with their current fill.
func filesystems() []agenttypes.Filesystem {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil
	}
	var out []agenttypes.Filesystem
	for _, m := range parseMounts(data) {
		fs := agenttypes.Filesystem{
			Mount: m.mount, Device: m.device, FSType: m.fstype, ReadOnly: m.readOnly,
		}
		var st syscall.Statfs_t
		// Statfs blocks forever on a wedged NFS mount. Not guarded here
		// because parseMounts rejects every network filesystem BY TYPE
		// before this line is reached, and a local block device does not
		// wedge a statfs. (This used to say "everything that is not a
		// /dev/ device", which was the older rule -- and the one that
		// dropped ZFS, whose source names a pool.)
		if err := syscall.Statfs(m.mount, &st); err == nil && st.Bsize > 0 {
			bs := int64(st.Bsize)
			fs.SizeBytes = int64(st.Blocks) * bs
			// Blocks-Bavail, not Blocks-Bfree: the reserved-for-root 5%
			// is unavailable to anything an operator runs, so counting it
			// as free is a number that disagrees with df.
			fs.UsedBytes = (int64(st.Blocks) - int64(st.Bavail)) * bs
			// btrfs and most network filesystems allocate inodes on
			// demand and report Files as 0. That is "no limit to show",
			// not "no inodes left", so both fields stay absent.
			if st.Files > 0 {
				fs.InodesTotal = int64(st.Files)
				fs.InodesUsed = int64(st.Files) - int64(st.Ffree)
			}
		}
		out = append(out, fs)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mount < out[j].Mount })
	return out
}

// blockDevices lists whole disks.
//
// /sys/block holds only whole devices -- partitions live under them -- so
// no partition filtering is needed. Two exclusions remain: entries with
// no device/ symlink (loop, ram, device-mapper targets, which are views
// of storage counted elsewhere) and removable media (the CD-ROM every
// hypervisor attaches, and USB sticks, which are not the machine's
// storage).
func blockDevices() []agenttypes.BlockDevice {
	entries, err := os.ReadDir(sysBlockDir)
	if err != nil {
		return nil
	}
	var out []agenttypes.BlockDevice
	for _, e := range entries {
		dir := filepath.Join(sysBlockDir, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "device")); err != nil {
			continue
		}
		if sysInt(dir, "removable") == 1 {
			continue
		}
		devDir := filepath.Join(dir, "device")
		d := agenttypes.BlockDevice{
			Name:       e.Name(),
			Rotational: sysInt(filepath.Join(dir, "queue"), "rotational") == 1,
			Vendor:     diskVendor(sysStr(devDir, "vendor")),
			Model:      sysStr(devDir, "model"),
			// NVMe publishes the drive serial directly; SCSI and SATA
			// publish the VPD identifier instead, and wwid is where the
			// kernel writes it. Neither is present on most virtual disks,
			// which have no identity to publish.
			Serial: firstNonEmpty(sysStr(devDir, "serial"), sysStr(devDir, "wwid")),
		}
		// size is in 512-byte sectors regardless of the device's own
		// block size; this is a kernel ABI constant, not a guess.
		if s := sysInt(dir, "size"); s > 0 {
			d.SizeBytes = s * 512
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func sysStr(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// sysInt returns -1 for anything unreadable, so callers can tell "the
// kernel said zero" from "there was nothing to read" -- the difference
// between a link with no negotiated speed and a disk of size 0.
func sysInt(dir, name string) int64 {
	v, err := strconv.ParseInt(sysStr(dir, name), 10, 64)
	if err != nil {
		return -1
	}
	return v
}
