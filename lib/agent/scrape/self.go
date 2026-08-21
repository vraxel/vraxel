package scrape

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// runSelf pushes the embedded collector's own exposition, when the
// server asked for it (settings.selfLabels non-nil) and the host has a
// collector to ask (cfg.Self non-nil).
//
// It is the full tier's transport: the same body node_exporter would
// have served, pushed the same way a scraped target's body is, so the
// series that land in VictoriaMetrics are indistinguishable from a
// node_exporter deployment -- which is the compatibility promise.
//
// A separate loop rather than a targetLoop because there is nothing to
// scrape: the body comes from process memory, so the failure modes a
// target loop exists to contain (a hung exporter, a slow socket) do not
// exist here. What it does share is the pacing discipline: the interval
// is re-read every round, the start is phase-offset so a fleet does not
// push in lockstep, and a worker slot bounds it against a slow VM like
// any other push.
func (s *Scraper) runSelf(ctx context.Context) {
	if s.cfg.Self == nil {
		return
	}
	// Phase-offset once, keyed on the labels the server hands out: they
	// carry the host identity, which is the one thing guaranteed to
	// differ across a fleet (the push URL is the same everywhere). The
	// offset is taken on the first round that actually has labels --
	// before settings arrive every round is a no-op anyway.
	phased := false
	for {
		timer := time.NewTimer(s.currentSettings().interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.selfChanged:
			timer.Stop()
			continue
		case <-timer.C:
		}
		if set := s.currentSettings(); !phased && set.selfLabels != nil {
			phased = true
			select {
			case <-ctx.Done():
				return
			case <-time.After(phaseOffset(renderLabels(set.selfLabels), set.interval)):
			}
		}
		if err := s.pushSelf(ctx); err != nil && ctx.Err() == nil {
			s.cfg.Log.Warnf("scrape: push node metrics: %v", err)
		}
	}
}

func (s *Scraper) pushSelf(ctx context.Context) error {
	set := s.currentSettings()
	if set.pushURL == "" || set.selfLabels == nil {
		return nil
	}
	body, at := s.cfg.Self()
	if len(body) == 0 {
		return nil
	}
	if !s.acquire() {
		// The same no-queueing rule as a target round: a full pool means
		// pushes are already piling up, and adding this one would stack
		// them further. Metrics tolerate the gap.
		s.cfg.Log.Warnf("scrape: worker pool full, skipping node metrics push")
		return nil
	}
	defer s.release()

	// Gzipped, unlike a target push. Target bodies arrive gzipped from
	// the exporter and pass through untouched; this body is born
	// uncompressed in this process, and at fleet scale an uncompressed
	// ~100KB every interval per host is real bandwidth.
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(body); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	pctx, cancel := context.WithTimeout(ctx, scrapeTimeout(set.interval))
	defer cancel()
	req, err := http.NewRequestWithContext(pctx, http.MethodPost,
		pushRequestURL(set.pushURL, set.selfLabels, at), &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Content-Encoding", "gzip")
	if set.ingestToken != "" {
		req.Header.Set("Authorization", "Bearer "+set.ingestToken)
	}
	resp, err := s.remote.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return nil
}
