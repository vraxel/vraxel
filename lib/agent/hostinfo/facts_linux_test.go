//go:build linux

package hostinfo

import (
	"os"
	"path/filepath"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// fakeNet builds a sysfs-shaped tree: one directory per interface, an
// optional uevent carrying DEVTYPE, an optional device/ entry standing in
// for the symlink real hardware has.
func fakeNet(t *testing.T, ifaces map[string]struct {
	devType  string
	hardware bool
},
) string {
	t.Helper()
	root := t.TempDir()
	for name, spec := range ifaces {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Every interface has a uevent; only the kernel's own device
		// classes put a DEVTYPE in it, which is the distinction under test.
		uevent := "INTERFACE=" + name + "\nIFINDEX=2\n"
		if spec.devType != "" {
			uevent = "DEVTYPE=" + spec.devType + "\n" + uevent
		}
		if err := os.WriteFile(filepath.Join(dir, "uevent"), []byte(uevent), 0o644); err != nil {
			t.Fatal(err)
		}
		if spec.hardware {
			if err := os.Mkdir(filepath.Join(dir, "device"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

// The regression this classifier exists for: bond, bridge and vlan
// interfaces have no hardware behind them, and the previous rule (drop
// anything without a device/ symlink) dropped all three. On a bonded
// machine the addresses are on the bond, so dropping it left a NIC list
// with no address on it anywhere.
func TestNICKindKeepsAddressCarryingKernelDevices(t *testing.T) {
	root := fakeNet(t, map[string]struct {
		devType  string
		hardware bool
	}{
		"eth0":     {hardware: true},
		"bond0":    {devType: "bond"},
		"br0":      {devType: "bridge"},
		"eth0.100": {devType: "vlan"},
		// Enslaved hardware keeps its own class: it is still a physical
		// port, it just no longer holds the address.
		"eth1": {hardware: true},
		// Neither hardware nor an address-carrying class. A tunnel's
		// address belongs to the tunnel, not to the machine's network
		// position, and wireguard and dummy are the same case.
		"wg0":  {},
		"tun0": {devType: "tun"},
	})

	for name, want := range map[string]string{
		"eth0":     agenttypes.NICPhysical,
		"eth1":     agenttypes.NICPhysical,
		"bond0":    agenttypes.NICBond,
		"br0":      agenttypes.NICBridge,
		"eth0.100": agenttypes.NICVLAN,
		"wg0":      "",
		"tun0":     "",
	} {
		if got := nicKind(filepath.Join(root, name)); got != want {
			t.Errorf("nicKind(%s) = %q, want %q", name, got, want)
		}
	}
}

// An interface directory that does not exist at all must classify as
// nothing rather than panic: /sys/class/net is read once and an interface
// can be torn down between the listing and the read.
func TestNICKindMissingDir(t *testing.T) {
	if got := nicKind(filepath.Join(t.TempDir(), "gone")); got != "" {
		t.Errorf("nicKind(missing) = %q, want empty", got)
	}
}

func TestLinkName(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "eth0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// sysfs writes master as a relative symlink up and back down.
	if err := os.Symlink("../bond0", filepath.Join(dir, "master")); err != nil {
		t.Fatal(err)
	}
	if got := linkName(dir, "master"); got != "bond0" {
		t.Errorf("master = %q, want bond0", got)
	}
	// A standalone interface has no master link, and no master.
	if got := linkName(dir, "driver"); got != "" {
		t.Errorf("absent link = %q, want empty", got)
	}
}
