// Package pgnotify multiplexes Postgres LISTEN/NOTIFY across many
// channels onto a single dedicated connection. Modules declare a
// typed Channel[T] (name + payload type) and call Subscribe at install
// time; one goroutine LISTENs on all channels and dispatches each
// notification to its handler with the payload already JSON-decoded.
//
// # Why
//
// WaitForNotification holds its connection for the listener's lifetime.
// Per-channel listeners scale linearly with feature count: each new
// LISTEN-driven feature adds one raw PG connection per replica. CICD's
// upcoming run / step / log / runner channels would multiply that by
// 5x. Single connection multiplexing keeps it at 1 regardless of
// channel count.
//
// # Lifecycle
//
//  1. apis/install.go creates one Multiplexer at process start.
//  2. Each module's NewModule defines `var XxxChannel = pgnotify.NewChannel[Payload]("name")`
//     and calls `XxxChannel.Subscribe(mux, handler)`.
//  3. apis/install.go calls `mux.Start(ctx)` once after all modules are wired.
//     Start opens a dedicated PG connection (NOT from the shared pgxpool),
//     issues `LISTEN c` for every registered channel, and dispatches in a
//     goroutine.
//  4. ctx cancellation closes the connection and returns the goroutine cleanly.
//  5. On connection error mid-flight (network blip, PG restart) the goroutine
//     reconnects after listenBackoff and re-LISTENs every channel. Notifications
//     fired during the gap are LOST -- consistent with single-listener implementations
//     and acceptable because every NOTIFY consumer treats it as a "refetch trigger",
//     not a durable event log.
//
// # Adding a new channel (5 lines)
//
//	type FooEventPayload struct{ ID int64 `json:"id"` }
//	var FooChannel = pgnotify.NewChannel[FooEventPayload]("module_foo_event")
//
//	// install.go:
//	store.FooChannel.Subscribe(mux, func(p store.FooEventPayload) { hub.Publish(...) })
//	// publish site:
//	store.FooChannel.Publish(ctx, db.GetPool(), FooEventPayload{...})
//
// # Handler contract
//
// Handlers run synchronously on the dispatch goroutine. They MUST be
// non-blocking -- typical body is `hub.Publish(...)` to an in-memory
// statushub or a similar fire-and-forget. Long-running work in a
// handler delays every other channel's dispatch.
//
// Handler panics are recovered so one bad handler does not take down
// the dispatcher; the panic is logged and the next notification proceeds.
package pgnotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vraxel.io/vraxel/lib/logger"
)

// listenBackoff is the wait between reconnect attempts after a
// connection failure. 1s matches the pre-multiplexer per-listener
// backoff; the multiplexer just centralises it.
const listenBackoff = time.Second

// rawHandler is the internal handler shape (raw bytes). Channel[T].Subscribe
// wraps a typed handler into one of these via a closure that decodes JSON.
type rawHandler func(payload []byte)

// Multiplexer is the per-process LISTEN multiplexer. Created once,
// shared by every module that needs cross-instance NOTIFY fan-out.
//
// A single channel may have multiple subscribers; dispatch invokes each
// in registration order on the same dispatch goroutine. This lets
// unrelated subsystems react to the same event (e.g. service mutation
// triggers both the live-refresh hub and the liveness poller) without
// either being aware of the other.
type Multiplexer struct {
	pool *pgxpool.Pool

	mu       sync.Mutex
	handlers map[string][]rawHandler
	started  bool
}

// New constructs a Multiplexer. The pool is used only as a source of
// the ConnConfig for dialling the dedicated LISTEN connection -- no
// pool slot is permanently consumed.
func New(pool *pgxpool.Pool) *Multiplexer {
	if pool == nil {
		panic("pgnotify: pool is required")
	}
	return &Multiplexer{
		pool:     pool,
		handlers: make(map[string][]rawHandler),
	}
}

// register appends a raw handler. Internal: called via Channel[T].Subscribe.
// Calling after Start panics so misuse surfaces at boot rather than as
// silently-dropped notifications.
func (m *Multiplexer) register(channel string, h rawHandler) {
	if channel == "" {
		panic("pgnotify: channel must be non-empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		panic("pgnotify: Subscribe called after Start (channel=" + channel + ")")
	}
	m.handlers[channel] = append(m.handlers[channel], h)
}

// Start opens a dedicated PG connection, issues LISTEN for every
// subscribed channel, and runs the dispatch goroutine. Returns
// immediately. The goroutine runs until ctx is cancelled (server
// shutdown).
//
// On connection error the goroutine waits listenBackoff then dials
// again and re-LISTENs every channel. ctx-cancel exits cleanly without
// reconnect.
//
// Calling Start more than once panics.
func (m *Multiplexer) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		panic("pgnotify: Start called twice")
	}
	m.started = true
	channels := make([]string, 0, len(m.handlers))
	for ch := range m.handlers {
		channels = append(channels, ch)
	}
	m.mu.Unlock()

	if len(channels) == 0 {
		// No subscribers -- skip the goroutine entirely so we don't
		// dial PG just to idle.
		logger.Infof("pgnotify multiplexer: no channels subscribed, idle")
		return
	}

	logger.Infof("pgnotify multiplexer: starting (%d channels)", len(channels))
	go m.run(ctx, channels)
}

