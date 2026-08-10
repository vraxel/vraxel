package websocket

import (
	"context"
	"io"
	"time"

	cws "github.com/coder/websocket"
)

// Re-export status codes so callers do not need to import coder/websocket.
const (
	StatusNormalClosure = cws.StatusNormalClosure
	StatusGoingAway     = cws.StatusGoingAway
	StatusInternalError = cws.StatusInternalError
)

// Conn wraps a coder/websocket.Conn to isolate the third-party dependency.
// It automatically sends WebSocket ping frames every 15 seconds to keep
// the connection alive through proxies and load balancers.
//
// Concurrency contract (mirrors coder/websocket):
//   - One goroutine may call ReadMessage at a time.
//   - One or more goroutines may call WriteBinary / WriteMessage; the
//     underlying library serialises writes via an internal mutex, so
//     concurrent writes are safe (just serialized).
//   - Read and Write are independent — a reader goroutine and a
//     writer goroutine running simultaneously is the supported pattern.
//   - The internal keepalive goroutine sends Ping frames; this is
//     coordinated with WriteMessage by the underlying library, so
//     callers never need to lock around Pings.
//
// Pattern used by terminal-style handlers (provision progress,
// interactive SSH terminal, future ops/dev WS handlers): one
// "ForwardResizeFromWS" reader goroutine + the main WriteBinary
// dispatch loop. Both are safe under the contract above.
type Conn struct {
	inner  *cws.Conn
	cancel context.CancelFunc
}

// NewConn creates a Conn wrapping the given coder/websocket connection.
// Starts a background goroutine that sends ping frames every 15 seconds.
func NewConn(c *cws.Conn) *Conn {
	ctx, cancel := context.WithCancel(context.Background())
	conn := &Conn{inner: c, cancel: cancel}

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := c.Ping(ctx); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return conn
}

// ReadMessage reads a complete WebSocket message.
// It returns the message type and the raw bytes.
func (c *Conn) ReadMessage(ctx context.Context) (cws.MessageType, []byte, error) {
	msgType, reader, err := c.inner.Reader(ctx)
	if err != nil {
		return 0, nil, err
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, nil, err
	}
	return msgType, data, nil
}

// WriteMessage writes a complete WebSocket message of the given type.
func (c *Conn) WriteMessage(ctx context.Context, msgType cws.MessageType, data []byte) error {
	return c.inner.Write(ctx, msgType, data)
}

// WriteBinary writes a binary WebSocket message.
func (c *Conn) WriteBinary(ctx context.Context, data []byte) error {
	return c.WriteMessage(ctx, cws.MessageBinary, data)
}

// Close sends a close frame and stops the keepalive goroutine.
func (c *Conn) Close(code cws.StatusCode, reason string) error {
	c.cancel() // stop keepalive
	return c.inner.Close(code, reason)
}

// DrainReads starts a background goroutine that reads and DISCARDS every
// incoming client frame, and returns a context cancelled the moment the
// client disconnects. Use it in write-only handlers (log / progress /
// message tails) as the FIRST thing after the upgrade, rebinding the
// handler ctx:
//
//	ctx = conn.DrainReads(ctx)
//
// Why: an upgraded WebSocket connection is hijacked, so the http.Request
// context passed to the handler does NOT cancel when the peer goes away
// (it only cancels once ServeHTTP returns, which cannot happen while the
// handler is blocked). A pure-writer that selects on <-ctx.Done() and a
// tail channel therefore never notices a disconnect while the upstream is
// idle -- it leaks its goroutine plus the upstream stream (VictoriaLogs
// HTTP tail, SSH tail -f, ...) until a stray line makes the next Write
// fail. Draining the read side cancels ctx on disconnect so both the write
// loop and any upstream opened with that ctx unwind promptly.
//
// Unlike coder/websocket's CloseRead, a client DATA frame is discarded, not
// treated as a policy violation that closes the connection: the shared
// frontend TaskTerminalDialog sends resize frames to every stream it opens,
// including these write-only progress/log tails, and those must be ignored,
// not kill the stream. Control frames (ping/pong/close) are still answered
// by the reader, keeping the connection alive through proxies.
//
// Contract: after DrainReads you MUST NOT call ReadMessage yourself -- the
// drain goroutine owns the read side. Interactive handlers (exec / console
// / chat) that read the peer must NOT use this.
func (c *Conn) DrainReads(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		for {
			_, r, err := c.inner.Reader(ctx)
			if err != nil {
				return // peer gone or ctx cancelled -- unwind the writer
			}
			_, _ = io.Copy(io.Discard, r) // discard the client frame (resize/keepalive)
		}
	}()
	return ctx
}

// SetReadLimit sets the maximum size of a single message the connection
// will read. The default is 32768 bytes. Use -1 for no limit.
func (c *Conn) SetReadLimit(limit int64) {
	c.inner.SetReadLimit(limit)
}

// Inner returns the underlying coder/websocket connection.
// Use sparingly; prefer the wrapper methods.
func (c *Conn) Inner() *cws.Conn {
	return c.inner
}
