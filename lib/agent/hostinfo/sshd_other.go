//go:build !linux

package hostinfo

import agenttypes "vraxel.io/vraxel/lib/agent/types"

// sshdConfig returns nothing off Linux, where this collector's whole
// premise -- an sshd installed at a known path, run as root -- does not
// hold.
func sshdConfig() *agenttypes.SSHDConfig { return nil }
