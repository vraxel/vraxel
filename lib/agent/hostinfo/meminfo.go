package hostinfo

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// parseMemTotalBytes reads the MemTotal line of /proc/meminfo and returns
// it in bytes. Pure over the file bytes so it unit-tests on a non-Linux
// dev machine. /proc/meminfo reports kB; the result is normalised to
// bytes. Only the total is read -- it is a static hardware spec (host
// inventory shown at registration), not a utilisation figure. Used and
// available memory are metrics and come from node_exporter.
func parseMemTotalBytes(data []byte) (int64, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || fields[0] != "MemTotal:" {
			continue
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse MemTotal %q: %w", fields[1], err)
		}
		return kb * 1024, nil
	}
	return 0, fmt.Errorf("/proc/meminfo has no MemTotal line")
}
