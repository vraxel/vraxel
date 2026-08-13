package scrape

import (
	"context"
	"testing"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// newRecordTestLoop builds a loop registered against s for url, without
// starting its goroutine: these tests exercise record's bookkeeping, not
// the scrape itself.
func newRecordTestLoop(s *Scraper, url string) *targetLoop {
	return newTargetLoop(context.Background(), s, agenttypes.ScrapeTarget{URL: url}, time.Second)
}

// TestRecordDropsAResultFromARemovedTarget pins the reason a removed
// target used to keep reporting forever. refresh cancels the loop and
// deletes its results entry under s.mu, but cancelling does not join: a
// round already in flight blocks in record on that same mutex and writes
// back afterwards. Nothing would ever remove the entry again -- later
// refreshes reconcile against s.targets, which no longer knows the URL --
// so the synthetic push emitted up=0 for a target that no longer exists,
// until the agent process restarted.
func TestRecordDropsAResultFromARemovedTarget(t *testing.T) {
	s := New(Config{Log: testLogger{t}})
	const url = "http://127.0.0.1:9100/metrics"

	loop := newRecordTestLoop(s, url)
	s.targets[url] = loop
	s.record(loop, result{up: true})
	if len(s.results) != 1 {
		t.Fatalf("a registered loop must be recorded; results = %d", len(s.results))
	}

	// What refresh does when the server stops listing this target.
	loop.stop()
	delete(s.targets, url)
	delete(s.results, url)

	// The cancelled round finally reaches record.
	s.record(loop, result{up: false})

	if len(s.results) != 0 {
		t.Fatalf("a removed target's late round was recorded anyway: %+v", s.results)
	}
}

// TestRecordDropsAResultFromASupersededLoop covers the same race for a
// target that is REPLACED rather than removed: a changed interval or
// label set tears the loop down and rebuilds it for the same URL in one
// pass. Keying the guard on the URL alone would let the outgoing loop's
// cancelled round land on its successor's entry, flagging a healthy
// target down until the next round overwrote it.
func TestRecordDropsAResultFromASupersededLoop(t *testing.T) {
	s := New(Config{Log: testLogger{t}})
	const url = "http://127.0.0.1:9100/metrics"

	old := newRecordTestLoop(s, url)
	s.targets[url] = old

	// refresh replaces the loop, same URL.
	old.stop()
	delete(s.targets, url)
	delete(s.results, url)
	fresh := newRecordTestLoop(s, url)
	s.targets[url] = fresh
	s.record(fresh, result{up: true})

	// The outgoing loop's cancelled round arrives late.
	s.record(old, result{up: false})

	if got := s.results[url]; !got.up {
		t.Fatal("a superseded loop overwrote its successor's result with up=0")
	}
}
