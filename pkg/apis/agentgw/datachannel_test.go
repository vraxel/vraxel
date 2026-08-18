package agentgw

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cws "github.com/coder/websocket"
	"github.com/hashicorp/yamux"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	ws "vraxel.io/vraxel/lib/websocket"
	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

func testYamuxCfg() *yamux.Config {
	c := yamux.DefaultConfig()
	c.EnableKeepAlive = false // determinism over net.Pipe; keepalive is not under test
	c.LogOutput = io.Discard
	return c
}

// testHub builds a DataHub wired the way install.go wires it, so trigger
// routes through a ChannelRouter instead of dereferencing nil.
func testHub(reg *Registry, store channelRoutingStore) *DataHub {
	return NewDataHub(NewChannelRouter("inst", reg, store))
}

// fakeAgent plays the agent end of a data channel: yamux server, one
// stream, StreamOpen -> the given answer, then echo (when accepted). It
// reports the open it saw on gotOpen.
func fakeAgent(t *testing.T, conn net.Conn, answer agenttypes.StreamAccept, gotOpen chan<- agenttypes.StreamOpen) {
	t.Helper()
	sess, err := yamux.Server(conn, testYamuxCfg())
	if err != nil {
		t.Errorf("fakeAgent yamux server: %v", err)
		return
	}
	defer sess.Close()
	stream, err := sess.AcceptStream()
	if err != nil {
		return
	}
	var open agenttypes.StreamOpen
	if err := agenttypes.ReadStreamHeader(stream, &open); err != nil {
		t.Errorf("fakeAgent read header: %v", err)
		return
	}
	gotOpen <- open
	if err := agenttypes.WriteStreamHeader(stream, answer); err != nil {
		return
	}
	if answer.Ok {
		_, _ = io.Copy(stream, stream)
	} else {
		_ = stream.Close()
	}
}

// pipedHub wires a gateway-side yamux session (already registered for
// hostID) to a fakeAgent, returning the hub and the open the agent saw.
func pipedHub(t *testing.T, hostID int64, answer agenttypes.StreamAccept) (*DataHub, chan agenttypes.StreamOpen) {
	t.Helper()
	gwConn, agentConn := net.Pipe()
	gwSess, err := yamux.Client(gwConn, testYamuxCfg())
	if err != nil {
		t.Fatalf("gateway yamux client: %v", err)
	}
	t.Cleanup(func() { gwSess.Close(); gwConn.Close(); agentConn.Close() })

	gotOpen := make(chan agenttypes.StreamOpen, 1)
	go fakeAgent(t, agentConn, answer, gotOpen)

	hub := testHub(NewRegistry(&fakeAgentStore{}, "inst"), &fakeAgentStore{})
	hub.register(hostID, gwSess)
	return hub, gotOpen
}

func TestDataHubOpenStreamRoundTrip(t *testing.T) {
	hub, gotOpen := pipedHub(t, 7, agenttypes.StreamAccept{Ok: true})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := hub.OpenStream(ctx, 7, agenttypes.StreamOpen{Kind: agenttypes.StreamKindTCP, Target: "127.0.0.1:9"})
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	defer conn.Close()

	if open := <-gotOpen; open.Kind != agenttypes.StreamKindTCP || open.Target != "127.0.0.1:9" {
		t.Fatalf("agent saw open %+v, want tcp -> 127.0.0.1:9", open)
	}
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("echo = %q, want ping", buf)
	}
}

