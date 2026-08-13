package sshclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
)

// WindowSize is the rows/cols pair used to size a remote PTY. Matches
// the kubesphere / kubectl exec convention so frontend ResizePayload
// frames map straight through.
type WindowSize struct {
	Rows uint16
	Cols uint16
}

// PtyOpts configures a single ExecPty call.
//
// InitialSize is sent on session.RequestPty before the command starts
// so dnf / yum / mkfs / parted etc. format their column-aligned output
// for the right terminal width from the first byte.
//
// ResizeCh, when non-nil, lets the caller forward live window-size
// changes (browser FitAddon → WS MsgResize → executor.Resize → here).
// Each value sent on the channel turns into a session.WindowChange
// call. The channel is drained until the command exits or ctx is done;
// the caller is responsible for closing it.
//
// LiveOutput, when non-nil, receives PTY bytes as they arrive so a
// streaming WS log viewer can render output character-by-character
// instead of in a single end-of-task burst. PTY merges stdout and
// stderr by design, so a single writer covers everything.
//
// Stdin, when non-nil, is read once and its bytes are written to the
// PTY's stdin before the connector starts forwarding output. The
// canonical use is feeding a sudo password (`sudo -S` reads its
// password from stdin); the bytes never reach the actual command
// because sudo strips them. Stdin is closed after the data is sent so
// any subprocess reading from stdin sees EOF — matches the behaviour
// of non-interactive `ssh host cmd <<<password`.
//
// EchoOff disables PTY echo (termios ECHO=0). Required when feeding a
// password via Stdin so the password isn't reflected back into
// LiveOutput / Stdout. Most vraxel deploy paths run non-interactive
// commands that never read stdin, so disabling echo is also harmless
// for the no-password case.
type PtyOpts struct {
	InitialSize WindowSize
	ResizeCh    <-chan WindowSize
	LiveOutput  io.Writer
	Stdin       io.Reader
	EchoOff     bool
}

// ExecPty runs cmd inside an allocated PTY of opts.InitialSize and
// returns the captured combined output. PTY merges stdout and stderr;
// the returned ExecResult.Stderr is always empty — callers that
// previously relied on stderr separation must check ExitCode and
// scan Stdout instead.
//
// Why this exists: the non-PTY Exec/ExecWithSudo paths produce
// terminal output that's neither plainly formatted (no TTY → tools
// fall back to inconsistent shapes) nor properly column-aligned for
// any specific width. Allocating a PTY of a known size lets us
// match what the user sees on a real `ssh root@host` terminal,
// which is what xterm.js on the frontend expects.
func (c *Client) ExecPty(ctx context.Context, cmd string, opts PtyOpts) (*ExecResult, error) {
	session, err := c.newSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	rows, cols := ptyWindowSize(opts)

	// xterm-256color matches the TERM env our interactive terminal
	// already advertises. EchoOff turns off the local PTY's stdin echo
	// so password bytes fed via Stdin don't leak into LiveOutput.
	modes := ssh.TerminalModes{}
	if opts.EchoOff {
		modes[ssh.ECHO] = 0
	}
	if err := session.RequestPty("xterm-256color", int(rows), int(cols), modes); err != nil {
		return nil, fmt.Errorf("sshclient: request pty: %w", err)
	}

	buf := wirePtyOutput(session, opts)

	// Pre-feed stdin (typically the sudo password). Drain through a
	// pipe instead of session.Stdin so the writer end can be closed
	// once the data is sent — leaving stdin open would let any
	// subprocess that does read from stdin block waiting for input.
	if opts.Stdin != nil {
		if err := feedPtyStdin(session, opts.Stdin); err != nil {
			return nil, err
		}
	}

	// Resize forwarder: a single goroutine reads from opts.ResizeCh
	// for the lifetime of the command and calls session.WindowChange.
	// forwardCtx is a derived context this function cancels when the
	// command exits — without it, a session.Run that completes
	// naturally would deadlock waiting on resizeDone (the goroutine
	// is parked in select { ctx.Done, ResizeCh } and the caller's
	// ResizeCh is typically closed via defer AFTER ExecPty returns,
	// so neither branch fires).
	forwardCtx, cancelForward := context.WithCancel(ctx)
	defer cancelForward()
	resizeDone := forwardPtyResize(forwardCtx, session, opts.ResizeCh)

	done := make(chan error, 1)
	var once sync.Once
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case <-ctx.Done():
		once.Do(func() { _ = session.Signal(ssh.SIGKILL) })
		cancelForward()
		<-resizeDone
		return &ExecResult{Stdout: buf.Bytes()}, ctx.Err()
	case err := <-done:
		cancelForward()
		<-resizeDone
		return &ExecResult{Stdout: buf.Bytes(), ExitCode: exitCode(err)}, err
	}
}

