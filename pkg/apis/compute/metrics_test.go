package compute

import (
	"context"
	"errors"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

type fakeMetricsBackend struct {
	got MetricsRequest
	res *HostMetrics
	err error
}

func (f *fakeMetricsBackend) Series(_ context.Context, _ int64, req MetricsRequest) (*HostMetrics, error) {
	f.got = req
	return f.res, f.err
}

type fakeHostReader struct {
	modstore.HostStore
	missing bool
}

func (f fakeHostReader) GetByID(context.Context, int64, scope.Filter) (*modstore.HostRow, error) {
	if f.missing {
		return nil, pgerrors.ErrNotFound
	}
	return &modstore.HostRow{ID: 7}, nil
}

func metricsCtx() apiserver.Ctx {
	return apiserver.Ctx{Context: context.Background()}
}

func TestMetricsVerbForwardsWindow(t *testing.T) {
	backend := &fakeMetricsBackend{res: &HostMetrics{Count: 1}}
	o := hostMetricsOps{hosts: fakeHostReader{}, backend: backend}

	res, err := o.series(metricsCtx(), 7, list.Query{Filters: map[string]any{
		"from_ms": "1000", "to_ms": "121000", "step_sec": "15", "series": "cpu.used_pct,mem.used_pct",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.(*HostMetrics).Count != 1 {
		t.Fatalf("backend result not returned: %+v", res)
	}
	if backend.got.FromMs != 1000 || backend.got.ToMs != 121000 || backend.got.StepSec != 15 {
		t.Fatalf("window not forwarded: %+v", backend.got)
	}
	if len(backend.got.Series) != 2 || backend.got.Series[1] != "mem.used_pct" {
		t.Fatalf("series filter not forwarded: %+v", backend.got.Series)
	}
}

func TestMetricsVerbBounds(t *testing.T) {
	o := hostMetricsOps{hosts: fakeHostReader{}, backend: &fakeMetricsBackend{}}

	if _, err := o.series(metricsCtx(), 7, list.Query{Filters: map[string]any{
		"from_ms": "5000", "to_ms": "5000",
	}}); err == nil {
		t.Fatal("an empty window must be a 400, not a data-channel dial")
	}
	// 25h at 15s is over MetricsMaxPoints.
	if _, err := o.series(metricsCtx(), 7, list.Query{Filters: map[string]any{
		"from_ms": "0", "to_ms": "90000000", "step_sec": "15",
	}}); err == nil {
		t.Fatal("an over-wide window must be a 400")
	}
}

func TestMetricsVerbScopeCheckComesFirst(t *testing.T) {
	backend := &fakeMetricsBackend{}
	o := hostMetricsOps{hosts: fakeHostReader{missing: true}, backend: backend}
	_, err := o.series(metricsCtx(), 7, list.Query{})
	if err == nil {
		t.Fatal("a host outside the caller's scope must 404")
	}
	if backend.got.ToMs != 0 {
		t.Fatal("the backend must not be consulted for a host the caller may not read")
	}
}

func TestMetricsFailureMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{agentgw.ErrHostUnreachable, 503},
		{agentgw.ErrHostOnAnotherInstance, 503},
		{context.DeadlineExceeded, 503},
		{&agentgw.StreamRejected{Code: agenttypes.StreamErrUnknownKind, Message: "unknown stream kind metrics"}, 409},
		{&agentgw.StreamRejected{Code: agenttypes.StreamErrOpFailed, Message: "this host does not collect metrics"}, 409},
		{errors.New("boom"), 503},
	}
	for _, c := range cases {
		got := metricsFailure(c.err)
		var se *apierrors.StatusError
		if !errors.As(got, &se) || se.Status != c.status {
			t.Fatalf("%v -> %v, want status %d", c.err, got, c.status)
		}
	}
	// The outdated-agent case must name the fix.
	var se *apierrors.StatusError
	_ = errors.As(metricsFailure(&agentgw.StreamRejected{Code: agenttypes.StreamErrUnknownKind}), &se)
	if se == nil || se.Message == "" || !contains(se.Message, "reinstall") {
		t.Fatalf("outdated agent must be told to reinstall: %+v", se)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// A misaligned window sitting exactly at the point cap gains a bucket
// once both ends are grid-aligned. The verb must count the way the agent
// counts, or it waves through a request the agent then 409s.
func TestMetricsVerbBoundsMatchAgentAlignment(t *testing.T) {
	o := hostMetricsOps{hosts: fakeHostReader{}, backend: &fakeMetricsBackend{res: &HostMetrics{}}}
	if _, err := o.series(metricsCtx(), 7, list.Query{Filters: map[string]any{
		"from_ms": "1", "to_ms": "30000001", "step_sec": "15",
	}}); err == nil {
		t.Fatal("2001 aligned buckets must be a 400 here, not a data-channel 409")
	}
}
