package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	"vraxel.io/vraxel/lib/agentdialer"
	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/lib/rest"
	ws "vraxel.io/vraxel/lib/websocket"
	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
)

// modeAgent is hosts.connectivity_mode for a machine reached through its
// own outbound agent. The alternative, 'ssh', has no terminal here: it
// would need a credential store, which vraxel does not have.
const modeAgent = "agent"

// maxTerminalsPerUser caps concurrent terminals one user may hold across
// all hosts. Each one pins a goroutine set and a PTY on a managed
// machine, so the cap is what stops a stuck tab from accumulating them.
const maxTerminalsPerUser = 20

// AgentDialerHolder carries a late-injected data-channel dialer.
//
// Injection is late because assembly runs compute first: the gateway
// needs compute's HostRegistrar, so the gateway (and its DataHub) does
// not exist when compute's routes are built. A nil dialer means the data
// plane was never wired, and the terminal says so rather than panicking.
type AgentDialerHolder struct {
	mu sync.RWMutex
	d  *agentdialer.Dialer
}

// NewAgentDialerHolder returns an empty holder to pass into compute at
// assembly time; the dialer is Set once the gateway is built.
func NewAgentDialerHolder() *AgentDialerHolder { return &AgentDialerHolder{} }

func (h *AgentDialerHolder) Set(d *agentdialer.Dialer) {
	h.mu.Lock()
	h.d = d
	h.mu.Unlock()
}

func (h *AgentDialerHolder) Get() *agentdialer.Dialer {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.d
}

// wsMessage is one decoded browser message handed to the input pump.
type wsMessage struct {
	msgType byte
	payload []byte
}

// NewTerminalHandler bridges a browser WebSocket to a PTY on the host,
// carried by the host's own outbound data channel. The host needs no
// sshd, no inbound port and no credential held here.
func NewTerminalHandler(hosts modstore.HostStore, sessionMgr *ws.SessionManager, dialer *AgentDialerHolder) rest.WebSocketHandler {
	return func(ctx context.Context, params map[string]string, conn *ws.Conn) {
		defer conn.Close(ws.StatusNormalClosure, "")
		runTerminalSession(ctx, params, conn, hosts, sessionMgr, dialer.Get())
	}
}

