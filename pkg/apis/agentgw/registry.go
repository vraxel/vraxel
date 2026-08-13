package agentgw

import (
	"context"
	"sync"
	"time"

	"vraxel.io/vraxel/lib/logger"
	ws "vraxel.io/vraxel/lib/websocket"
	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

// Session is one live control channel. It is the in-process half of the
// (agent -> instance) binding whose durable half is
// host_agents.instance_id.
type Session struct {
	AgentID     string
	HostID      int64
	Version     string
	Conn        *ws.Conn
	ConnectedAt time.Time

	// ctx is cancelled when this session ends, whether by peer
	// disconnect, supersession or server shutdown. Callers that write to
	// the channel (Step 5's job dispatch) scope their work to it so a
	// dead socket cannot leave goroutines blocked on a write.
	ctx context.Context
	// stop cancels ctx. Held so a reconnect that supersedes an older
	// socket can tear the old one down.
	stop context.CancelFunc
}

// Context returns the session's lifetime context.
func (s *Session) Context() context.Context { return s.ctx }

// Registry maps live control channels by agent and by host, and keeps
// host_agents in sync with them.
//
// It is deliberately process-local. A control channel is a TCP socket, so
// it is pinned to whichever instance accepted it; the shared view across
// instances is the host_agents.instance_id column, which Step 7 uses to
// forward data-channel requests to the owning instance. Nothing here
// assumes a single instance.
//
// There is no explicit shutdown teardown: every session's context
// derives from the server's signal-bound context (handler.ctx), so a
// SIGTERM cancels each session's read loop, which unblocks immediately,
// removes itself (flipping host_agents to offline) and closes the
// socket. Agents then reconnect to a surviving instance without waiting
// on a TCP timeout.
type Registry struct {
	mu      sync.RWMutex
	byAgent map[string]*Session
	byHost  map[int64]*Session

	agents     gwstore.AgentStore
	instanceID string

	// connMu serializes concurrent connects of the SAME agent; see Add.
	connMu   sync.Mutex
	connLock map[string]*agentConnLock
}

// agentConnLock is one agent's connect mutex plus the refcount that says
// when it can be dropped from the map (so the map does not grow with
// every agent that ever connected).
type agentConnLock struct {
	mu   sync.Mutex
	refs int
}

// NewRegistry builds an empty registry bound to this instance's id.
func NewRegistry(agents gwstore.AgentStore, instanceID string) *Registry {
	return &Registry{
		byAgent:    make(map[string]*Session),
		byHost:     make(map[int64]*Session),
		agents:     agents,
		instanceID: instanceID,
		connLock:   make(map[string]*agentConnLock),
	}
}

// lockAgent takes this agent's connect mutex and returns its release.
func (r *Registry) lockAgent(agentID string) func() {
	r.connMu.Lock()
	l := r.connLock[agentID]
	if l == nil {
		l = &agentConnLock{}
		r.connLock[agentID] = l
	}
	l.refs++
	r.connMu.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		r.connMu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(r.connLock, agentID)
		}
		r.connMu.Unlock()
	}
}

// InstanceID returns the id this registry records as channel owner.
func (r *Registry) InstanceID() string { return r.instanceID }

