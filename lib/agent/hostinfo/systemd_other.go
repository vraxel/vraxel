//go:build !linux

package hostinfo

import agenttypes "vraxel.io/vraxel/lib/agent/types"

// SystemdUnits returns nothing off Linux, where there is no systemd to ask.
func SystemdUnits() []agenttypes.SystemdUnit { return nil }
