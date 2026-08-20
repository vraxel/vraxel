// Package nodemetrics is the agent's built-in host metrics collector:
// it runs node_exporter's own collector set in-process every FineStep,
// keeps a day of history in memory, and answers windowed queries from
// it.
//
// It exists so that installing the agent is the whole installation. The
// alternative -- deploy node_exporter, then a scrape target, then a
// time-series database to receive it -- is three moving parts before a
// host page can draw a CPU line, and the first of them needs a job
// engine to put it there. Running node_exporter's code in-process (see
// source_linux.go for why it is imported rather than reimplemented or
// forked as a child) keeps that promise while producing node_exporter's
// exact metric surface.
//
// # The history lives here, not on the server
//
// The two things this data is used for want opposite shapes. Current
// utilisation is nine numbers per host, wanted for EVERY host at once so
// the host list can sort and filter on it: that has to be central, and
// it rides the heartbeat into a single row per host. History is tens of
// thousands of points per host, wanted for ONE host at a time, whenever
// somebody opens a chart: that is naturally sharded, and shipping it to
// a central store would mean a continuous ingest stream, a retention
// policy, and a write path sized for every host whether or not anyone is
// looking.
//
// So it stays in the ring, and the server reads a window over the data
// channel on demand. Nothing is transmitted while no chart is open. The
// price is stated plainly, because it is real: a host that is offline
// has no history to serve, which is exactly when somebody wants it. The
// last heartbeat's snapshot survives on the server, and root-cause
// analysis after the fact is what the full tier -- node_exporter into
// VictoriaMetrics -- is for.
//
// # Two vocabularies
//
// The ring stores node_exporter's metric names and labels; the query API
// speaks derived chart series (agenttypes.Series*). Keeping both is what
// lets a deployment switch to the full tier without changing a
// dashboard, a PromQL expression or a line of frontend: the stored names
// are the compatibility promise, and the chart names are what both tiers
// translate into.
package nodemetrics

import (
	"context"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// Logger is the minimal logging surface this package needs.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
}

// Config is what the host program supplies.
type Config struct {
	Log Logger
	// SeriesLimit caps how many series this host may hold. Zero uses
	// DefaultSeriesLimit.
	SeriesLimit int
}

// Collector samples the host and owns its ring.
type Collector struct {
	cfg  Config
	ring *Ring
	// lastDropped is the previous round's over-cap count, touched only
	// by Run's goroutine. It exists so a host that is permanently over
	// the cap says so once rather than once every fifteen seconds.
	lastDropped int
}

// New builds a Collector. It reads nothing until Run.
func New(cfg Config) *Collector {
	return &Collector{cfg: cfg, ring: NewRing(cfg.SeriesLimit)}
}

// Run samples until ctx ends. It returns immediately on a platform with
// no /proc, leaving the ring empty and every read answering "no data".
func (c *Collector) Run(ctx context.Context) {
	src, err := newSource(c.cfg.Log)
	if err != nil {
		c.cfg.Log.Warnf("node metrics: %v; this host reports no utilisation", err)
		return
	}

	// Sample once before waiting out the first tick, so a host that has
	// just come up has a reading rather than a blank fifteen seconds.
	// Two rounds are still needed before anything rate-based exists.
	c.round(src)

	t := time.NewTicker(FineStep)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.round(src)
		}
	}
}

func (c *Collector) round(src *source) {
	dropped := c.ring.Add(src.collect(time.Now().UnixMilli()))
	if dropped == c.lastDropped {
		return
	}
	if dropped > 0 {
		c.cfg.Log.Warnf("node metrics: %d series past the %d-series cap are not collected; "+
			"charts for the newest disks or interfaces on this host will be missing",
			dropped, c.ring.limit)
	} else {
		c.cfg.Log.Infof("node metrics: back within the %d-series cap", c.ring.limit)
	}
	c.lastDropped = dropped
}

// Summary is the current-utilisation snapshot for the heartbeat, or nil
// when it cannot be derived in full.
func (c *Collector) Summary() *agenttypes.MetricsSummary { return c.ring.Summary() }

// Query answers one windowed read of the history.
func (c *Collector) Query(fromMs, toMs int64, stepSec int, names []string) (agenttypes.MetricsResult, error) {
	return c.ring.Query(fromMs, toMs, stepSec, names)
}
