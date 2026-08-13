package agentgw

import (
	"context"
	"fmt"
	"net/http"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/logger"
	ws "vraxel.io/vraxel/lib/websocket"
	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

// The heartbeat cadence is client.HeartbeatInterval (15s), a protocol
// constant compiled into the agent; agentStaleAfter (install.go, 60s) is
// four missed beats. There is no server ping loop: the WS layer already
// keeps the transport alive, and the heartbeat frame already proves the
// agent's frame loop, so an app-level ping had no distinct failure mode
// to detect.

// helloTimeout bounds how long a freshly upgraded socket may stay silent
// before sending its hello frame.
const helloTimeout = 10 * time.Second

// clockSkewWarnMs is the drift at which a host's clock is worth a log
// line: past this, metric timestamps are visibly wrong (design §4.2).
const clockSkewWarnMs = 30_000

// handleChannel upgrades the persistent control channel. GET /api/agent/v1/channel.
func (h *protocolHandler) handleChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, row, ok := h.authAgent(w, r)
	if !ok {
		return
	}

	// Origin verification is skipped deliberately: the peer is a Go
	// process, not a browser, and sends no Origin header. Authentication
	// already happened above via the bearer token.
	conn, err := ws.Accept(w, r, &ws.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		logger.Warnf("agentgw channel: upgrade for agent %s: %v", claims.AgentID, err)
		return
	}
	conn.SetReadLimit(agenttypes.MaxFrameBytes)

	sessCtx, cancel := context.WithCancel(h.ctx)
	defer cancel()

	hello, err := readHello(sessCtx, conn)
	if err != nil {
		logger.Warnf("agentgw channel: agent %s hello: %v", claims.AgentID, err)
		_ = conn.Close(ws.StatusInternalError, "hello required")
		return
	}

	sess := &Session{
		AgentID:     claims.AgentID,
		HostID:      claims.HostID,
		Version:     hello.AgentVersion,
		Conn:        conn,
		ConnectedAt: time.Now(),
		ctx:         sessCtx,
		stop:        cancel,
	}

	// Reconcile BEFORE registering the session. Registration is what makes
	// this host dispatchable, and reconciliation fails every in-flight job
	// the hello did not list -- so doing it the other way round leaves a
	// window in which a driver dispatches onto the fresh channel and the
	// reconcile, working from the older hello snapshot, immediately kills
	// that brand-new job while the agent is already running it.
	h.runManager.OnAgentReconnect(sessCtx, claims.HostID, hello.RunningJobs, sess)

	connectedAt := h.registry.Add(sessCtx, sess, clockSkew(hello.ClockUnixMs))
	logger.Infof("agentgw: agent %s (host %d) channel open, version=%q token_version=%d",
		claims.AgentID, claims.HostID, hello.AgentVersion, row.TokenVersion)
	defer h.registry.Remove(context.WithoutCancel(h.ctx), sess)

	// Hand the agent its session token for the REST surface, then keep
	// handing it fresh ones. connectedAt is this connection's epoch:
	// authSession rejects a token whose epoch no longer matches
	// host_agents.connected_at, so it dies on reconnect. A zero time means
	// MarkOnline failed; skip issuing rather than mint a token that can
	// never validate, and let the first heartbeat re-claim the row and
	// issue one then.
	if !connectedAt.IsZero() {
		sess.SetEpoch(connectedAt)
		h.issueSessionToken(sessCtx, sess)
	}
	go h.refreshSessionToken(sessCtx, sess)

	h.readLoop(sessCtx, sess)
	_ = conn.Close(ws.StatusNormalClosure, "")
}

// issueSessionToken mints and delivers one session token for the epoch
// the session currently holds. A delivery failure is recorded so the next
// heartbeat retries it.
func (h *protocolHandler) issueSessionToken(ctx context.Context, sess *Session) {
	epoch := sess.Epoch()
	if epoch.IsZero() {
		return
	}
	gen := sess.tokenGen.Add(1)
	token, err := h.sessionSigner.Issue(sess.HostID, h.registry.InstanceID(), epoch.UnixMicro())
	if err != nil {
		logger.Warnf("agentgw: issue session token for agent %s: %v", sess.AgentID, err)
		return
	}
	err = WriteFrame(ctx, sess.Conn, agenttypes.Frame{
		Type: agenttypes.FrameTypeSessionToken, ID: fmt.Sprintf("st-%d", gen), Token: token,
	})
	sess.tokenSent.Store(err == nil)
	if err != nil {
		logger.Warnf("agentgw: send session token to agent %s: %v", sess.AgentID, err)
	}
}

