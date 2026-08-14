package agentgw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	cws "github.com/coder/websocket"

	ws "vraxel.io/vraxel/lib/websocket"
	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

// fakeAgentStore records the host_agents writes the registry performs so
// a test can assert the durable side of a session's lifecycle without a
// database.
type fakeAgentStore struct {
	mu      sync.Mutex
	online  []onlineCall
	offline []offlineCall
	touched []touchCall
	orphans []time.Duration
	stale   []time.Duration
	// touchLost makes Touch report that the row belongs to somebody else,
	// which is what drives the re-claim path.
	touchLost bool
	// markOnlineAt is what MarkOnline stamps the row with.
	markOnlineAt time.Time
}

type onlineCall struct {
	hostID      int64
	instanceID  string
	version     string
	clockSkewMs int64
}
type offlineCall struct {
	hostID     int64
	instanceID string
}
type touchCall struct {
	hostID      int64
	clockSkewMs int64
}

func (f *fakeAgentStore) Bind(context.Context, gwstore.BindInput) (*gwstore.AgentRow, error) {
	return nil, nil
}
func (f *fakeAgentStore) FindByProductUUID(context.Context, []string) ([]gwstore.AgentRow, error) {
	return nil, nil
}
func (f *fakeAgentStore) FindByMachineID(context.Context, string) ([]gwstore.AgentRow, error) {
	return nil, nil
}
func (f *fakeAgentStore) RefreshFingerprint(context.Context, int64, gwstore.FingerprintInput) error {
	return nil
}
func (f *fakeAgentStore) MoveBinding(context.Context, int64, int64) error { return nil }
func (f *fakeAgentStore) GetByAgentID(context.Context, string) (*gwstore.AgentRow, error) {
	return nil, nil
}
func (f *fakeAgentStore) GetByHostID(context.Context, int64) (*gwstore.AgentRow, error) {
	return nil, nil
}

func (f *fakeAgentStore) CheckIdentity(context.Context, int64, string, time.Duration) (bool, error) {
	return false, nil
}

func (f *fakeAgentStore) MarkOnline(_ context.Context, hostID int64, instanceID, version string, skew int64) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.online = append(f.online, onlineCall{hostID, instanceID, version, skew})
	return f.markOnlineAt, nil
}

func (f *fakeAgentStore) Touch(_ context.Context, hostID int64, _ string, skew int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touched = append(f.touched, touchCall{hostID, skew})
	return !f.touchLost, nil
}

func (f *fakeAgentStore) MarkOffline(_ context.Context, hostID int64, instanceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.offline = append(f.offline, offlineCall{hostID, instanceID})
	return nil
}

func (f *fakeAgentStore) MarkOrphansOffline(_ context.Context, after time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.orphans = append(f.orphans, after)
	return nil
}

func (f *fakeAgentStore) MarkStaleOffline(_ context.Context, after time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stale = append(f.stale, after)
	return nil
}

func (f *fakeAgentStore) snapshot() ([]onlineCall, []offlineCall, []touchCall) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]onlineCall(nil), f.online...),
		append([]offlineCall(nil), f.offline...),
		append([]touchCall(nil), f.touched...)
}

// newTestSession spins a throwaway WebSocket and returns the server-side
// wrapper. Sessions hold real connections (Add closes a superseded one),
// so a nil-conn stub would not exercise the path that matters.
func newTestSession(t *testing.T, agentID string, hostID int64) *Session {
	t.Helper()
	connCh := make(chan *ws.Conn, 1)
	// The handler parks until cleanup: an upgraded connection is
	// hijacked, so r.Context() does NOT cancel when the peer goes away,
	// and httptest.Server.Close waits for outstanding handlers.
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
	client, _, err := cws.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		cancel()
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { client.CloseNow() })
	cancel()

	select {
	case c := <-connCh:
		sessCtx, stop := context.WithCancel(context.Background())
		t.Cleanup(stop)
		return &Session{
			AgentID:     agentID,
			HostID:      hostID,
			Conn:        c,
			ConnectedAt: time.Now(),
			stop:        stop,
			ctx:         sessCtx,
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server never accepted the websocket")
		return nil
	}
}

