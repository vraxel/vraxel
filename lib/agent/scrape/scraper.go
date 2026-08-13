// Package scrape collects metrics from the exporters on this host and
// pushes them to VictoriaMetrics (design §5.9).
//
// This is not a push model. Pull semantics are intact -- the target list
// comes from the platform, collection is periodic, and `up` still exists -- only
// the origin of the pull moved from the control plane to the host, which
// is what vmagent and Prometheus agent mode do. It exists because the platform
// cannot dial the host at all, and it costs no fidelity: extra_label has
// override semantics, so the series are identical to VM scraping the
// host directly.
//
// Nothing here parses the exposition format. A scrape response body is
// copied straight into the push request, gzip and all.
package scrape

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"vraxel.io/vraxel/lib/agent/loopback"
	"vraxel.io/vraxel/lib/agent/transport"
	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

const (
	// defaultInterval is the scrape period.
	defaultInterval = 15 * time.Second
	// refreshInterval is how often the target list is refetched. The list
	// changes only when the platform deploys or removes something on this host, and
	// config.reload makes that immediate, so this is the fallback.
	refreshInterval = 60 * time.Second
	// maxWorkers bounds concurrent scrape+push operations. Bounded and
	// separate from the control channel's goroutines: when VM is slow,
	// dropping samples is acceptable, starving the heartbeat is not --
	// 60s of missed beats costs the whole host's manageability. Metrics
	// are permanently second class inside this process.
	maxWorkers = 4
	// pushRetries is how many times a failed scrape+push is retried
	// before the samples are dropped. Metrics tolerate gaps; unbounded
	// retries turn a one-minute VM hiccup into an agent-side pile-up.
	pushRetries = 1
	// importPath is VM's Prometheus-exposition import endpoint.
	importPath = "/api/v1/import/prometheus"
)

// scrapeTimeout bounds one scrape+push. It must stay below the
// interval: a pathological exporter hangs rather than errors
// (node_exporter's filesystem collector on a wedged NFS mount,
// mysqld_exporter during a slow query), and without a timeout every
// round would leave another goroutine and connection behind until the
// agent dies of it.
func scrapeTimeout(interval time.Duration) time.Duration { return interval * 2 / 3 }

// errNoToken means the control channel has not delivered a session token
// yet. It is the normal state for the first second of every boot, so it
// is not worth a warning: at ten thousand hosts a rollout would produce
// ten thousand of them, all describing something that fixes itself.
var errNoToken = errors.New("no session token yet")

// Logger is the minimal logging surface this package needs.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
}

// Config is what the host program supplies.
type Config struct {
	// ServerURL is the server base URL, used to fetch targets.
	ServerURL string
	// Token returns the current session token.
	Token func() string
	// TLS carries the trust store for the server and VictoriaMetrics. Nil
	// uses the system defaults. Exporters are on loopback and plain HTTP,
	// so it never applies to them.
	TLS *tls.Config
	Log Logger
}

// Scraper owns the per-target loops and the target list.
type Scraper struct {
	cfg Config
	// exporters talks to this host's exporters and enforces the loopback
	// rule in its dialer. remote talks to the server and to VM, which are
	// remote by definition -- pointing the loopback dialer at those would
	// make the agent refuse to fetch its own target list.
	exporters *http.Client
	remote    *http.Client
	workers   chan struct{}

	// refreshNow lets config.reload skip the 60s wait.
	refreshNow chan struct{}
	// settingsChanged wakes the synthetic loop when the server's answer
	// changes what it should do. Without it that loop would keep waiting
	// out the interval it read at startup -- the default, since no target
	// list has arrived yet -- and the first batch would be one default
	// interval late no matter what period the server asked for.
	settingsChanged chan struct{}

	mu       sync.Mutex
	targets  map[string]*targetLoop
	settings settings
	results  map[string]result
}

// settings is the part of the server's answer that is not the target
// list itself.
type settings struct {
	pushURL     string
	ingestToken string
	interval    time.Duration
}

// result is one target's last outcome, the raw material for the
// synthetic up / scrape_duration_seconds series.
type result struct {
	labels   map[string]string
	up       bool
	duration time.Duration
}

