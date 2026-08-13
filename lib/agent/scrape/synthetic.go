package scrape

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// runSynthetic pushes the two series promscrape would have generated:
// per-target `up` and `scrape_duration_seconds`.
//
// They matter more here than in a direct-scrape setup. `up` is what
// distinguishes "the exporter is down" from "no data", and with
// collection moved onto the host it is the only signal that survives an
// exporter dying while the agent stays healthy. (The agent itself dying
// is covered from the other side: the server writes the agent-up metric from
// heartbeats, design §6.4.)
//
// Only these two. Anything else promscrape reports (scrape_samples_*)
// would require parsing the exposition body, and not parsing it is the
// property this whole path is built on.
//
// One request for all targets, not one per target: at ten thousand hosts
// a second push per target would double the request rate on VM to buy
// nothing, since the batch is a few hundred bytes either way.
//
// The period is re-read every round rather than fixed at startup: when
// this goroutine starts, no target list has arrived yet, so the interval
// is still the default. A ticker built then would keep the default
// forever and ignore whatever period the server asked for.
func (s *Scraper) runSynthetic(ctx context.Context) {
	for {
		timer := time.NewTimer(s.currentSettings().interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.settingsChanged:
			// New interval: re-arm rather than serve out the old wait.
			timer.Stop()
			continue
		case <-timer.C:
		}
		if err := s.pushSynthetic(ctx); err != nil && ctx.Err() == nil {
			s.cfg.Log.Warnf("scrape: push synthetic series: %v", err)
		}
	}
}

func (s *Scraper) pushSynthetic(ctx context.Context) error {
	set := s.currentSettings()
	if set.pushURL == "" {
		return nil
	}

	s.mu.Lock()
	results := make(map[string]result, len(s.results))
	for k, v := range s.results {
		results[k] = v
	}
	s.mu.Unlock()
	if len(results) == 0 {
		return nil
	}

	body := buildSyntheticBody(results)
	// No extra_label here: these lines carry their labels inline, because
	// one request covers targets with different label sets.
	pctx, cancel := context.WithTimeout(ctx, scrapeTimeout(set.interval))
	defer cancel()

	req, err := http.NewRequestWithContext(pctx, http.MethodPost,
		pushRequestURL(set.pushURL, nil, time.Now()), strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
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

// buildSyntheticBody renders the exposition text. Targets are emitted in
// URL order so the output is deterministic and diffable in tests.
func buildSyntheticBody(results map[string]result) string {
	urls := make([]string, 0, len(results))
	for u := range results {
		urls = append(urls, u)
	}
	sort.Strings(urls)

	var b strings.Builder
	for _, u := range urls {
		r := results[u]
		labels := renderLabels(r.labels)
		up := 0
		if r.up {
			up = 1
		}
		fmt.Fprintf(&b, "up%s %d\n", labels, up)
		fmt.Fprintf(&b, "scrape_duration_seconds%s %f\n", labels, r.duration.Seconds())
	}
	return b.String()
}

// renderLabels renders a label set as {k="v",...}, sorted.
func renderLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteString(`="`)
		b.WriteString(escapeLabelValue(labels[k]))
		b.WriteString(`"`)
	}
	b.WriteByte('}')
	return b.String()
}

// escapeLabelValue applies the exposition format's three escapes. A
// hostname with a quote in it is not realistic, but an unescaped one
// would corrupt the whole batch rather than one line.
func escapeLabelValue(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(v)
}
