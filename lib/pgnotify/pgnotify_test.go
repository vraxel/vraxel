package pgnotify

import (
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// muxStub gives tests access to the dispatch path without dialing PG.
// Production code never instantiates Multiplexer this way; we only
// inject handlers via the unexported register() to exercise the
// generic typed-handler wrapping path.
func muxStub() *Multiplexer {
	return &Multiplexer{
		handlers: make(map[string][]rawHandler),
	}
}

func TestNewChannel_PanicsOnEmpty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("NewChannel(\"\") did not panic")
		}
	}()
	NewChannel[int]("")
}

func TestNewChannel_PanicsOnInvalidName(t *testing.T) {
	cases := []string{
		"Foo",      // uppercase
		"1foo",     // leading digit
		"foo-bar",  // hyphen
		"foo bar",  // space
		"foo;DROP", // SQL meta
		"_foo",     // leading underscore
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("NewChannel(%q) did not panic", name)
				}
				if !strings.Contains(r.(string), "match") {
					t.Errorf("panic message %q does not mention regex constraint", r)
				}
			}()
			NewChannel[int](name)
		})
	}
}

func TestNewChannel_AcceptsValidName(t *testing.T) {
	for _, name := range []string{"a", "foo", "foo_bar", "module_resource_event", "x1"} {
		c := NewChannel[int](name)
		if c.Name != name {
			t.Errorf("Name = %q, want %q", c.Name, name)
		}
	}
}

func TestSubscribe_PanicsAfterStart(t *testing.T) {
	mux := muxStub()
	mux.started = true
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("Subscribe after Start did not panic")
		}
	}()
	NewChannel[int]("foo").Subscribe(mux, func(int) {})
}

func TestSubscribe_MultipleHandlersFanOut(t *testing.T) {
	// Two unrelated subsystems may subscribe to the same channel
	// (e.g. live-refresh hub + liveness poller both react to
	// app_services_event). Both must receive every dispatch in
	// registration order.
	mux := muxStub()
	ch := NewChannel[int]("foo")
	var order []int
	var mu sync.Mutex
	ch.Subscribe(mux, func(v int) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, 10+v)
	})
	ch.Subscribe(mux, func(v int) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, 20+v)
	})

	mux.dispatch("foo", []byte(`1`))

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != 11 || order[1] != 21 {
		t.Fatalf("got %v, want [11 21]", order)
	}
}

func TestSubscribe_PanicsOnNilHandler(t *testing.T) {
	mux := muxStub()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("Subscribe(nil) did not panic")
		}
	}()
	NewChannel[int]("foo").Subscribe(mux, nil)
}

func TestStart_PanicsOnSecondCall(t *testing.T) {
	mux := muxStub()
	mux.started = true // simulate already started without dialing PG
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("second Start did not panic")
		}
	}()
	mux.Start(nil)
}

func TestDispatch_TypedHandlerReceivesUnmarshalledPayload(t *testing.T) {
	mux := muxStub()
	type Payload struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	got := make(chan Payload, 1)
	NewChannel[Payload]("foo").Subscribe(mux, func(p Payload) { got <- p })

	mux.dispatch("foo", []byte(`{"id":42,"name":"hello"}`))
	select {
	case p := <-got:
		if p.ID != 42 || p.Name != "hello" {
			t.Errorf("payload = %+v, want {42 hello}", p)
		}
	case <-time.After(time.Second):
		t.Fatal("handler not invoked")
	}
}

