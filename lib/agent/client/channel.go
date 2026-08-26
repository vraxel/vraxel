package client

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	// FactsInterval is how often the agent re-reads its inventory. An
	// hour because the things it describes -- sockets, disks, NICs, BIOS
	// -- change by human action, and an hour is already far tighter than
	// the reboot that most such changes require anyway.
	FactsInterval = 1 * time.Hour
	// ProcessesInterval is how often the agent re-reads its workload.
	// Tighter than facts because this is the one inventory that changes
	// without anybody touching the machine: a deploy replaces a service,
	// a container is rescheduled, an operator restarts something. Five
	// minutes costs a few thousand procfs reads and, on a machine where
	// nothing moved, sends nothing at all.
	ProcessesInterval = 5 * time.Minute
	// AccountsInterval matches facts: accounts change by human action,
	// and an hour is already tighter than the change itself.
	AccountsInterval = 1 * time.Hour
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
	// maxFactsListEntries caps each inventory list, for the same reason
	// maxHeartbeatProbes exists: an oversize frame is refused by
	// EncodeFrame, and a host with 500 LVM volumes would then never
	// report its inventory at all -- losing its CPU model and kernel
	// version, which fit easily, over a list nobody can read anyway.
	// Truncation is logged; silent capping would read as completeness.
	maxFactsListEntries = 128
	// inventoryBudget is how many bytes one report's lists may encode to,
	// leaving the rest of MaxFrameBytes for the envelope.
	//
	// A COUNT cap cannot do this job on its own, because an entry's size
	// has no bound. One account with forty authorized keys outweighs a
	// hundred service accounts, so a bastion blows the frame limit at an
	// entry count nowhere near 128 -- and an oversize frame is REFUSED by
	// EncodeFrame, which means that host reports no accounts at all
	// rather than most of them. Measured on an ordinary machine the whole
	// accounts report is 5.6 KB, so this bites only where it should.
	inventoryBudget = 48 * 1024
	// The accounts report carries three lists in ONE frame, so they share
	// the budget. Users get most of it: they are the largest entries and
	// the ones anybody reads.
	accountsUsersBudget  = 32 * 1024
	accountsGroupsBudget = 8 * 1024
	accountsRulesBudget  = 8 * 1024
	// The sshd summary rides the same frame. All of it is fixed-size
	// except the four access lists, and those are the only part a
	// configuration can make arbitrarily long -- AllowUsers takes as many
	// names as somebody types. Small on purpose: the three budgets above
	// already add up to inventoryBudget, so this comes out of the
	// envelope headroom, and an access list nobody can read on a screen
	// is not worth the whole report being refused.
	accountsSSHDListBudget = 1024
)

// bootNonce identifies this agent process for the lifetime of the
// process. Package level rather than per-Channel because that is exactly
// its meaning: one running agent, one value, resent unchanged on every
// reconnect so the server can tell a reconnect from a second process
// claiming the same identity (see agenttypes.Frame.BootNonce).
//
// Never persisted. A value on disk would be copied by the disk clone it
// exists to detect.
var bootNonce = newBootNonce()

