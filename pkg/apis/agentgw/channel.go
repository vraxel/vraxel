package agentgw

import (
	"context"
	"encoding/json"
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

// identityConflictCooldown is how long an agent id stays refused after two
// live processes were caught claiming it (a cloned disk, almost always).
//
// It is a sliding window rather than a latch, so the host heals itself
// once the duplicate is shut down: the survivor keeps reconnecting, stops
// producing conflicts, and is admitted when the window lapses. A latch
// would need an operator and an admin UI to clear, and would leave the
// host unmanageable in the meantime.
//
// Long enough that both clones back off to their 60s ceiling and the
// operator sees a steady stream of log lines rather than a blip.
const identityConflictCooldown = 15 * time.Minute

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

	// Is the machine holding this credential the one it was issued to?
	// Asked before anything else observable happens, because admitting a
	// copy marks the host online under a channel belonging to a different
	// machine -- and then dispatches that host's jobs, carrying that
	// host's secrets, onto it.
	if !h.verifyMachine(sessCtx, row, hello) {
		_ = conn.Close(ws.StatusPolicyViolation,
			"this credential was issued to a different machine; re-onboard this host to get its own")
		return
	}

	// Identity check before anything else observable happens. A contended
	// agent id means two live processes are claiming it, and admitting
	// either would mark the host online under a channel we cannot trust
	// and let the reconciler fail the other one's running jobs.
	if h.identityContended(sessCtx, row, hello.BootNonce) {
		_ = conn.Close(ws.StatusPolicyViolation,
			"another agent is already using this identity; this host looks cloned from an onboarded one")
		return
	}

	sess := &Session{
		AgentID: row.AgentID,
		// From the row, never from the token: a merge moves an agent to
		// another host without reissuing credentials, so the token's copy
		// can be one host out of date.
		HostID:      row.HostID,
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
	h.runManager.OnAgentReconnect(sessCtx, row.HostID, hello.RunningJobs, sess)

	connectedAt := h.registry.Add(sessCtx, sess, clockSkew(hello.ClockUnixMs))
	logger.Infof("agentgw: agent %s (host %d) channel open, version=%q token_version=%d",
		row.AgentID, row.HostID, hello.AgentVersion, row.TokenVersion)
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

// identityContended reports whether this agent id is currently claimed by
// more than one live agent process.
//
// A store failure admits the connection. The check is a safety net against
// a misconfigured fleet, not an authentication decision -- the bearer token
// already settled that -- so a database blip must not disconnect every
// agent in the deployment.
func (h *protocolHandler) identityContended(ctx context.Context, row *gwstore.AgentRow, bootNonce string) bool {
	contended, err := h.agents.CheckIdentity(ctx, row.HostID, bootNonce, identityConflictCooldown)
	if err != nil {
		logger.Warnf("agentgw channel: identity check for agent %s: %v", row.AgentID, err)
		return false
	}
	if contended {
		logger.Warnf("agentgw channel: refusing agent %s (host %d): its identity is claimed by more than one "+
			"live agent process. This host was most likely cloned from an onboarded one -- reset "+
			"/etc/machine-id on the copies and re-onboard them. Retries are refused for %s.",
			row.AgentID, row.HostID, identityConflictCooldown)
		// The duplicate holding the channel has to go too, or it would
		// simply keep it: it never reconnects, so it is never re-checked.
		// Since a clone boots after the machine it was copied from, the
		// incumbent is usually the copy -- leaving it in place would hand
		// the host to the clone and lock the real one out.
		if h.registry.Evict(row.HostID, "this host's agent identity is contended") {
			logger.Warnf("agentgw channel: also dropped the channel host %d was already holding", row.HostID)
		}
	}
	return contended
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
	// claims.HostID is deliberately NOT compared. It is a snapshot of
	// where this agent belonged when its token was issued, and the row is
	// the authority on where it belongs now -- which is what lets an
	// operator merge two host records without the machine having to
	// re-onboard. Nothing is lost by dropping the check: agent_id is the
	// signed identity, and the row it names carries the host id every
	// caller below uses.
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
		h.recordMetrics(ctx, sess, f.Metrics)
		h.alerts.Evaluate(ctx, sess.HostID, f.Metrics)
	case agenttypes.FrameTypeHello:
		// A second hello on an established channel is harmless; treat it
		// as a heartbeat so a reconnect-confused agent still stays fresh.
		h.touch(ctx, sess, clockSkew(f.ClockUnixMs))
	case agenttypes.FrameTypeHostFacts:
		h.recordFacts(ctx, sess, f.Facts)
	case agenttypes.FrameTypeHostProcesses:
		h.recordProcesses(ctx, sess, f.Processes)
	case agenttypes.FrameTypeHostAccounts:
		h.recordAccounts(ctx, sess, f.Accounts)
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

// recordMetrics persists a heartbeat's utilisation snapshot. Nil is the
// ordinary case for the first beat after an agent starts (the collector
// needs two samples) and for agents that predate the field, so it is
// not an event, let alone an error. A failed write costs nothing but
// staleness the next beat repairs, so it is logged and swallowed --
// utilisation display must never be able to take a control channel down.
func (h *protocolHandler) recordMetrics(ctx context.Context, sess *Session, m *agenttypes.MetricsSummary) {
	if m == nil {
		return
	}
	// A nil trend must become [] before Marshal, which would otherwise
	// render it as JSON null -- valid jsonb, but a second spelling of
	// "no buckets" every reader would have to know about.
	trend := []byte("[]")
	if len(m.CPUTrend) > 0 {
		if b, err := json.Marshal(m.CPUTrend); err == nil {
			trend = b
		}
	}
	if err := h.agents.UpsertMetrics(ctx, sess.HostID, gwstore.MetricsInput{
		SampledAt:    time.UnixMilli(m.SampledAtMs),
		CPUUsedPct:   m.CPUUsedPct,
		MemUsedPct:   m.MemUsedPct,
		DiskUsedPct:  m.DiskUsedPct,
		DiskUsedPath: m.DiskUsedPath,
		// Zero here means the agent predates these fields (or found no
		// filesystem, which on Linux it does not). Either way the honest
		// store is NULL, not a host with no disk.
		DiskUsedBytes:  nonZero(m.DiskUsedBytes),
		DiskTotalBytes: nonZero(m.DiskTotalBytes),
		Load1:          m.Load1,
		Load5:          m.Load5,
		Load15:         m.Load15,
		NetRxBps:       m.NetRxBps,
		NetTxBps:       m.NetTxBps,
		CPUTrend:       trend,
	}); err != nil {
		logger.Warnf("agentgw: record metrics for host %d: %v", sess.HostID, err)
	}
}

// recordFacts persists the machine's inventory.
//
// Like recordMetrics, a failed write is logged and swallowed: the agent
// resends its whole inventory on the next reconnect, so the worst case is
// a detail page showing yesterday's hardware, and no display concern may
// be able to take a control channel down.
//
// The three lists are re-marshalled rather than passed through: the frame
// was decoded into typed structs, so what goes to the jsonb column is
// this server's own encoding of a shape it understands, not whatever
// bytes a host put on the wire.
func (h *protocolHandler) recordFacts(ctx context.Context, sess *Session, facts *agenttypes.HostFacts) {
	if facts == nil {
		return
	}
	in := gwstore.FactsInput{
		Virtualization:    facts.Virtualization,
		CPUModel:          facts.CPUModel,
		CPUSockets:        facts.CPUSockets,
		CPUCoresPerSocket: facts.CPUCoresPerSocket,
		CPUThreadsPerCore: facts.CPUThreadsPerCore,
		KernelVersion:     facts.KernelVersion,
		OSID:              facts.OSID,
		OSVersionID:       facts.OSVersionID,
		SystemVendor:      facts.SystemVendor,
		ProductName:       facts.ProductName,
		BIOSVersion:       facts.BIOSVersion,
		BIOSDate:          facts.BIOSDate,
		BoardName:         facts.BoardName,
		BoardSerial:       facts.BoardSerial,
		ChassisType:       facts.ChassisType,
		SerialNumber:      facts.SerialNumber,
		AssetTag:          facts.AssetTag,
		Timezone:          facts.Timezone,
		DefaultGateway:    facts.DefaultGateway,
		KernelCmdline:     facts.KernelCmdline,
		ClockSync:         facts.ClockSync,
		NICs:              marshalList(facts.NICs),
		Filesystems:       marshalList(facts.Filesystems),
		BlockDevices:      marshalList(facts.BlockDevices),
		Swaps:             marshalList(facts.Swaps),
		DNS:               marshalDNS(facts.DNSServers, facts.DNSSearch),
		CPUMitigations:    marshalList(facts.CPUMitigations),
		SSHHostKeys:       marshalList(facts.SSHHostKeys),
	}
	if err := h.agents.UpsertFacts(ctx, sess.HostID, in); err != nil {
		logger.Warnf("agentgw: record facts for host %d: %v", sess.HostID, err)
	}
}

// recordProcesses persists the machine's workload, and recordAccounts who
// can use it. Same swallow-and-log contract as recordFacts: the agent
// resends its whole inventory on the next reconnect, so the worst case is
// a detail page showing an older snapshot, and no display concern may be
// able to take a control channel down.
func (h *protocolHandler) recordProcesses(ctx context.Context, sess *Session, p *agenttypes.HostProcesses) {
	if p == nil {
		return
	}
	if err := h.agents.UpsertProcesses(ctx, sess.HostID, marshalList(p.Groups), marshalList(p.Units)); err != nil {
		logger.Warnf("agentgw: record processes for host %d: %v", sess.HostID, err)
	}
}

func (h *protocolHandler) recordAccounts(ctx context.Context, sess *Session, a *agenttypes.HostAccounts) {
	if a == nil {
		return
	}
	in := gwstore.AccountsInput{
		Users:     marshalList(a.Users),
		Groups:    marshalList(a.Groups),
		SudoRules: marshalList(a.SudoRules),
	}
	if err := h.agents.UpsertAccounts(ctx, sess.HostID, in); err != nil {
		logger.Warnf("agentgw: record accounts for host %d: %v", sess.HostID, err)
	}
}

// marshalList encodes one inventory list, rendering an empty or
// unencodable one as [] rather than as JSON null -- valid jsonb, but a
// second spelling of "nothing here" every reader would have to know.
func marshalList[T any](items []T) []byte {
	if len(items) == 0 {
		return []byte("[]")
	}
	b, err := json.Marshal(items)
	if err != nil {
		return []byte("[]")
	}
	return b
}

// marshalDNS encodes the resolver configuration as one object. Re-encoded
// here from typed fields rather than passed through, the same as every
// other list: what reaches the column is this server's encoding of a
// shape it understands.
func marshalDNS(servers, search []string) []byte {
	b, err := json.Marshal(struct {
		Servers []string `json:"servers,omitempty"`
		Search  []string `json:"search,omitempty"`
	}{Servers: servers, Search: search})
	if err != nil {
		return []byte("{}")
	}
	return b
}

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

// verifyMachine reports whether this connection may proceed, and records
// a machine-id reset when it sees one.
//
// The refusal it can produce is the deterministic half of clone handling.
// The boot-nonce check below it catches two live processes alternating on
// one identity -- which requires them to overlap, and only ever says
// "somebody is duplicated", never which one is the impostor. This says
// exactly which, on the first connection, from evidence a copy cannot
// forge by accident: the original keeps its host, the copy is told to
// onboard as itself.
func (h *protocolHandler) verifyMachine(ctx context.Context, row *gwstore.AgentRow, hello *agenttypes.Frame) bool {
	fp := NewFingerprint("", hello.Fingerprint, time.Now())

	switch VerifyMachine(row, fp) {
	case VerdictForeignMachine:
		logger.Warnf("agentgw channel: refused agent %s (host %d): credential was issued to machine %s, caller is %s",
			row.AgentID, row.HostID, row.ProductUUID, fp.ProductUUID)
		// Also recorded on the row. From the operator's side this is a host
		// that will not come online, and the reason is on a machine they
		// may not have thought to look at -- a log line only helps someone
		// who already suspects where to look, and the agent will retry
		// forever in the meantime.
		if err := h.agents.RecordForeignMachine(ctx, row.HostID, fp.ProductUUID); err != nil {
			// Non-fatal: the refusal itself is the safety property, and
			// failing to write the explanation is no reason to change it.
			logger.Warnf("agentgw channel: record foreign machine for host %d: %v", row.HostID, err)
		}
		return false
	case VerdictMachineIDReset:
		if err := h.agents.RefreshFingerprint(ctx, row.HostID, fp.ToStore(row.IdentitySource)); err != nil {
			logger.Warnf("agentgw channel: record machine-id reset for host %d: %v", row.HostID, err)
		}
		logger.Infof("agentgw channel: host %d reset its machine id (%s -> %s)", row.HostID, row.MachineID, fp.MachineID)
	case VerdictHardwareChanged:
		if err := h.agents.RefreshFingerprint(ctx, row.HostID, fp.ToStore(row.IdentitySource)); err != nil {
			logger.Warnf("agentgw channel: record hardware change for host %d: %v", row.HostID, err)
		}
		logger.Infof("agentgw channel: host %d hardware identity changed (%s -> %s), machine-id unchanged",
			row.HostID, row.ProductUUID, fp.ProductUUID)
	}
	return true
}

// nonZero turns an absent-or-zero wire number into a NULL for the store.
// The summary is a fixed struct with no way to say "I did not measure
// this", so zero is the only spelling an older agent has for it.
func nonZero(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
