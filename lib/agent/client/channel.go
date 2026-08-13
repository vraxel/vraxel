package client

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	ws "vraxel.io/vraxel/lib/websocket"
)

const (
	// HeartbeatInterval is the agent's heartbeat cadence. The server
	// declares an agent offline after 60s of silence, so four beats fit
	// inside the window.
	HeartbeatInterval = 15 * time.Second
	// reconnectMin / reconnectMax bound the exponential backoff between
	// reconnect attempts. Capped at 60s because a control channel is
	// cheap and an agent that stays disconnected is an unmanaged host --
	// the cost of retrying a dead server once a minute is negligible next
	// to the cost of a host taking ten minutes to come back after a
	// server restart.
	reconnectMin = 1 * time.Second
	reconnectMax = 60 * time.Second
	// dialTimeout bounds one connection attempt including the upgrade.
	dialTimeout = 30 * time.Second
	// writeTimeout bounds one frame write, so a half-open socket surfaces
	// within a beat or two instead of at the kernel's TCP timeout.
	writeTimeout = 10 * time.Second
	// maxHeartbeatProbes caps the probe digest a heartbeat carries.
	// Sized so the worst case (long names plus 256-byte failure
	// messages) stays well inside MaxFrameBytes.
	maxHeartbeatProbes = 128
)

// Logger is the minimal logging surface the channel needs, so this
// package stays free of a logging dependency.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
}

// Channel maintains the persistent control channel to the server.
type Channel struct {
	ServerURL string
	// AgentToken returns the durable credential, read fresh on every
	// dial. A func rather than a string because the agent renews it in
	// the background: a value captured at startup would keep being used
	// on every reconnect, and once the server rotates the old one the
	// agent locks itself out until someone restarts it.
	AgentToken func() string
	Version    string
	Log        Logger
	// HTTPClient performs the WebSocket upgrade, carrying the TLS trust
	// store. Nil uses the library default.
	HTTPClient *http.Client

	// OnFrame receives every server frame the channel does not handle
	// itself. send writes a frame back over the current connection
	// (serialized, targets whichever socket is live), which the job runner
	// uses to ack a dispatch.
	OnFrame func(ctx context.Context, f agenttypes.Frame, send SendFunc)

	// RunningJobs, if set, reports the jobs still executing so a reconnect
	// hello tells the server not to re-dispatch them (design §4.4.5).
	RunningJobs func() []int64

	// ProbeStates, if set, is sampled on every heartbeat so the server
	// gets a full probe digest periodically (design §5.6). Change events
	// travel separately via Send; the digest is what makes a lost change
	// event self-correct instead of leaving the server permanently wrong.
	ProbeStates func() []agenttypes.ProbeState

	// live holds the current session's writer, so callers outside the
	// frame loop (probe verdicts, pending_restart) can push a frame
	// without one being handed to them first.
	live atomic.Pointer[SendFunc]
}

// Send writes a frame on the live control channel. Returns an error when
// no session is up: an agent that cannot reach the server drops the
// frame rather than queueing it, because every frame this carries is a
// current-state report that the next heartbeat or reconnect resends
// anyway.
func (c *Channel) Send(f agenttypes.Frame) error {
	send := c.live.Load()
	if send == nil {
		return errors.New("control channel is not connected")
	}
	return (*send)(f)
}

// SendFunc writes a frame back over the current control-channel
// connection. Safe for concurrent use; targets the socket live at call
// time.
type SendFunc func(agenttypes.Frame) error

// Run dials the control channel and keeps it up until ctx is done,
// reconnecting with exponential backoff.
//
// Never returns an error: a disconnected agent is a transient state, not
// a fatal one. The process staying alive and retrying is what lets a host
// survive a server restart without operator action.
func (c *Channel) Run(ctx context.Context) {
	backoff := reconnectMin
	for {
		if ctx.Err() != nil {
			return
		}
		start := time.Now()
		err := c.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			c.Log.Warnf("control channel: %v", err)
		}
		// A session that stayed up a while proves the server is healthy,
		// so the next failure starts over at the short delay instead of
		// inheriting backoff from an outage that is already resolved.
		if time.Since(start) > reconnectMax {
			backoff = reconnectMin
		}
		// A rejected credential is not a transient fault: retrying it
		// every second changes nothing and buries both logs in noise. It
		// needs an operator (re-register with a fresh join token), so back
		// all the way off and keep the process alive to notice a fix.
		if errors.Is(err, ErrUnauthorized) {
			c.Log.Warnf("control channel: the server rejected this agent's token; " +
				"re-onboard the host with `install-agent.sh --force-register` to recover")
			backoff = reconnectMax
		}
		c.Log.Infof("control channel: reconnecting in %s", backoff)
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

// jittered spreads a backoff delay uniformly over [d/2, d] (equal
// jitter): still exponential in the worst case, but the retry instant is
// randomised. Without it a mass disconnect -- a
// server instance dying with thousands of agents pinned to it --
// would have every agent retry in lockstep and stampede the surviving
// instances. Spreading the wait desynchronises the reconnect storm, which
// is a hard requirement at the ten-thousand-agent scale this targets.
func jittered(d time.Duration) time.Duration {
	half := d / 2
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

// session runs one connection from dial to disconnect.
func (c *Channel) session(ctx context.Context) error {
	sessCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	conn, err := c.dial(sessCtx)
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(agenttypes.MaxFrameBytes)

	// Every write carries its own deadline. On a half-open connection
	// (NAT dropped the session, peer powered off) the socket buffer fills
	// and a deadline-free write blocks until the kernel gives up on TCP --
	// many minutes during which the agent believes it is connected while
	// the server has long since marked it offline.
	var writeMu sync.Mutex
	send := func(f agenttypes.Frame) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		wctx, cancelWrite := context.WithTimeout(sessCtx, writeTimeout)
		defer cancelWrite()
		return writeFrame(wctx, conn, f)
	}

	live := SendFunc(send)
	c.live.Store(&live)
	defer c.live.Store(nil)

	// hello first: the server refuses to register the session until it
	// arrives, because it carries the version and clock the host_agents
	// row must be online with, plus the jobs still running so a reconnect
	// is not re-dispatched.
	if err := send(agenttypes.Frame{
		Type:         agenttypes.FrameTypeHello,
		ID:           "hello-1",
		AgentVersion: c.Version,
		ClockUnixMs:  time.Now().UnixMilli(),
		RunningJobs:  c.runningJobs(),
	}); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}
	c.Log.Infof("control channel: connected to %s", c.ServerURL)

	// A failed heartbeat tears the session down instead of merely stopping
	// the beat: the read side of a half-open connection can stay blocked
	// for the kernel's whole TCP retry budget, so without this the agent
	// stops proving liveness but never reconnects either.
	go c.heartbeatLoop(sessCtx, send, cancel)

	for {
		_, data, err := conn.ReadMessage(sessCtx)
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		f, err := agenttypes.DecodeFrame(data)
		if err != nil {
			c.Log.Warnf("control channel: undecodable frame: %v", err)
			continue
		}
		c.handle(sessCtx, f, send)
	}
}

