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

	mMemFree       = "node_memory_MemFree_bytes"
	mPSICPU        = "node_pressure_cpu_waiting_seconds_total"
	mPSIMem        = "node_pressure_memory_waiting_seconds_total"
	mPSIIO         = "node_pressure_io_waiting_seconds_total"
	mCtxSwitches   = "node_context_switches_total"
	mInterrupts    = "node_intr_total"
	mSwapIn        = "node_vmstat_pswpin"
	mSwapOut       = "node_vmstat_pswpout"
	mPgFault       = "node_vmstat_pgfault"
	mPgMajFault    = "node_vmstat_pgmajfault"
	mOOMKill       = "node_vmstat_oom_kill"
	mFSFiles       = "node_filesystem_files"
	mFSFilesFree   = "node_filesystem_files_free"
	mDiskReadOps   = "node_disk_reads_completed_total"
	mDiskWriteOps  = "node_disk_writes_completed_total"
	mDiskReadTime  = "node_disk_read_time_seconds_total"
	mDiskWriteTime = "node_disk_write_time_seconds_total"
	mNetRxPkts     = "node_network_receive_packets_total"
	mNetTxPkts     = "node_network_transmit_packets_total"
	mNetRxErrs     = "node_network_receive_errs_total"
	mNetTxErrs     = "node_network_transmit_errs_total"
	mNetRxDrops    = "node_network_receive_drop_total"
	mNetTxDrops    = "node_network_transmit_drop_total"
	mTCPRetrans    = "node_netstat_Tcp_RetransSegs"
	mTCPInUse      = "node_sockstat_TCP_inuse"
	mSocketsUsed   = "node_sockstat_sockets_used"
	mConntrack     = "node_nf_conntrack_entries"
	mConntrackMax  = "node_nf_conntrack_entries_limit"
	mTime          = "node_time_seconds"
	mProcsRunning  = "node_procs_running"
	mProcsBlocked  = "node_procs_blocked"
	mFDAllocated   = "node_filefd_allocated"
	mFDMaximum     = "node_filefd_maximum"
	mTimexOffset   = "node_timex_offset_seconds"
	mTimexSync     = "node_timex_sync_status"
	mHwmonTemp     = "node_hwmon_temp_celsius"

	// Label dimensions.
	lMode       = "mode"
	lDevice     = "device"
	lMountpoint = "mountpoint"
	lFSType     = "fstype"
	lKind       = "kind"
	lChip       = "chip"
	lSensor     = "sensor"

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
	//
	// The coverage test carries four slots of slack, and that slack is
	// what makes the flagship "last hour at 15s" request work at all:
	// the newest sample lags the caller's clock by up to one sampling
	// period, so a from of now-1h sits just past the tier's exact span
	// and an exact test would silently downgrade every such window to
	// 60s. Buckets the slack admits but the tier does not hold answer
	// null -- a sliver of gap at the chart's left edge, against the
	// whole window losing its resolution.
	coarseMs := CoarseStep.Milliseconds()
	fine := int64(stepSec)*1000 < coarseMs &&
		fromMs >= r.newestMs-int64(fineSlots+4)*FineStep.Milliseconds()

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
	r.buildPressure(g, want, add)
	r.buildSockets(g, want, add)
	r.buildSystem(g, want, add)
	return out
}

type addFunc func(name string, labels map[string]string, values []agenttypes.MetricValue)

// The four shapes almost every series takes. Written once because the
// alternative is thirty near-identical fifteen-line blocks, in which a
// transposed metric name is invisible.

// addGauge adds an unlabelled gauge, optionally scaled.
func (r *Ring) addGauge(g grid, want func(string) bool, add addFunc, chart, metric string, scale float64) {
	if !want(chart) {
		return
	}
	s := r.find(metric)
	if s == nil {
		return
	}
	vs := make([]agenttypes.MetricValue, g.count)
	for i := range vs {
		vs[i] = agenttypes.MetricValue(s.gauge(g, i) * scale)
	}
	add(chart, nil, vs)
}

