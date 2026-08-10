package audit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockSink records batch calls for testing.
type mockSink struct {
	mu     sync.Mutex
	events []Event
	calls  int
}

func (m *mockSink) BatchCreate(_ context.Context, events []Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.events = append(m.events, events...)
	return nil
}

func (m *mockSink) totalEvents() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *mockSink) totalCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestWriter_BatchFlush(t *testing.T) {
	sink := &mockSink{}
	w := NewWriter(sink, WriterConfig{
		ChannelSize:   1000,
		BatchSize:     5,
		FlushInterval: 10 * time.Second, // long interval so only batch size triggers
	})

	w.Start(context.Background())

	// Send exactly BatchSize events to trigger a flush
	for i := range 5 {
		w.Log(Event{Action: "create", Detail: JSONString(string(rune('A' + i)))})
	}

	// Wait for flush to happen
	time.Sleep(100 * time.Millisecond)

	if got := sink.totalEvents(); got != 5 {
		t.Errorf("expected 5 events, got %d", got)
	}

	w.Stop()
}

func TestWriter_TimerFlush(t *testing.T) {
	sink := &mockSink{}
	w := NewWriter(sink, WriterConfig{
		ChannelSize:   1000,
		BatchSize:     100, // large batch so timer triggers first
		FlushInterval: 50 * time.Millisecond,
	})

	w.Start(context.Background())

	w.Log(Event{Action: "login"})
	w.Log(Event{Action: "create"})

	// Wait for timer flush
	time.Sleep(200 * time.Millisecond)

	if got := sink.totalEvents(); got != 2 {
		t.Errorf("expected 2 events, got %d", got)
	}

	w.Stop()
}

func TestWriter_GracefulDrain(t *testing.T) {
	sink := &mockSink{}
	w := NewWriter(sink, WriterConfig{
		ChannelSize:   1000,
		BatchSize:     1000,             // large batch
		FlushInterval: 10 * time.Second, // long interval
	})

	w.Start(context.Background())

	// Enqueue events
	for range 50 {
		w.Log(Event{Action: "update"})
	}

	// Stop should drain all events
	w.Stop()

	if got := sink.totalEvents(); got != 50 {
		t.Errorf("expected 50 events after drain, got %d", got)
	}
}

func TestWriter_ChannelFullDrop(t *testing.T) {
	sink := &mockSink{}
	w := NewWriter(sink, WriterConfig{
		ChannelSize:   5,
		BatchSize:     1000,
		FlushInterval: 10 * time.Second,
	})
	// Don't start — channel will fill up

	var dropped atomic.Int32
	for range 10 {
		select {
		case w.ch <- Event{Action: "test"}:
		default:
			dropped.Add(1)
		}
	}

	if dropped.Load() != 5 {
		t.Errorf("expected 5 drops, got %d", dropped.Load())
	}
}
