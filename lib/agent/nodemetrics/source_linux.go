//go:build linux

package nodemetrics

import (
	"fmt"
	"io"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/node_exporter/collector"
)

// source runs node_exporter's own collector set, in this process.
//
// The requirement is node_exporter's metric surface -- a concrete output
// defined by somebody else's code. There are only two honest ways to
// produce that: run their code, or rewrite it. Rewriting was tried here
// (seven families over prometheus/procfs) and rejected: it can never
// finish (hwmon, edac, zfs and ethtool are not in /proc), and every
// upstream fix -- like the filesystem collector's mount-timeout that
// keeps a wedged NFS mount from hanging the sampler -- has to be
// re-learned the hard way. Since the agent must stay one binary with no
// child processes, running their code means importing it. Grafana Alloy
// ships its node metrics exactly this way (prometheus.exporter.unix
// embeds this package), so the pattern carries fleet-scale precedent.
//
// The import is heavy -- it brings node_exporter's dependency tree into
// the shared agent lib -- and that is accepted deliberately: the binary
// grows by tens of megabytes, and in exchange the metric set, its
// filters and its kernel-version quirks are all pinned to upstream
// instead of to anyone's judgement here.
type source struct {
	log Logger
	reg *prometheus.Registry
	// lastGatherErr keeps a repeating gather error from logging once per
	// round forever; it logs when the message changes.
	lastGatherErr string
}

// newSource drives the collector package's global kingpin flags once and
// builds the collector set with every default intact.
//
// The flag dance is the embedding cost: registerCollector wires each
// collector's enablement to a kingpin.Flag, and those flags only take
// their default values when the parser RUNS. The agent's real command
// line never goes anywhere near this -- the agent parses its own flags
// with the standard library -- so what is parsed here is an empty, constant
// argument list: every collector at its upstream default, proven once in
// CI rather than trusted at ten thousand boots. Terminate(nil) unhooks
// kingpin's os.Exit so a failure is an error return, not a dead agent.
func newSource(log Logger) (*source, error) {
	app := kingpin.CommandLine
	app.Terminate(nil)
	app.UsageWriter(io.Discard)
	app.ErrorWriter(io.Discard)
	if _, err := app.Parse(nil); err != nil {
		return nil, fmt.Errorf("apply collector defaults: %w", err)
	}

	nc, err := collector.NewNodeCollector(slogTo(log))
	if err != nil {
		return nil, fmt.Errorf("build node collector: %w", err)
	}
	// A private registry: nothing else may register into it, and the
	// go/process self-metrics the default registry carries stay out of
	// the ring.
	reg := prometheus.NewRegistry()
	if err := reg.Register(nc); err != nil {
		return nil, fmt.Errorf("register node collector: %w", err)
	}
	return &source{log: log, reg: reg}, nil
}

// collect runs every collector once and converts the result.
//
// Gather returns partial results alongside its error, and partial is
// exactly what is wanted: one broken collector must not cost the round.
// Per-collector failures are additionally visible in the data itself,
// as node_scrape_collector_success{collector=...} 0.
func (s *source) collect(atMs int64) Sample {
	fams, err := s.reg.Gather()
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if msg != s.lastGatherErr {
		if msg != "" {
			s.log.Warnf("node metrics: gather: %s", msg)
		}
		s.lastGatherErr = msg
	}
	return familiesToSample(atMs, fams)
}