// addRate adds an unlabelled counter as a per-second rate, optionally
// scaled (PSI wants x100 to read as a percentage).
func (r *Ring) addRate(g grid, want func(string) bool, add addFunc, chart, metric string, scale float64) {
	if !want(chart) {
		return
	}
	s := r.find(metric)
	if s == nil {
		return
	}
	vs := make([]agenttypes.MetricValue, g.count)
	for i := range vs {
		vs[i] = agenttypes.MetricValue(s.rate(g, i) * scale)
	}
	add(chart, nil, vs)
}

// addRatioPct adds 100 * num/den from two unlabelled gauges.
func (r *Ring) addRatioPct(g grid, want func(string) bool, add addFunc, chart, num, den string) {
	if !want(chart) {
		return
	}
	n, d := r.find(num), r.find(den)
	if n == nil || d == nil {
		return
	}
	vs := make([]agenttypes.MetricValue, g.count)
	for i := range vs {
		vs[i] = agenttypes.MetricValueNone
		nv, dv := n.gauge(g, i), d.gauge(g, i)
		if math.IsNaN(nv) || math.IsNaN(dv) || dv <= 0 {
			continue
		}
		vs[i] = agenttypes.MetricValue(clampPct(100 * nv / dv))
	}
	add(chart, nil, vs)
}

// addDevRate adds one per-second rate per value of the given label.
// keep, when set, drops label values that do not belong on the chart.
func (r *Ring) addDevRate(g grid, want func(string) bool, add addFunc, chart, metric, label string, keep func(string) bool) {
	if !want(chart) {
		return
	}
	byLabel, names := r.group(metric, label)
	for _, d := range names {
		if keep != nil && !keep(d) {
			continue
		}
		vs := make([]agenttypes.MetricValue, g.count)
		for i := range vs {
			vs[i] = agenttypes.MetricValue(byLabel[d].rate(g, i))
		}
		add(chart, map[string]string{label: d}, vs)
	}
}

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
	r.addRate(g, want, add, agenttypes.SeriesCtxSwitches, mCtxSwitches, 1)
	r.addRate(g, want, add, agenttypes.SeriesInterrupts, mInterrupts, 1)

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

	// The composition behind the percentage. Plain gauges: MemTotal minus
	// free/buffers/cached is what an operator adds up by eye, and doing
	// the subtraction here would hide which term moved.
	r.addGauge(g, want, add, agenttypes.SeriesMemTotal, mMemTotal, 1)
	r.addGauge(g, want, add, agenttypes.SeriesMemFree, mMemFree, 1)
	r.addGauge(g, want, add, agenttypes.SeriesMemBuffers, mMemBuffers, 1)
	r.addGauge(g, want, add, agenttypes.SeriesMemCached, mMemCached, 1)

	r.addRate(g, want, add, agenttypes.SeriesSwapInPps, mSwapIn, 1)
	r.addRate(g, want, add, agenttypes.SeriesSwapOutPps, mSwapOut, 1)
	r.buildPageFaults(g, want, add)
	r.buildOOMKills(g, want, add)

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

// buildPageFaults emits minor and major faults as one series labelled by
// kind. Minor faults are ordinary -- a healthy host does millions -- and
// carry no information on their own; they are here so the major line has
// something to be read against.
func (r *Ring) buildPageFaults(g grid, want func(string) bool, add addFunc) {
	if !want(agenttypes.SeriesPageFaults) {
		return
	}
	for _, p := range []struct{ kind, metric string }{
		{"minor", mPgFault},
		{"major", mPgMajFault},
	} {
		s := r.find(p.metric)
		if s == nil {
			continue
		}
		vs := make([]agenttypes.MetricValue, g.count)
		for i := range vs {
			vs[i] = agenttypes.MetricValue(s.rate(g, i))
		}
		add(agenttypes.SeriesPageFaults, map[string]string{lKind: p.kind}, vs)
	}
}

