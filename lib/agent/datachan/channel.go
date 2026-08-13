// Package datachan is the agent side of the data channel (design §4.3):
// one persistent WSS per host, carrying every synchronous session as a
// yamux logical stream.
//
// It has no vraxel dependencies beyond the wire contract in lib/agent/types
// and the WebSocket wrapper, so the whole package ports to another
// product by supplying a Config.
package datachan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	cws "github.com/coder/websocket"
	"github.com/hashicorp/yamux"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	ws "vraxel.io/vraxel/lib/websocket"
)

const (
	// dialTimeout bounds one connection attempt including the upgrade.
	dialTimeout = 30 * time.Second
	// idleTimeout closes a data channel that has carried no stream for
	// this long. The channel is demand-driven: a host nobody is looking at
	// holds one connection (control), not two. At ten thousand hosts that
	// halves the idle connection count, and the cost of bringing it back
	// is one dial on the next request.
	idleTimeout = 5 * time.Minute
	// reconnectMin / reconnectMax bound the backoff between failed dials.
	reconnectMin = 1 * time.Second
	reconnectMax = 60 * time.Second
	// keepAliveInterval is yamux's own liveness ping inside the WSS. It
	// detects a half-open channel that the TCP layer still believes is up.
	keepAliveInterval = 30 * time.Second
	// writeTimeout bounds one yamux frame write for the same reason the
	// control channel bounds its own writes: a filled socket buffer on a
	// half-open connection would otherwise block for the kernel's whole
	// TCP retry budget.
	writeTimeout = 10 * time.Second
)

// Logger is the minimal logging surface this package needs. Declared
// here rather than imported so the package carries no logging
// dependency into another product.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
}

// Config is everything the data channel needs from its host program.
type Config struct {
	// ServerURL is the vraxel-server base URL (http:// or https://).
	ServerURL string
	// Token returns the current session token. Read per dial, because the
	// token rotates on every control-channel reconnect.
	Token func() string
	// Guard decides which targets a tcp stream may reach. Required: a nil
	// guard would make the agent an open proxy into the customer network.
	Guard *Guard
	// HTTPClient performs the WebSocket upgrade, carrying the TLS trust
	// store. Nil uses the library default.
	HTTPClient *http.Client
	// Shell is the login shell for pty streams with no explicit command.
	// Defaults to /bin/bash, falling back to /bin/sh.
	Shell string
	// Log receives connection lifecycle messages.
	Log Logger
}

// Channel maintains the single data channel and serves streams on it.
//
// It is demand-driven: Run parks until Ensure is called (the server asks
// for the channel over the control channel when it has work), serves
// until the connection breaks or goes idle, then parks again.
type Channel struct {
	cfg Config

	wake chan struct{}

	mu      sync.Mutex
	streams int
	lastUse time.Time
}

// New builds a Channel. Panics on a nil Guard: an unguarded data channel
// turns the agent into a pivot into the customer's network, which is the
// one property this design exists to prevent.
func New(cfg Config) *Channel {
	if cfg.Guard == nil {
		panic("datachan: Guard is required")
	}
	return &Channel{cfg: cfg, wake: make(chan struct{}, 1)}
}

// Ensure asks for the data channel to be up. Non-blocking and
// idempotent: a pending wake-up absorbs further calls, and a call while
// the channel is already serving is a no-op.
func (c *Channel) Ensure() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// Run parks until Ensure is called, then keeps the data channel up until
// it goes idle or ctx ends. Never returns an error for the same reason
// the control channel does not: a broken data channel is transient, and
// the process must stay alive to serve the next request.
func (c *Channel) Run(ctx context.Context) {
	backoff := reconnectMin
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.wake:
		}
		for {
			if ctx.Err() != nil {
				return
			}
			start := time.Now()
			err := c.session(ctx)
			if ctx.Err() != nil {
				return
			}
			if err == nil {
				// Clean idle shutdown: park until the server asks again.
				backoff = reconnectMin
				break
			}
			c.cfg.Log.Warnf("data channel: %v", err)
			if time.Since(start) > reconnectMax {
				backoff = reconnectMin
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(jittered(backoff)):
			}
			if backoff < reconnectMax {
				backoff *= 2
				if backoff > reconnectMax {
					backoff = reconnectMax
				}
			}
		}
	}
}

