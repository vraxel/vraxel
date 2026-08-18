package agentdialer

import (
	"context"
	"errors"
	"net"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// fakeOpener records the one call the Dialer makes, so each shape can be
// asserted without a live data channel.
type fakeOpener struct {
	gotHost int64
	gotOpen agenttypes.StreamOpen
	conn    net.Conn
	err     error
}

func (f *fakeOpener) OpenStream(_ context.Context, hostID int64, open agenttypes.StreamOpen) (net.Conn, error) {
	f.gotHost = hostID
	f.gotOpen = open
	return f.conn, f.err
}

func TestDialForShapesTCPTarget(t *testing.T) {
	fo := &fakeOpener{}
	if _, _ = New(fo).DialFor(7)(context.Background(), "tcp", "127.0.0.1:9100"); fo.gotHost != 7 {
		t.Fatalf("host = %d, want 7", fo.gotHost)
	}
	if fo.gotOpen.Kind != agenttypes.StreamKindTCP || fo.gotOpen.Target != "127.0.0.1:9100" {
		t.Fatalf("open = %+v, want tcp -> 127.0.0.1:9100", fo.gotOpen)
	}
}

func TestStreamForPassesOpenVerbatim(t *testing.T) {
	fo := &fakeOpener{}
	open := agenttypes.StreamOpen{Kind: agenttypes.StreamKindPTY, Command: []string{"bash"}, Cols: 80, Rows: 24}
	if _, _ = New(fo).StreamFor(context.Background(), 9, open); fo.gotHost != 9 {
		t.Fatalf("host = %d, want 9", fo.gotHost)
	}
	if fo.gotOpen.Kind != agenttypes.StreamKindPTY || fo.gotOpen.Cols != 80 || fo.gotOpen.Command[0] != "bash" {
		t.Fatalf("open = %+v, not passed through", fo.gotOpen)
	}
}

func TestHTTPClientForDialsThroughOpener(t *testing.T) {
	fo := &fakeOpener{err: errors.New("dialed")}
	// The request fails at dial (no real conn), but the opener records the
	// host:port the transport asked for, proving the client tunnels.
	_, _ = New(fo).HTTPClientFor(3).Get("http://10.0.0.1:8080/health")
	if fo.gotHost != 3 {
		t.Fatalf("host = %d, want 3", fo.gotHost)
	}
	if fo.gotOpen.Kind != agenttypes.StreamKindTCP || fo.gotOpen.Target != "10.0.0.1:8080" {
		t.Fatalf("open = %+v, want tcp -> 10.0.0.1:8080", fo.gotOpen)
	}
}
