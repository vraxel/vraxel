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
	device string
	mount  string
	fstype string
}

// parseMounts returns the real filesystems, first mount of each device
// only.
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
		if len(f) < 3 || !agenttypes.RealFSType(f[2]) {
			continue
		}
		// Only real block devices. A network or fuse mount whose source
		// is a URL is storage, but it is not THIS machine's storage, and
		// the inventory question is what this machine has.
		if !strings.HasPrefix(f[0], "/dev/") {
			continue
		}
		if _, dup := seen[f[0]]; dup {
			continue
		}
		seen[f[0]] = struct{}{}
		// The kernel octal-escapes spaces and tabs in both fields.
		out = append(out, mountEntry{device: unescapeMount(f[0]), mount: unescapeMount(f[1]), fstype: f[2]})
	}
	return out
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