// refreshSessionToken re-issues the session token for the life of the
// channel.
//
// A session token expires (sessionTokenTTL) but a control channel does
// not: it routinely stays up for days. Issuing only at connect time meant
// that after one TTL every REST call the agent made -- bundle, vars,
// events, result -- 401'd, so every job dispatched to a long-lived
// channel failed, and the agent could not even report the failure. The
// TTL is still worth having (it bounds how long a leaked token is usable
// even while its channel lives), so the answer is to keep renewing it
// rather than to widen it.
func (h *protocolHandler) refreshSessionToken(ctx context.Context, sess *Session) {
	t := time.NewTicker(sessionTokenRefresh)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.issueSessionToken(ctx, sess)
		}
	}
}

// authAgent validates the bearer agent token against host_agents.
//
// This is the ONLY place an agent token is accepted. REST endpoints added
// in later steps take the short-lived session token handed down this
// channel instead, so a leaked 90-day token cannot be replayed against
// e.g. the job-vars endpoint (design §4.1).
func (h *protocolHandler) authAgent(w http.ResponseWriter, r *http.Request) (*AgentClaims, *gwstore.AgentRow, bool) {
	token, ok := bearerToken(r)
	if !ok {
		http.Error(w, "missing bearer agent-token", http.StatusUnauthorized)
		return nil, nil, false
	}
	claims, err := h.signer.Parse(token)
	if err != nil {
		http.Error(w, "invalid agent-token", http.StatusUnauthorized)
		return nil, nil, false
	}
	row, err := h.agents.GetByAgentID(r.Context(), claims.AgentID)
	if err != nil {
		// Only a genuinely unknown agent id is a credential problem (the
		// host row, and its cascading host_agents row, was deleted): 401
		// tells the agent to stop hammering and wait for an operator to
		// re-onboard it. A DB failure must NOT take that path -- it would
		// tell every agent at once that its token was rejected, and each
		// would log "re-onboard this host" for what is a server outage.
		if se := apierrors.FromDomain(err, "agent"); se != nil && apierrors.IsNotFound(se) {
			http.Error(w, "unknown agent", http.StatusUnauthorized)
		} else {
			logger.Warnf("agentgw channel: look up agent %s: %v", claims.AgentID, err)
			http.Error(w, "agent lookup failed", http.StatusServiceUnavailable)
		}
		return nil, nil, false
	}
	if row.HostID != claims.HostID {
		http.Error(w, "agent-token host mismatch", http.StatusUnauthorized)
		return nil, nil, false
	}
	// token_version is the revocation lever: bumping the column (a
	// re-registration, or an explicit revoke later) invalidates every
	// token minted before it without needing a token blacklist.
	if row.TokenVersion != claims.TokenVersion {
		http.Error(w, "agent-token revoked", http.StatusUnauthorized)
		return nil, nil, false
	}
	return claims, row, true
}

// readHello consumes the mandatory first frame. Taking version + clock
// before registering the session means host_agents is never briefly
// online-with-unknown-version.
func readHello(ctx context.Context, conn *ws.Conn) (*agenttypes.Frame, error) {
	cctx, cancel := context.WithTimeout(ctx, helloTimeout)
	defer cancel()
	_, data, err := conn.ReadMessage(cctx)
	if err != nil {
		return nil, err
	}
	f, err := agenttypes.DecodeFrame(data)
	if err != nil {
		return nil, err
	}
	if f.Type != agenttypes.FrameTypeHello {
		return nil, fmt.Errorf("first frame is %q, want %q", f.Type, agenttypes.FrameTypeHello)
	}
	return &f, nil
}

