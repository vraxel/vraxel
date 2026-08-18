package agentgw

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	cws "github.com/coder/websocket"
	"github.com/hashicorp/yamux"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	"vraxel.io/vraxel/lib/logger"
	ws "vraxel.io/vraxel/lib/websocket"
)

// dataDialTimeout bounds waiting for an agent to bring its data channel
// up after channel.open. It matches the agent's own dial timeout
// (lib/agent/datachan, 30s): the agent must dial out, upgrade and
// register, so giving up sooner would fail a request it was about to
// serve.
const dataDialTimeout = 30 * time.Second

// dataYamuxKeepAlive / dataYamuxWriteTimeout mirror the agent side
// (lib/agent/datachan). The two ends must agree on keepalive, or one
// tears the session down as dead while the other believes it is healthy.
const (
	dataYamuxKeepAlive    = 30 * time.Second
	dataYamuxWriteTimeout = 10 * time.Second
)

// StreamRejected is the agent's refusal of a StreamOpen (target not on
// loopback, port not allowed, local dial failed). Distinct from a
// transport error so a caller can tell "the agent said no" from "the
// tunnel broke".
type StreamRejected struct {
	Code    string
	Message string
}

func (e *StreamRejected) Error() string {
	return fmt.Sprintf("agent rejected stream: %s (%s)", e.Message, e.Code)
}

// DataHub holds the data-channel yamux sessions this instance has
// accepted, keyed by host, and opens logical streams on them.
//
// One channel per host carries every synchronous session (terminal, log
// tail, file op) as a separate yamux stream, so a second terminal costs a
// stream rather than a TLS handshake -- and a burst of them cannot exhaust
// anything the agent has to accept from outside.
//
// The sessions map is process-local by nature: a data channel is a socket
// pinned to whichever instance the agent's dial landed on, and the agent
// cannot be told where to dial (it reaches one load-balanced address, and
// an instance's internal address is unroutable from a managed host). So a
// channel held by a sibling is not reachable from here; the router says so
// by name.
type DataHub struct {
	router *ChannelRouter

	mu       sync.Mutex
	sessions map[int64]*yamux.Session
	waiters  map[int64][]chan struct{}
}

func NewDataHub(router *ChannelRouter) *DataHub {
	return &DataHub{
		router:   router,
		sessions: make(map[int64]*yamux.Session),
		waiters:  make(map[int64][]chan struct{}),
	}
}

// accept wraps a freshly upgraded data-channel socket as the yamux CLIENT
// -- the gateway opens streams, so the agent is the yamux server -- and
// serves it until it dies. Blocks: the caller runs it on the request
// goroutine so the socket's lifetime is the handler's.
func (h *DataHub) accept(ctx context.Context, hostID int64, conn *ws.Conn) {
	// cws.NetConn sets the read limit to -1, so the 32 KiB message default
	// does not apply here: this channel carries bulk payloads and yamux's
	// window is the flow control. Do NOT add SetReadLimit -- it would sever
	// any session whose yamux frame exceeds the cap.
	netConn := cws.NetConn(ctx, conn.Inner(), cws.MessageBinary)

	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = dataYamuxKeepAlive
	cfg.ConnectionWriteTimeout = dataYamuxWriteTimeout
	cfg.LogOutput = io.Discard
	sess, err := yamux.Client(netConn, cfg)
	if err != nil {
		logger.Warnf("agentgw data-channel: yamux client for host %d: %v", hostID, err)
		return
	}
	defer sess.Close()

	h.register(hostID, sess)
	defer h.unregister(hostID, sess)

	logger.Infof("agentgw: data channel for host %d up", hostID)
	select {
	case <-ctx.Done():
	case <-sess.CloseChan():
	}
}

// register installs a session, superseding any previous one for the host
// (the agent redials its data channel on reconnect, leaving the old yamux
// session dead), and wakes every waiter parked in await.
func (h *DataHub) register(hostID int64, sess *yamux.Session) {
	h.mu.Lock()
	prev := h.sessions[hostID]
	h.sessions[hostID] = sess
	waiters := h.waiters[hostID]
	delete(h.waiters, hostID)
	h.mu.Unlock()

	if prev != nil && prev != sess {
		_ = prev.Close()
	}
	for _, ch := range waiters {
		close(ch)
	}
}

// unregister drops a session only if it is still the current one for its
// host, so a superseded session tearing down later cannot evict its
// replacement.
func (h *DataHub) unregister(hostID int64, sess *yamux.Session) {
	h.mu.Lock()
	if h.sessions[hostID] == sess {
		delete(h.sessions, hostID)
	}
	h.mu.Unlock()
}

// OpenStream opens one logical stream to a host's agent, bringing the
// data channel up first if it is not already. Implements the SessionOpener
// the cross-module dialer expects.
func (h *DataHub) OpenStream(ctx context.Context, hostID int64, open agenttypes.StreamOpen) (net.Conn, error) {
	sess, err := h.await(ctx, hostID)
	if err != nil {
		return nil, err
	}
	return h.openOn(ctx, sess, hostID, open)
}

