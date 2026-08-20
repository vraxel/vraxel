package nodemetrics

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// FineStep is both the sampling period and the high-resolution tier's
	// bucket width. One constant for both because they have to be equal:
	// a tier finer than the sampling rate stores holes, and a tier coarser
	// than it throws away samples that were already paid for.
	FineStep = 15 * time.Second
	// fineSlots covers one hour at FineStep.
	fineSlots = 240

	// CoarseStep / coarseSlots cover 24 hours. Two tiers rather than one
	// because the two things this data is for want opposite resolutions:
	// "what is happening right now" wants seconds and only needs minutes
	// of depth, and "was this box busy overnight" wants a day of depth and
	// cannot tell 15s from 60s on a 400-pixel chart. A single 24h tier at
	// 15s would cost four times the memory to answer the second question
	// no better.
	CoarseStep  = 60 * time.Second
	coarseSlots = 1440

	// DefaultSeriesLimit caps how many series one host may hold.
	//
	// The bound exists because the series count is NOT under this code's
	// control: it grows with cores, mounts and network interfaces, and
	// the collector set runs with node_exporter's own defaults, which
	// chart every veth a Kubernetes node creates. A typical server lands
	// between six hundred and fifteen hundred series (8-20MB of ring); a
	// busy k8s node a few thousand. The cap is not a target, it is the
	// point past which this stops being a host page and the answer is
	// the full tier -- at 13KB a series it also bounds the ring near
	// 100MB, which is as much memory as an agent may ever take.
	DefaultSeriesLimit = 8192
)

// Kind says how a stored value must be read back. It is a property of
// the metric, decided once by the collector, because the derivation
// layer cannot tell a counter from a gauge by looking at values --
// a gauge that only goes up looks exactly like a counter.
type Kind uint8

const (
	// Gauge is read as its last value in the bucket.
	Gauge Kind = iota
	// Counter is read as the rise across the bucket, corrected for
	// resets.
	Counter
)

// Label is one dimension of a series. Order does not matter to
// correctness -- the key builder sorts -- but the collector emits them
// sorted anyway so the sort is free.
type Label struct{ Name, Value string }

// Point is one reading in a Sample, named and labelled the way
// node_exporter names it.
type Point struct {
	Name   string
	Labels []Label
	Kind   Kind
	Value  float64
}

// Sample is one collection round.
type Sample struct {
	AtMs   int64
	Points []Point
}

// Series is one metric's history, in two resolutions.
//
// Values are float64, not float32. The tempting saving is real -- a
// thousand series times 1680 slots is 13MB one way and half that the
// other -- and it is a trap: float32 carries 24 bits of mantissa, so a network
// interface's cumulative byte counter loses single-byte resolution at
// 16MB and whole-kilobyte resolution around 16GB. Differencing two such
// values over 15 seconds then yields quantisation noise larger than the
// throughput being measured. The counters this holds are exactly the
// ones that outgrow float32 within hours of boot.
type Series struct {
	Name   string
	Labels []Label
	Kind   Kind

	fine   tier
	coarse tier
}

func newSeries(p Point) *Series {
	return &Series{
		Name:   p.Name,
		Labels: append([]Label(nil), p.Labels...),
		Kind:   p.Kind,
		fine:   newTier(FineStep, fineSlots),
		coarse: newTier(CoarseStep, coarseSlots),
	}
}

func (s *Series) put(atMs int64, v float64) {
	s.fine.put(atMs, v)
	s.coarse.put(atMs, v)
}

func (s *Series) tier(fine bool) *tier {
	if fine {
		return &s.fine
	}
	return &s.coarse
}

// Label returns the value of one dimension, or "".
func (s *Series) Label(name string) string {
	for _, l := range s.Labels {
		if l.Name == name {
			return l.Value
		}
	}
	return ""
}

// tier is a fixed-resolution ring over a regular time grid.
//
// Timestamps are not stored per slot. The grid is regular by
// construction, so a slot's time is derived from the newest slot's time
// and the distance back to it -- which is the whole reason for a fixed
// grid, and what makes 1680 samples per series cost 1680 float64s and
// nothing else.
type tier struct {
	stepMs int64
	values []float64
	// lastIdx holds the newest write, -1 before the first one; lastMs is
	// its grid time.
	lastIdx int
	lastMs  int64
}