// run is the dispatch goroutine. Reconnects on connection error;
// returns cleanly on ctx cancellation.
func (m *Multiplexer) run(ctx context.Context, channels []string) {
	defer logger.Infof("pgnotify multiplexer: stopped")
	for {
		if ctx.Err() != nil {
			return
		}
		err := m.runOnce(ctx, channels)
		if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		logger.Warnf("pgnotify multiplexer: %v (retry in %s)", err, listenBackoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(listenBackoff):
		}
	}
}

// runOnce dials, LISTENs every channel, dispatches notifications until
// ctx-cancel or connection error.
func (m *Multiplexer) runOnce(ctx context.Context, channels []string) error {
	connCfg := m.pool.Config().ConnConfig.Copy()
	conn, err := pgx.ConnectConfig(ctx, connCfg)
	if err != nil {
		return fmt.Errorf("dial dedicated listen conn: %w", err)
	}
	defer conn.Close(context.Background())

	for _, ch := range channels {
		if _, err := conn.Exec(ctx, "LISTEN "+ch); err != nil { // lint:allow-raw-sql
			return fmt.Errorf("LISTEN %s: %w", ch, err)
		}
	}
	logger.Infof("pgnotify multiplexer: subscribed to %d channels", len(channels))

	for {
		notif, err := conn.WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("wait notification: %w", err)
		}
		m.dispatch(notif.Channel, []byte(notif.Payload))
	}
}

// dispatch fans one notification out to every registered handler.
// Each handler runs under its own recover so one bad handler does not
// skip the rest (or take down the dispatcher).
func (m *Multiplexer) dispatch(channel string, payload []byte) {
	m.mu.Lock()
	handlers := m.handlers[channel]
	m.mu.Unlock()
	for _, h := range handlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("pgnotify: handler panic on channel %q: %v", channel, r)
				}
			}()
			h(payload)
		}()
	}
}

// Channel is the typed handle for a NOTIFY channel. Defined once per
// channel (typically as a package-level var in the module's store/)
// and used at both publish and subscribe sites for compile-time
// payload-type safety.
//
// Channel name is the literal Postgres LISTEN/NOTIFY identifier and
// MUST follow the convention "<module>_<resource>_event" so multiple
// Vraxel modules don't accidentally share names (e.g. "app_services_event",
// "cicd_run_event", "iam_rbac_invalidate").
type Channel[T any] struct {
	Name string
}

// channelNameRE constrains channel names to safe PG identifiers:
// lowercase letter start, then lowercase letters / digits / underscore.
// We concatenate the name into "LISTEN <name>" without quoting, so
// allowing arbitrary characters would either silently mis-route (PG
// folds unquoted identifiers to lowercase) or break the SQL altogether
// on hyphens / digits as first char.
var channelNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// NewChannel constructs a Channel[T]. Panics if name is empty or does
// not match the safe-identifier regex; both are programming errors
// that should fail fast at boot rather than at first NOTIFY.
func NewChannel[T any](name string) Channel[T] {
	if name == "" {
		panic("pgnotify.NewChannel: name must be non-empty")
	}
	if !channelNameRE.MatchString(name) {
		panic("pgnotify.NewChannel: name must match ^[a-z][a-z0-9_]*$, got " + name)
	}
	return Channel[T]{Name: name}
}

// Publish marshals payload as JSON and fires pg_notify. Errors are
// logged at warn -- pg_notify is best-effort fan-out, persistence has
// already happened upstream. Returns nothing because callers cannot
// usefully recover from a NOTIFY failure (the row is already saved).
//
// pool is supplied at the call site rather than carried by Channel
// to avoid forcing every package to inject a pgnotify.Multiplexer
// just for publishing -- modules already have *pgxpool.Pool via
// *db.DB.GetPool().
func (c Channel[T]) Publish(ctx context.Context, pool *pgxpool.Pool, payload T) {
	raw, err := json.Marshal(payload)
	if err != nil {
		logger.Warnf("pgnotify: %s marshal: %v", c.Name, err)
		return
	}
	if _, err := pool.Exec(ctx, "SELECT pg_notify($1, $2)", c.Name, string(raw)); err != nil { // lint:allow-raw-sql
		logger.Warnf("pgnotify: %s notify: %v", c.Name, err)
	}
}