// jittered spreads a backoff delay over [d/2, d]. Same reason as the
// control channel: ten thousand agents must not retry in lockstep.
func jittered(d time.Duration) time.Duration {
	half := d / 2
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

// session runs one data channel from dial to close. Returns nil when the
// channel closed because it went idle, an error otherwise.
func (c *Channel) session(ctx context.Context) error {
	sessCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	conn, err := c.dial(sessCtx)
	if err != nil {
		return err
	}
	defer conn.CloseNow()

	// yamux needs a byte stream; coder/websocket gives one over binary
	// messages. No read limit is set: unlike the control channel this
	// carries bulk payloads, and yamux's own window is the flow control.
	netConn := cws.NetConn(sessCtx, conn.Inner(), cws.MessageBinary)

	ycfg := yamux.DefaultConfig()
	ycfg.EnableKeepAlive = true
	ycfg.KeepAliveInterval = keepAliveInterval
	ycfg.ConnectionWriteTimeout = writeTimeout
	ycfg.LogOutput = io.Discard
	// The gateway opens streams, so the agent is the yamux server.
	sess, err := yamux.Server(netConn, ycfg)
	if err != nil {
		return fmt.Errorf("yamux: %w", err)
	}
	defer sess.Close()

	c.cfg.Log.Infof("data channel: connected to %s", c.cfg.ServerURL)
	c.touch()

	idle := c.watchIdle(sessCtx, sess)
	defer idle()

	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		stream, err := sess.AcceptStream()
		if err != nil {
			if c.idleClosed() || sessCtx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept stream: %w", err)
		}
		c.streamStart()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer c.streamDone()
			defer stream.Close()
			c.serve(sessCtx, stream)
		}()
	}
}

func (c *Channel) dial(ctx context.Context) (*ws.Conn, error) {
	token := ""
	if c.cfg.Token != nil {
		token = c.cfg.Token()
	}
	if token == "" {
		return nil, errors.New("no session token yet")
	}

	dctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	url := wsURL(c.cfg.ServerURL) + agenttypes.DataChannelPath
	// resp.Body is deliberately not closed; see the same call in
	// lib/agent/client/channel.go for why.
	conn, resp, err := ws.Dial(dctx, url, &ws.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + token}},
		HTTPClient: c.cfg.HTTPClient,
	})
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("dial %s: %w (http %d)", url, err, resp.StatusCode)
		}
		return nil, fmt.Errorf("dial %s: %w", url, err)
	}
	return conn, nil
}

// serve reads a stream's opening header and dispatches by kind.
func (c *Channel) serve(ctx context.Context, stream net.Conn) {
	var open agenttypes.StreamOpen
	if err := agenttypes.ReadStreamHeader(stream, &open); err != nil {
		c.cfg.Log.Warnf("data channel: unreadable stream header: %v", err)
		return
	}
	switch open.Kind {
	case agenttypes.StreamKindTCP:
		c.serveTCP(ctx, stream, open)
	case agenttypes.StreamKindPTY:
		c.servePTY(ctx, stream, open)
	case agenttypes.StreamKindExec:
		c.serveExec(ctx, stream, open)
	case agenttypes.StreamKindFile:
		c.serveFile(ctx, stream, open)
	default:
		reject(stream, agenttypes.StreamErrUnknownKind, "unknown stream kind "+open.Kind)
	}
}

// accept / reject write the handshake answer. Every handler answers
// exactly once before any payload flows.
func accept(w io.Writer) error {
	return agenttypes.WriteStreamHeader(w, agenttypes.StreamAccept{Ok: true})
}

func reject(w io.Writer, code, msg string) {
	_ = agenttypes.WriteStreamHeader(w, agenttypes.StreamAccept{Ok: false, Code: code, Error: msg})
}

// --- idle accounting ---

func (c *Channel) streamStart() {
	c.mu.Lock()
	c.streams++
	c.lastUse = time.Now()
	c.mu.Unlock()
}

func (c *Channel) streamDone() {
	c.mu.Lock()
	c.streams--
	c.lastUse = time.Now()
	c.mu.Unlock()
}

func (c *Channel) touch() {
	c.mu.Lock()
	c.lastUse = time.Now()
	c.mu.Unlock()
}

// idleClosed reports whether the session ended because nothing used it.
// Read after AcceptStream fails, to tell a deliberate idle close from a
// real transport failure.
func (c *Channel) idleClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.streams == 0 && time.Since(c.lastUse) >= idleTimeout
}

// watchIdle closes the session once it has been unused for idleTimeout.
// Returns a stop func.
func (c *Channel) watchIdle(ctx context.Context, sess *yamux.Session) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(idleTimeout / 4)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				if c.idleClosed() {
					c.cfg.Log.Infof("data channel: idle for %s, closing", idleTimeout)
					_ = sess.Close()
					return
				}
			}
		}
	}()
	return func() { close(stop) }
}

// wsURL rewrites an http(s) server URL to its ws(s) equivalent.
func wsURL(serverURL string) string {
	u := strings.TrimRight(serverURL, "/")
	switch {
	case strings.HasPrefix(u, "https://"):
		return "wss://" + strings.TrimPrefix(u, "https://")
	case strings.HasPrefix(u, "http://"):
		return "ws://" + strings.TrimPrefix(u, "http://")
	default:
		return u
	}
}
