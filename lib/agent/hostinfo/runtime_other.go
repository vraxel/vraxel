//go:build !linux

package hostinfo

import agenttypes "vraxel.io/vraxel/lib/agent/types"

// The workload and account collectors read /proc, /etc/shadow and
// sudoers, none of which exist off Linux. Empty rather than an error,
// matching Facts: the agent is built for other platforms only so the
// repository compiles on a developer's machine, and a collector that
// refuses is no more useful there than one that finds nothing.
//
// Stubbed at the two /proc-reading primitives rather than at the public
// functions, so everything above them -- Processes, the cpu sampler, the
// grouping and the arithmetic -- is one implementation compiled
// everywhere and testable here.

func collectProcesses(map[int64]float64) agenttypes.HostProcesses {
	return agenttypes.HostProcesses{}
}

func sampleJiffies() map[int64]int64 { return nil }

// Accounts returns nothing off Linux.
func Accounts() agenttypes.HostAccounts { return agenttypes.HostAccounts{} }
