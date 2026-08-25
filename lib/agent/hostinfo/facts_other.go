//go:build !linux

package hostinfo

import agenttypes "vraxel.io/vraxel/lib/agent/types"

// Facts is empty off Linux. The agent ships for Linux hosts; this exists
// so the shared lib still builds on a developer's machine.
func Facts() agenttypes.HostFacts { return agenttypes.HostFacts{} }