func (c *Channel) dial(ctx context.Context) (*ws.Conn, error) {
	dctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	url := wsURL(c.ServerURL) + agenttypes.ProtocolPathPrefix + "channel"
	// resp.Body is intentionally not closed: on a WebSocket handshake the
	// library takes the body over on success and returns a NopCloser on
	// failure, so "you never need to close resp.Body yourself". resp is
	// read only for StatusCode in the error branch. (IDE leak warnings
	// here are false positives.)
	conn, resp, err := ws.Dial(dctx, url, &ws.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + c.AgentToken()}},
		HTTPClient: c.HTTPClient,
	})
	if err != nil {
		if resp != nil {
			if resp.StatusCode == http.StatusUnauthorized {
				return nil, fmt.Errorf("dial %s: %w", url, ErrUnauthorized)
			}
			return nil, fmt.Errorf("dial %s: %w (http %d)", url, err, resp.StatusCode)
		}
		return nil, fmt.Errorf("dial %s: %w", url, err)
	}
	return conn, nil
}

// ErrUnauthorized reports that the server refused this agent's token
// (revoked by a re-registration, or the host row was deleted). Distinct
// from a transport failure because no amount of retrying fixes it.
var ErrUnauthorized = errors.New("agent token rejected by the server")

func (c *Channel) heartbeatLoop(ctx context.Context, send SendFunc, endSession context.CancelFunc) {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()
	seq := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			seq++
			if err := send(agenttypes.Frame{
				Type:        agenttypes.FrameTypeHeartbeat,
				ID:          fmt.Sprintf("hb-%d", seq),
				ClockUnixMs: time.Now().UnixMilli(),
				ProbeStates: c.probeStates(),
			}); err != nil {
				c.Log.Warnf("control channel: heartbeat failed (%v); reconnecting", err)
				endSession()
				return
			}
		}
	}
}

func (c *Channel) handle(ctx context.Context, f agenttypes.Frame, send SendFunc) {
	if c.OnFrame != nil {
		c.OnFrame(ctx, f, send)
	}
}

// probeStates samples the probe digest for a heartbeat, or nil when no
// probe runner is wired.
//
// The digest is capped. EncodeFrame rejects anything over 64 KiB and the
// heartbeat loop treats a send failure as "tear the session down", so a
// host with enough probes would fail every heartbeat and reconnect
// forever -- losing the whole host's manageability to make room for
// metrics that are, by design, second class. Unhealthy probes are kept
// first: they are what an operator is looking for, and the server
// reconciles the rest from the change events.
func (c *Channel) probeStates() []agenttypes.ProbeState {
	if c.ProbeStates == nil {
		return nil
	}
	states := c.ProbeStates()
	if len(states) <= maxHeartbeatProbes {
		return states
	}
	out := make([]agenttypes.ProbeState, 0, maxHeartbeatProbes)
	for _, s := range states {
		if !s.Healthy && len(out) < maxHeartbeatProbes {
			out = append(out, s)
		}
	}
	for _, s := range states {
		if s.Healthy && len(out) < maxHeartbeatProbes {
			out = append(out, s)
		}
	}
	c.Log.Warnf("control channel: %d probes exceed the heartbeat digest cap; reporting %d",
		len(states), len(out))
	return out
}

// runningJobs reports the in-flight job ids for the reconnect hello, or
// nil when no job runner is wired.
func (c *Channel) runningJobs() []int64 {
	if c.RunningJobs == nil {
		return nil
	}
	return c.RunningJobs()
}

// writeFrame encodes and sends one frame over the control channel. The
// encode + 64 KiB check is shared with the server via
// agenttypes.EncodeFrame; only the transport write is client-specific.
func writeFrame(ctx context.Context, conn *ws.Conn, f agenttypes.Frame) error {
	data, err := agenttypes.EncodeFrame(f)
	if err != nil {
		return err
	}
	return conn.WriteText(ctx, data)
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
