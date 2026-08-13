package client

import (
	"fmt"
	"strings"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

type nopLogger struct{}

func (nopLogger) Infof(string, ...any) {}
func (nopLogger) Warnf(string, ...any) {}

// TestProbeDigestCannotBurstTheFrame is the one that matters: an
// over-large heartbeat fails to encode, the heartbeat loop treats a send
// failure as "tear the session down", and the agent would reconnect
// forever -- losing the host's manageability to make room for probe
// state, which is second class by design.
func TestProbeDigestCannotBurstTheFrame(t *testing.T) {
	states := make([]agenttypes.ProbeState, 400)
	for i := range states {
		states[i] = agenttypes.ProbeState{
			Name:    fmt.Sprintf("workspace-alpha/service-%03d/liveness-probe", i),
			Healthy: i%2 == 0,
			Message: strings.Repeat("x", 256),
		}
	}
	c := &Channel{Log: nopLogger{}, ProbeStates: func() []agenttypes.ProbeState { return states }}

	got := c.probeStates()
	if len(got) > maxHeartbeatProbes {
		t.Fatalf("digest has %d states, cap is %d", len(got), maxHeartbeatProbes)
	}
	if _, err := agenttypes.EncodeFrame(agenttypes.Frame{
		Type:        agenttypes.FrameTypeHeartbeat,
		ProbeStates: got,
	}); err != nil {
		t.Fatalf("a capped heartbeat still does not encode: %v", err)
	}

	// Unhealthy first: those are what an operator is looking for.
	for _, s := range got {
		if s.Healthy {
			break
		}
	}
	unhealthy := 0
	for _, s := range got {
		if !s.Healthy {
			unhealthy++
		}
	}
	if unhealthy != maxHeartbeatProbes {
		t.Fatalf("kept %d unhealthy states, want the cap filled with them first", unhealthy)
	}
}

func TestProbeDigestPassesThroughWhenSmall(t *testing.T) {
	states := []agenttypes.ProbeState{{Name: "a"}, {Name: "b", Healthy: true}}
	c := &Channel{Log: nopLogger{}, ProbeStates: func() []agenttypes.ProbeState { return states }}
	if got := c.probeStates(); len(got) != 2 {
		t.Fatalf("digest = %v, want both states untouched", got)
	}
}

func TestProbeDigestNilWithoutARunner(t *testing.T) {
	if got := (&Channel{Log: nopLogger{}}).probeStates(); got != nil {
		t.Fatalf("digest = %v, want nil", got)
	}
}
