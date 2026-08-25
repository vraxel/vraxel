package hostinfo

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// cpuTopology is what /proc/cpuinfo says about the processors.
//
// Sockets counts DISTINCT "physical id" values rather than lines: on a
// multi-socket machine every logical CPU repeats its socket's id, and on
// a VMware guest the ids are not even contiguous (0, 2, 4, 6 for four
// sockets), so neither the count of lines nor the maximum id is the
// answer.
func parseCPUInfo(data []byte) (model string, sockets, coresPerSocket, threadsPerCore int32) {
	physical := map[string]struct{}{}
	logical := int32(0)
	cores, siblings := int32(0), int32(0)

	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "processor":
			logical++
		case "model name":
			if model == "" {
				model = v
			}
		case "physical id":
			physical[v] = struct{}{}
		case "cpu cores":
			if n, err := strconv.Atoi(v); err == nil && cores == 0 {
				cores = int32(n)
			}
		case "siblings":
			if n, err := strconv.Atoi(v); err == nil && siblings == 0 {
				siblings = int32(n)
			}
		}
	}

	// A kernel that omits "physical id" (many ARM boards, some
	// containers) still has processors, and one socket is the truthful
	// reading of "all of them, with nothing saying otherwise".
	sockets = int32(len(physical))
	if sockets == 0 && logical > 0 {
		sockets = 1
	}
	coresPerSocket = cores
	if coresPerSocket == 0 && sockets > 0 {
		coresPerSocket = logical / sockets
	}
	if cores > 0 && siblings > 0 {
		threadsPerCore = siblings / cores
	}
	return model, sockets, coresPerSocket, threadsPerCore
}

// hypervisorVendors maps a DMI identity to a virtualization name. Matched
// as a substring against sys_vendor and product_name together, because
// which of the two carries the brand varies by hypervisor: VMware puts it
// in both, KVM only in product_name ("KVM Virtual Machine"), Hyper-V
// splits it across the pair.
//
// Order matters. "Microsoft Corporation" also appears on physical Surface
// hardware, so it is checked last and only reached when the CPUID
// hypervisor bit already established that this is a guest.
var hypervisorVendors = []struct {
	needle string
	virt   string
}{
	{"vmware", agenttypes.VirtVMware},
	{"virtualbox", agenttypes.VirtVirtualBox},
	{"innotek", agenttypes.VirtVirtualBox},
	{"parallels", agenttypes.VirtParallels},
	{"xen", agenttypes.VirtXen},
	{"bhyve", agenttypes.VirtBhyve},
	{"kvm", agenttypes.VirtKVM},
	{"amazon ec2", agenttypes.VirtKVM},
	{"alibaba", agenttypes.VirtKVM},
	{"openstack", agenttypes.VirtKVM},
	{"bochs", agenttypes.VirtQEMU},
	{"qemu", agenttypes.VirtQEMU},
	{"microsoft", agenttypes.VirtHyperV},
}

// detectVirt decides what runs this machine.
//
// Two independent signals, in this order:
//
//  1. The CPUID hypervisor bit, surfaced as the "hypervisor" CPU flag.
//     The CPU itself reporting that it is virtualised is as close to
//     authoritative as this gets, and no hypervisor in normal
//     configuration hides it.
//  2. DMI strings, to name WHICH hypervisor.
//
// Splitting them is what makes "physical" trustworthy. Guessing from DMI
// alone would call a Dell PowerEdge a guest the moment somebody shipped a
// board whose vendor string contains one of these needles; requiring the
// CPU flag first means a bare-metal machine can only ever be reported as
// physical. And a guest whose DMI is scrubbed (some clouds do this)
// lands on VirtUnknown rather than being called bare metal, which is the
// one wrong answer that would matter.
func detectVirt(cpuinfo []byte, sysVendor, productName string) string {
	if !hasHypervisorFlag(cpuinfo) {
		return agenttypes.VirtPhysical
	}
	dmi := strings.ToLower(sysVendor + " " + productName)
	for _, h := range hypervisorVendors {
		if strings.Contains(dmi, h.needle) {
			return h.virt
		}
	}
	return agenttypes.VirtUnknown
}

// hasHypervisorFlag reports whether the CPU advertises CPUID.1:ECX[31].
//
// Scanned as a whole field rather than with strings.Contains over the
// file, because "hypervisor" is also a substring of nothing else in
// /proc/cpuinfo today and that is exactly the kind of thing a kernel
// release changes.
func hasHypervisorFlag(cpuinfo []byte) bool {
	sc := bufio.NewScanner(bytes.NewReader(cpuinfo))
	// flags on a machine with many CPU features exceeds bufio's default
	// 64 KiB line budget on some kernels; the whole file is small.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok || strings.TrimSpace(k) != "flags" {
			continue
		}
		for _, f := range strings.Fields(v) {
			if f == "hypervisor" {
				return true
			}
		}
	}
	return false
}

// mountEntry is one line of /proc/mounts.
type mountEntry struct {
	device   string
	mount    string
	fstype   string
	readOnly bool
}