// readLoop consumes frames until the peer goes away.
//
// Each read carries an idle deadline of agentStaleAfter. A silently
// severed connection (NAT drop, host powered off) produces no FIN, so a
// deadline-free read blocks until the kernel gives up on TCP, which can
// be many minutes. During that window the registry still hands this dead
// socket out for dispatch, and the writes appear to succeed. The agent
// beats every 15s, so 60s of silence is unambiguous.
func (h *protocolHandler) readLoop(ctx context.Context, sess *Session) {
	for {
		rctx, cancel := context.WithTimeout(ctx, agentStaleAfter)
		_, data, err := sess.Conn.ReadMessage(rctx)
		// Read the deadline's own state BEFORE cancelling it: after
		// cancel() rctx.Err() is always non-nil, so testing it afterwards
		// reported every ordinary disconnect as an idle timeout and sent
		// operators looking for a network fault that never happened.
		idle := rctx.Err() != nil
		cancel()
		if err != nil {
			if idle && ctx.Err() == nil {
				logger.Warnf("agentgw: agent %s (host %d) sent nothing for %s; closing the channel",
					sess.AgentID, sess.HostID, agentStaleAfter)
			}
			return
		}
		f, err := agenttypes.DecodeFrame(data)
		if err != nil {
			logger.Warnf("agentgw: agent %s sent undecodable frame: %v", sess.AgentID, err)
			continue
		}
		h.handleFrame(ctx, sess, &f)
	}
}

// touch records a heartbeat and keeps the agent's session token in step
// with it: the beat is the only regular event on an idle channel, so it
// is where both the re-claim and the delivery retry belong.
func (h *protocolHandler) touch(ctx context.Context, sess *Session, skew int64) {
	if reclaimedAt := h.registry.Touch(ctx, sess, skew); !reclaimedAt.IsZero() {
		// The epoch moved, so every token minted for the old one is dead.
		sess.SetEpoch(reclaimedAt)
		h.issueSessionToken(ctx, sess)
		return
	}
	if !sess.tokenSent.Load() {
		h.issueSessionToken(ctx, sess)
	}
}

func (h *protocolHandler) handleFrame(ctx context.Context, sess *Session, f *agenttypes.Frame) {
	switch f.Type {
	case agenttypes.FrameTypeHeartbeat:
		skew := clockSkew(f.ClockUnixMs)
		if skew > clockSkewWarnMs || skew < -clockSkewWarnMs {
			logger.Warnf("agentgw: agent %s clock skew %dms exceeds %dms; metric timestamps from this host will be wrong until NTP is fixed",
				sess.AgentID, skew, clockSkewWarnMs)
		}
		h.touch(ctx, sess, skew)
	case agenttypes.FrameTypeHello:
		// A second hello on an established channel is harmless; treat it
		// as a heartbeat so a reconnect-confused agent still stays fresh.
		h.touch(ctx, sess, clockSkew(f.ClockUnixMs))
	case agenttypes.FrameTypeJobAck:
		// The agent accepted a dispatched job: flip it to running so the
		// driver stops re-dispatching and starts its timeout clock.
		h.runManager.OnJobAck(ctx, sess.HostID, f.JobID)
	case agenttypes.FrameTypeError:
		logger.Warnf("agentgw: agent %s reported error ref=%s code=%s: %s", sess.AgentID, f.Ref, f.Code, f.Message)
	default:
		// Unknown frame types are ignored, keeping forward compatibility
		// with agents newer than the server.
	}
}

// WriteFrame encodes and sends one control frame over the lib/websocket
// connection. The encode + 64 KiB check is shared with the agent client
// via agenttypes.EncodeFrame; only the transport write is server-specific.
//
// Every write gets its own deadline. An agent that is alive but not
// reading (a stalled TCP window) would otherwise block the write
// indefinitely, and because the WebSocket library serialises writes, one
// such peer would wedge the run driver's goroutine behind it -- the
// driver stops heartbeating, and the sweep starts taking its runs away.
func WriteFrame(ctx context.Context, conn *ws.Conn, f agenttypes.Frame) error {
	data, err := agenttypes.EncodeFrame(f)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, frameWriteTimeout)
	defer cancel()
	return conn.WriteText(wctx, data)
}

// frameWriteTimeout bounds one control-frame write.
const frameWriteTimeout = 10 * time.Second

// clockSkew returns agentClock - serverClock in milliseconds at the moment
// the frame is processed. It therefore INCLUDES one-way network latency
// plus server scheduling delay, so it is a coarse NTP-drift indicator, not
// a precise offset -- a few tens of ms of the value is transport jitter,
// not clock error. That is fine for its only use: clockSkewWarnMs (30s) is
// orders of magnitude larger than any latency, so the check still reliably
// flags a host whose clock is wrong enough to corrupt metric timestamps.
func clockSkew(agentClockUnixMs int64) int64 {
	if agentClockUnixMs == 0 {
		return 0
	}
	return agentClockUnixMs - time.Now().UnixMilli()
}
