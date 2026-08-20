package agentgw

import (
	"context"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// The heartbeat's snapshot must land in the store keyed by the session's
// host, trend serialised with its nulls intact -- and its absence must
// write nothing, because most beats from a freshly started or old agent
// carry none.
func TestRecordMetrics(t *testing.T) {
	store := &fakeAgentStore{}
	h := &protocolHandler{agents: store}
	sess := &Session{AgentID: "a-1", HostID: 42}

	h.recordMetrics(context.Background(), sess, nil)
	if len(store.metrics) != 0 {
		t.Fatalf("a beat without a snapshot must not write: %+v", store.metrics)
	}

	h.recordMetrics(context.Background(), sess, &agenttypes.MetricsSummary{
		SampledAtMs:  1_755_600_000_000,
		CPUUsedPct:   37.5,
		MemUsedPct:   61,
		DiskUsedPct:  82,
		DiskUsedPath: "/data",
		Load1:        1.5,
		NetRxBps:     1000,
		CPUTrend:     []agenttypes.MetricValue{12, agenttypes.MetricValueNone, 14},
	})
	if len(store.metrics) != 1 {
		t.Fatalf("snapshot not stored: %+v", store.metrics)
	}
	got := store.metrics[0]
	if got.hostID != 42 {
		t.Fatalf("stored under host %d, want 42", got.hostID)
	}
	if got.in.CPUUsedPct != 37.5 || got.in.DiskUsedPath != "/data" || got.in.Load1 != 1.5 {
		t.Fatalf("summary mangled: %+v", got.in)
	}
	if got.in.SampledAt.UnixMilli() != 1_755_600_000_000 {
		t.Fatalf("sampledAt: %v", got.in.SampledAt)
	}
	if string(got.in.CPUTrend) != "[12,null,14]" {
		t.Fatalf("trend must keep its nulls: %s", got.in.CPUTrend)
	}
}