func TestDataHubOpenStreamSurfacesRejection(t *testing.T) {
	hub, _ := pipedHub(t, 7, agenttypes.StreamAccept{
		Ok: false, Code: agenttypes.StreamErrTargetNotAllowed, Error: "not loopback",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := hub.OpenStream(ctx, 7, agenttypes.StreamOpen{Kind: agenttypes.StreamKindTCP, Target: "8.8.8.8:53"})

	var rej *StreamRejected
	if !errors.As(err, &rej) {
		t.Fatalf("err = %v, want *StreamRejected", err)
	}
	if rej.Code != agenttypes.StreamErrTargetNotAllowed {
		t.Fatalf("reject code = %q, want %q", rej.Code, agenttypes.StreamErrTargetNotAllowed)
	}
}

func TestDataHubOpenStreamHostOffline(t *testing.T) {
	// No control channel anywhere and no online row: the agent cannot be
	// asked to dial, so this must fail immediately rather than park for the
	// full dial timeout.
	hub := testHub(NewRegistry(&fakeAgentStore{}, "inst"), &fakeAgentStore{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := hub.OpenStream(ctx, 999, agenttypes.StreamOpen{Kind: agenttypes.StreamKindTCP, Target: "127.0.0.1:9"})
	if !errors.Is(err, ErrHostUnreachable) {
		t.Fatalf("err = %v, want ErrHostUnreachable", err)
	}
}

// rowStore answers GetByHostID with one fixed row; every other method is
// the fake's no-op.
type rowStore struct {
	*fakeAgentStore
	row *gwstore.AgentRow
}

func (o *rowStore) GetByHostID(context.Context, int64) (*gwstore.AgentRow, error) {
	return o.row, nil
}

func TestDataHubOpenStreamHostOnAnotherInstance(t *testing.T) {
	// The agent is online, just connected to a sibling. Forwarding is not
	// built, so this fails -- but it must NOT read as "the host is
	// unreachable", and the message must name the instance holding it.
	// Getting this wrong sends an operator hunting a live agent.
	store := &rowStore{
		fakeAgentStore: &fakeAgentStore{},
		row:            &gwstore.AgentRow{HostID: 5, Status: agentStatusOnline, InstanceID: "other-instance"},
	}
	hub := testHub(NewRegistry(&fakeAgentStore{}, "inst"), store)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := hub.OpenStream(ctx, 5, agenttypes.StreamOpen{Kind: agenttypes.StreamKindPTY})

	if !errors.Is(err, ErrHostOnAnotherInstance) {
		t.Fatalf("err = %v, want ErrHostOnAnotherInstance", err)
	}
	if errors.Is(err, ErrHostUnreachable) {
		t.Fatal("a host on a sibling instance must not report as unreachable")
	}
	if !strings.Contains(err.Error(), "other-instance") {
		t.Fatalf("err %q does not name the holding instance", err)
	}
}

func TestDataHubOpenStreamStaleSelfClaim(t *testing.T) {
	// The row names THIS instance but the registry has no session: our own
	// socket died and the row has not caught up. Forwarding to ourselves
	// would loop, so the honest answer is unreachable.
	store := &rowStore{
		fakeAgentStore: &fakeAgentStore{},
		row:            &gwstore.AgentRow{HostID: 5, Status: agentStatusOnline, InstanceID: "inst"},
	}
	hub := testHub(NewRegistry(&fakeAgentStore{}, "inst"), store)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := hub.OpenStream(ctx, 5, agenttypes.StreamOpen{Kind: agenttypes.StreamKindPTY})
	if !errors.Is(err, ErrHostUnreachable) {
		t.Fatalf("err = %v, want ErrHostUnreachable", err)
	}
}

func TestDataHubDemandTriggerThenWake(t *testing.T) {
	// The full demand-driven path: no data channel yet, OpenStream sends
	// channel.open over the control channel, parks, and completes once the
	// agent dials its data channel in.
	reg := NewRegistry(&fakeAgentStore{}, "inst")
	hub := testHub(reg, &fakeAgentStore{})
	ctrlSess, ctrlClient := newReadableControlSession(t, "agent-7", 7)
	reg.Add(context.Background(), ctrlSess, 0)

	type result struct {
		conn net.Conn
		err  error
	}
	done := make(chan result, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, err := hub.OpenStream(ctx, 7, agenttypes.StreamOpen{Kind: agenttypes.StreamKindTCP, Target: "127.0.0.1:9"})
		done <- result{conn, err}
	}()

	// The demand signal must actually reach the control channel.
	if f := readControlFrame(t, ctrlClient); f.Type != agenttypes.FrameTypeChannelOpen {
		t.Fatalf("control channel got %q, want %q", f.Type, agenttypes.FrameTypeChannelOpen)
	}

	// The agent now dials its data channel in.
	gwConn, agentConn := net.Pipe()
	gwSess, err := yamux.Client(gwConn, testYamuxCfg())
	if err != nil {
		t.Fatalf("gateway yamux client: %v", err)
	}
	t.Cleanup(func() { gwSess.Close(); gwConn.Close(); agentConn.Close() })
	gotOpen := make(chan agenttypes.StreamOpen, 1)
	go fakeAgent(t, agentConn, agenttypes.StreamAccept{Ok: true}, gotOpen)
	hub.register(7, gwSess)

	res := <-done
	if res.err != nil {
		t.Fatalf("OpenStream after trigger: %v", res.err)
	}
	defer res.conn.Close()
	if _, err := res.conn.Write([]byte("go")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 2)
	if _, err := io.ReadFull(res.conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != "go" {
		t.Fatalf("echo = %q, want go", buf)
	}
}

func TestDataChannelEndpointEndToEnd(t *testing.T) {
	const hostID = int64(42)
	signer := NewSessionTokenSigner([]byte("master"))
	epoch := time.Unix(1_700_000_000, 0)
	store := &rowStore{fakeAgentStore: &fakeAgentStore{}, row: &gwstore.AgentRow{Status: agentStatusOnline, ConnectedAt: &epoch}}
	reg := NewRegistry(store, "inst")
	hub := testHub(reg, &fakeAgentStore{})

	hctx, hcancel := context.WithCancel(context.Background())
	h := &protocolHandler{agents: store, sessionSigner: signer, registry: reg, dataHub: hub, ctx: hctx}
	srv := httptest.NewServer(http.HandlerFunc(h.serve))
	t.Cleanup(srv.Close)
	t.Cleanup(hcancel) // runs before srv.Close: unblocks accept so the handler returns

	tok, err := signer.Issue(hostID, "inst", epoch.UnixMicro())
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	agentConn, _, err := cws.Dial(ctx, "ws"+srv.URL[4:]+agenttypes.DataChannelPath, &cws.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + tok}},
	})
	if err != nil {
		t.Fatalf("agent dial data channel: %v", err)
	}
	t.Cleanup(func() { agentConn.CloseNow() })

	gotOpen := make(chan agenttypes.StreamOpen, 1)
	go fakeAgent(t, cws.NetConn(context.Background(), agentConn, cws.MessageBinary), agenttypes.StreamAccept{Ok: true}, gotOpen)

	// accept registers asynchronously as the server handler runs.
	waitFor(t, func() bool { return hub.get(hostID) != nil })

	conn, err := hub.OpenStream(ctx, hostID, agenttypes.StreamOpen{Kind: agenttypes.StreamKindTCP, Target: "127.0.0.1:80"})
	if err != nil {
		t.Fatalf("OpenStream over real endpoint: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("xy")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != "xy" {
		t.Fatalf("echo = %q, want xy", buf)
	}
}

func TestDataChannelEndpointRejectsMissingToken(t *testing.T) {
	// Guards that authSession runs BEFORE the upgrade: a tokenless dial must
	// 401, not get a live data channel.
	signer := NewSessionTokenSigner([]byte("master"))
	store := &rowStore{fakeAgentStore: &fakeAgentStore{}, row: &gwstore.AgentRow{Status: agentStatusOnline}}
	reg := NewRegistry(store, "inst")
	h := &protocolHandler{agents: store, sessionSigner: signer, registry: reg, dataHub: testHub(reg, &fakeAgentStore{}), ctx: context.Background()}
	srv := httptest.NewServer(http.HandlerFunc(h.serve))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, resp, err := cws.Dial(ctx, "ws"+srv.URL[4:]+agenttypes.DataChannelPath, nil)
	if err == nil {
		t.Fatal("tokenless dial upgraded; auth not enforced before upgrade")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", resp)
	}
}

// liveYamuxClient returns a gateway-side yamux session whose agent end is
// drained, so Close() frames do not block on the synchronous pipe.
func liveYamuxClient(t *testing.T) *yamux.Session {
	t.Helper()
	a, b := net.Pipe()
	cli, err := yamux.Client(a, testYamuxCfg())
	if err != nil {
		t.Fatalf("yamux client: %v", err)
	}
	go func() {
		if srv, err := yamux.Server(b, testYamuxCfg()); err == nil {
			_, _ = srv.AcceptStream()
		}
	}()
	t.Cleanup(func() { cli.Close(); a.Close(); b.Close() })
	return cli
}

func TestDataHubRegisterSupersedesAndClosesPrev(t *testing.T) {
	// The agent redials its data channel on reconnect; the stale yamux
	// session must be dropped and closed, not left addressable.
	hub := testHub(NewRegistry(&fakeAgentStore{}, "inst"), &fakeAgentStore{})
	s1 := liveYamuxClient(t)
	hub.register(7, s1)
	s2 := liveYamuxClient(t)
	hub.register(7, s2)

	if hub.get(7) != s2 {
		t.Fatal("register did not supersede with the fresh session")
	}
	select {
	case <-s1.CloseChan():
	case <-time.After(2 * time.Second):
		t.Fatal("superseded session was not closed")
	}
}

func TestDataHubOpenStreamHonorsContextDuringHandshake(t *testing.T) {
	// The agent accepts the stream and reads the open header but never
	// answers. Without ctx enforcement the handshake would block until yamux
	// keepalive (30s); with it, OpenStream must return when ctx expires.
	gwConn, agentConn := net.Pipe()
	gwSess, err := yamux.Client(gwConn, testYamuxCfg())
	if err != nil {
		t.Fatalf("gateway yamux client: %v", err)
	}
	t.Cleanup(func() { gwSess.Close(); gwConn.Close(); agentConn.Close() })
	go func() {
		srv, err := yamux.Server(agentConn, testYamuxCfg())
		if err != nil {
			return
		}
		stream, err := srv.AcceptStream()
		if err != nil {
			return
		}
		var open agenttypes.StreamOpen
		_ = agenttypes.ReadStreamHeader(stream, &open)
		_, _ = io.Copy(io.Discard, stream) // hang on the answer, unwind on close
	}()

	hub := testHub(NewRegistry(&fakeAgentStore{}, "inst"), &fakeAgentStore{})
	hub.register(9, gwSess)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = hub.OpenStream(ctx, 9, agenttypes.StreamOpen{Kind: agenttypes.StreamKindTCP, Target: "127.0.0.1:9"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("OpenStream blocked %s; ctx not honored during handshake", elapsed)
	}
}

// newReadableControlSession stands up a control channel whose server side
// goes into a Session and whose client side is returned so the test can
// read the frames the gateway writes.
func newReadableControlSession(t *testing.T, agentID string, hostID int64) (*Session, *cws.Conn) {
	t.Helper()
	connCh := make(chan *ws.Conn, 1)
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := ws.Accept(w, r, &ws.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		connCh <- c
		<-done
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(done) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, _, err := cws.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial control: %v", err)
	}
	t.Cleanup(func() { client.CloseNow() })

	c := <-connCh
	sctx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	return &Session{AgentID: agentID, HostID: hostID, Conn: c, ConnectedAt: time.Now(), ctx: sctx, stop: stop}, client
}

func readControlFrame(t *testing.T, c *cws.Conn) agenttypes.Frame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read control frame: %v", err)
	}
	f, err := agenttypes.DecodeFrame(data)
	if err != nil {
		t.Fatalf("decode control frame: %v", err)
	}
	return f
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}
