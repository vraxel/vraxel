package compute

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	apierrors "vraxel.io/vraxel/lib/api/errors"
)

// fakeVMQuery serves /api/v1/query_range with canned matrices keyed by a
// substring of the query.
type fakeVMQuery struct {
	srv *httptest.Server
	mu  sync.Mutex
	got []url.Values
}

func newFakeVMQuery(t *testing.T, answer func(query string) string) *fakeVMQuery {
	f := &fakeVMQuery{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		f.mu.Lock()
		f.got = append(f.got, q)
		f.mu.Unlock()
		body := answer(q.Get("query"))
		if body == "" {
			body = `{"status":"success","data":{"resultType":"matrix","result":[]}}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func matrix(metric string, points string) string {
	return fmt.Sprintf(
		`{"status":"success","data":{"resultType":"matrix","result":[{"metric":%s,"values":[%s]}]}}`,
		metric, points)
}

func TestVMBackendGridMapping(t *testing.T) {
	// Window 0..60000ms at 15s: buckets at 0,15,30,45. The answer skips
	// t=30 -- that bucket must come back null, not zero.
	vm := newFakeVMQuery(t, func(query string) string {
		if query == vmVocabulary[agenttypes.SeriesCPUUsedPct].expr {
			return matrix(`{}`, `[0,"10.5"],[15,"11"],[45,"12"]`)
		}
		return ""
	})

	b := NewVMMetrics(vm.srv.URL)
	res, err := b.Series(context.Background(), 42, MetricsRequest{
		FromMs: 0, ToMs: 60_000, StepSec: 15,
		Series: []string{agenttypes.SeriesCPUUsedPct},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 4 || res.StepSec != 15 || len(res.Series) != 1 {
		t.Fatalf("grid: %+v", res)
	}
	vs := res.Series[0].Values
	if vs[0] == nil || *vs[0] != 10.5 || vs[1] == nil || *vs[1] != 11 {
		t.Fatalf("values: %v %v", vs[0], vs[1])
	}
	if vs[2] != nil {
		t.Fatalf("a missing point must be null, got %v", *vs[2])
	}
	if vs[3] == nil || *vs[3] != 12 {
		t.Fatalf("last bucket: %v", vs[3])
	}

	// The host filter is the security property: it must ride
	// extra_filters on every query.
	q := vm.got[0]
	if got := q.Get("extra_filters[]"); got != `{host_id="42",job="vraxel-agent"}` {
		t.Fatalf("host filter: %q", got)
	}
	if q.Get("step") != "15s" || q.Get("start") != "0.000" {
		t.Fatalf("range params: step=%q start=%q", q.Get("step"), q.Get("start"))
	}
}

func TestVMBackendDimensionLabels(t *testing.T) {
	vm := newFakeVMQuery(t, func(query string) string {
		if query == vmVocabulary[agenttypes.SeriesNetRxBps].expr {
			// VM carries the full label soup; only the declared dimension
			// may survive into the API.
			return matrix(`{"device":"eth0","host_id":"42","job":"vraxel-agent","instance":"x"}`, `[0,"1000"]`)
		}
		return ""
	})
	b := NewVMMetrics(vm.srv.URL)
	res, err := b.Series(context.Background(), 42, MetricsRequest{
		FromMs: 0, ToMs: 15_000, StepSec: 15,
		Series: []string{agenttypes.SeriesNetRxBps},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := res.Series[0]
	if len(s.Labels) != 1 || s.Labels["device"] != "eth0" {
		t.Fatalf("labels must be the dimension alone: %+v", s.Labels)
	}
}

func TestVMBackendQueriesWholeVocabularyByDefault(t *testing.T) {
	vm := newFakeVMQuery(t, func(string) string { return "" })
	b := NewVMMetrics(vm.srv.URL)
	res, err := b.Series(context.Background(), 1, MetricsRequest{FromMs: 0, ToMs: 15_000, StepSec: 15})
	if err != nil {
		t.Fatal(err)
	}
	if len(vm.got) != len(vmVocabularyOrder) {
		t.Fatalf("expected one query per vocabulary family, got %d of %d", len(vm.got), len(vmVocabularyOrder))
	}
	if res.Series == nil {
		t.Fatal("an empty answer must be [], the agentlive shape the frontend iterates -- not null")
	}
}

func TestVMBackendUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	b := NewVMMetrics(srv.URL)
	_, err := b.Series(context.Background(), 1, MetricsRequest{FromMs: 0, ToMs: 15_000, StepSec: 15})
	var se *apierrors.StatusError
	if !errors.As(err, &se) || se.Status != 503 {
		t.Fatalf("a dead backend must be a 503: %v", err)
	}
}