// parseMounts returns this machine's own filesystems, first mount of
// each device only.
//
// The device dedup is the same rule the metrics summary applies (see
// nodemetrics.summaryDisk): a bind mount and a btrfs subvolume are extra
// mountpoints over blocks that were already counted, so listing them
// would show an operator more storage than the machine has. Keeping the
// FIRST occurrence keeps the shortest path, which is the one a person
// would name.
func parseMounts(data []byte) []mountEntry {
	var out []mountEntry
	seen := map[string]struct{}{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		// Filtered by TYPE, not by whether the source looks like
		// /dev/something. ZFS names a pool and btrfs can name a subvolume,
		// so a device-path test would drop two real local filesystems --
		// and it would drop exactly the ones the metrics summary keeps,
		// which is the disagreement RealFSType exists to prevent. Type is
		// also what keeps statfs off a wedged NFS mount, since the network
		// types are excluded before the call below is ever reached.
		if len(f) < 3 || !agenttypes.RealFSType(f[2]) {
			continue
		}
		if _, dup := seen[f[0]]; dup {
			continue
		}
		seen[f[0]] = struct{}{}
		// The kernel octal-escapes spaces and tabs in both fields.
		e := mountEntry{device: unescapeMount(f[0]), mount: unescapeMount(f[1]), fstype: f[2]}
		if len(f) > 3 {
			e.readOnly = hasMountOption(f[3], "ro")
		}
		out = append(out, e)
	}
	return out
}

// hasMountOption reports whether a comma-separated option list contains
// one option exactly.
//
// Exact tokens, not a substring search: "ro" appears inside "errors=
// remount-ro", which is the DEFAULT on ext4 and would make every healthy
// root filesystem report itself faulted.
func hasMountOption(opts, want string) bool {
	for opts != "" {
		var o string
		o, opts, _ = strings.Cut(opts, ",")
		if o == want {
			return true
		}
	}
	return false
}

// chassisTypes maps the SMBIOS enclosure enum (DSP0134 table 17) onto the
// Chassis* groups. Only the values a server room contains are listed:
// everything else -- and every hypervisor, which reports 1 "Other" --
// falls through to empty, because naming an enclosure the machine could
// not identify is worse than showing nothing.
var chassisTypes = map[int64]string{
	3:  agenttypes.ChassisDesktop, // Desktop
	4:  agenttypes.ChassisDesktop, // Low Profile Desktop
	5:  agenttypes.ChassisDesktop, // Pizza Box
	6:  agenttypes.ChassisTower,   // Mini Tower
	7:  agenttypes.ChassisTower,   // Tower
	8:  agenttypes.ChassisLaptop,  // Portable
	9:  agenttypes.ChassisLaptop,  // Laptop
	10: agenttypes.ChassisLaptop,  // Notebook
	14: agenttypes.ChassisLaptop,  // Sub Notebook
	17: agenttypes.ChassisServer,  // Main Server Chassis
	23: agenttypes.ChassisRack,    // Rack Mount Chassis
	28: agenttypes.ChassisBlade,   // Blade
	29: agenttypes.ChassisBlade,   // Blade Enclosure
	30: agenttypes.ChassisLaptop,  // Tablet
	31: agenttypes.ChassisLaptop,  // Convertible
	32: agenttypes.ChassisLaptop,  // Detachable
}

// chassisType translates one SMBIOS chassis code.
func chassisType(code int64) string {
	return chassisTypes[code]
}

// parseRouteGateway returns the IPv4 next hop for 0.0.0.0/0 from
// /proc/net/route, or empty when the machine has no default route.
//
// The addresses in this file are the kernel's in-memory 32-bit words
// printed as hex, so on every little-endian machine "0201010A" is
// 10.1.1.2 read back to front. Decoded a byte at a time rather than with
// a bit-shift, so the code says which byte goes where.
//
// Lowest metric wins. A machine with two uplinks has two default routes
// and the kernel uses the cheaper one; reporting whichever came first in
// the file would name the standby gateway half the time.
func parseRouteGateway(data []byte) string {
	best, found := int64(0), ""
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		// Iface Destination Gateway Flags RefCnt Use Metric ...
		if len(f) < 7 || f[1] != "00000000" {
			continue
		}
		// RTF_UP|RTF_GATEWAY. A default route with no gateway bit is an
		// on-link route out an interface, which has no next hop to name.
		flags, err := strconv.ParseUint(f[3], 16, 32)
		if err != nil || flags&0x0002 == 0 {
			continue
		}
		metric, err := strconv.ParseInt(f[6], 10, 64)
		if err != nil {
			continue
		}
		ip := decodeRouteAddr(f[2])
		if ip == "" || (found != "" && metric >= best) {
			continue
		}
		best, found = metric, ip
	}
	return found
}

// decodeRouteAddr turns one 8-digit little-endian hex word into dotted
// quad form.
func decodeRouteAddr(hex string) string {
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || v == 0 {
		return ""
	}
	return strconv.FormatUint(v&0xff, 10) + "." +
		strconv.FormatUint(v>>8&0xff, 10) + "." +
		strconv.FormatUint(v>>16&0xff, 10) + "." +
		strconv.FormatUint(v>>24&0xff, 10)
}

// parseOSRelease returns /etc/os-release's ID and VERSION_ID.
//
// Separate from hostinfo's osRelease, which builds the display string the
// hello carries: that one wants NAME ("Debian GNU/Linux") because it is
// read by a person, this one wants ID ("debian") because it is compared
// by a query. Sharing a parser would mean one of the two callers reading
// a field it has no use for.
func parseOSRelease(data []byte) (id, versionID string) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		switch k {
		case "ID":
			id = strings.Trim(v, `"`)
		case "VERSION_ID":
			versionID = strings.Trim(v, `"`)
		}
	}
	return id, versionID
}

// unescapeMount undoes the \0NN octal escaping /proc/mounts applies to
// space, tab, newline and backslash.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