func newTier(step time.Duration, slots int) tier {
	t := tier{stepMs: step.Milliseconds(), values: make([]float64, slots), lastIdx: -1}
	for i := range t.values {
		t.values[i] = math.NaN()
	}
	return t
}

// put stores v in the slot covering atMs.
//
// Later writes to the same slot overwrite: the coarse tier sees four
// samples per bucket and keeps the last, which is what both readings
// want. For a gauge it is the freshest value; for a counter it is the
// endpoint that differencing needs.
func (t *tier) put(atMs int64, v float64) {
	grid := atMs / t.stepMs * t.stepMs
	switch {
	case t.lastIdx < 0:
		t.lastIdx, t.lastMs = 0, grid
		t.values[0] = v

	case grid == t.lastMs:
		t.values[t.lastIdx] = v

	case grid > t.lastMs:
		steps := (grid - t.lastMs) / t.stepMs
		if steps >= int64(len(t.values)) {
			// The gap is wider than the whole ring: nothing stored is
			// still in range, so start over rather than leave stale
			// values that would read as recent.
			for i := range t.values {
				t.values[i] = math.NaN()
			}
			t.lastIdx, t.lastMs = 0, grid
			t.values[0] = v
			return
		}
		// Skipped slots are holes, not repeats of the last value. A
		// suspended VM that resumes must show a gap in the chart, not a
		// flat line it never actually held.
		for i := int64(1); i < steps; i++ {
			t.values[(t.lastIdx+int(i))%len(t.values)] = math.NaN()
		}
		t.lastIdx = (t.lastIdx + int(steps)) % len(t.values)
		t.lastMs = grid
		t.values[t.lastIdx] = v

	default:
		// grid < lastMs: the wall clock jumped backwards (an NTP step on
		// a freshly booted VM is the ordinary cause). Dropping the sample
		// is the only safe answer -- writing an older slot would leave a
		// newer counter value sitting behind an older one, which reads
		// back as a reset and draws a spike.
	}
}

// oldestMs is the grid time of the oldest slot still in range.
func (t *tier) oldestMs() int64 {
	return t.lastMs - int64(len(t.values)-1)*t.stepMs
}

// at returns the value stored at grid time ms, which MUST be aligned to
// the tier's step, or NaN if that slot holds nothing.
func (t *tier) at(ms int64) float64 {
	if t.lastIdx < 0 || ms > t.lastMs {
		return math.NaN()
	}
	back := (t.lastMs - ms) / t.stepMs
	if back >= int64(len(t.values)) {
		return math.NaN()
	}
	idx := t.lastIdx - int(back)
	if idx < 0 {
		idx += len(t.values)
	}
	return t.values[idx]
}

// maxGap is how far apart two stored points may be and still be treated
// as consecutive.
//
// Beyond it the counter advanced by an unknown amount while nobody was
// looking, and charging that whole advance to the bucket where sampling
// resumed would draw a spike that never happened. Three steps tolerates
// a couple of missed rounds without tolerating a restart.
func (t *tier) maxGap() int64 { return 3 * t.stepMs }

// rise sums a counter's increases over (fromMs, toMs], correcting for
// resets, and reports whether any usable pair of points was found.
//
// Reset handling follows PromQL: a value lower than its predecessor
// means the counter restarted, and the increase attributable to that
// step is the new value itself. Without this a reboot, a device
// re-enumeration or a 64-bit wrap turns into a large negative rate --
// which on a chart is far more alarming than the event that caused it.
func (t *tier) rise(fromMs, toMs int64) (float64, bool) {
	if t.lastIdx < 0 {
		return 0, false
	}
	gap := t.maxGap()
	start := alignUp(fromMs-gap, t.stepMs)
	if o := t.oldestMs(); start < o {
		start = o
	}
	end := toMs
	if end > t.lastMs {
		end = t.lastMs
	}

	total, ok := 0.0, false
	prevMs, prevV := int64(0), math.NaN()
	for ms := start; ms <= end; ms += t.stepMs {
		v := t.at(ms)
		if math.IsNaN(v) {
			continue
		}
		if !math.IsNaN(prevV) && ms > fromMs && ms-prevMs <= gap {
			d := v - prevV
			if d < 0 {
				d = v
			}
			total += d
			ok = true
		}
		prevMs, prevV = ms, v
	}
	return total, ok
}

