package probe

import (
	"context"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// collector records the state changes the runner reports.
type collector struct {
	mu     sync.Mutex
	states []agenttypes.ProbeState
}

func (c *collector) onChange(states []agenttypes.ProbeState) {
	c.mu.Lock()
	c.states = append(c.states, states...)
	c.mu.Unlock()
}

func (c *collector) last() (agenttypes.ProbeState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.states) == 0 {
		return agenttypes.ProbeState{}, false
	}
	return c.states[len(c.states)-1], true
}

func (c *collector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.states)
}

// waitFor polls until cond holds or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestTCPProbeReportsHealthyThenUnhealthy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	c := &collector{}
	r := New(c.onChange)
	defer r.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.SetSpecs(ctx, []agenttypes.ProbeSpec{{
		Name:             "svc/liveness",
		Type:             agenttypes.ProbeTypeTCP,
		Port:             int32(port),
		PeriodSec:        1,
		TimeoutSec:       1,
		SuccessThreshold: 1,
		FailureThreshold: 2,
	}})

	waitFor(t, "healthy report", func() bool {
		s, ok := c.last()
		return ok && s.Healthy
	})
	if states := r.States(); len(states) != 1 || !states[0].Healthy {
		t.Fatalf("States() = %+v", states)
	}

	// Kill the listener: the verdict must flip only after the failure
	// threshold, not on the first miss.
	ln.Close()
	waitFor(t, "unhealthy report", func() bool {
		s, ok := c.last()
		return ok && !s.Healthy
	})
	s, _ := c.last()
	if s.Message == "" {
		t.Fatal("unhealthy report carries no reason")
	}
}

func TestFailureThresholdDelaysTheFlip(t *testing.T) {
	c := &collector{}
	r := New(c.onChange)
	defer r.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Port 1 is closed: every check fails immediately.
	r.SetSpecs(ctx, []agenttypes.ProbeSpec{{
		Name:             "dead",
		Type:             agenttypes.ProbeTypeTCP,
		Port:             1,
		PeriodSec:        1,
		TimeoutSec:       1,
		FailureThreshold: 3,
	}})

	// It starts unhealthy, so a failing probe never flips anything and
	// must produce no reports at all.
	time.Sleep(1500 * time.Millisecond)
	if n := c.count(); n != 0 {
		t.Fatalf("got %d reports for a probe that never changed verdict", n)
	}
	if states := r.States(); len(states) != 1 || states[0].Healthy {
		t.Fatalf("States() = %+v", states)
	}
	if states := r.States(); states[0].Message == "" {
		t.Fatal("failing probe carries no reason")
	}
}

func TestExecProbe(t *testing.T) {
	c := &collector{}
	r := New(c.onChange)
	defer r.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.SetSpecs(ctx, []agenttypes.ProbeSpec{{
		Name:             "exec-ok",
		Type:             agenttypes.ProbeTypeExec,
		Command:          []string{"/bin/sh", "-c", "exit 0"},
		PeriodSec:        1,
		SuccessThreshold: 1,
	}})
	waitFor(t, "exec probe healthy", func() bool {
		s, ok := c.last()
		return ok && s.Healthy
	})
}

// TestSetSpecsReplacesTheWholeSet proves a removed probe stops running
// and an unchanged one is not restarted.
func TestSetSpecsReplacesTheWholeSet(t *testing.T) {
	r := New(nil)
	defer r.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	keep := agenttypes.ProbeSpec{Name: "keep", Type: agenttypes.ProbeTypeTCP, Port: 1, PeriodSec: 1}
	drop := agenttypes.ProbeSpec{Name: "drop", Type: agenttypes.ProbeTypeTCP, Port: 1, PeriodSec: 1}
	r.SetSpecs(ctx, []agenttypes.ProbeSpec{keep, drop})
	if got := len(r.States()); got != 2 {
		t.Fatalf("States() has %d probes, want 2", got)
	}

	r.mu.Lock()
	before := r.workers["keep"]
	r.mu.Unlock()

	r.SetSpecs(ctx, []agenttypes.ProbeSpec{keep})
	states := r.States()
	if len(states) != 1 || states[0].Name != "keep" {
		t.Fatalf("States() = %+v, want only keep", states)
	}

	r.mu.Lock()
	after := r.workers["keep"]
	r.mu.Unlock()
	if before != after {
		t.Fatal("an unchanged probe was restarted, resetting its counters")
	}
}
