// Package hostinfo collects the facts vr-agent reports about the machine
// it runs on: the static hardware specs sent once at registration and the
// default-route interface address.
//
// It reports NO live utilisation: every host metric series comes from
// node_exporter (scraped in Step 11), so an agent-side /proc sample would
// have no consumer. Only the static totals (memory size, disk size) are
// read here, as inventory shown in the host list the moment a host is
// onboarded -- before node_exporter is even installed.
package hostinfo

import (
	"os"
	"runtime"
	"strings"
)

// Static describes the machine's fixed specs, reported at registration.
type Static struct {
	MachineID string
	Hostname  string
	OS        string
	Arch      string
	CPUCores  int32
	// MemoryMB / DiskGB carry BINARY units: memTotalBytes()/1024^2 and
	// diskTotalBytes()/1024^3, i.e. MiB / GiB. Named MB / GB (not MiB /
	// GiB) on purpose -- that matches hosts.memory_mb / disk_gb, the rest
	// of vraxel, and every OS tool (free -m, df -BG, VMware) that prints
	// "MB/GB" while meaning binary. Display-only host inventory; no code
	// does arithmetic that the MB-vs-MiB gap would break.
	MemoryMB       int64
	DiskGB         int64
	DefaultRouteIP string
}

// Collect gathers everything /register needs.
func Collect() Static {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	_, ip := DefaultRoute()
	return Static{
		MachineID:      MachineID(),
		Hostname:       host,
		OS:             osRelease(),
		Arch:           runtime.GOARCH,
		CPUCores:       int32(runtime.NumCPU()),
		MemoryMB:       memTotalBytes() / (1024 * 1024),
		DiskGB:         diskTotalBytes("/") / (1024 * 1024 * 1024),
		DefaultRouteIP: ip,
	}
}

// machineIDPaths are the stable per-installation identifiers, in the
// order systemd itself falls back through. /etc/machine-id is generated
// once at install and survives reboots, hostname changes and NIC swaps,
// which is exactly the stability the server's idempotent-registration
// derivation needs.
var machineIDPaths = []string{"/etc/machine-id", "/var/lib/dbus/machine-id"}

// MachineID returns this machine's stable identifier.
//
// The hostname fallback is deliberately weak and deliberately last: on a
// machine with no machine-id, two hosts sharing a hostname would derive
// the same agent id and fight over one host row. That is worse than the
// alternative only if it happens silently, so the agent logs the
// fallback at startup.
func MachineID() string {
	for _, p := range machineIDPaths {
		if b, err := os.ReadFile(p); err == nil {
			if id := strings.TrimSpace(string(b)); id != "" {
				return id
			}
		}
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		return ""
	}
	return "hostname:" + host
}

// osRelease returns a human-readable OS identifier such as
// "Rocky Linux 8.10". Falls back to GOOS when /etc/os-release is absent.
func osRelease() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return runtime.GOOS
	}
	var name, version string
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"`)
		switch k {
		case "NAME":
			name = v
		case "VERSION_ID":
			version = v
		}
	}
	switch {
	case name != "" && version != "":
		return name + " " + version
	case name != "":
		return name
	default:
		return runtime.GOOS
	}
}