func newBootNonce() string {
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		// crypto/rand failing is fatal for the process on every platform
		// this runs on; degrading to a predictable value would silently
		// turn clone detection off, which is worse than not starting.
		panic("agent: cannot read crypto/rand for the boot nonce: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

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

	// Fingerprint, if set, is sampled on every connect and sent with the
	// hello. Sampled rather than captured because the one identity change
	// we must notice -- an operator resetting /etc/machine-id to
	// de-clone a host -- happens between reconnects, and a value read once
	// at startup would keep reporting the pre-fix machine forever.
	Fingerprint func() agenttypes.MachineFingerprint

	// RunningJobs, if set, reports the jobs still executing so a reconnect
	// hello tells the server not to re-dispatch them (design §4.4.5).
	RunningJobs func() []int64

	// ProbeStates, if set, is sampled on every heartbeat so the server
	// gets a full probe digest periodically (design §5.6). Change events
	// travel separately via Send; the digest is what makes a lost change
	// event self-correct instead of leaving the server permanently wrong.
	ProbeStates func() []agenttypes.ProbeState

	// MetricsSummary, if set, is sampled on every heartbeat: the
	// current-utilisation snapshot the host list sorts and filters on.
	// Nil results are fine and expected -- the collector needs two
	// samples before it can say anything honest.
	MetricsSummary func() *agenttypes.MetricsSummary

	// Facts, if set, is the machine's inventory. Sampled once per session
	// and then hourly, and sent only when it differs from what this
	// session already sent -- see factsLoop.
	Facts func() agenttypes.HostFacts

	// Processes and Accounts are the two runtime inventories, on the same
	// send-only-when-changed contract as Facts and at their own cadences.
	// Nil disables the loop entirely, which is what a platform with no
	// collector for them gets.
	Processes func() agenttypes.HostProcesses
	Accounts  func() agenttypes.HostAccounts

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

// machineFingerprint samples the machine identity for this hello, or
// returns the zero value when the embedder supplied no source. The zero
// value is a valid answer: the server treats an absent fingerprint as
// "unverifiable" and admits the agent, which is what keeps a build
// without one from locking itself out.
func (c *Channel) machineFingerprint() agenttypes.MachineFingerprint {
	if c.Fingerprint == nil {
		return agenttypes.MachineFingerprint{}
	}
	return c.Fingerprint()
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
				"mint a join token in vraxel and re-run the install command on this host to recover")
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
		BootNonce:    bootNonce,
		Fingerprint:  c.machineFingerprint(),
	}); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}
	c.Log.Infof("control channel: connected to %s", c.ServerURL)

	// Published only now. Exposing the socket before hello let an
	// unsolicited Send from elsewhere in the agent -- a probe verdict
	// flipping at the wrong moment -- reach the server first, which reads
	// it as a protocol violation and drops the connection.
	live := SendFunc(send)
	c.live.Store(&live)
	defer c.live.Store(nil)

	// A failed heartbeat tears the session down instead of merely stopping
	// the beat: the read side of a half-open connection can stay blocked
	// for the kernel's whole TCP retry budget, so without this the agent
	// stops proving liveness but never reconnects either.
	go c.heartbeatLoop(sessCtx, send, cancel)
	// Its own goroutine, and a send failure here does NOT end the session:
	// inventory is the least urgent thing the channel carries, and a host
	// whose facts frame was dropped is a host with a stale hardware list,
	// not a host that has stopped working.
	go c.factsLoop(sessCtx, send)
	go c.processesLoop(sessCtx, send)
	go c.accountsLoop(sessCtx, send)

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
				Metrics:     c.metricsSummary(),
			}); err != nil {
				c.Log.Warnf("control channel: heartbeat failed (%v); reconnecting", err)
				endSession()
				return
			}
		}
	}
}

// factsLoop reports the machine's inventory: once as soon as the session
// is up, then hourly, and each time only if it changed. See
// reportOnChange for why the dedup works the way it does.
func (c *Channel) factsLoop(ctx context.Context, send SendFunc) {
	if c.Facts == nil {
		return
	}
	reportOnChange(ctx, c, send, FactsInterval, "host facts",
		func() agenttypes.HostFacts {
			f := c.Facts()
			// Generic, so a free function rather than a method: Go has no
			// generic methods.
			f.NICs = capFacts(f.NICs, c.Log, "nics")
			f.Filesystems = capFacts(f.Filesystems, c.Log, "filesystems")
			f.BlockDevices = capFacts(f.BlockDevices, c.Log, "block devices")
			return f
		},
		func(f agenttypes.HostFacts) agenttypes.Frame {
			return agenttypes.Frame{Type: agenttypes.FrameTypeHostFacts, ID: "facts-1", Facts: &f}
		})
}

// processesLoop reports the machine's workload, and accountsLoop who can
// use it. Same contract as factsLoop in every respect that matters: first
// frame per session unconditional, then only on change.
func (c *Channel) processesLoop(ctx context.Context, send SendFunc) {
	if c.Processes == nil {
		return
	}
	reportOnChange(ctx, c, send, ProcessesInterval, "host processes",
		func() agenttypes.HostProcesses {
			p := c.Processes()
			p.Groups = capBySize(capFacts(p.Groups, c.Log, "process groups"), inventoryBudget, c.Log, "process groups")
			return p
		},
		func(p agenttypes.HostProcesses) agenttypes.Frame {
			return agenttypes.Frame{Type: agenttypes.FrameTypeHostProcesses, ID: "processes-1", Processes: &p}
		})
}

