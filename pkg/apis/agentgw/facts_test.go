package agentgw

import (
	"context"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// A host.facts frame lands in the store keyed by the session's host, with
// the scalars carried across and the lists re-encoded as jsonb. An absent
// payload writes nothing, which is what a frame from a future agent that
// dropped the field would look like.
func TestRecordFacts(t *testing.T) {
	store := &fakeAgentStore{}
	h := &protocolHandler{agents: store}
	sess := &Session{AgentID: "a-1", HostID: 42}

	h.recordFacts(context.Background(), sess, nil)
	if len(store.factsCalls()) != 0 {
		t.Fatalf("a frame without facts must not write: %+v", store.facts)
	}

	h.recordFacts(context.Background(), sess, &agenttypes.HostFacts{
		Virtualization:    agenttypes.VirtVMware,
		CPUModel:          "Intel(R) Core(TM) i7-8850H CPU @ 2.60GHz",
		CPUSockets:        4,
		CPUCoresPerSocket: 1,
		CPUThreadsPerCore: 1,
		KernelVersion:     "6.12.96+deb13-amd64",
		SystemVendor:      "VMware, Inc.",
		ProductName:       "VMware Virtual Platform",
		BIOSVersion:       "6.00",
		NICs: []agenttypes.NIC{
			{Name: "ens33", MAC: "00:0c:29:c4:67:a6", IPv4: []string{"10.1.1.10/24"}, SpeedMbps: 1000, MTU: 1500, State: "up"},
		},
		Filesystems: []agenttypes.Filesystem{
			{Mount: "/", Device: "/dev/mapper/debian--vg-root", FSType: "ext4", SizeBytes: 103079215104, UsedBytes: 38807334912},
		},
	})

	calls := store.factsCalls()
	if len(calls) != 1 {
		t.Fatalf("facts not stored: %+v", calls)
	}
	got := calls[0]
	if got.hostID != 42 {
		t.Fatalf("stored under host %d, want 42", got.hostID)
	}
	if got.in.Virtualization != agenttypes.VirtVMware || got.in.CPUSockets != 4 || got.in.KernelVersion != "6.12.96+deb13-amd64" {
		t.Fatalf("scalars mangled: %+v", got.in)
	}
	const wantNICs = `[{"name":"ens33","mac":"00:0c:29:c4:67:a6","ipv4":["10.1.1.10/24"],"speedMbps":1000,"mtu":1500,"state":"up"}]`
	if string(got.in.NICs) != wantNICs {
		t.Fatalf("nics = %s\nwant %s", got.in.NICs, wantNICs)
	}
	// An empty list must reach jsonb as [], never as null: the column is
	// NOT NULL and "no block devices" is a real answer for a container.
	if string(got.in.BlockDevices) != "[]" {
		t.Fatalf("empty list = %s, want []", got.in.BlockDevices)
	}
}
