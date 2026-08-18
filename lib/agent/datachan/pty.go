package datachan

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

const (
	// ptyReadChunk is the read buffer for process output. Below the
	// framed-message ceiling with room to spare.
	ptyReadChunk = 32 * 1024
	// ptyExitGrace bounds how long a killed process gets before the
	// stream is torn down regardless.
	ptyExitGrace = 2 * time.Second
)

// A terminal is not torn down for being idle. Its liveness is the viewer,
// not the keystrokes: the gateway closes this stream the moment the
// browser disconnects (the read loop below catches that and kills the
// process), and yamux keepalive (channel.go, 30s) tears the whole session
// down if the gateway itself vanishes. A data-activity timeout on top of
// those has no failure mode of its own to catch -- it would only kill a
// healthy session someone is reading, e.g. a long file open in less.
// Idle-session policy, if a deployment wants one, belongs to the gateway
// that holds the human's WebSocket, not to a hardcoded constant that
// would take a fleet upgrade to change.

// serviceEnvPrefixes name variables that belong to the AGENT's service
// manager, not to a person's shell. They leak in through os.Environ()
// because the agent is a systemd unit, and a couple of them are not
// merely noise: a program run in the terminal that speaks sd_notify would
// report against the agent's own unit, and JOURNAL_STREAM changes how
// systemd-aware tools log.
//
// A denylist rather than an allowlist so the system's own settings --
// LANG and the LC_* that decide whether this terminal can render the
// filenames on the machine, TZ, anything an operator put in
// /etc/environment -- survive. An allowlist would have to guess at those,
// and guessing wrong is silently mojibake.
var serviceEnvPrefixes = []string{
	"NOTIFY_SOCKET=",
	"LISTEN_FDS=",
	"LISTEN_PID=",
	"LISTEN_FDNAMES=",
	"JOURNAL_STREAM=",
	"INVOCATION_ID=",
	// Same mechanism as NOTIFY_SOCKET, one step further: sd_notify's
	// watchdog ping is addressed by NOTIFY_SOCKET but gated on these. A
	// program started from the terminal would keep petting the AGENT's
	// watchdog, so systemd would stop restarting a hung agent. Nothing
	// sets WatchdogSec= on the unit today; the list is here so that the
	// day someone does is not the day this breaks.
	"WATCHDOG_USEC=",
	"WATCHDOG_PID=",
	"MANAGERPID=",
	"SYSTEMD_EXEC_PID=",
}

// startDir is where the shell starts: what the caller asked for, or the
// user's home. Empty is what makes the shell inherit the AGENT's working
// directory, which is the install path its unit file names -- never
// somewhere anyone means to be standing.
func startDir(requested string, me *user.User) string {
	if requested != "" {
		return requested
	}
	if me == nil {
		return ""
	}
	return me.HomeDir
}

// shellArgv0 is argv[0] for the process. A leading '-' is the only signal
// a shell has that it is a login shell, and it is what makes /etc/profile
// and ~/.profile run -- where PATH gets the entries every other tool on
// the machine assumes.
//
// Only when we chose the shell. A caller that named a command asked for
// that command, not for a login.
func shellArgv0(requestedCommand []string, path string) string {
	if len(requestedCommand) > 0 {
		return path
	}
	return "-" + filepath.Base(path)
}