// Publisher carries the pool a Channel needs to Publish, for callers that
// own no *db.DB of their own -- notably pkg/apis/shared infrastructure that
// coordinates across instances (task cancel, ws sessions) but has no store
// sub-layer and no tables. Business modules keep calling
// Channel.Publish(ctx, d.GetPool(), ...) directly; this handle is the
// equivalent for a package that only holds the pgnotify layer. Keeping the
// *pgxpool.Pool sealed inside lib/pgnotify lets those shared packages stay
// free of any pgx import (see pkg/apis layer rules).
type Publisher struct{ pool *pgxpool.Pool }

// NewPublisher wraps a pool as a publish handle. Wired once at install time.
func NewPublisher(pool *pgxpool.Pool) *Publisher { return &Publisher{pool: pool} }

// PublishVia is Publish with the pool taken from a Publisher handle. A nil
// Publisher is a no-op, mirroring the single-instance fallback where no
// coordination pool was wired.
func (c Channel[T]) PublishVia(ctx context.Context, p *Publisher, payload T) {
	if p == nil {
		return
	}
	c.Publish(ctx, p.pool, payload)
}

// Subscribe appends handler on the multiplexer for this channel.
// Must be called BEFORE mux.Start. Multiple handlers per channel are
// allowed and dispatched in registration order. Invalid JSON in a
// notification is logged and dropped without invoking the handler (a
// publisher with stale code shipping a bad payload should not crash
// the listener).
//
// handler runs synchronously on the dispatcher goroutine; see package
// doc for the non-blocking handler contract.
func (c Channel[T]) Subscribe(mux *Multiplexer, handler func(T)) {
	if handler == nil {
		panic("pgnotify: Subscribe handler must be non-nil (channel=" + c.Name + ")")
	}
	mux.register(c.Name, func(payload []byte) {
		var p T
		if err := json.Unmarshal(payload, &p); err != nil {
			logger.Warnf("pgnotify: %s invalid payload: %v", c.Name, err)
			return
		}
		handler(p)
	})
}

// asyncQueueSize is the per-SubscribeAsync buffer. 256 is deep enough that
// it only fills if the drain goroutine wedges (a hung DB), in which case
// dropping is the correct safety valve for a process-wide NOTIFY bus.
const asyncQueueSize = 256

// AsyncQueue is the handle SubscribeAsync returns. Its Submit lets a second
// producer (e.g. a startup reconcile scan) feed the same serialized drain
// goroutine the subscription feeds, so all work for the channel runs FIFO
// on one goroutine regardless of source.
type AsyncQueue[T any] struct {
	ch   chan T
	name string
}

// Submit enqueues v without blocking. Returns false and logs if the queue
// is full (drop-on-full). Safe to call from the pgnotify dispatch goroutine.
func (q *AsyncQueue[T]) Submit(v T) bool {
	select {
	case q.ch <- v:
		return true
	default:
		logger.Warnf("pgnotify: %s async queue full, dropping event", q.name)
		return false
	}
}

// SubmitBlocking enqueues v, blocking until space frees or ctx is done.
// For producers that are NOT on the dispatch goroutine (a startup scan)
// and must not drop -- never call this from a pgnotify handler.
func (q *AsyncQueue[T]) SubmitBlocking(ctx context.Context, v T) error {
	select {
	case q.ch <- v:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SubscribeAsync is Subscribe for handlers that must BLOCK (a DB read, a
// re-NOTIFY, an SSH dial). Instead of running handler inline on the shared
// dispatch goroutine -- where any block stalls every other channel -- it
// enqueues onto a bounded queue drained by one dedicated goroutine, so the
// dispatch goroutine stays free and same-channel events stay FIFO.
//
// accept is an OPTIONAL predicate run on the dispatch goroutine BEFORE
// enqueuing (it must be non-blocking, same contract as Subscribe): return
// false to drop the event cheaply without queuing it -- for self-echo
// filters ("ignore my own writes") and has-subscriber gates that should
// not consume queue space. nil enqueues everything.
//
// handler runs on the drain goroutine and MAY block. On a full queue the
// event is dropped and logged (best-effort NOTIFY semantics). The drain
// stops when ctx is cancelled. The returned handle's Submit/SubmitBlocking
// let other producers feed the same drain.
func (c Channel[T]) SubscribeAsync(mux *Multiplexer, ctx context.Context, accept func(T) bool, handler func(T)) *AsyncQueue[T] {
	if handler == nil {
		panic("pgnotify: SubscribeAsync handler must be non-nil (channel=" + c.Name + ")")
	}
	q := &AsyncQueue[T]{ch: make(chan T, asyncQueueSize), name: c.Name}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case v := <-q.ch:
				handler(v)
			}
		}
	}()
	c.Subscribe(mux, func(v T) {
		if accept != nil && !accept(v) {
			return
		}
		q.Submit(v)
	})
	return q
}