func (c *Channel) accountsLoop(ctx context.Context, send SendFunc) {
	if c.Accounts == nil {
		return
	}
	reportOnChange(ctx, c, send, AccountsInterval, "host accounts",
		func() agenttypes.HostAccounts {
			a := c.Accounts()
			// Three lists, one frame, one shared budget -- see
			// inventoryBudget.
			a.Users = capBySize(capFacts(a.Users, c.Log, "accounts"), accountsUsersBudget, c.Log, "accounts")
			a.Groups = capBySize(capFacts(a.Groups, c.Log, "groups"), accountsGroupsBudget, c.Log, "groups")
			a.SudoRules = capBySize(capFacts(a.SudoRules, c.Log, "sudo rules"), accountsRulesBudget, c.Log, "sudo rules")
			capSSHDLists(a.SSHD, c.Log)
			return a
		},
		func(a agenttypes.HostAccounts) agenttypes.Frame {
			return agenttypes.Frame{Type: agenttypes.FrameTypeHostAccounts, ID: "accounts-1", Accounts: &a}
		})
}

// reportOnChange is the shared body of the three inventory loops: sample,
// and send only if the sample differs from the last one this session sent.
//
// The dedup state is local to the call, which makes it per-session, which
// makes the first frame of every session unconditional. That is the
// point: the agent cannot know what the server still holds -- the row may
// have been restored from a backup, or merged, or the write may have
// failed -- and one frame per reconnect is a rounding error against a
// channel that already beats every 15 seconds. Within a session, where
// the server's state IS known, nothing is resent.
//
// Compared as marshalled JSON rather than by a hash: these are a few KB,
// the comparison happens minutes apart, and a hash would add a way for two
// different inventories to look identical in exchange for nothing.
//
// A free function because Go has no generic methods.
func reportOnChange[T any](
	ctx context.Context, c *Channel, send SendFunc,
	every time.Duration, what string,
	sample func() T, build func(T) agenttypes.Frame,
) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	sent := ""
	for {
		v := sample()
		if encoded, err := json.Marshal(v); err != nil {
			c.Log.Warnf("control channel: encode %s: %v", what, err)
		} else if string(encoded) != sent {
			if err := send(build(v)); err != nil {
				c.Log.Warnf("control channel: send %s: %v", what, err)
			} else {
				sent = string(encoded)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// capFacts trims one inventory list to what a frame can carry, saying so
// when it has to. Logged on every resample rather than once: this is a
// standing property of the host, and an hourly line is the only place it
// is visible at all.
// capSSHDLists bounds the only part of the sshd summary a configuration
// can make arbitrarily long.
//
// Same reason as every other cap here: an oversize frame is REFUSED by
// EncodeFrame, so one host with a thousand names in AllowUsers would
// report no accounts at all -- losing the user list, the groups and the
// sudo rules over an access list nobody was going to read.
func capSSHDLists(c *agenttypes.SSHDConfig, log Logger) {
	if c == nil {
		return
	}
	c.AllowUsers = capBySize(c.AllowUsers, accountsSSHDListBudget, log, "sshd allowUsers")
	c.AllowGroups = capBySize(c.AllowGroups, accountsSSHDListBudget, log, "sshd allowGroups")
	c.DenyUsers = capBySize(c.DenyUsers, accountsSSHDListBudget, log, "sshd denyUsers")
	c.DenyGroups = capBySize(c.DenyGroups, accountsSSHDListBudget, log, "sshd denyGroups")
}

// capBySize trims a list until its encoded form fits a byte budget,
// after capFacts has already applied the count cap.
//
// Both caps exist because they catch different things: the count cap
// bounds a list of small entries (500 LVM volumes), and this bounds a
// short list of large ones (a jump host's accounts, each with several
// keys). Either one alone leaves a real machine unable to report.
//
// The next length is estimated from the ratio rather than stepped down,
// so this converges in an iteration or two over a list that may be
// thousands long.
func capBySize[T any](items []T, budget int, log Logger, what string) []T {
	full := len(items)
	for len(items) > 0 {
		b, err := json.Marshal(items)
		if err != nil {
			return items
		}
		if len(b) <= budget {
			break
		}
		next := len(items) * budget / len(b)
		if next >= len(items) {
			next = len(items) - 1
		}
		items = items[:next]
	}
	if len(items) < full {
		log.Warnf("host inventory: reporting %d of %d %s; the rest exceed the %d-byte frame budget",
			len(items), full, what, budget)
	}
	return items
}

func capFacts[T any](items []T, log Logger, what string) []T {
	if len(items) <= maxFactsListEntries {
		return items
	}
	log.Warnf("host inventory: reporting %d of %d %s; the rest exceed the control frame limit",
		maxFactsListEntries, len(items), what)
	return items[:maxFactsListEntries]
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
func (c *Channel) metricsSummary() *agenttypes.MetricsSummary {
	if c.MetricsSummary == nil {
		return nil
	}
	return c.MetricsSummary()
}

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