// Add installs a session, superseding any previous channel for the same
// agent, and flips host_agents to online under this instance. It returns
// the connected_at the row was stamped with, which the caller signs into
// the session token as connEpoch (design §4.1); a zero time means the
// MarkOnline write failed and no token should be issued.
//
// Supersede rather than reject: an agent whose network dropped
// re-dials while the server may still hold a half-open socket it has not
// noticed yet. Rejecting the new connection would leave the agent
// unreachable until the dead socket's TCP timeout, which can be minutes.
func (r *Registry) Add(ctx context.Context, sess *Session, clockSkewMs int64) time.Time {
	// Serialize per agent across BOTH the map swap and the MarkOnline
	// write. They must stay in the same order: connected_at is the session
	// token's epoch, so if two connects for one agent interleave and the
	// loser's UPDATE lands last, the row ends up stamped with the dead
	// connection's timestamp and the live agent's token never validates.
	// Per-agent, so one slow DB write cannot block other agents.
	unlock := r.lockAgent(sess.AgentID)
	defer unlock()

	r.mu.Lock()
	prev, superseded := r.byAgent[sess.AgentID]
	r.byAgent[sess.AgentID] = sess
	r.byHost[sess.HostID] = sess
	r.mu.Unlock()

	if superseded {
		logger.Infof("agentgw: agent %s reconnected, superseding previous channel", sess.AgentID)
		// Outside the lock, and asynchronously: a WebSocket close is a
		// handshake, so on a half-dead socket it blocks until the
		// library's internal timeout. Doing that under r.mu would stall
		// every other agent's connect and disconnect behind one dead
		// peer. Cancelling the context is the part that must be
		// immediate; the close frame is courtesy.
		prev.stop()
		go closeSession(prev, "superseded by new channel")
	}

	connectedAt, err := r.agents.MarkOnline(ctx, sess.HostID, r.instanceID, sess.Version, clockSkewMs)
	if err != nil {
		logger.Warnf("agentgw: mark agent %s online: %v", sess.AgentID, err)
		return time.Time{}
	}
	return connectedAt
}

// Remove drops a session if it is still the current one for its agent,
// and flips host_agents to offline.
//
// The identity check matters: a superseded session's read loop unwinds
// AFTER the replacement has already registered, and an unconditional
// delete there would evict the live channel and mark a connected agent
// offline.
func (r *Registry) Remove(ctx context.Context, sess *Session) {
	// Same per-agent lock as Add, for the same reason: without it a slow
	// Remove can pass its identity check, be overtaken by the reconnect's
	// Add + MarkOnline, and only then write MarkOffline -- flipping a row
	// whose channel is live. Every session-token check for that host then
	// fails until the next heartbeat repairs it.
	unlock := r.lockAgent(sess.AgentID)
	defer unlock()

	r.mu.Lock()
	current, ok := r.byAgent[sess.AgentID]
	if !ok || current != sess {
		r.mu.Unlock()
		return
	}
	delete(r.byAgent, sess.AgentID)
	delete(r.byHost, sess.HostID)
	r.mu.Unlock()

	// MarkOffline is guarded by instance_id in SQL, so even a delayed
	// call cannot clobber a row another instance has since claimed.
	if err := r.agents.MarkOffline(ctx, sess.HostID, r.instanceID); err != nil {
		logger.Warnf("agentgw: mark agent %s offline: %v", sess.AgentID, err)
	}
	logger.Infof("agentgw: agent %s (host %d) channel closed", sess.AgentID, sess.HostID)
}

// Touch records a heartbeat, which also restores status='online' for the
// row this instance owns (see the SQL): the stale sweep can misfire while
// DB writes are unavailable, and the next beat must heal that rather than
// leave a connected agent marked offline forever.
func (r *Registry) Touch(ctx context.Context, hostID int64, clockSkewMs int64) {
	if err := r.agents.Touch(ctx, hostID, r.instanceID, clockSkewMs); err != nil {
		logger.Warnf("agentgw: touch host %d: %v", hostID, err)
	}
}

// Get returns the live session for an agent, or nil.
func (r *Registry) Get(agentID string) *Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byAgent[agentID]
}

// GetByHost returns the live session for a host, or nil. Step 7's data
// channel dispatch resolves hosts this way.
func (r *Registry) GetByHost(hostID int64) *Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byHost[hostID]
}

// Count reports how many channels this instance currently holds.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byAgent)
}

// closeSession sends the close frame. Always called off the hot path:
// the handshake waits for the peer's reply, which a dead peer never
// sends.
func closeSession(s *Session, reason string) {
	_ = s.Conn.Close(ws.StatusGoingAway, reason)
}