// last returns the newest stored value in (fromMs, toMs], or NaN.
func (t *tier) last(fromMs, toMs int64) float64 {
	if t.lastIdx < 0 {
		return math.NaN()
	}
	hi := toMs
	if hi > t.lastMs {
		hi = t.lastMs
	}
	hi = hi / t.stepMs * t.stepMs
	oldest := t.oldestMs()
	for ms := hi; ms > fromMs && ms >= oldest; ms -= t.stepMs {
		if v := t.at(ms); !math.IsNaN(v) {
			return v
		}
	}
	return math.NaN()
}

// Ring is every series this host reports, and the only mutable state the
// collector owns.
//
// One mutex covers both writing and reading. The write is a few dozen
// float stores every 15 seconds; the read builds a whole response while
// holding it, which for the widest query is a few milliseconds once in a
// while. Splitting them would buy nothing and would have to answer what
// a query means when it overlaps a sample.
type Ring struct {
	limit int

	mu       sync.Mutex
	byKey    map[string]*Series
	order    []*Series
	newestMs int64
	// dropped counts series refused for exceeding limit in the last
	// round, so the collector can say so once instead of once a round.
	dropped int
}

// NewRing builds an empty ring. A limit of zero uses DefaultSeriesLimit.
func NewRing(limit int) *Ring {
	if limit <= 0 {
		limit = DefaultSeriesLimit
	}
	return &Ring{limit: limit, byKey: map[string]*Series{}}
}

// Add stores one collection round and returns how many of its points
// were refused for exceeding the series limit.
func (r *Ring) Add(s Sample) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.dropped = 0
	for _, p := range s.Points {
		key := seriesKey(p.Name, p.Labels)
		ser, ok := r.byKey[key]
		if !ok {
			if len(r.order) >= r.limit {
				r.dropped++
				continue
			}
			ser = newSeries(p)
			r.byKey[key] = ser
			r.order = append(r.order, ser)
		}
		ser.put(s.AtMs, p.Value)
	}
	if s.AtMs > r.newestMs {
		r.newestMs = s.AtMs
	}
	return r.dropped
}

// NewestMs is the time of the most recent sample, or 0 before the first.
func (r *Ring) NewestMs() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.newestMs
}

// find returns the single series with this name and no dimensions.
func (r *Ring) find(name string) *Series { return r.byKey[name] }

// group returns every series named name keyed by its value for the
// dimension dim, plus those keys in sorted order. It is for dimensions
// that identify a series -- device, mountpoint -- where one key is one
// series by construction.
//
// Sorted rather than insertion-ordered so a chart's line colours do not
// shuffle when a disk is added and the agent restarts.
func (r *Ring) group(name, dim string) (map[string]*Series, []string) {
	out := map[string]*Series{}
	for _, s := range r.order {
		if s.Name != name {
			continue
		}
		if v := s.Label(dim); v != "" {
			out[v] = s
		}
	}
	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return out, keys
}

// groupAll is group for dimensions that do NOT identify a series.
// node_cpu_seconds_total carries {cpu, mode}: grouped by mode, every
// core contributes one series, and a map to a single *Series would keep
// whichever core happened to come last.
func (r *Ring) groupAll(name, dim string) (map[string][]*Series, []string) {
	out := map[string][]*Series{}
	for _, s := range r.order {
		if s.Name != name {
			continue
		}
		if v := s.Label(dim); v != "" {
			out[v] = append(out[v], s)
		}
	}
	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return out, keys
}

// seriesKey is a series' identity: its name and every label, sorted.
func seriesKey(name string, labels []Label) string {
	if len(labels) == 0 {
		return name
	}
	ls := append([]Label(nil), labels...)
	sort.Slice(ls, func(i, j int) bool { return ls[i].Name < ls[j].Name })
	var b strings.Builder
	b.WriteString(name)
	for _, l := range ls {
		b.WriteByte('|')
		b.WriteString(l.Name)
		b.WriteByte('=')
		b.WriteString(l.Value)
	}
	return b.String()
}

// alignUp rounds x up to a multiple of step.
func alignUp(x, step int64) int64 { return (x + step - 1) / step * step }