func TestDispatch_Int64ChannelMatchesStrconvBytes(t *testing.T) {
	// Byte-compat invariant: the iam migration relies on JSON's int64
	// encoding being byte-identical to strconv.FormatInt. Locking that
	// in here so a future code change cannot silently break the
	// rolling-upgrade contract.
	mux := muxStub()
	want := []int64{123, -456, 0, 9223372036854775807}
	// Buffer sized for all dispatches so the handler never blocks; the
	// dispatcher in production is also non-blocking by contract.
	got := make(chan int64, len(want))
	NewChannel[int64]("foo").Subscribe(mux, func(v int64) { got <- v })

	for _, raw := range []string{"123", "-456", "0", "9223372036854775807"} {
		mux.dispatch("foo", []byte(raw))
	}
	for _, w := range want {
		select {
		case v := <-got:
			if v != w {
				t.Errorf("got %d, want %d", v, w)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing dispatch for %d", w)
		}
	}
}

func TestDispatch_BadJSONDoesNotInvokeHandler(t *testing.T) {
	mux := muxStub()
	type Payload struct {
		ID int64 `json:"id"`
	}
	called := atomic.Bool{}
	NewChannel[Payload]("foo").Subscribe(mux, func(Payload) { called.Store(true) })

	mux.dispatch("foo", []byte(`not json{{`))
	if called.Load() {
		t.Errorf("handler invoked on invalid JSON; want skip")
	}
}

func TestDispatch_UnknownChannelIgnored(t *testing.T) {
	mux := muxStub()
	called := atomic.Bool{}
	NewChannel[int]("foo").Subscribe(mux, func(int) { called.Store(true) })

	mux.dispatch("bar", []byte(`1`))
	if called.Load() {
		t.Errorf("handler invoked on unrelated channel")
	}
}

func TestDispatch_HandlerPanicDoesNotPropagate(t *testing.T) {
	// One bad handler must not take down the dispatcher; subsequent
	// notifications on the same or other channels keep working.
	mux := muxStub()
	NewChannel[int]("bad").Subscribe(mux, func(int) {
		panic("oops")
	})
	good := atomic.Int32{}
	NewChannel[int]("good").Subscribe(mux, func(int) {
		good.Add(1)
	})

	// Repeated bad-channel dispatch must not crash; good still works.
	for i := 0; i < 10; i++ {
		mux.dispatch("bad", []byte(`1`))
	}
	for i := 0; i < 5; i++ {
		mux.dispatch("good", []byte(`1`))
	}
	if good.Load() != 5 {
		t.Errorf("good count = %d, want 5 (bad handler likely propagated)", good.Load())
	}
}

func TestDispatch_PanicInOneFanOutDoesNotSkipNext(t *testing.T) {
	// On a channel with multiple handlers, a panic in handler N must
	// not prevent handler N+1 from running.
	mux := muxStub()
	ch := NewChannel[int]("foo")
	after := atomic.Bool{}
	ch.Subscribe(mux, func(int) { panic("boom") })
	ch.Subscribe(mux, func(int) { after.Store(true) })

	mux.dispatch("foo", []byte(`1`))
	if !after.Load() {
		t.Fatal("second handler not invoked after first panicked")
	}
}

func TestDispatch_RaceWithLateRegister(t *testing.T) {
	// Confirms that even though register grabs mu, dispatch reads the
	// map under the same lock, so concurrent register-then-dispatch is
	// race-safe (would be flagged by -race otherwise).
	mux := muxStub()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := NewChannel[int]("ch_" + string(rune('a'+i)))
			ch.Subscribe(mux, func(int) {})
			mux.dispatch(ch.Name, []byte(`1`))
		}()
	}
	wg.Wait()
}

func TestNew_PanicsOnNilPool(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("New(nil) did not panic")
		}
	}()
	_ = New(nil)
}

// TestSubscribeAsync_AcceptFilterAndSecondProducer verifies the async
// drain: accept runs on the dispatch side (rejected events never reach the
// handler), accepted dispatch events AND direct Submit calls both feed the
// same drain goroutine and are handled.
func TestSubscribeAsync_AcceptFilterAndSecondProducer(t *testing.T) {
	mux := muxStub()
	ch := NewChannel[int]("async_bar")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan int, 8)
	q := ch.SubscribeAsync(mux, ctx,
		func(v int) bool { return v%2 == 0 }, // dispatch-side: drop odds
		func(v int) { got <- v })

	mux.dispatch("async_bar", []byte(`2`)) // accepted -> drained -> handled
	mux.dispatch("async_bar", []byte(`3`)) // rejected by accept, never handled
	if !q.Submit(4) {                      // second producer, same drain
		t.Fatal("Submit(4) dropped unexpectedly")
	}

	seen := []int{<-got, <-got}
	sort.Ints(seen)
	if seen[0] != 2 || seen[1] != 4 {
		t.Fatalf("handled %v, want [2 4] (odd 3 filtered on the dispatch side)", seen)
	}
	select {
	case v := <-got:
		t.Fatalf("unexpected extra handled value %d", v)
	default:
	}
}

// TestSubscribeAsync_DropsWhenFull verifies drop-on-full: with the drain
// wedged, Submit stops accepting once the bounded queue fills instead of
// blocking (which on the real dispatch goroutine would stall the NOTIFY bus).
func TestSubscribeAsync_DropsWhenFull(t *testing.T) {
	mux := muxStub()
	ch := NewChannel[int]("async_full")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	release := make(chan struct{})
	q := ch.SubscribeAsync(mux, ctx, nil, func(int) { <-release }) // wedge the drain

	accepted := 0
	total := asyncQueueSize + 8
	for i := 0; i < total; i++ {
		if q.Submit(i) {
			accepted++
		}
	}
	if accepted >= total {
		t.Fatalf("accepted all %d submits; expected drop-on-full once the queue filled", total)
	}
	if accepted < asyncQueueSize {
		t.Fatalf("accepted only %d, want at least the %d-deep buffer", accepted, asyncQueueSize)
	}
	close(release)
}

// TestPublishVia_NilPublisherNoop: a nil Publisher must be a no-op (no panic,
// no pool deref), mirroring the single-instance fallback where shared
// infrastructure was wired without a coordination pool.
func TestPublishVia_NilPublisherNoop(t *testing.T) {
	ch := NewChannel[string]("pv_noop")
	ch.PublishVia(context.Background(), nil, "x")
}
