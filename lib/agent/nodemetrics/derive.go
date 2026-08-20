package nodemetrics

import (
	"errors"
	"math"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// node_exporter metric names, which is what the ring stores.
//
// The ring speaks node_exporter and the query API speaks chart series
// (agenttypes.Series*), and this file is the only place the two meet.
// Keeping the storage vocabulary is what lets the same collector feed
// VictoriaMetrics later without renaming anything: the compatibility
// promise -- existing dashboards, PromQL and alert rules keep working --
// is a property of these strings.
const (
	mCPU        = "node_cpu_seconds_total"
	mMemTotal   = "node_memory_MemTotal_bytes"
	mMemAvail   = "node_memory_MemAvailable_bytes"
	mMemCached  = "node_memory_Cached_bytes"
	mMemBuffers = "node_memory_Buffers_bytes"
	mSwapTotal  = "node_memory_SwapTotal_bytes"
	mSwapFree   = "node_memory_SwapFree_bytes"
	mLoad1      = "node_load1"
	mLoad5      = "node_load5"
	mLoad15     = "node_load15"
	mFSSize     = "node_filesystem_size_bytes"
	mFSAvail    = "node_filesystem_avail_bytes"
	mDiskRead   = "node_disk_read_bytes_total"
	mDiskWrite  = "node_disk_written_bytes_total"
	mDiskIOTime = "node_disk_io_time_seconds_total"
	mNetRx      = "node_network_receive_bytes_total"
	mNetTx      = "node_network_transmit_bytes_total"
	mBootTime   = "node_boot_time_seconds"

	// Label dimensions.
	lMode       = "mode"
	lDevice     = "device"
	lMountpoint = "mountpoint"
	lFSType     = "fstype"

	// CPU modes that mean "the CPU was available". iowait belongs here:
	// the processor was idle waiting on a device, and counting a slow
	// disk as CPU load sends whoever reads the chart after the wrong
	// resource.
	modeIdle   = "idle"
	modeIOWait = "iowait"
)

var (
	// ErrBadWindow means the requested window is empty or inverted.
	ErrBadWindow = errors.New("metrics: window must be from < to")
	// ErrTooManyPoints means the window divided by the step exceeds
	// MetricsMaxPoints.
	ErrTooManyPoints = errors.New("metrics: too many points for one query")
)

// grid is the output resolution one query resolved to.
type grid struct {
	fine   bool
	fromMs int64
	stepMs int64
	count  int
}

// bounds returns bucket i as the half-open interval (from, to].
//
// Half-open and closed at the TOP, which is what makes a counter's rise
// over consecutive buckets add up to its rise over the whole window with
// no interval counted twice and none skipped.
func (g grid) bounds(i int) (int64, int64) {
	from := g.fromMs + int64(i)*g.stepMs
	return from, from + g.stepMs
}

// gauge reads the series' value in bucket i.
func (s *Series) gauge(g grid, i int) float64 {
	from, to := g.bounds(i)
	return s.tier(g.fine).last(from, to)
}

// rise reads the counter's increase across bucket i.
func (s *Series) rise(g grid, i int) float64 {
	from, to := g.bounds(i)
	v, ok := s.tier(g.fine).rise(from, to)
	if !ok {
		return math.NaN()
	}
	return v
}

// rate reads the counter's increase across bucket i as a per-second
// figure.
//
// The divisor is the bucket's nominal width, not the measured distance
// between the two samples used. They differ by the sampler's jitter,
// which a ticker keeps in the milliseconds; carrying a real timestamp
// per slot to close that gap would double the ring's memory to move a
// chart line by less than a pixel.
func (s *Series) rate(g grid, i int) float64 {
	return s.rise(g, i) / (float64(g.stepMs) / 1000)
}

// Query builds one windowed, downsampled answer.
//
// The window and step are snapped to a tier the ring actually holds and
// echoed back in the result, so the caller states what it wants rather
// than what the agent stores. Snapping the window to the grid (rather
// than starting buckets wherever the request happened to land) is what
// keeps a polling chart still: unaligned buckets shift on every refresh
// and every point moves a little, which reads as noise in the data.
func (r *Ring) Query(fromMs, toMs int64, stepSec int, names []string) (agenttypes.MetricsResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if toMs <= fromMs {
		return agenttypes.MetricsResult{}, ErrBadWindow
	}
	if stepSec <= 0 {
		stepSec = int(CoarseStep / time.Second)
	}

	// The fine tier only covers the last hour, so a window reaching
	// further back has to be answered from the coarse one -- and a caller
	// asking for minute buckets gains nothing from the fine tier anyway.
	coarseMs := CoarseStep.Milliseconds()
	fine := int64(stepSec)*1000 < coarseMs &&
		fromMs >= r.newestMs-int64(fineSlots-1)*FineStep.Milliseconds()

	stepMs := coarseMs
	if fine {
		stepMs = FineStep.Milliseconds()
	}
	if req := int64(stepSec) * 1000; req > stepMs {
		stepMs = alignUp(req, stepMs)
	}

	g := grid{fine: fine, fromMs: fromMs / stepMs * stepMs, stepMs: stepMs}
	g.count = int((alignUp(toMs, stepMs) - g.fromMs) / stepMs)
	if g.count <= 0 {
		return agenttypes.MetricsResult{}, ErrBadWindow
	}
	if g.count > agenttypes.MetricsMaxPoints {
		return agenttypes.MetricsResult{}, ErrTooManyPoints
	}

	out := agenttypes.MetricsResult{
		FromMs:  g.fromMs,
		StepSec: int(stepMs / 1000),
		Count:   g.count,
	}
	out.Series = r.build(g, wanted(names))
	return out, nil
}

// wanted turns the requested name list into a predicate. An empty list
// means everything, which is what the host detail page asks for.
func wanted(names []string) func(string) bool {
	if len(names) == 0 {
		return func(string) bool { return true }
	}
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return func(n string) bool { _, ok := set[n]; return ok }
}

// build derives every requested chart series from the stored
// node_exporter ones. Callers hold r.mu.
func (r *Ring) build(g grid, want func(string) bool) []agenttypes.MetricsSeries {
	var out []agenttypes.MetricsSeries
	add := func(name string, labels map[string]string, values []agenttypes.MetricValue) {
		// A series with nothing in the window is left out rather than
		// returned as a row of nulls: an empty line in a chart legend
		// says "this exists but is broken", when the truth is usually
		// that the disk was mounted after the window began.
		for _, v := range values {
			if !v.IsNone() {
				out = append(out, agenttypes.MetricsSeries{Name: name, Labels: labels, Values: values})
				return
			}
		}
	}

	r.buildCPU(g, want, add)
	r.buildMemory(g, want, add)
	r.buildLoad(g, want, add)
	r.buildFilesystems(g, want, add)
	r.buildDisks(g, want, add)
	r.buildNetwork(g, want, add)
	return out
}

type addFunc func(name string, labels map[string]string, values []agenttypes.MetricValue)

// cpuModeRises sums each CPU mode's rise across every series carrying
// it -- one per core, since node_exporter's cpu collector labels by
// {cpu, mode} -- plus the all-mode totals the shares are measured
// against. A mode's bucket is NaN when no core reported in it.
//
// The summing is what makes per-core storage compatible with whole-host
// derivation: grouping by mode alone would keep one arbitrary core's
// series per mode, and the "CPU usage" of a 64-core machine would be
// whichever core happened to be stored last.
func (r *Ring) cpuModeRises(g grid) (map[string][]float64, []string, []float64) {
	byMode, names := r.groupAll(mCPU, lMode)
	if len(byMode) == 0 {
		return nil, nil, nil
	}
	rises := make(map[string][]float64, len(byMode))
	totals := make([]float64, g.count)
	for _, name := range names {
		vs := make([]float64, g.count)
		for i := range vs {
			sum, any := 0.0, false
			for _, s := range byMode[name] {
				if v := s.rise(g, i); !math.IsNaN(v) {
					sum += v
					any = true
				}
			}
			if !any {
				vs[i] = math.NaN()
				continue
			}
			vs[i] = sum
			totals[i] += sum
		}
		rises[name] = vs
	}
	return rises, names, totals
}

// cpuUsedValues folds mode rises into the used-percentage series.
func cpuUsedValues(count int, rises map[string][]float64, totals []float64) []agenttypes.MetricValue {
	used := make([]agenttypes.MetricValue, count)
	for i := range used {
		used[i] = agenttypes.MetricValueNone
		if totals[i] <= 0 {
			continue
		}
		idle := nonNaN(rises[modeIdle], i) + nonNaN(rises[modeIOWait], i)
		used[i] = agenttypes.MetricValue(clampPct(100 * (1 - idle/totals[i])))
	}
	return used
}

func (r *Ring) buildCPU(g grid, want func(string) bool, add addFunc) {
	wantUsed, wantMode := want(agenttypes.SeriesCPUUsedPct), want(agenttypes.SeriesCPUModePct)
	if !wantUsed && !wantMode {
		return
	}
	rises, names, totals := r.cpuModeRises(g)
	if rises == nil {
		return
	}

	if wantUsed {
		add(agenttypes.SeriesCPUUsedPct, nil, cpuUsedValues(g.count, rises, totals))
	}

	if wantMode {
		for _, name := range names {
			vs := make([]agenttypes.MetricValue, g.count)
			for i := range vs {
				vs[i] = agenttypes.MetricValueNone
				if totals[i] > 0 && !math.IsNaN(rises[name][i]) {
					vs[i] = agenttypes.MetricValue(clampPct(100 * rises[name][i] / totals[i]))
				}
			}
			add(agenttypes.SeriesCPUModePct, map[string]string{lMode: name}, vs)
		}
	}
}

func (r *Ring) buildMemory(g grid, want func(string) bool, add addFunc) {
	total, avail := r.find(mMemTotal), r.find(mMemAvail)
	if total != nil && avail != nil {
		wantPct, wantBytes := want(agenttypes.SeriesMemUsedPct), want(agenttypes.SeriesMemUsed)
		if wantPct || wantBytes {
			pct := make([]agenttypes.MetricValue, g.count)
			used := make([]agenttypes.MetricValue, g.count)
			for i := range pct {
				pct[i], used[i] = agenttypes.MetricValueNone, agenttypes.MetricValueNone
				t, a := total.gauge(g, i), avail.gauge(g, i)
				if math.IsNaN(t) || math.IsNaN(a) || t <= 0 {
					continue
				}
				used[i] = agenttypes.MetricValue(t - a)
				pct[i] = agenttypes.MetricValue(clampPct(100 * (t - a) / t))
			}
			if wantPct {
				add(agenttypes.SeriesMemUsedPct, nil, pct)
			}
			if wantBytes {
				add(agenttypes.SeriesMemUsed, nil, used)
			}
		}
	}

	if !want(agenttypes.SeriesSwapUsed) {
		return
	}
	swTotal, swFree := r.find(mSwapTotal), r.find(mSwapFree)
	if swTotal == nil || swFree == nil {
		return
	}
	vs := make([]agenttypes.MetricValue, g.count)
	for i := range vs {
		vs[i] = agenttypes.MetricValueNone
		t, f := swTotal.gauge(g, i), swFree.gauge(g, i)
		// A host with swap off reports SwapTotal 0. Zero is the honest
		// answer to "how much swap is in use", not a division by zero.
		if math.IsNaN(t) || math.IsNaN(f) {
			continue
		}
		if t <= 0 {
			vs[i] = 0
			continue
		}
		vs[i] = agenttypes.MetricValue(clampPct(100 * (t - f) / t))
	}
	add(agenttypes.SeriesSwapUsed, nil, vs)
}

func (r *Ring) buildLoad(g grid, want func(string) bool, add addFunc) {
	for _, p := range []struct{ chart, metric string }{
		{agenttypes.SeriesLoad1, mLoad1},
		{agenttypes.SeriesLoad5, mLoad5},
		{agenttypes.SeriesLoad15, mLoad15},
	} {
		s := r.find(p.metric)
		if s == nil || !want(p.chart) {
			continue
		}
		vs := make([]agenttypes.MetricValue, g.count)
		for i := range vs {
			vs[i] = agenttypes.MetricValue(s.gauge(g, i))
		}
		add(p.chart, nil, vs)
	}
}

func (r *Ring) buildFilesystems(g grid, want func(string) bool, add addFunc) {
	wantPct, wantSize := want(agenttypes.SeriesFSUsedPct), want(agenttypes.SeriesFSSize)
	if !wantPct && !wantSize {
		return
	}
	sizes, mounts := r.group(mFSSize, lMountpoint)
	avails, _ := r.group(mFSAvail, lMountpoint)
	for _, mp := range mounts {
		labels := map[string]string{lMountpoint: mp}
		size := sizes[mp]
		if wantSize {
			vs := make([]agenttypes.MetricValue, g.count)
			for i := range vs {
				vs[i] = agenttypes.MetricValue(size.gauge(g, i))
			}
			add(agenttypes.SeriesFSSize, labels, vs)
		}
		avail, ok := avails[mp]
		if !wantPct || !ok {
			continue
		}
		vs := make([]agenttypes.MetricValue, g.count)
		for i := range vs {
			vs[i] = agenttypes.MetricValueNone
			t, a := size.gauge(g, i), avail.gauge(g, i)
			if math.IsNaN(t) || math.IsNaN(a) || t <= 0 {
				continue
			}
			vs[i] = agenttypes.MetricValue(clampPct(100 * (t - a) / t))
		}
		add(agenttypes.SeriesFSUsedPct, labels, vs)
	}
}

func (r *Ring) buildDisks(g grid, want func(string) bool, add addFunc) {
	for _, p := range []struct{ chart, metric string }{
		{agenttypes.SeriesDiskReadBps, mDiskRead},
		{agenttypes.SeriesDiskWriteBps, mDiskWrite},
	} {
		if !want(p.chart) {
			continue
		}
		devs, names := r.group(p.metric, lDevice)
		for _, d := range names {
			vs := make([]agenttypes.MetricValue, g.count)
			for i := range vs {
				vs[i] = agenttypes.MetricValue(devs[d].rate(g, i))
			}
			add(p.chart, map[string]string{lDevice: d}, vs)
		}
	}

	if !want(agenttypes.SeriesDiskUtilPct) {
		return
	}
	// io_time is seconds of wall clock during which the device had at
	// least one request in flight, so its rise per second of wall clock
	// IS the busy fraction: no division by anything else, and it is
	// capped at 100 because a device with a deep queue can accumulate
	// slightly more than one second per second across rounding.
	devs, names := r.group(mDiskIOTime, lDevice)
	for _, d := range names {
		vs := make([]agenttypes.MetricValue, g.count)
		for i := range vs {
			vs[i] = agenttypes.MetricValue(clampPct(100 * devs[d].rate(g, i)))
		}
		add(agenttypes.SeriesDiskUtilPct, map[string]string{lDevice: d}, vs)
	}
}

func (r *Ring) buildNetwork(g grid, want func(string) bool, add addFunc) {
	for _, p := range []struct{ chart, metric string }{
		{agenttypes.SeriesNetRxBps, mNetRx},
		{agenttypes.SeriesNetTxBps, mNetTx},
	} {
		if !want(p.chart) {
			continue
		}
		devs, names := r.group(p.metric, lDevice)
		for _, d := range names {
			vs := make([]agenttypes.MetricValue, g.count)
			for i := range vs {
				vs[i] = agenttypes.MetricValue(devs[d].rate(g, i))
			}
			add(p.chart, map[string]string{lDevice: d}, vs)
		}
	}
}

// nonNaN reads vs[i] treating an absent value as zero. Used only where
// the caller has already established that the bucket has data, so an
// absent mode means that mode contributed nothing rather than that the
// bucket is unknown.
func nonNaN(vs []float64, i int) float64 {
	if vs == nil || math.IsNaN(vs[i]) {
		return 0
	}
	return vs[i]
}

// clampPct holds a percentage inside [0,100].
//
// The arithmetic can leave it slightly outside: counters are read a few
// microseconds apart rather than simultaneously, and a device can report
// marginally more busy time than wall clock. Showing 100.4% would make a
// reader doubt every other number on the page.
func clampPct(v float64) float64 {
	switch {
	case math.IsNaN(v):
		return v
	case v < 0:
		return 0
	case v > 100:
		return 100
	}
	return v
}