func runTerminalSession(
	ctx context.Context,
	params map[string]string,
	conn *ws.Conn,
	hosts modstore.HostStore,
	sessionMgr *ws.SessionManager,
	dialer *agentdialer.Dialer,
) {
	if dialer == nil {
		sendStatus(ctx, conn, "error", "the agent data plane is not available on this server")
		return
	}

	host, hostID, ok := lookupStreamHost(ctx, params, conn, hosts)
	if !ok {
		return
	}
	if host.ConnectivityMode != modeAgent {
		sendStatus(ctx, conn, "error", "this host has no agent; install one to open a terminal")
		return
	}

	userID := strconv.FormatInt(currentUserID(ctx), 10)
	if sessionMgr.CountByResource(userID, "host") >= maxTerminalsPerUser {
		sendStatus(ctx, conn, "error",
			fmt.Sprintf("you already have %d terminals open", maxTerminalsPerUser))
		return
	}

	// One reader for the socket, feeding a channel. Nothing else reads
	// conn: two concurrent readers on a WebSocket interleave frames.
	incoming, wsCtx, wsCancel := startBrowserReader(ctx, conn)
	defer wsCancel()

	sessionCtx, sessionCancel := context.WithCancel(wsCtx)
	defer sessionCancel()

	label := host.Name
	if host.Hostname != "" {
		label = host.Hostname
	}
	sess := sessionMgr.Acquire(conn, userID, "host", strconv.FormatInt(hostID, 10), label, sessionCancel)
	defer sessionMgr.Release(sess.ID)

	// Open at the size the browser reports, so the first prompt is drawn
	// at the right width instead of being reflowed by a later resize.
	cols, rows := parseTerminalSize(params["cols"], params["rows"])
	stream, err := dialer.StreamFor(sessionCtx, hostID, agenttypes.StreamOpen{
		Kind: agenttypes.StreamKindPTY,
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		logger.Warnf("terminal: open pty on host %d: %v", hostID, err)
		sendStatus(ctx, conn, "error", openFailureReason(err, "terminal"))
		return
	}
	defer stream.Close()

	sendStatus(sessionCtx, conn, "connected", label)

	var wg sync.WaitGroup
	idle := time.NewTimer(sessionMgr.IdleTimeout())
	defer idle.Stop()

	wg.Add(3)
	go pumpAgentToBrowser(&wg, sessionCtx, conn, stream, sessionCancel)
	go pumpBrowserToAgent(&wg, sessionCtx, incoming, stream, idle, sessionMgr, sessionCancel)
	go watchTerminalIdle(&wg, sessionCtx, conn, idle, sessionCancel)

	// Whatever ends the session -- shell exit, browser gone, idle timeout
	// -- unblock the pumps NOW. A yamux local Close only half-closes the
	// write side, so the output pump's blocked Read would otherwise wait
	// on the agent's round trip, or on the 30s keepalive if the agent is
	// gone, holding this handler and its session slot the whole time. The
	// deadline is what unblocks the Read; Close is the FIN that tells the
	// agent to kill the shell.
	<-sessionCtx.Done()
	_ = stream.SetDeadline(time.Now())
	_ = stream.Close()
	wg.Wait()
}

// openFailureReason turns a stream-open failure into something the
// operator can act on. what names the thing that failed to open --
// "terminal", "log stream" -- for the sentences that mention it.
//
// These are five different problems -- the machine is not connected, it
// is connected to a different replica, it never answered, the agent
// refused, or the tunnel broke -- and collapsing them into one sentence
// means the person reading it cannot tell "install an agent" from "wait
// a moment" from "file a bug". The detail is in the server log either
// way; this is what reaches the person who hit it.
func openFailureReason(err error, what string) string {
	switch {
	case errors.Is(err, agentgw.ErrHostUnreachable):
		return "this host's agent is not connected"
	case errors.Is(err, agentgw.ErrHostOnAnotherInstance):
		return "this host is connected to another server replica, which cannot be reached yet"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "the host did not open its data channel in time"
	}
	var rejected *agentgw.StreamRejected
	if errors.As(err, &rejected) {
		return "the agent refused the " + what + ": " + rejected.Message
	}
	return "could not open a " + what + " on this host"
}

// lookupStreamHost resolves the host in the caller's scope for the WS
// stream routes (terminal, logs). The scope comes from the URL, so a
// workspace-scoped route cannot reach a host in another workspace even
// with a valid id.
func lookupStreamHost(
	ctx context.Context,
	params map[string]string,
	conn *ws.Conn,
	hosts modstore.HostStore,
) (*modstore.HostRow, int64, bool) {
	hostID, err := strconv.ParseInt(params["hostId"], 10, 64)
	if err != nil || hostID <= 0 {
		sendStatus(ctx, conn, "error", "invalid host id")
		return nil, 0, false
	}
	wsID, _ := parseScopeID(params["workspaceId"])
	nsID, _ := parseScopeID(params["namespaceId"])
	host, err := hosts.GetByID(ctx, hostID, scope.FromIDs(wsID, nsID))
	if err != nil {
		// Only a genuinely missing row is "not found". Reporting a database
		// outage that way sends an operator looking for a deleted record
		// while the real fault is Postgres, and says nothing in the log to
		// correct them.
		if apierrors.IsNotFound(apierrors.FromDomain(err, "host")) {
			sendStatus(ctx, conn, "error", "host not found")
			return nil, 0, false
		}
		logger.Warnf("terminal: look up host %d: %v", hostID, err)
		sendStatus(ctx, conn, "error", "could not look up this host")
		return nil, 0, false
	}
	return host, hostID, true
}

// startBrowserReader owns the only read loop on the socket and decodes
// each message onto a channel. wsCtx is cancelled when the socket dies,
// which is how a closed tab tears the session down.
func startBrowserReader(ctx context.Context, conn *ws.Conn) (<-chan wsMessage, context.Context, context.CancelFunc) {
	incoming := make(chan wsMessage, 16)
	wsCtx, wsCancel := context.WithCancel(ctx)

	go func() {
		defer close(incoming)
		for {
			_, data, err := conn.ReadMessage(ctx)
			if err != nil {
				wsCancel()
				return
			}
			msgType, payload, err := ws.DecodeMessage(data)
			if err != nil {
				continue
			}
			select {
			case incoming <- wsMessage{msgType: msgType, payload: payload}:
			case <-wsCtx.Done():
				return
			}
		}
	}()

	return incoming, wsCtx, wsCancel
}

// pumpAgentToBrowser turns process output into browser data frames, and
// the shell's exit into a closing status.
func pumpAgentToBrowser(
	wg *sync.WaitGroup,
	sessionCtx context.Context,
	conn *ws.Conn,
	stream io.Reader,
	sessionCancel context.CancelFunc,
) {
	defer wg.Done()
	for {
		typ, payload, err := agenttypes.ReadMessage(stream)
		if err != nil {
			sessionCancel()
			return
		}
		switch typ {
		case agenttypes.MsgData:
			if werr := conn.WriteBinary(sessionCtx, ws.EncodeMessage(ws.MsgData, payload)); werr != nil {
				sessionCancel()
				return
			}
		case agenttypes.MsgExit:
			sendStatus(sessionCtx, conn, "exited", exitMessage(payload))
			sessionCancel()
			return
		}
	}
}

// pumpBrowserToAgent forwards keystrokes as stdin and window changes as
// PTY resizes, resetting the idle timer on real input.
func pumpBrowserToAgent(
	wg *sync.WaitGroup,
	sessionCtx context.Context,
	incoming <-chan wsMessage,
	stream io.Writer,
	idle *time.Timer,
	sessionMgr *ws.SessionManager,
	sessionCancel context.CancelFunc,
) {
	defer wg.Done()
	for {
		select {
		case msg, ok := <-incoming:
			if !ok {
				sessionCancel()
				return
			}
			switch msg.msgType {
			case ws.MsgData:
				resetIdleTimer(idle, sessionMgr.IdleTimeout())
				if err := agenttypes.WriteMessage(stream, agenttypes.MsgData, msg.payload); err != nil {
					sessionCancel()
					return
				}
			case ws.MsgResize:
				resize, err := ws.DecodeResizePayload(msg.payload)
				if err != nil {
					continue
				}
				// Bounded before the uint16 conversion, not after: cols=65536
				// truncates to 0, and the agent hands a resize straight to
				// pty.Setsize with no defaulting of its own, leaving a real
				// machine's PTY at zero columns. The opening size is checked
				// the same way -- it is the same value arriving by another
				// route, and only one of the two used to be guarded.
				if validTerminalSize(resize.Cols, resize.Rows) {
					_ = agenttypes.WriteJSONMessage(stream, agenttypes.MsgResize, agenttypes.PTYResize{
						Cols: uint16(resize.Cols), Rows: uint16(resize.Rows),
					})
				}
			}
		case <-sessionCtx.Done():
			return
		}
	}
}

// watchTerminalIdle closes a session nobody has typed into. The window is
// the SessionManager's, shared with every other interactive stream.
func watchTerminalIdle(
	wg *sync.WaitGroup,
	sessionCtx context.Context,
	conn *ws.Conn,
	idle *time.Timer,
	sessionCancel context.CancelFunc,
) {
	defer wg.Done()
	select {
	case <-idle.C:
		sendStatus(sessionCtx, conn, "timeout", "terminal closed after being idle")
		sessionCancel()
	case <-sessionCtx.Done():
	}
}

// resetIdleTimer drains and restarts the timer. The drain is what makes
// this safe to call after the timer may already have fired.
func resetIdleTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}

