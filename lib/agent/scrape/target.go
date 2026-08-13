package scrape

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// targetLoop drives one exporter: wait out its phase offset, then scrape
// and push once per interval.
type targetLoop struct {
	s        *Scraper
	target   agenttypes.ScrapeTarget
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
}

func newTargetLoop(parent context.Context, s *Scraper, t agenttypes.ScrapeTarget, interval time.Duration) *targetLoop {
	ctx, cancel := context.WithCancel(parent)
	return &targetLoop{s: s, target: t, interval: interval, ctx: ctx, cancel: cancel}
}

func (l *targetLoop) stop() { l.cancel() }

func (l *targetLoop) run() {
	defer l.cancel()

	select {
	case <-l.ctx.Done():
		return
	case <-time.After(phaseOffset(l.target.URL, l.interval)):
	}

	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	for {
		l.round()
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// round runs one scrape+push, or records a failure if the worker pool is
// saturated.
//
// It never waits for a slot. A full pool means earlier rounds are still
// draining, so queueing this one would stack rounds on top of each
// other -- the pile-up that kills an agent when VM or an exporter slows
// down. Skipping records up=0, which is the honest signal: this interval
// produced no sample.
func (l *targetLoop) round() {
	if !l.s.acquire() {
		l.s.cfg.Log.Warnf("scrape: worker pool full, skipping %s", l.target.URL)
		l.s.record(l.target.URL, result{labels: l.target.Labels})
		return
	}
	defer l.s.release()

	start := time.Now()
	err := l.scrapeAndPush()
	if err != nil {
		l.s.cfg.Log.Warnf("scrape: %s: %v", l.target.URL, err)
	}
	l.s.record(l.target.URL, result{
		labels:   l.target.Labels,
		up:       err == nil,
		duration: time.Since(start),
	})
}

// scrapeAndPush copies one exporter's response straight into a VM import
// request.
//
// A retry re-scrapes rather than resending: the body is a stream, so it
// cannot be replayed, and re-scraping yields fresher samples anyway. The
// retry happens inside this worker slot, so retries can never add to the
// concurrency the pool bounds.
func (l *targetLoop) scrapeAndPush() error {
	var lastErr error
	for attempt := 0; attempt <= pushRetries; attempt++ {
		if l.ctx.Err() != nil {
			return l.ctx.Err()
		}
		if attempt > 0 {
			select {
			case <-l.ctx.Done():
				return l.ctx.Err()
			case <-time.After(time.Second):
			}
		}
		if err := l.once(); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func (l *targetLoop) once() error {
	set := l.s.currentSettings()
	if set.pushURL == "" {
		return fmt.Errorf("no push url")
	}
	ctx, cancel := context.WithTimeout(l.ctx, scrapeTimeout(l.interval))
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.target.URL, nil)
	if err != nil {
		return err
	}
	// Ask for gzip explicitly. The transport has compression disabled, so
	// Go will not decompress it behind our back and the compressed bytes
	// can be forwarded untouched: the exporter pays for compression, the
	// agent pays nothing, and the aggregate push bandwidth drops by
	// roughly an order of magnitude.
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := l.s.exporters.Do(req)
	if err != nil {
		return fmt.Errorf("scrape: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scrape: http %d", resp.StatusCode)
	}

	// The collection instant, not the ingestion instant. See
	// pushRequestURL.
	pushReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		pushRequestURL(set.pushURL, l.target.Labels, time.Now()), resp.Body)
	if err != nil {
		return err
	}
	pushReq.Header.Set("Content-Type", "text/plain")
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		pushReq.Header.Set("Content-Encoding", enc)
	}
	if set.ingestToken != "" {
		pushReq.Header.Set("Authorization", "Bearer "+set.ingestToken)
	}

	pushResp, err := l.s.remote.Do(pushReq)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	defer pushResp.Body.Close()
	// Drain so the connection returns to the idle pool instead of being
	// closed, which is what makes keep-alive actually reuse it.
	_, _ = io.Copy(io.Discard, io.LimitReader(pushResp.Body, 4<<10))
	if pushResp.StatusCode < 200 || pushResp.StatusCode >= 300 {
		return fmt.Errorf("push: http %d", pushResp.StatusCode)
	}
	return nil
}
