//go:build linux

package hostinfo

import (
	"context"
	"sort"
	"strings"
	"time"

	sd "github.com/coreos/go-systemd/v22/dbus"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// systemdTimeout bounds one collection. The socket is local and the two
// calls are cheap, so anything approaching this is a systemd that has
// stopped answering -- which must not stall the loop that reports the
// machine's workload.
const systemdTimeout = 5 * time.Second

// SystemdUnits reports what the supervisor was told to run: every enabled
// service, plus anything that has failed whatever its install state.
//
// Over systemd's PRIVATE socket rather than the system bus, and over
// D-Bus rather than by running systemctl. Three reasons, in order of how
// much they cost to get wrong:
//
// Failed state is not in any file. A unit that fails leaves nothing under
// /run/systemd/units -- verified by failing one on purpose -- so a
// file-reading collector cannot answer the question this exists for.
//
// D-Bus returns typed structs. systemctl returns text that is localised,
// column-aligned for a terminal, and free to change between releases;
// parsing it would put a display format in the path of a collector that
// runs unattended on every host.
//
// And it costs no new dependency: go-systemd and godbus are already in
// this module's graph. The private socket avoids dbus-daemon entirely,
// which is one less thing that has to be running for this to work.
func SystemdUnits() []agenttypes.SystemdUnit {
	ctx, cancel := context.WithTimeout(context.Background(), systemdTimeout)
	defer cancel()

	conn, err := sd.NewSystemdConnectionContext(ctx)
	if err != nil {
		// No systemd, or not root. Nothing is the honest answer: this
		// machine has no supervisor whose intentions we can report.
		return nil
	}
	defer conn.Close()

	// Install state first, so the runtime pass knows which units are
	// meant to be running. Keyed by unit file name, which is the unit
	// name -- ListUnitFiles returns absolute paths on some versions and
	// bare names on others, so only the last element is used.
	enabled := map[string]string{}
	files, err := conn.ListUnitFilesContext(ctx)
	if err != nil {
		return nil
	}
	for _, f := range files {
		if f.Type != "enabled" {
			continue
		}
		name := baseName(f.Path)
		// A template ("getty@.service") is not a runnable unit -- only
		// its instances are, and those appear in ListUnits under their
		// own names. Listing the template would report it as inactive
		// forever, which is true of every template and means nothing.
		if strings.Contains(name, "@.") {
			continue
		}
		enabled[name] = f.Type
	}

	units, err := conn.ListUnitsContext(ctx)
	if err != nil {
		return nil
	}
	out := make([]agenttypes.SystemdUnit, 0, len(enabled))
	for _, u := range units {
		// A unit is worth a row if it was meant to be running, or if it
		// failed. A failed unit that nobody enabled still matters: it was
		// pulled in by something, and it broke.
		state, want := enabled[u.Name]
		if !want && u.ActiveState != "failed" {
			continue
		}
		out = append(out, agenttypes.SystemdUnit{
			Name: u.Name, Enabled: state, Active: u.ActiveState,
			Sub: u.SubState, Description: u.Description,
		})
		delete(enabled, u.Name)
	}
	// Whatever is left was enabled but is not loaded at all -- systemd
	// has never been asked to start it since boot. Reported rather than
	// dropped: "enabled and absent" is the same finding as "enabled and
	// dead", and dropping it would make the list quietly incomplete.
	for name, state := range enabled {
		out = append(out, agenttypes.SystemdUnit{Name: name, Enabled: state, Active: "inactive"})
	}
	// Sorted, because neither call promises an order and this rides a
	// report compared byte for byte against the last one sent.
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// baseName is the last path element, for a value that may arrive as
// either "/usr/lib/systemd/system/ssh.service" or "ssh.service".
func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}
