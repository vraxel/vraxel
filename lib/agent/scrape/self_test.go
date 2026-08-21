package scrape

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// startSelfScraper mirrors startScraper with a Self source attached.
func startSelfScraper(t *testing.T, resp agenttypes.ScrapeTargetsResponse, body string) *Scraper {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != agenttypes.ScrapeTargetsPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	s := New(Config{
		ServerURL: srv.URL,
		Token:     func() string { return "session-token" },
		Self: func() ([]byte, time.Time) {
			return []byte(body), time.UnixMilli(1_755_600_000_000)
		},
		Log: testLogger{t},
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Run(ctx)
	return s
}

func TestSelfPush(t *testing.T) {
	vm := newFakeVM(t)
	startSelfScraper(t, agenttypes.ScrapeTargetsResponse{
		PushURL:     vm.srv.URL,
		IntervalSec: 1,
		NodeMetrics: &agenttypes.NodeMetricsPush{Labels: map[string]string{
			"job": "vraxel-agent", "host_id": "42",
		}},
	}, "node_load1 0.5\n")

	p := vm.waitForPush(t, "the self push", func(p push) bool {
		return strings.Contains(p.body, "node_load1")
	})
	if p.encoding != "gzip" {
		t.Fatalf("the self body is born uncompressed here and must ship gzipped, got %q", p.encoding)
	}
	labels := p.query["extra_label"]
	if len(labels) != 2 || labels[0] != "host_id=42" || labels[1] != "job=vraxel-agent" {
		t.Fatalf("server-issued labels must ride extra_label, sorted: %v", labels)
	}
	if p.query.Get("timestamp") != "1755600000000" {
		t.Fatalf("the batch must carry its sample time, got %q", p.query.Get("timestamp"))
	}
}

func TestSelfPushDisabledWithoutSwitch(t *testing.T) {
	vm := newFakeVM(t)
	// A push URL alone is the pre-existing exporter path; the embedded
	// collector must stay silent until the server flips its own switch.
	startSelfScraper(t, agenttypes.ScrapeTargetsResponse{
		PushURL:     vm.srv.URL,
		IntervalSec: 1,
	}, "node_load1 0.5\n")

	time.Sleep(2500 * time.Millisecond)
	for _, p := range vm.pushes() {
		if strings.Contains(p.body, "node_load1") {
			t.Fatalf("self push happened without the server's switch: %+v", p)
		}
	}
}