// buildOOMKills counts kills per BUCKET rather than per second. The
// series exists so that one kill is visible; divided by a 60s bucket it
// would be 0.016 and round away on the chart.
func (r *Ring) buildOOMKills(g grid, want func(string) bool, add addFunc) {
	if !want(agenttypes.SeriesOOMKills) {
		return
	}
	s := r.find(mOOMKill)
	if s == nil {
		return
	}
	vs := make([]agenttypes.MetricValue, g.count)
	for i := range vs {
		vs[i] = agenttypes.MetricValue(s.rise(g, i))
	}
	add(agenttypes.SeriesOOMKills, nil, vs)
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

// buildInodes emits inode usage per mountpoint. Its own walk rather than
// a branch inside buildFilesystems: the inode metrics are a separate pair
// with their own mountpoint set (a filesystem can report bytes and not
// inodes), and threading a second optional pair through that loop made
// the byte path harder to read than this whole function.
func (r *Ring) buildInodes(g grid, want func(string) bool, add addFunc) {
	if !want(agenttypes.SeriesFSInodesPct) {
		return
	}
	files, mounts := r.group(mFSFiles, lMountpoint)
	free, _ := r.group(mFSFilesFree, lMountpoint)
	for _, mp := range mounts {
		if !agenttypes.RealFSType(files[mp].Label(lFSType)) {
			continue
		}
		f, ok := free[mp]
		if !ok {
			continue
		}
		vs := make([]agenttypes.MetricValue, g.count)
		for i := range vs {
			vs[i] = agenttypes.MetricValueNone
			t, a := files[mp].gauge(g, i), f.gauge(g, i)
			// A filesystem with no inode table (btrfs, xfs with dynamic
			// allocation) reports 0 total. That is "not applicable", not
			// "completely full".
			if math.IsNaN(t) || math.IsNaN(a) || t <= 0 {
				continue
			}
			vs[i] = agenttypes.MetricValue(clampPct(100 * (t - a) / t))
		}
		add(agenttypes.SeriesFSInodesPct, map[string]string{lMountpoint: mp}, vs)
	}
}

func (r *Ring) buildFilesystems(g grid, want func(string) bool, add addFunc) {
	r.buildInodes(g, want, add)

	wantPct, wantSize := want(agenttypes.SeriesFSUsedPct), want(agenttypes.SeriesFSSize)
	if !wantPct && !wantSize {
		return
	}
	sizes, mounts := r.group(mFSSize, lMountpoint)
	avails, _ := r.group(mFSAvail, lMountpoint)
	for _, mp := range mounts {
		size := sizes[mp]
		// The same definition of "this machine's storage" the summary's
		// disk gauge and the reported filesystem list use. Charting a
		// tmpfs the gauge excluded gives an operator two disk answers
		// that disagree, with nothing on the page saying which is which.
		if !agenttypes.RealFSType(size.Label(lFSType)) {
			continue
		}
		labels := map[string]string{lMountpoint: mp}
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
		{agenttypes.SeriesDiskReadIOPS, mDiskReadOps},
		{agenttypes.SeriesDiskWriteIOPS, mDiskWriteOps},
	} {
		r.addDevRate(g, want, add, p.chart, p.metric, lDevice, nil)
	}

	// Mean seconds per operation: the rise in accumulated service time
	// divided by the rise in operation count over the SAME bucket. Not
	// derivable from either alone, which is why both counters exist.
	for _, p := range []struct{ chart, timeMetric, opsMetric string }{
		{agenttypes.SeriesDiskReadWait, mDiskReadTime, mDiskReadOps},
		{agenttypes.SeriesDiskWriteWait, mDiskWriteTime, mDiskWriteOps},
	} {
		if !want(p.chart) {
			continue
		}
		times, names := r.group(p.timeMetric, lDevice)
		ops, _ := r.group(p.opsMetric, lDevice)
		for _, d := range names {
			o, ok := ops[d]
			if !ok {
				continue
			}
			vs := make([]agenttypes.MetricValue, g.count)
			for i := range vs {
				vs[i] = agenttypes.MetricValueNone
				secs, n := times[d].rise(g, i), o.rise(g, i)
				// An idle device did zero operations. Its latency is not
				// zero, it is undefined -- and a flat zero line reads as
				// "instant", the opposite of what is worth knowing.
				if math.IsNaN(secs) || math.IsNaN(n) || n <= 0 {
					continue
				}
				vs[i] = agenttypes.MetricValue(secs / n)
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
		{agenttypes.SeriesNetRxPps, mNetRxPkts},
		{agenttypes.SeriesNetTxPps, mNetTxPkts},
		{agenttypes.SeriesNetRxErrs, mNetRxErrs},
		{agenttypes.SeriesNetTxErrs, mNetTxErrs},
		{agenttypes.SeriesNetRxDrops, mNetRxDrops},
		{agenttypes.SeriesNetTxDrops, mNetTxDrops},
	} {
		// Container and bridge plumbing is dropped by the same predicate
		// that shapes the summary's rx/tx and the reported NIC list. A
		// packet crossing a bridge is counted again on every veth it
		// traverses, so leaving them in draws a host pushing multiples of
		// its real traffic -- and on this test host it turned six new
		// series into sixty lines.
		r.addDevRate(g, want, add, p.chart, p.metric, lDevice, agenttypes.RealNetDevice)
	}
}

// buildPressure emits the PSI shares. Each is a counter of stalled
// seconds, so its rise per second of wall clock IS the stalled fraction
// -- the same derivation disk utilisation uses, times 100 to read as a
// percentage.
func (r *Ring) buildPressure(g grid, want func(string) bool, add addFunc) {
	for _, p := range []struct{ chart, metric string }{
		{agenttypes.SeriesPSICPUPct, mPSICPU},
		{agenttypes.SeriesPSIMemPct, mPSIMem},
		{agenttypes.SeriesPSIIOPct, mPSIIO},
	} {
		r.addRate(g, want, add, p.chart, p.metric, 100)
	}
}

func (r *Ring) buildSockets(g grid, want func(string) bool, add addFunc) {
	r.addRate(g, want, add, agenttypes.SeriesTCPRetrans, mTCPRetrans, 1)
	r.addGauge(g, want, add, agenttypes.SeriesTCPInUse, mTCPInUse, 1)
	r.addGauge(g, want, add, agenttypes.SeriesSocketsUsed, mSocketsUsed, 1)
	r.addRatioPct(g, want, add, agenttypes.SeriesConntrackPct, mConntrack, mConntrackMax)
}

func (r *Ring) buildSystem(g grid, want func(string) bool, add addFunc) {
	r.addGauge(g, want, add, agenttypes.SeriesProcsRunning, mProcsRunning, 1)
	r.addGauge(g, want, add, agenttypes.SeriesProcsBlocked, mProcsBlocked, 1)
	r.addRatioPct(g, want, add, agenttypes.SeriesFDUsedPct, mFDAllocated, mFDMaximum)
	r.addGauge(g, want, add, agenttypes.SeriesTimeDriftSec, mTimexOffset, 1)
	r.addGauge(g, want, add, agenttypes.SeriesTimeSynced, mTimexSync, 1)
	r.buildUptime(g, want, add)
	r.buildTemperature(g, want, add)
}

// buildUptime derives seconds since boot from the two clock gauges the
// same sample carries, so the answer is dated by the machine's own view
// of both and needs no clock from here.
func (r *Ring) buildUptime(g grid, want func(string) bool, add addFunc) {
	if !want(agenttypes.SeriesUptimeSec) {
		return
	}
	now, boot := r.find(mTime), r.find(mBootTime)
	if now == nil || boot == nil {
		return
	}
	vs := make([]agenttypes.MetricValue, g.count)
	for i := range vs {
		vs[i] = agenttypes.MetricValueNone
		n, b := now.gauge(g, i), boot.gauge(g, i)
		if math.IsNaN(n) || math.IsNaN(b) || n < b {
			continue
		}
		vs[i] = agenttypes.MetricValue(n - b)
	}
	add(agenttypes.SeriesUptimeSec, nil, vs)
}

// buildTemperature emits one series per sensor. Chip AND sensor, because
// a board reports several sensors per chip and either label alone
// collapses them onto one line.
func (r *Ring) buildTemperature(g grid, want func(string) bool, add addFunc) {
	if !want(agenttypes.SeriesTempCelsius) {
		return
	}
	for _, s := range r.matching(mHwmonTemp) {
		vs := make([]agenttypes.MetricValue, g.count)
		for i := range vs {
			vs[i] = agenttypes.MetricValue(s.gauge(g, i))
		}
		add(agenttypes.SeriesTempCelsius, map[string]string{
			lChip:   s.Label(lChip),
			lSensor: s.Label(lSensor),
		}, vs)
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
