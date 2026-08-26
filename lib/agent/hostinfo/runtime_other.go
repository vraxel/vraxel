//go:build !linux

package hostinfo

import agenttypes "vraxel.io/vraxel/lib/agent/types"

// Processes and Accounts read /proc, /etc/shadow and sudoers, none of
// which exist off Linux. Empty rather than an error, matching Facts: the
// agent is built for other platforms only so the repository compiles on a
// developer's machine, and a collector that refuses is no more useful
// there than one that finds nothing.

// Processes returns nothing off Linux.
func Processes() agenttypes.HostProcesses { return agenttypes.HostProcesses{} }

// ProcessesLive returns nothing off Linux.
func ProcessesLive() agenttypes.HostProcesses { return agenttypes.HostProcesses{} }

// Accounts returns nothing off Linux.
func Accounts() agenttypes.HostAccounts { return agenttypes.HostAccounts{} }
