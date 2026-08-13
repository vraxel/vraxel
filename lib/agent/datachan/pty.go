package datachan

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
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
	cmd.Dir = open.Dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	size := &pty.Winsize{Cols: open.Cols, Rows: open.Rows}
	if size.Cols == 0 {
		size.Cols = 80
	}
	if size.Rows == 0 {
		size.Rows = 24
	}

	f, err := pty.StartWithSize(cmd, size)
	if err != nil {
		reject(stream, agenttypes.StreamErrOpFailed, err.Error())
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
