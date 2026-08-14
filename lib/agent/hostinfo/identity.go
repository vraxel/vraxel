package hostinfo

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// dmiUUIDPaths hold the SMBIOS system UUID. The /sys/class path is the
// stable ABI; the /sys/devices one is where it actually lives, kept as a
// fallback for kernels that do not export the class symlink.
//
// Both are mode 0400: readable only by root, which the agent is.
var dmiUUIDPaths = []string{
	"/sys/class/dmi/id/product_uuid",
	"/sys/devices/virtual/dmi/id/product_uuid",
}

var uuidShape = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// junkProductUUIDs are values that identify a vendor's firmware defaults
// rather than a machine. Shipped identically across whole production
// batches, so treating one as an identity would merge every host that has
// it into a single row -- the exact failure this signal exists to prevent,
// with a far bigger blast radius than the clone case.
//
// 03000200-0400-0500-0006-000700080009 is the AMI sample UUID, present on
// a great many whitebox boards.
var junkProductUUIDs = map[string]bool{
	"00000000-0000-0000-0000-000000000000": true,
	"ffffffff-ffff-ffff-ffff-ffffffffffff": true,
	"03000200-0400-0500-0006-000700080009": true,
}

// ProductUUID returns the machine's SMBIOS UUID, lowercased, or "" when
// there is none worth trusting.
//
// Empty is a legitimate answer, not an error: containers and many ARM
// boards have no DMI at all. The server treats "" as "this machine cannot
// be auto-identified" and falls back to operator judgement rather than
// guessing.
func ProductUUID() string {
	for _, p := range dmiUUIDPaths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		v := strings.ToLower(strings.TrimSpace(string(b)))
		if v == "" || !uuidShape.MatchString(v) || junkProductUUIDs[v] {
			continue
		}
		return v
	}
	return ""
}

// MACs returns the permanent hardware addresses of this machine's
// physical interfaces, sorted so the same machine reports the same list
// across reboots regardless of enumeration order.
//
// Two filters, both aimed at the same thing -- only report addresses that
// belong to hardware:
//
//   - a device/ symlink: excludes lo, bridges, veth, bonds and every other
//     virtual interface, whose addresses are invented by the kernel or
//     inherited from a slave.
//   - addr_assign_type == 0 (NET_ADDR_PERM): excludes randomised and
//     administratively set addresses, which change without the machine
//     changing.
func MACs() []string {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		dir := filepath.Join("/sys/class/net", e.Name())
		if _, err := os.Stat(filepath.Join(dir, "device")); err != nil {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(dir, "addr_assign_type")); err == nil {
			if strings.TrimSpace(string(b)) != "0" {
				continue
			}
		}
		b, err := os.ReadFile(filepath.Join(dir, "address"))
		if err != nil {
			continue
		}
		mac := strings.ToLower(strings.TrimSpace(string(b)))
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		out = append(out, mac)
	}
	sort.Strings(out)
	return out
}

// uptimeSeconds returns how long this machine has been up, or 0 when that
// cannot be read.
//
// The server dates the boot itself (now() - uptime) rather than taking a
// timestamp from the agent: a freshly provisioned VM routinely has a wall
// clock years out until NTP lands, and a boot time computed from it would
// place the machine's existence in the wrong decade -- turning the
// coexistence test that separates clones from hardware swaps into noise.
func uptimeSeconds() int64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(b)), " ")
	secs, err := strconv.ParseFloat(first, 64)
	if err != nil || secs < 0 {
		return 0
	}
	return int64(secs)
}
