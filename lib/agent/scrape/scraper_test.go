package scrape

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

type testLogger struct{ t *testing.T }

func (l testLogger) Infof(f string, a ...any) { l.t.Logf("INFO  "+f, a...) }
func (l testLogger) Warnf(f string, a ...any) { l.t.Logf("WARN  "+f, a...) }

// push is one request the fake VM received.
type push struct {
	query    url.Values
	encoding string
	auth     string
	body     string
}

// fakeVM stands in for VictoriaMetrics' import endpoint.
type fakeVM struct {
	srv *httptest.Server
	mu  sync.Mutex
	got []push
}

func newFakeVM(t *testing.T) *fakeVM {
	v := &fakeVM{}
	v.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer gz.Close()
			reader = gz
		}
		body, _ := io.ReadAll(reader)
		v.mu.Lock()
		v.got = append(v.got, push{
			query:    r.URL.Query(),
			encoding: r.Header.Get("Content-Encoding"),
			auth:     r.Header.Get("Authorization"),
			body:     string(body),
		})
		v.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(v.srv.Close)
	return v
}

func (v *fakeVM) pushes() []push {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]push(nil), v.got...)
}

// waitForPush polls until a push matching pred arrives.
func (v *fakeVM) waitForPush(t *testing.T, what string, pred func(push) bool) push {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, p := range v.pushes() {
			if pred(p) {
				return p
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
	return push{}
}

// exporter serves a metrics body, gzipped when asked for.
func newExporter(t *testing.T, body string) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			defer gz.Close()
			_, _ = gz.Write([]byte(body))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/metrics"
}

// startScraper wires a Scraper against a fake vraxel-server serving the
// given targets response.
func startScraper(t *testing.T, resp agenttypes.ScrapeTargetsResponse) *Scraper {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != agenttypes.ScrapeTargetsPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer session-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	s := New(Config{
		ServerURL: srv.URL,
		Token:     func() string { return "session-token" },
		Log:       testLogger{t},
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Run(ctx)
	return s
}

func TestScrapePushesBodyUntouchedWithLabelsAndTimestamp(t *testing.T) {
	const metrics = "node_cpu_seconds_total{cpu=\"0\"} 12345\n"
	vm := newFakeVM(t)
	target := newExporter(t, metrics)

	before := time.Now().Add(-time.Second).UnixMilli()
	startScraper(t, agenttypes.ScrapeTargetsResponse{
		PushURL:     vm.srv.URL,
		IngestToken: "ingest-secret",
		IntervalSec: 1,
		Targets: []agenttypes.ScrapeTarget{{
			URL:    target,
			Labels: map[string]string{"job": "node", "host_id": "164"},
		}},
	})

	p := vm.waitForPush(t, "the exporter body", func(p push) bool {
		return strings.Contains(p.body, "node_cpu_seconds_total")
	})

	if p.body != metrics {
		t.Fatalf("body was rewritten: %q", p.body)
	}
	if p.encoding != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip passthrough", p.encoding)
	}
	if p.auth != "Bearer ingest-secret" {
		t.Fatalf("Authorization = %q", p.auth)
	}
	labels := p.query["extra_label"]
	if len(labels) != 2 || labels[0] != "host_id=164" || labels[1] != "job=node" {
		t.Fatalf("extra_label = %v, want sorted host_id then job", labels)
	}
	ts, err := strconv.ParseInt(p.query.Get("timestamp"), 10, 64)
	if err != nil {
		t.Fatalf("timestamp = %q: %v", p.query.Get("timestamp"), err)
	}
	if ts < before || ts > time.Now().Add(time.Second).UnixMilli() {
		t.Fatalf("timestamp %d is not the collection instant", ts)
	}
}

func TestSyntheticSeriesReportUpAndDuration(t *testing.T) {
	vm := newFakeVM(t)
	up := newExporter(t, "x 1\n")

	startScraper(t, agenttypes.ScrapeTargetsResponse{
		PushURL:     vm.srv.URL,
		IntervalSec: 1,
		Targets: []agenttypes.ScrapeTarget{
			{URL: up, Labels: map[string]string{"job": "up-one"}},
			// Port 1 is closed: this target can never be scraped.
			{URL: "http://127.0.0.1:1/metrics", Labels: map[string]string{"job": "down-one"}},
		},
	})

	p := vm.waitForPush(t, "synthetic series for both targets", func(p push) bool {
		return strings.Contains(p.body, `up{job="up-one"}`) &&
			strings.Contains(p.body, `up{job="down-one"}`)
	})
	if !strings.Contains(p.body, `up{job="up-one"} 1`) {
		t.Fatalf("healthy target did not report up=1: %q", p.body)
	}
	if !strings.Contains(p.body, `up{job="down-one"} 0`) {
		t.Fatalf("dead target did not report up=0: %q", p.body)
	}
	if !strings.Contains(p.body, `scrape_duration_seconds{job="up-one"}`) {
		t.Fatalf("no scrape_duration_seconds: %q", p.body)
	}
	// Anything requiring the body to be parsed must not appear.
	if strings.Contains(p.body, "scrape_samples") {
		t.Fatalf("emitted a series that would require parsing the body: %q", p.body)
	}
}

func TestNonLoopbackTargetRefused(t *testing.T) {
	vm := newFakeVM(t)
	s := startScraper(t, agenttypes.ScrapeTargetsResponse{
		PushURL:     vm.srv.URL,
		IntervalSec: 1,
		Targets: []agenttypes.ScrapeTarget{
			{URL: "http://10.1.1.10:9100/metrics", Labels: map[string]string{"job": "remote"}},
		},
	})

	time.Sleep(1500 * time.Millisecond)
	s.mu.Lock()
	n := len(s.targets)
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("started %d loops for a non-loopback target", n)
	}
	for _, p := range vm.pushes() {
		if strings.Contains(p.body, "remote") {
			t.Fatalf("pushed data for a refused target: %q", p.body)
		}
	}
}

// TestRemovedTargetStopsScraping proves the reconcile drops loops for
// targets the server no longer lists, so an uninstalled exporter stops
// producing up=0 forever.
func TestRemovedTargetStopsScraping(t *testing.T) {
	vm := newFakeVM(t)
	target := newExporter(t, "x 1\n")

	var mu sync.Mutex
	resp := agenttypes.ScrapeTargetsResponse{
		PushURL:     vm.srv.URL,
		IntervalSec: 1,
		Targets:     []agenttypes.ScrapeTarget{{URL: target, Labels: map[string]string{"job": "gone"}}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	s := New(Config{ServerURL: srv.URL, Token: func() string { return "session-token" }, Log: testLogger{t}})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Run(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		n := len(s.targets)
		s.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	resp.Targets = nil
	mu.Unlock()
	s.Refresh()

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		n, r := len(s.targets), len(s.results)
		s.mu.Unlock()
		if n == 0 && r == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the removed target is still being scraped")
}

func TestPhaseOffsetIsStableAndInsideTheInterval(t *testing.T) {
	const interval = 15 * time.Second
	a := phaseOffset("http://127.0.0.1:9100/metrics", interval)
	b := phaseOffset("http://127.0.0.1:9100/metrics", interval)
	c := phaseOffset("http://127.0.0.1:9104/metrics", interval)

	if a != b {
		t.Fatalf("offset is not stable: %v vs %v", a, b)
	}
	if a < 0 || a >= interval || c < 0 || c >= interval {
		t.Fatalf("offset outside [0, interval): %v %v", a, c)
	}
	if a == c {
		t.Fatal("two targets got the same phase, defeating the spread")
	}
}

func TestWorkerPoolIsBounded(t *testing.T) {
	s := New(Config{Log: testLogger{t}})
	for i := 0; i < maxWorkers; i++ {
		if !s.acquire() {
			t.Fatalf("could not take slot %d of %d", i, maxWorkers)
		}
	}
	if s.acquire() {
		t.Fatal("the pool handed out more slots than its bound")
	}
	s.release()
	if !s.acquire() {
		t.Fatal("a released slot was not reusable")
	}
}

func TestLabelValuesAreEscaped(t *testing.T) {
	body := buildSyntheticBody(map[string]result{
		"u": {labels: map[string]string{"job": `a"b\c`}, up: true},
	})
	if !strings.Contains(body, `up{job="a\"b\\c"} 1`) {
		t.Fatalf("label value was not escaped: %q", body)
	}
}

// TestSyntheticBodyIsDeterministic keeps the batch diffable.
func TestSyntheticBodyIsDeterministic(t *testing.T) {
	in := map[string]result{
		"http://127.0.0.1:9104/metrics": {labels: map[string]string{"job": "mysql"}, up: false},
		"http://127.0.0.1:9100/metrics": {labels: map[string]string{"job": "node"}, up: true},
	}
	first := buildSyntheticBody(in)
	for i := 0; i < 20; i++ {
		if got := buildSyntheticBody(in); got != first {
			t.Fatalf("output varies between runs:\n%q\n%q", first, got)
		}
	}
	if !bytes.HasPrefix([]byte(first), []byte(`up{job="node"}`)) {
		t.Fatalf("targets are not in URL order: %q", first)
	}
}

// TestLabelChangeRestartsTheTarget covers the reconcile bug that a
// URL-only comparison hides: the loop keeps the labels it started with,
// so a workspace rename or a job relabel never reaches VM.
func TestLabelChangeRestartsTheTarget(t *testing.T) {
	vm := newFakeVM(t)
	target := newExporter(t, "x 1\n")

	var mu sync.Mutex
	resp := agenttypes.ScrapeTargetsResponse{
		PushURL:     vm.srv.URL,
		IntervalSec: 1,
		Targets:     []agenttypes.ScrapeTarget{{URL: target, Labels: map[string]string{"job": "before"}}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	s := New(Config{ServerURL: srv.URL, Token: func() string { return "session-token" }, Log: testLogger{t}})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Run(ctx)

	vm.waitForPush(t, "a push with the original label", func(p push) bool {
		for _, l := range p.query["extra_label"] {
			if l == "job=before" {
				return true
			}
		}
		return false
	})

	mu.Lock()
	resp.Targets = []agenttypes.ScrapeTarget{{URL: target, Labels: map[string]string{"job": "after"}}}
	mu.Unlock()
	s.Refresh()

	vm.waitForPush(t, "a push with the updated label", func(p push) bool {
		for _, l := range p.query["extra_label"] {
			if l == "job=after" {
				return true
			}
		}
		return false
	})
}

// TestScrapeTargetsAreDialledByCheckedAddress proves the dialer, not just
// the list filter, enforces the loopback rule -- the filter runs once per
// refresh, and a name can resolve differently by the time it is dialled.
func TestScrapeTargetsAreDialledByCheckedAddress(t *testing.T) {
	s := New(Config{Log: testLogger{t}})
	tr, ok := s.exporters.Transport.(*http.Transport)
	if !ok || tr.DialContext == nil {
		t.Fatal("the exporter client does not install a dialer, so nothing checks the address at connect time")
	}
	if _, err := tr.DialContext(context.Background(), "tcp", "10.1.1.10:9100"); err == nil {
		t.Fatal("the dialer connected to a non-loopback address")
	}
}

// TestRemoteClientReachesNonLoopback is the counterpart, and the bug a
// loopback-everywhere client hides: vraxel-server and VM are remote by
// definition, so a single client carrying the exporter rule makes the
// agent refuse to fetch its own target list. Every httptest server binds
// 127.0.0.1, so no amount of end-to-end testing here would catch it --
// only the structure can.
func TestRemoteClientReachesNonLoopback(t *testing.T) {
	s := New(Config{Log: testLogger{t}})
	tr, ok := s.remote.Transport.(*http.Transport)
	if !ok {
		t.Fatal("the remote client has no transport")
	}
	// Asserted by behaviour, not by comparing dialers: the remote
	// transport carries the default dialer (and now a TLS config), so
	// "has no DialContext" stopped meaning anything. A cancelled context
	// makes the dial fail immediately -- the question is only WHICH
	// failure, and the loopback rule has a distinctive one.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tr.DialContext(ctx, "tcp", "10.1.1.10:9100")
	if err != nil && strings.Contains(err.Error(), "loopback") {
		t.Fatalf("the remote client enforces the loopback rule, so it cannot reach vraxel-server or VM: %v", err)
	}
	if s.remote == s.exporters {
		t.Fatal("one client serves both loopback and remote targets")
	}
}
