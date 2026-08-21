package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	"vraxel.io/vraxel/lib/agentdialer"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/rest"
	ws "vraxel.io/vraxel/lib/websocket"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
)

// Log sources the viewer can ask for. The browser picks a source; the
// argv it turns into is decided HERE, never client-side, because the
// exec stream runs whatever the gateway sends and the browser is not
// trusted to compose root commands.
const (
	// logSourceJournal is the systemd journal: syslog, every unit's
	// output, auth, and kernel messages in one stream.
	logSourceJournal = "journal"
	// logSourceKernel is the kernel ring buffer alone (dmesg).
	logSourceKernel = "kernel"
	// logSourceFile tails a plain file under /var/log -- app logs that
	// bypass the journal, and the fallback for hosts without systemd.
	logSourceFile = "file"
)

// Tail bounds. The default matches the viewer's middle option; the cap
// bounds the burst a stream replays on open, not its lifetime volume.
const (
	defaultLogTail = 500
	maxLogTail     = 10000
)

// maxLogStreamsPerUser caps concurrent log streams one user may hold
// across all hosts, for the same reason the terminal has a cap: each
// stream pins goroutines here and a follow process on a managed machine.
// Counted separately from terminals -- watching logs must not eat shell
// slots, or the other way round.
const maxLogStreamsPerUser = 20

// logPriorities are the syslog severities journalctl -p accepts. -p N
// means "N and more severe", which is what a viewer filter wants.
var logPriorities = map[string]bool{
	"emerg": true, "alert": true, "crit": true, "err": true,
	"warning": true, "notice": true, "info": true, "debug": true,
}

