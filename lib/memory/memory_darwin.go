package memory

import "vraxel.io/vraxel/lib/logger"

// This has been adapted from github.com/pbnjay/memory.
func sysTotalMemory() int {
	s, err := sysctlUint64("hw.memsize")
	if err != nil {
		logger.Panicf("FATAL: cannot determine system memory: %s", err)
	}
	return int(s)
}
