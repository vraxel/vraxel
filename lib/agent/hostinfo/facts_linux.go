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

	return agenttypes.HostFacts{
		Virtualization:    detectVirt(cpuinfo, vendor, product),
		CPUModel:          model,
		CPUSockets:        sockets,
		CPUCoresPerSocket: coresPerSocket,
		CPUThreadsPerCore: threadsPerCore,
		KernelVersion:     kernelVersion(),
		SystemVendor:      vendor,
		ProductName:       product,
		BIOSVersion:       dmiField("bios_version"),
		SerialNumber:      dmiField("product_serial"),
		Timezone:          timezone(),

		NICs:         nics(),
		Filesystems:  filesystems(),
		BlockDevices: blockDevices(),
	}
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
// container and bridge plumbing by name, which is what an operator means
// by "not my NIC". The device/ symlink check drops what remains of the
// kernel's invented interfaces -- bonds, vlans, tunnels -- which have no
// hardware behind them at all.
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
		if _, err := os.Stat(filepath.Join(dir, "device")); err != nil {
			continue
		}
		n := agenttypes.NIC{
			Name:  name,
			MAC:   strings.ToLower(sysStr(dir, "address")),
			State: sysStr(dir, "operstate"),
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
		n.IPv4, n.IPv6 = addrsOf(name)
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
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
		fs := agenttypes.Filesystem{Mount: m.mount, Device: m.device, FSType: m.fstype}
		var st syscall.Statfs_t
		// Statfs blocks forever on a wedged NFS mount. Not guarded here
		// because parseMounts already dropped everything that is not a
		// /dev/ device, and a local block device does not wedge a statfs.
		if err := syscall.Statfs(m.mount, &st); err == nil && st.Bsize > 0 {
			bs := int64(st.Bsize)
			fs.SizeBytes = int64(st.Blocks) * bs
			// Blocks-Bavail, not Blocks-Bfree: the reserved-for-root 5%
			// is unavailable to anything an operator runs, so counting it
			// as free is a number that disagrees with df.
			fs.UsedBytes = (int64(st.Blocks) - int64(st.Bavail)) * bs
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
		d := agenttypes.BlockDevice{
			Name:       e.Name(),
			Rotational: sysInt(filepath.Join(dir, "queue"), "rotational") == 1,
			Model:      sysStr(filepath.Join(dir, "device"), "model"),
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