// New builds a Scraper.
//
// One client for both directions, with compression disabled: the agent
// asks exporters for gzip and forwards the compressed bytes untouched,
// so Go's transparent decompression would defeat the whole point. Idle
// connections are reused because at ten thousand hosts a fresh TCP (and
// TLS) handshake per push would dominate the cost on both ends.
func New(cfg Config) *Scraper {
	return &Scraper{
		cfg: cfg,
		exporters: &http.Client{
			Transport: &http.Transport{
				// Enforcing the loopback rule in the dialer closes the gap
				// between checking a target and connecting to it: whatever
				// the resolver answers at dial time is what gets checked,
				// so a second, different DNS answer cannot slip through.
				DialContext:         loopback.DialContext,
				DisableCompression:  true,
				MaxIdleConnsPerHost: 2,
				MaxConnsPerHost:     4,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		remote:          &http.Client{Transport: remoteTransport(cfg.TLS)},
		workers:         make(chan struct{}, maxWorkers),
		refreshNow:      make(chan struct{}, 1),
		settingsChanged: make(chan struct{}, 1),
		targets:         map[string]*targetLoop{},
		results:         map[string]result{},
		settings:        settings{interval: defaultInterval},
	}
}

// remoteTransport is the transport for the server and VictoriaMetrics.
// Compression stays off here too: a push body is bytes the exporter
// already gzipped, forwarded with the encoding header set by hand.
func remoteTransport(tlsCfg *tls.Config) *http.Transport {
	t := transport.NewTransport(tlsCfg)
	t.DisableCompression = true
	t.MaxIdleConnsPerHost = 2
	t.MaxConnsPerHost = 4
	return t
}

// Refresh asks for an immediate target-list refetch. Non-blocking.
func (s *Scraper) Refresh() {
	select {
	case s.refreshNow <- struct{}{}:
	default:
	}
}

// Run keeps the target list current and the per-target loops running
// until ctx ends.
func (s *Scraper) Run(ctx context.Context) {
	go s.runSynthetic(ctx)

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		if err := s.refresh(ctx); err != nil && ctx.Err() == nil && !errors.Is(err, errNoToken) {
			s.cfg.Log.Warnf("scrape: refresh targets: %v", err)
		}
		select {
		case <-ctx.Done():
			s.stopAll()
			return
		case <-ticker.C:
		case <-s.refreshNow:
		}
	}
}

// refresh fetches the target list and reconciles the running loops to
// it: start the new, stop the removed, leave the unchanged alone so a
// refresh does not reset every target's phase.
func (s *Scraper) refresh(ctx context.Context) error {
	resp, err := s.fetchTargets(ctx)
	if err != nil {
		return err
	}

	interval := defaultInterval
	if resp.IntervalSec > 0 {
		interval = time.Duration(resp.IntervalSec) * time.Second
	}

	s.mu.Lock()
	next := settings{pushURL: resp.PushURL, ingestToken: resp.IngestToken, interval: interval}
	changed := next != s.settings
	s.settings = next
	wanted := map[string]agenttypes.ScrapeTarget{}
	for _, t := range resp.Targets {
		if err := checkLoopback(t.URL); err != nil {
			s.cfg.Log.Warnf("scrape: refusing target %s: %v", t.URL, err)
			continue
		}
		wanted[t.URL] = t
	}
	for url, loop := range s.targets {
		t, keep := wanted[url]
		if keep && resp.PushURL != "" && sameTarget(loop.target, t) && loop.interval == interval {
			continue
		}
		loop.stop()
		delete(s.targets, url)
		delete(s.results, url)
	}
	var started []*targetLoop
	if resp.PushURL != "" {
		for url, t := range wanted {
			if _, running := s.targets[url]; running {
				continue
			}
			loop := newTargetLoop(ctx, s, t, interval)
			s.targets[url] = loop
			started = append(started, loop)
		}
	}
	s.mu.Unlock()

	if changed {
		select {
		case s.settingsChanged <- struct{}{}:
		default:
		}
	}
	for _, loop := range started {
		go loop.run()
	}
	return nil
}

func (s *Scraper) fetchTargets(ctx context.Context) (agenttypes.ScrapeTargetsResponse, error) {
	var out agenttypes.ScrapeTargetsResponse
	token := ""
	if s.cfg.Token != nil {
		token = s.cfg.Token()
	}
	if token == "" {
		return out, errNoToken
	}
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(rctx, http.MethodGet,
		strings.TrimRight(s.cfg.ServerURL, "/")+agenttypes.ScrapeTargetsPath, nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.remote.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("http %d", resp.StatusCode)
	}
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

func (s *Scraper) stopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for url, loop := range s.targets {
		loop.stop()
		delete(s.targets, url)
	}
}

// record stores a target's outcome for the synthetic series.
func (s *Scraper) record(targetURL string, r result) {
	s.mu.Lock()
	s.results[targetURL] = r
	s.mu.Unlock()
}

func (s *Scraper) currentSettings() settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// acquire takes a worker slot without blocking. A full pool means the
// previous rounds are still draining, and the caller records a failed
// scrape rather than queueing: queueing is how a slow VM turns into an
// unbounded goroutine pile.
func (s *Scraper) acquire() bool {
	select {
	case s.workers <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Scraper) release() { <-s.workers }

// phaseOffset spreads targets across the interval by a stable hash, so
// ten thousand hosts do not all hit VM on the same second. Prometheus
// offsets its targets the same way, for the same reason.
func phaseOffset(key string, interval time.Duration) time.Duration {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return time.Duration(h.Sum64() % uint64(interval))
}

// pushRequestURL builds the import URL for one target: its labels as
// extra_label pairs and the collection instant as the batch timestamp.
//
// The timestamp matters. Without it VM stamps samples at ingestion, so
// ten minutes of backlog after a network outage all lands on the second
// it recovered and every rate() over that window is wrong. Passing it as
// a query arg keeps the body opaque; stamping each line would mean
// rewriting the body and giving up the zero-parse property.
func pushRequestURL(base string, labels map[string]string, at time.Time) string {
	q := url.Values{}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		q.Add("extra_label", k+"="+labels[k])
	}
	q.Set("timestamp", strconv.FormatInt(at.UnixMilli(), 10))
	return strings.TrimRight(base, "/") + importPath + "?" + q.Encode()
}

// checkLoopback rejects a target the agent must not scrape, at the point
// the list arrives, so a refused target is logged once instead of once
// per interval. The dialer enforces the same rule again at connect time,
// which is what actually makes it safe.
func checkLoopback(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("unparsable url: %w", err)
	}
	_, err = loopback.ResolveHost(u.Hostname())
	return err
}

// sameTarget reports whether a target is unchanged, labels included.
//
// Comparing the URL alone would keep the running loop and quietly go on
// using the labels it was started with, so a workspace rename or a job
// relabel would never reach VM until the agent restarted.
func sameTarget(a, b agenttypes.ScrapeTarget) bool {
	if a.URL != b.URL || len(a.Labels) != len(b.Labels) {
		return false
	}
	for k, v := range a.Labels {
		if b.Labels[k] != v {
			return false
		}
	}
	return true
}