func TestRegistryAddRemoveLifecycle(t *testing.T) {
	store := &fakeAgentStore{}
	r := NewRegistry(store, "inst-a")
	sess := newTestSession(t, "agent-1", 100)

	r.Add(context.Background(), sess, 250)
	if got := r.Count(); got != 1 {
		t.Fatalf("Count after Add = %d, want 1", got)
	}
	if r.Get("agent-1") != sess {
		t.Fatal("Get did not return the registered session")
	}
	if r.GetByHost(100) != sess {
		t.Fatal("GetByHost did not return the registered session")
	}

	online, _, _ := store.snapshot()
	if len(online) != 1 || online[0] != (onlineCall{100, "inst-a", "", 250}) {
		t.Fatalf("MarkOnline calls = %+v, want one {100 inst-a  250}", online)
	}

	if reclaimed := r.Touch(context.Background(), sess, -30); !reclaimed.IsZero() {
		t.Fatalf("Touch on a row we own reported a re-claim at %v, want none", reclaimed)
	}
	if _, _, touched := store.snapshot(); len(touched) != 1 || touched[0] != (touchCall{100, -30}) {
		t.Fatalf("Touch calls = %+v", touched)
	}

	r.Remove(context.Background(), sess)
	if got := r.Count(); got != 0 {
		t.Fatalf("Count after Remove = %d, want 0", got)
	}
	if r.Get("agent-1") != nil || r.GetByHost(100) != nil {
		t.Fatal("session still resolvable after Remove")
	}
	_, offline, _ := store.snapshot()
	if len(offline) != 1 || offline[0] != (offlineCall{100, "inst-a"}) {
		t.Fatalf("MarkOffline calls = %+v, want one {100 inst-a}", offline)
	}
}

// TestRegistryTouchReclaimsARowOwnedByAnotherInstance pins the heartbeat's
// self-healing half. Touch is guarded on instance_id while MarkOnline is
// not, so a MarkOnline that failed at accept time, or a sibling's delayed
// write during a reconnect, leaves the row owned by an instance that does
// not hold the socket. Every later beat then matches nothing -- silently,
// and for as long as the channel stays up, which on a healthy agent is
// days. The beat has to take the row back.
func TestRegistryTouchReclaimsARowOwnedByAnotherInstance(t *testing.T) {
	store := &fakeAgentStore{touchLost: true}
	store.markOnlineAt = time.Unix(1700000000, 0)
	r := NewRegistry(store, "inst-a")
	sess := newTestSession(t, "agent-1", 100)

	r.Add(context.Background(), sess, 0)
	before, _, _ := store.snapshot()

	reclaimed := r.Touch(context.Background(), sess, 0)
	if reclaimed.IsZero() {
		t.Fatal("Touch on a row owned elsewhere did not re-claim it")
	}
	after, _, _ := store.snapshot()
	if len(after) != len(before)+1 {
		t.Fatalf("MarkOnline calls = %d, want one more than the %d before the beat", len(after), len(before))
	}
}

func TestRegistryRemoveIsIdempotent(t *testing.T) {
	store := &fakeAgentStore{}
	r := NewRegistry(store, "inst-a")
	sess := newTestSession(t, "agent-1", 100)

	r.Add(context.Background(), sess, 0)
	r.Remove(context.Background(), sess)
	r.Remove(context.Background(), sess)

	if _, offline, _ := store.snapshot(); len(offline) != 1 {
		t.Fatalf("MarkOffline called %d times for one session, want 1", len(offline))
	}
}

func TestRegistryReconnectSupersedesWithoutEvictingTheLiveSession(t *testing.T) {
	// The regression this guards: a superseded session's read loop
	// unwinds AFTER the replacement has registered. An unconditional
	// delete in Remove would then evict the live channel and mark a
	// connected agent offline -- the host would go dark until its next
	// reconnect.
	store := &fakeAgentStore{}
	r := NewRegistry(store, "inst-a")

	old := newTestSession(t, "agent-1", 100)
	r.Add(context.Background(), old, 0)

	fresh := newTestSession(t, "agent-1", 100)
	r.Add(context.Background(), fresh, 0)

	if got := r.Count(); got != 1 {
		t.Fatalf("Count after reconnect = %d, want 1", got)
	}
	if r.Get("agent-1") != fresh {
		t.Fatal("registry still points at the superseded session")
	}

	// The old session's read loop now notices its closed socket.
	r.Remove(context.Background(), old)

	if r.Get("agent-1") != fresh {
		t.Fatal("late Remove of the superseded session evicted the live one")
	}
	if _, offline, _ := store.snapshot(); len(offline) != 0 {
		t.Fatalf("MarkOffline called %+v for a reconnect; the agent never went offline", offline)
	}
}

func TestRegistryEvictDropsTheIncumbentChannel(t *testing.T) {
	// The regression this guards: refusing the connection that exposed a
	// contended identity is not enough on its own. The duplicate already
	// holding the channel never reconnects, so it is never re-checked --
	// and because a clone boots after the machine it was copied from, the
	// incumbent is usually the copy. Without this the clone keeps the host
	// and the real machine stays locked out.
	store := &fakeAgentStore{}
	r := NewRegistry(store, "inst-a")

	sess := newTestSession(t, "agent-1", 100)
	r.Add(context.Background(), sess, 0)

	if !r.Evict(100, "contended") {
		t.Fatal("Evict reported no session for a host that has one")
	}
	select {
	case <-sess.ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Evict left the session context live; its read loop would never unwind")
	}

	// Idempotent: the read loop's Remove has not run yet, and a second
	// refused connection must not panic on the way through.
	r.Remove(context.Background(), sess)
	if r.Evict(100, "contended") {
		t.Fatal("Evict reported a session after the host's channel was already gone")
	}
}