// loginEnv is the environment a login session gets: what the system set,
// minus what belongs to the agent's unit, plus the identity variables a
// shell and everything under it read.
//
// HOME especially: without it a shell falls back to the passwd entry for
// some things and to "/" for others, so a stray `cd` or a tool writing a
// dotfile lands somewhere nobody expects.
func loginEnv(me *user.User) []string {
	env := make([]string, 0, len(os.Environ())+5)
	for _, kv := range os.Environ() {
		if hasAnyPrefix(kv, serviceEnvPrefixes) {
			continue
		}
		env = append(env, kv)
	}
	// TERM last-wins over anything inherited: this terminal is an xterm on
	// the viewer's side whatever the agent was started under.
	env = append(env, "TERM=xterm-256color")
	if me != nil {
		env = append(env,
			"HOME="+me.HomeDir,
			"USER="+me.Username,
			"LOGNAME="+me.Username,
		)
	}
	return env
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// servePTY forks a process under a PTY and bridges it to the stream.
//
// Only the web terminal (design §5.2) uses this, because only it needs a
// real terminal: interactivity, Ctrl-C, resize. Service log tail runs
// without a PTY through serveExec, so its output does not pick up the
// \r\n translation a terminal's line discipline would add. Neither needs
// sshd on the host.
func (c *Channel) servePTY(ctx context.Context, stream net.Conn, open agenttypes.StreamOpen) {
	argv := open.Command
	if len(argv) == 0 {
		argv = []string{c.shell()}
	}

	ptyCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ptyCtx, argv[0], argv[1:]...)
	// Fails on a machine whose uid has no passwd entry, which a minimal
	// container image can produce. Then there is no home to start in and
	// no name to set, so the shell gets what it would have got before --
	// said out loud, because otherwise it is a terminal that silently
	// opens in the wrong place.
	me, err := user.Current()
	if err != nil {
		c.cfg.Log.Warnf("data channel: cannot resolve the current user (%v); "+
			"the terminal will start in the agent's directory", err)
	}

	cmd.Dir = startDir(open.Dir, me)
	cmd.Env = loginEnv(me)
	cmd.Args[0] = shellArgv0(open.Command, argv[0])

	size := &pty.Winsize{Cols: open.Cols, Rows: open.Rows}
	if size.Cols == 0 {
		size.Cols = 80
	}
	if size.Rows == 0 {
		size.Rows = 24
	}

	f, startErr := pty.StartWithSize(cmd, size)
	if startErr != nil {
		reject(stream, agenttypes.StreamErrOpFailed, startErr.Error())
		return
	}
	defer func() { _ = f.Close() }()

	if err := accept(stream); err != nil {
		return
	}

	var writeMu sync.Mutex
	write := func(typ byte, payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return agenttypes.WriteMessage(stream, typ, payload)
	}

	// Process output -> stream.
	outDone := make(chan struct{})
	go func() {
		defer close(outDone)
		buf := make([]byte, ptyReadChunk)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				if werr := write(agenttypes.MsgData, buf[:n]); werr != nil {
					cancel()
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Stream -> process, plus resize control messages.
	go func() {
		for {
			typ, payload, err := agenttypes.ReadMessage(stream)
			if err != nil {
				// The viewer closed the tab: kill the process rather than
				// leaving an orphan shell attached to a dead PTY.
				cancel()
				return
			}
			switch typ {
			case agenttypes.MsgData:
				if _, err := f.Write(payload); err != nil {
					cancel()
					return
				}
			case agenttypes.MsgResize:
				var rs agenttypes.PTYResize
				if err := json.Unmarshal(payload, &rs); err != nil {
					continue
				}
				_ = pty.Setsize(f, &pty.Winsize{Cols: rs.Cols, Rows: rs.Rows})
			}
		}
	}()

	waitErr := cmd.Wait()
	// Draining the PTY after exit is what makes the last lines of output
	// (a command's final result, a shell's exit banner) reach the viewer
	// instead of being cut off by the exit message.
	select {
	case <-outDone:
	case <-time.After(ptyExitGrace):
	}

	exit := agenttypes.PTYExit{Code: exitCode(waitErr)}
	if waitErr != nil && exit.Code == -1 {
		exit.Error = waitErr.Error()
	}
	_ = writeJSON(&writeMu, stream, agenttypes.MsgExit, exit)
}

func (c *Channel) shell() string {
	if c.cfg.Shell != "" {
		return c.cfg.Shell
	}
	if _, err := os.Stat("/bin/bash"); err == nil {
		return "/bin/bash"
	}
	return "/bin/sh"
}

// exitCode extracts a process exit status, returning 0 on success and -1
// when the failure was not an exit status at all (signal kill, exec
// error) so the caller can attach the message instead.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// writeJSON is WriteJSONMessage under the caller's write mutex.
func writeJSON(mu *sync.Mutex, w io.Writer, typ byte, v any) error {
	mu.Lock()
	defer mu.Unlock()
	return agenttypes.WriteJSONMessage(w, typ, v)
}