// exitMessage renders a PTYExit payload as the text the browser shows.
func exitMessage(payload []byte) string {
	var exit agenttypes.PTYExit
	if err := json.Unmarshal(payload, &exit); err != nil {
		return "shell exited"
	}
	switch {
	case exit.Error != "":
		return "shell exited: " + exit.Error
	case exit.Code != 0:
		return fmt.Sprintf("shell exited with code %d", exit.Code)
	default:
		return "shell exited normally"
	}
}

// sendStatus pushes one lifecycle message to the browser.
func sendStatus(ctx context.Context, conn *ws.Conn, status, message string) {
	msg, err := ws.EncodeStatusMessage(&ws.StatusPayload{Status: status, Message: message})
	if err != nil {
		return
	}
	_ = conn.WriteBinary(ctx, msg)
}

// Terminal dimension bounds. Generous enough for any real window, and
// closed at both ends because these numbers become a uint16 on the wire
// and then an ioctl on a managed machine.
const (
	minTerminalCols, maxTerminalCols = 10, 1000
	minTerminalRows, maxTerminalRows = 5, 500
)

func validTerminalSize(cols, rows int) bool {
	return cols >= minTerminalCols && cols <= maxTerminalCols &&
		rows >= minTerminalRows && rows <= maxTerminalRows
}

// parseTerminalSize reads the browser's window size, falling back to the
// library defaults per dimension. Out of range falls back rather than
// clamping: a client asking for 100000 columns is malfunctioning, and
// honouring half of its request hides that better than ignoring it.
func parseTerminalSize(colsStr, rowsStr string) (cols, rows int) {
	cols, rows = ws.DefaultCols, ws.DefaultRows
	if c, err := strconv.Atoi(colsStr); err == nil && c >= minTerminalCols && c <= maxTerminalCols {
		cols = c
	}
	if r, err := strconv.Atoi(rowsStr); err == nil && r >= minTerminalRows && r <= maxTerminalRows {
		rows = r
	}
	return cols, rows
}

// currentUserID is the authenticated user, or 0 when the context carries
// no identity. Only used to key session counts, never written to a row.
func currentUserID(ctx context.Context) int64 {
	id, _ := oidc.UserIDFromContext(ctx)
	return id
}

// terminalIdleTimeout closes a terminal nobody has typed into. Long
// enough that a shell left open beside a running job survives, short
// enough that a forgotten tab does not hold a root PTY overnight.
const terminalIdleTimeout = 30 * time.Minute

// NewTerminalSessions builds the registry of live terminals. One per
// process, shared by every terminal, because the per-user cap it
// enforces is a property of the process rather than of any one route.
func NewTerminalSessions() *ws.SessionManager { return ws.NewSessionManager(terminalIdleTimeout) }