// ptyWindowSize resolves the PTY rows/cols from opts, defaulting to a
// wide-enough terminal that dnf / yum / mkfs column alignment never wraps
// mid-word inside a typical browser dialog. Frontend FitAddon will
// override with the real container size on first MsgResize. Extracted
// from ExecPty.
func ptyWindowSize(opts PtyOpts) (rows, cols uint16) {
	rows = uint16(40)
	cols = uint16(120)
	if opts.InitialSize.Rows > 0 {
		rows = opts.InitialSize.Rows
	}
	if opts.InitialSize.Cols > 0 {
		cols = opts.InitialSize.Cols
	}
	return rows, cols
}

// wirePtyOutput points session.Stdout/Stderr at a capture buffer, teeing
// to opts.LiveOutput when set so the WS log viewer sees bytes the moment
// they arrive instead of waiting for session.Run to return at end of
// command. Returns the capture buffer. Extracted from ExecPty.
func wirePtyOutput(session *ssh.Session, opts PtyOpts) *bytes.Buffer {
	var buf bytes.Buffer
	var stdoutWriter io.Writer = &buf
	if opts.LiveOutput != nil {
		stdoutWriter = io.MultiWriter(&buf, opts.LiveOutput)
	}
	session.Stdout = stdoutWriter
	// PTY merges stderr into stdout, but ssh-go still wires Stderr
	// separately — point it at the same sinks so any out-of-band
	// stderr writes are also captured + streamed.
	session.Stderr = stdoutWriter
	return &buf
}

// feedPtyStdin pre-feeds stdin (typically the sudo password) through a
// pipe so the writer end can be closed once the data is sent — leaving
// stdin open would let any subprocess that does read from stdin block
// waiting for input. Extracted from ExecPty.
func feedPtyStdin(session *ssh.Session, stdin io.Reader) error {
	stdinPipe, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("sshclient: stdin pipe: %w", err)
	}
	go func() {
		defer stdinPipe.Close()
		_, _ = io.Copy(stdinPipe, stdin)
	}()
	return nil
}

// forwardPtyResize starts the resize forwarder goroutine and returns a
// channel closed once it exits. A single goroutine reads from resizeCh
// for the lifetime of the command and calls session.WindowChange. When
// resizeCh is nil the returned channel is already closed. Extracted from
// ExecPty.
func forwardPtyResize(forwardCtx context.Context, session *ssh.Session, resizeCh <-chan WindowSize) chan struct{} {
	resizeDone := make(chan struct{})
	if resizeCh == nil {
		close(resizeDone)
		return resizeDone
	}
	go func() {
		defer close(resizeDone)
		for {
			select {
			case <-forwardCtx.Done():
				return
			case ws, ok := <-resizeCh:
				if applyPtyResize(session, ws, ok) {
					return
				}
			}
		}
	}()
	return resizeDone
}

// applyPtyResize handles one value read off the resize channel and
// reports whether the forwarder loop should stop. ok=false (channel
// closed) stops the loop; a zero-dimension frame is skipped; otherwise
// session.WindowChange is called. Extracted from forwardPtyResize's
// select body so the goroutine stays flat.
func applyPtyResize(session *ssh.Session, ws WindowSize, ok bool) (stop bool) {
	if !ok {
		return true
	}
	if ws.Rows == 0 || ws.Cols == 0 {
		return false
	}
	_ = session.WindowChange(int(ws.Rows), int(ws.Cols))
	return false
}