// openOn performs the stream handshake on a session this instance holds.
func (h *DataHub) openOn(ctx context.Context, sess *yamux.Session, hostID int64, open agenttypes.StreamOpen) (net.Conn, error) {
	stream, err := sess.OpenStream()
	if err != nil {
		// The session was in the map but is not usable; drop it so the next
		// call re-triggers a fresh channel instead of reusing a dead one.
		h.unregister(hostID, sess)
		return nil, fmt.Errorf("open stream to host %d: %w", hostID, err)
	}

	// yamux stream I/O takes no context. A blocked Read is unblocked by a
	// deadline, NOT by Close (a local Close only half-closes the write
	// side), so bound the handshake by pushing the read deadline to now
	// when ctx ends -- covering both a deadline and an outright cancel.
	stop := context.AfterFunc(ctx, func() { _ = stream.SetDeadline(time.Now()) })

	werr := agenttypes.WriteStreamHeader(stream, open)
	var ans agenttypes.StreamAccept
	rerr := werr
	if werr == nil {
		rerr = agenttypes.ReadStreamHeader(stream, &ans)
	}

	// stop() reports whether it prevented the AfterFunc: false means ctx
	// ended (the deadline fired, unwinding any block above), so honor ctx
	// regardless of the I/O error it produced. After stop() returns, no
	// deadline can be set later, so a returned stream is deadline-free.
	if !stop() {
		_ = stream.Close()
		return nil, ctx.Err()
	}
	if werr != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("write stream header to host %d: %w", hostID, werr)
	}
	if rerr != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("read stream accept from host %d: %w", hostID, rerr)
	}
	if !ans.Ok {
		_ = stream.Close()
		return nil, &StreamRejected{Code: ans.Code, Message: ans.Error}
	}
	return stream, nil
}

// await returns the host's data session, asking for one if none is up.
//
// The waiter is registered BEFORE channel.open is sent, and under the same
// lock as the "is it up" check. An agent that dials in during that window
// would otherwise wake nobody, and this caller would sit out the full
// dial timeout against a session that had already arrived.
func (h *DataHub) await(ctx context.Context, hostID int64) (*yamux.Session, error) {
	h.mu.Lock()
	if sess := h.sessions[hostID]; sess != nil {
		h.mu.Unlock()
		return sess, nil
	}
	ready := make(chan struct{})
	h.waiters[hostID] = append(h.waiters[hostID], ready)
	h.mu.Unlock()
	defer h.cancelWaiter(hostID, ready)

	if err := h.trigger(ctx, hostID); err != nil {
		return nil, err
	}

	select {
	case <-ready:
		if sess := h.get(hostID); sess != nil {
			return sess, nil
		}
		return nil, fmt.Errorf("data channel for host %d closed before use", hostID)
	case <-time.After(dataDialTimeout):
		return nil, fmt.Errorf("host %d did not open a data channel within %s", hostID, dataDialTimeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// trigger asks the host to bring its data channel up, over its control
// channel.
func (h *DataHub) trigger(ctx context.Context, hostID int64) error {
	// channel.open carries no parameters -- it only asks the agent to
	// ensure the data channel is up. Idempotent on the agent side, so
	// concurrent openers each sending it is harmless.
	return h.router.Send(ctx, hostID, agenttypes.Frame{Type: agenttypes.FrameTypeChannelOpen})
}

func (h *DataHub) get(hostID int64) *yamux.Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions[hostID]
}

// cancelWaiter removes one parked waiter after a timeout or cancellation,
// so a host that never dials in does not leak channels into the map.
func (h *DataHub) cancelWaiter(hostID int64, ready chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.waiters[hostID]
	for i, ch := range list {
		if ch == ready {
			h.waiters[hostID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(h.waiters[hostID]) == 0 {
		delete(h.waiters, hostID)
	}
}

// handleDataChannel upgrades the persistent data channel. GET
// /api/agent/v1/data-channel. One per host; every synchronous session is
// a yamux stream on it. Authenticated by session token, like every other
// non-control endpoint -- the durable agent token stays confined to the
// control channel.
func (h *protocolHandler) handleDataChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hostID, ok := h.authSession(w, r)
	if !ok {
		return
	}

	// Origin verification is skipped for the same reason as the control
	// channel: the peer is a Go process with no Origin header, already
	// authenticated by the bearer session token above.
	conn, err := ws.Accept(w, r, &ws.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		logger.Warnf("agentgw data-channel: upgrade for host %d: %v", hostID, err)
		return
	}

	// Bound to the server context, not the request: accept blocks for the
	// channel's whole life, and a SIGTERM must break it so the agent
	// reconnects rather than hanging on a TCP timeout.
	sessCtx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	h.dataHub.accept(sessCtx, hostID, conn)
	_ = conn.Close(ws.StatusNormalClosure, "")
}
