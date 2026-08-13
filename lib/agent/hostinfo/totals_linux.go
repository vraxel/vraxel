//go:build linux

package hostinfo

import (
	"os"
	"syscall"
)

// memTotalBytes reads total physical memory from /proc/meminfo. A read or
// parse failure yields 0 -- a missing spec must never block registration.
func memTotalBytes() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	total, err := parseMemTotalBytes(data)
	if err != nil {
		return 0
	}
	return total
}

// diskTotalBytes reports the total size of the filesystem holding root.
// The root filesystem is the honest answer for "how much disk does this
// host have" in the inventory sense; per-mount detail belongs to
// node_exporter, which Step 11 wires up.
func diskTotalBytes(root string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(root, &st); err != nil {
		return 0
	}
	return int64(st.Blocks) * int64(st.Bsize)
}
