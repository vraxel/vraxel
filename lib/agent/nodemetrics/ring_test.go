package nodemetrics

import (
	"math"
	"testing"
	"time"
)

const stepMs = int64(15000)

func mkTier() tier { return newTier(FineStep, fineSlots) }

func TestTierOverwriteAndRead(t *testing.T) {
	tr := mkTier()
	tr.put(0, 1)
	tr.put(7000, 2) // same slot: overwrite
	if got := tr.at(0); got != 2 {
		t.Fatalf("same-slot overwrite: got %v, want 2", got)
	}
	tr.put(stepMs, 3)
	if got := tr.at(stepMs); got != 3 {
		t.Fatalf("next slot: got %v, want 3", got)
	}
}

func TestTierGapIsHoles(t *testing.T) {
	tr := mkTier()
	tr.put(0, 1)
	tr.put(3*stepMs, 4)
	if !math.IsNaN(tr.at(stepMs)) || !math.IsNaN(tr.at(2*stepMs)) {
		t.Fatalf("skipped slots must be holes, got %v %v", tr.at(stepMs), tr.at(2*stepMs))
	}
	if tr.at(0) != 1 || tr.at(3*stepMs) != 4 {
		t.Fatalf("endpoints lost across gap")
	}
}

func TestTierGapWiderThanRingResets(t *testing.T) {
	tr := mkTier()
	tr.put(0, 1)
	tr.put(int64(fineSlots)*stepMs, 9)
	if !math.IsNaN(tr.at(0)) {
		t.Fatalf("a gap wider than the ring must clear stale values")
	}
	if got := tr.at(int64(fineSlots) * stepMs); got != 9 {
		t.Fatalf("post-reset value lost: got %v", got)
	}
}

func TestTierClockBackwardsDropped(t *testing.T) {
	tr := mkTier()
	tr.put(4*stepMs, 10)
	tr.put(2*stepMs, 99)
	if !math.IsNaN(tr.at(2 * stepMs)) {
		t.Fatalf("a backwards write must be dropped")
	}
}

func TestTierRiseCounterReset(t *testing.T) {
	tr := mkTier()
	tr.put(0, 100)
	tr.put(stepMs, 110)
	tr.put(2*stepMs, 5) // reset: contributes its own value
	got, ok := tr.rise(0, 2*stepMs)
	if !ok || got != 15 {
		t.Fatalf("rise with reset: got %v ok=%v, want 15", got, ok)
	}
}

func TestTierRiseSkipsWideGaps(t *testing.T) {
	tr := mkTier()
	tr.put(0, 100)
	tr.put(4*stepMs, 200) // 4 steps > maxGap of 3
	got, ok := tr.rise(0, 4*stepMs)
	if ok || got != 0 {
		t.Fatalf("a pair wider than maxGap must not count, got %v ok=%v", got, ok)
	}
}

func TestTierRiseAttributedToLaterBucket(t *testing.T) {
	tr := mkTier()
	tr.put(0, 100)
	tr.put(stepMs, 130)
	// The delta's later sample sits at stepMs, so it belongs to the
	// bucket (0, stepMs], not to (stepMs, 2*stepMs].
	if got, ok := tr.rise(0, stepMs); !ok || got != 30 {
		t.Fatalf("bucket (0,step]: got %v ok=%v, want 30", got, ok)
	}
	if got, ok := tr.rise(stepMs, 2*stepMs); ok || got != 0 {
		t.Fatalf("bucket (step,2step]: got %v ok=%v, want none", got, ok)
	}
}

func TestTierLast(t *testing.T) {
	tr := mkTier()
	tr.put(0, 1)
	tr.put(stepMs, 2)
	if got := tr.last(0, 5*stepMs); got != 2 {
		t.Fatalf("last: got %v, want 2", got)
	}
	if got := tr.last(stepMs, 5*stepMs); !math.IsNaN(got) {
		t.Fatalf("empty window must read NaN, got %v", got)
	}
}

func TestRingSeriesLimit(t *testing.T) {
	r := NewRing(2)
	dropped := r.Add(Sample{AtMs: 0, Points: []Point{
		{Name: "a", Kind: Gauge, Value: 1},
		{Name: "b", Kind: Gauge, Value: 2},
		{Name: "c", Kind: Gauge, Value: 3},
	}})
	if dropped != 1 {
		t.Fatalf("dropped: got %d, want 1", dropped)
	}
	// A later round with the same three: the two known series update, the
	// third stays refused.
	dropped = r.Add(Sample{AtMs: stepMs, Points: []Point{
		{Name: "a", Kind: Gauge, Value: 4},
		{Name: "b", Kind: Gauge, Value: 5},
		{Name: "c", Kind: Gauge, Value: 6},
	}})
	if dropped != 1 || len(r.order) != 2 {
		t.Fatalf("cap must hold: dropped=%d series=%d", dropped, len(r.order))
	}
}

func TestRingSeriesIdentityIncludesLabels(t *testing.T) {
	r := NewRing(0)
	r.Add(Sample{AtMs: 0, Points: []Point{
		{Name: "m", Labels: []Label{{Name: "device", Value: "a"}}, Kind: Gauge, Value: 1},
		{Name: "m", Labels: []Label{{Name: "device", Value: "b"}}, Kind: Gauge, Value: 2},
	}})
	if len(r.order) != 2 {
		t.Fatalf("distinct label sets must be distinct series, got %d", len(r.order))
	}
	devs, names := r.group("m", "device")
	if len(names) != 2 || devs["a"] == devs["b"] {
		t.Fatalf("group lost a series: %v", names)
	}
}

func TestGroupAllKeepsEverySeries(t *testing.T) {
	r := NewRing(0)
	r.Add(Sample{AtMs: 0, Points: []Point{
		{Name: mCPU, Labels: []Label{{Name: "cpu", Value: "0"}, {Name: lMode, Value: "idle"}}, Kind: Counter, Value: 1},
		{Name: mCPU, Labels: []Label{{Name: "cpu", Value: "1"}, {Name: lMode, Value: "idle"}}, Kind: Counter, Value: 2},
	}})
	byMode, names := r.groupAll(mCPU, lMode)
	if len(names) != 1 || len(byMode["idle"]) != 2 {
		t.Fatalf("groupAll must keep one series per core: %v -> %d", names, len(byMode["idle"]))
	}
}

// sanity-pin the memory story the sizing comments tell: two tiers of
// float64 cost ~13.4KB per series.
func TestTierFootprint(t *testing.T) {
	perSeries := (fineSlots + coarseSlots) * 8
	if perSeries != 13440 {
		t.Fatalf("per-series footprint changed: %d bytes", perSeries)
	}
	if FineStep != 15*time.Second || CoarseStep != 60*time.Second {
		t.Fatalf("tier steps changed; update the design doc and this test")
	}
}