// logUnitPattern admits systemd unit names and their globs. The first
// character is never "-" so a unit can never read as a flag, and the
// value is one argv element (no shell anywhere), so this is a
// plausibility check, not an escaping exercise.
var logUnitPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9@_.:\\*-]*$`)

// logCommandFromParams turns the viewer's query parameters into the
// argv the agent will exec. Everything the browser sent is validated or
// bounded before it becomes part of a root process's command line.
func logCommandFromParams(params map[string]string) ([]string, error) {
	tail := strconv.Itoa(parseLogTail(params["tail"]))
	// Follow defaults on: the panel is a live view, and the absent-param
	// case is a hand-built URL rather than the UI.
	follow := params["follow"] != "false"

	switch params["source"] {
	case logSourceJournal, logSourceKernel:
		// short-iso rather than the default output: locale-independent
		// timestamps, and present since systemd 206 so it does not strand
		// old hosts the way newer flags (--no-hostname) would.
		argv := []string{"journalctl", "--no-pager", "-o", "short-iso", "-n", tail}
		if params["source"] == logSourceKernel {
			argv = append(argv, "-k")
		} else if unit := params["unit"]; unit != "" {
			if len(unit) > 256 || !logUnitPattern.MatchString(unit) {
				return nil, errors.New("invalid unit name")
			}
			argv = append(argv, "-u", unit)
		}
		if prio := params["priority"]; prio != "" {
			if !logPriorities[prio] {
				return nil, errors.New("invalid priority")
			}
			argv = append(argv, "-p", prio)
		}
		if follow {
			argv = append(argv, "-f")
		}
		return argv, nil

	case logSourceFile:
		if err := validateLogPath(params["path"]); err != nil {
			return nil, err
		}
		argv := []string{"tail", "-n", tail}
		if follow {
			// -F, not -f: logrotate moves the file out from under a plain
			// -f mid-watch, which is exactly when someone is watching.
			argv = []string{"tail", "-F", "-n", tail}
		}
		return append(argv, "--", params["path"]), nil
	}
	return nil, errors.New("unknown log source")
}

// validateLogPath keeps the file source inside /var/log. The permission
// this route runs under says "may read this host's logs"; without the
// prefix it would silently mean "may read any file as root", which is a
// different grant. A symlink under /var/log pointing elsewhere can still
// escape -- planting one takes root on the host already, so the check
// guards the grant's meaning, not the machine.
func validateLogPath(path string) error {
	if path == "" {
		return errors.New("path is required for the file source")
	}
	if len(path) > 512 {
		return errors.New("path too long")
	}
	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			return errors.New("path contains control characters")
		}
	}
	if !strings.HasPrefix(path, "/var/log/") || filepath.Clean(path) != path {
		return errors.New("path must be a clean absolute path under /var/log/")
	}
	return nil
}

// parseLogTail bounds the replayed backlog. Garbage falls back to the
// default; a too-large ask is clamped rather than rejected, because
// "give me lots" is served by the cap where a malformed terminal size
// (the fallback precedent) would be mis-served by half-honouring.
func parseLogTail(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultLogTail
	}
	if n > maxLogTail {
		return maxLogTail
	}
	return n
}

// NewHostLogsHandler bridges a browser WebSocket to a log stream on the
// host: journalctl or tail run by the agent over its own outbound data
// channel, read-only. The same carrier as the terminal, minus stdin.
func NewHostLogsHandler(hosts modstore.HostStore, sessionMgr *ws.SessionManager, dialer *AgentDialerHolder) rest.WebSocketHandler {
	return func(ctx context.Context, params map[string]string, conn *ws.Conn) {
		defer conn.Close(ws.StatusNormalClosure, "")
		runLogsSession(ctx, params, conn, hosts, sessionMgr, dialer.Get())
	}
}

func runLogsSession(
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
		sendStatus(ctx, conn, "error", "this host has no agent; install one to view its logs")
		return
	}

	argv, err := logCommandFromParams(params)
	if err != nil {
		sendStatus(ctx, conn, "error", err.Error())
		return
	}

	userID := strconv.FormatInt(currentUserID(ctx), 10)
	if sessionMgr.CountByResource(userID, "host-logs") >= maxLogStreamsPerUser {
		sendStatus(ctx, conn, "error",
			fmt.Sprintf("you already have %d log streams open", maxLogStreamsPerUser))
		return
	}

	incoming, wsCtx, wsCancel := startBrowserReader(ctx, conn)
	defer wsCancel()

	sessionCtx, sessionCancel := context.WithCancel(wsCtx)
	defer sessionCancel()

	label := host.Name
	if host.Hostname != "" {
		label = host.Hostname
	}
	sess := sessionMgr.Acquire(conn, userID, "host-logs", strconv.FormatInt(hostID, 10), label, sessionCancel)
	defer sessionMgr.Release(sess.ID)

	stream, err := dialer.StreamFor(sessionCtx, hostID, agenttypes.StreamOpen{
		Kind:    agenttypes.StreamKindExec,
		Command: argv,
	})
	if err != nil {
		logger.Warnf("host logs: open exec on host %d: %v", hostID, err)
		sendStatus(ctx, conn, "error", openFailureReason(err, "log stream"))
		return
	}
	defer stream.Close()

	sendStatus(sessionCtx, conn, "connected", label)

	var wg sync.WaitGroup
	wall := time.NewTimer(sessionMgr.IdleTimeout())
	defer wall.Stop()

	wg.Add(3)
	go pumpLogsToBrowser(&wg, sessionCtx, conn, stream, sessionCancel)
	go drainLogsInput(&wg, sessionCtx, incoming, sessionCancel)
	go watchLogsWall(&wg, sessionCtx, conn, wall, sessionCancel)

	// Same shutdown as the terminal: the deadline unblocks the pump's
	// Read now, Close is the FIN that kills the process on the host.
	<-sessionCtx.Done()
	_ = stream.SetDeadline(time.Now())
	_ = stream.Close()
	wg.Wait()
}

// pumpLogsToBrowser forwards process output and turns its exit into a
// closing status. The terminal's pump with the exit line reworded: what
// ended here is a log stream, not somebody's shell.
func pumpLogsToBrowser(
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
			sendStatus(sessionCtx, conn, "exited", logEndMessage(payload))
			sessionCancel()
			return
		}
	}
}

// drainLogsInput discards whatever the browser sends. A log viewer has
// no stdin, but the channel must still be read: startBrowserReader
// blocks handing a message over, and with nobody receiving, a client
// that sends anyway would wedge the reader and hide the socket close
// that tears the session down.
func drainLogsInput(
	wg *sync.WaitGroup,
	sessionCtx context.Context,
	incoming <-chan wsMessage,
	sessionCancel context.CancelFunc,
) {
	defer wg.Done()
	for {
		select {
		case _, ok := <-incoming:
			if !ok {
				sessionCancel()
				return
			}
		case <-sessionCtx.Done():
			return
		}
	}
}

// watchLogsWall ends a stream that has been open for the whole session
// window. Unlike the terminal there is no input to prove a person is
// still there, so the timer never resets: without a wall, a forgotten
// follow tab holds a journalctl on a managed machine indefinitely. One
// click in the viewer reconnects.
func watchLogsWall(
	wg *sync.WaitGroup,
	sessionCtx context.Context,
	conn *ws.Conn,
	wall *time.Timer,
	sessionCancel context.CancelFunc,
) {
	defer wg.Done()
	select {
	case <-wall.C:
		sendStatus(sessionCtx, conn, "timeout", "log stream closed after its session window; reconnect to keep following")
		sessionCancel()
	case <-sessionCtx.Done():
	}
}

// logEndMessage renders the exec's exit for the viewer. Code 0 is the
// ordinary end of a non-follow read and says nothing alarming.
func logEndMessage(payload []byte) string {
	var exit agenttypes.PTYExit
	if err := json.Unmarshal(payload, &exit); err != nil {
		return "log stream ended"
	}
	switch {
	case exit.Error != "":
		return "log stream ended: " + exit.Error
	case exit.Code != 0:
		return fmt.Sprintf("log stream ended with code %d", exit.Code)
	default:
		return "log stream ended"
	}
}
