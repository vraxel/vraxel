package datachan

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"sync"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// serveExec runs a process without a PTY and streams its output.
//
// This is the log-tail path (design §5.5). It is deliberately NOT the
// PTY path: the log panel scans lines rather than rendering a terminal,
// so a PTY here would only add \r\n translation and colour escapes that
// the SSH log tail it replaces never produced. Running journalctl -f
// through a plain pipe keeps the agent output byte-for-byte identical to
// the existing SSH stream, which is the whole promise of the migration.
//
// stdout and stderr are merged onto one stream because the consumer is a
// single ordered log view; splitting them would reorder interleaved
// lines. There is no resize and no exit body -- the client closes the
// stream to stop the follow, and the process dies with the context.
func (c *Channel) serveExec(ctx context.Context, stream net.Conn, open agenttypes.StreamOpen) {
	if len(open.Command) == 0 {
		reject(stream, agenttypes.StreamErrOpFailed, "exec stream has no command")
		return
	}

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(execCtx, open.Command[0], open.Command[1:]...)
	cmd.Dir = open.Dir

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		reject(stream, agenttypes.StreamErrOpFailed, err.Error())
		return
	}
	cmd.Stderr = cmd.Stdout // one ordered stream; see above

	if err := cmd.Start(); err != nil {
		reject(stream, agenttypes.StreamErrOpFailed, err.Error())
		return
	}
	if err := accept(stream); err != nil {
		cancel()
		_ = cmd.Wait()
		return
	}

	var writeMu sync.Mutex

	// Output -> stream.
	outDone := make(chan struct{})
	go func() {
		defer close(outDone)
		buf := make([]byte, ptyReadChunk)
		for {
			n, rerr := pipe.Read(buf)
			if n > 0 {
				writeMu.Lock()
				werr := agenttypes.WriteMessage(stream, agenttypes.MsgData, buf[:n])
				writeMu.Unlock()
				if werr != nil {
					cancel()
					return
				}
			}
			if rerr != nil {
				return
			}
		}
	}()

	// The client closing the stream is the stop signal: a follow has no
	// natural end, so the reader ending means "the viewer went away". A
	// quiet follow is NOT torn down for being quiet -- a service that logs
	// nothing for an hour is exactly what someone opens the panel to
	// watch, and killing it would drop the line they were waiting for.
	// The two real ends are covered: this loop catches the viewer leaving,
	// and yamux keepalive catches the gateway vanishing.
	go func() {
		for {
			if _, _, err := agenttypes.ReadMessage(stream); err != nil {
				cancel()
				return
			}
		}
	}()

	// Drain the pipe to EOF BEFORE Wait: cmd.StdoutPipe's contract is
	// that Wait closes the pipe, so calling it while the output goroutine
	// is still reading turns a clean EOF into a "file already closed"
	// error. The process exiting closes its stdout, which ends the read,
	// so outDone fires without help from Wait.
	<-outDone
	waitErr := cmd.Wait()

	exit := agenttypes.PTYExit{Code: exitCode(waitErr)}
	if waitErr != nil && exit.Code == -1 && !errors.Is(execCtx.Err(), context.Canceled) {
		exit.Error = waitErr.Error()
	}
	_ = writeJSON(&writeMu, stream, agenttypes.MsgExit, exit)
}
