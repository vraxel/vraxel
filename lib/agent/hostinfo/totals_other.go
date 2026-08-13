//go:build !linux

package hostinfo

// memTotalBytes / diskTotalBytes are 0 off Linux: /proc does not exist and
// syscall.Statfs_t has a different layout. The stubs exist so the agent
// still builds and unit-tests on a macOS dev machine; the production agent
// is linux/amd64 and linux/arm64 only.
func memTotalBytes() int64        { return 0 }
func diskTotalBytes(string) int64 { return 0 }
