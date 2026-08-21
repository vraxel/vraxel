package nodemetrics

import (
	"errors"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// feedCPU writes n+1 samples, 15s apart from t=0, of a two-core machine
// where each interval every core adds 5s idle and 10s user.
func feedCPU(r *Ring, n int) {
	for i := 0; i <= n; i++ {
		at := int64(i) * stepMs
		var pts []Point
		for _, cpu := range []string{"0", "1"} {
			pts = append(pts,
				Point{Name: mCPU, Labels: []Label{{Name: "cpu", Value: cpu}, {Name: lMode, Value: modeIdle}},
					Kind: Counter, Value: 5 * float64(i)},
				Point{Name: mCPU, Labels: []Label{{Name: "cpu", Value: cpu}, {Name: lMode, Value: "user"}},
					Kind: Counter, Value: 10 * float64(i)},
			)
		}
		r.Add(Sample{AtMs: at, Points: pts})
	}
}

func TestQueryCPUUsedSumsAcrossCores(t *testing.T) {
	r := NewRing(0)
	feedCPU(r, 4)

	res, err := r.Query(0, 4*stepMs, 15, []string{agenttypes.SeriesCPUUsedPct})
	if err != nil {
		t.Fatal(err)
	}
	if res.StepSec != 15 || res.Count != 4 || len(res.Series) != 1 {
		t.Fatalf("grid: step=%d count=%d series=%d", res.StepSec, res.Count, len(res.Series))
	}
	// Per bucket per core: idle 5, user 10 -> total 30, idle 10.
	// used = 100*(1-10/30). A per-core collapse would read one core and
	// still produce the same ratio -- so the giveaway is checked via
	// mode_pct totals below; here the value itself is pinned.
	want := 100 * (1 - 10.0/30.0)
	for i, v := range res.Series[0].Values {
		if v.IsNone() || !close(float64(v), want) {
			t.Fatalf("bucket %d: got %v, want %v", i, v, want)
		}
	}
}

func TestQueryCPUModePct(t *testing.T) {
	r := NewRing(0)
	feedCPU(r, 2)
	res, err := r.Query(0, 2*stepMs, 15, []string{agenttypes.SeriesCPUModePct})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 2 {
		t.Fatalf("want idle+user series, got %d", len(res.Series))
	}
	for _, s := range res.Series {
		want := map[string]float64{modeIdle: 100 * 10 / 30.0, "user": 100 * 20 / 30.0}[s.Labels[lMode]]
		if v := float64(s.Values[0]); !close(v, want) {
			t.Fatalf("mode %s: got %v, want %v", s.Labels[lMode], v, want)
		}
	}
}

func TestQuerySnapsStepToTier(t *testing.T) {
	r := NewRing(0)
	feedCPU(r, 4)
	// 10s is finer than the fine tier: snapped up to 15s.
	res, err := r.Query(0, 4*stepMs, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StepSec != 15 {
		t.Fatalf("step must snap up to the tier: got %d", res.StepSec)
	}
}

func TestQueryOldWindowUsesCoarseTier(t *testing.T) {
	r := NewRing(0)
	// Newest at 26h so a from of 0 lies outside the fine tier's hour.
	base := int64(26) * 3600 * 1000
	r.Add(Sample{AtMs: base, Points: []Point{{Name: mLoad1, Kind: Gauge, Value: 1}}})
	res, err := r.Query(base-2*3600*1000, base, 15, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StepSec != 60 {
		t.Fatalf("a window beyond the fine tier must answer at 60s, got %d", res.StepSec)
	}
}

func TestQueryBounds(t *testing.T) {
	r := NewRing(0)
	feedCPU(r, 1)
	if _, err := r.Query(100, 100, 15, nil); !errors.Is(err, ErrBadWindow) {
		t.Fatalf("empty window: %v", err)
	}
	if _, err := r.Query(0, int64(agenttypes.MetricsMaxPoints+1)*60*1000, 60, nil); !errors.Is(err, ErrTooManyPoints) {
		t.Fatalf("over-wide window: %v", err)
	}
}

func TestQueryEmptySeriesLeftOut(t *testing.T) {
	r := NewRing(0)
	feedCPU(r, 2)
	// Window entirely before any data: nothing has a value, so nothing is
	// returned -- not rows of nulls. Placed inside the fine tier's reach
	// so the tier choice is not what empties it.
	res, err := r.Query(-10*stepMs, -8*stepMs, 15, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 0 {
		t.Fatalf("all-hole series must be omitted, got %d", len(res.Series))
	}
}

func TestQueryMemAndFS(t *testing.T) {
	r := NewRing(0)
	fsLabels := []Label{{Name: lDevice, Value: "/dev/sda1"}, {Name: lFSType, Value: "ext4"}, {Name: lMountpoint, Value: "/"}}
	for i := 0; i <= 1; i++ {
		r.Add(Sample{AtMs: int64(i) * stepMs, Points: []Point{
			{Name: mMemTotal, Kind: Gauge, Value: 1000},
			{Name: mMemAvail, Kind: Gauge, Value: 250},
			{Name: mFSSize, Labels: fsLabels, Kind: Gauge, Value: 100},
			{Name: mFSAvail, Labels: fsLabels, Kind: Gauge, Value: 20},
		}})
	}
	res, err := r.Query(0, stepMs, 15, []string{agenttypes.SeriesMemUsedPct, agenttypes.SeriesFSUsedPct})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, s := range res.Series {
		got[s.Name] = float64(s.Values[0])
	}
	if !close(got[agenttypes.SeriesMemUsedPct], 75) {
		t.Fatalf("mem: %v", got)
	}
	if !close(got[agenttypes.SeriesFSUsedPct], 80) {
		t.Fatalf("fs: %v", got)
	}
}

func TestQueryDiskUtilClamped(t *testing.T) {
	r := NewRing(0)
	labels := []Label{{Name: lDevice, Value: "sda"}}
	// io_time grows faster than wall clock (deep queue): must clamp at 100.
	r.Add(Sample{AtMs: 0, Points: []Point{{Name: mDiskIOTime, Labels: labels, Kind: Counter, Value: 0}}})
	r.Add(Sample{AtMs: stepMs, Points: []Point{{Name: mDiskIOTime, Labels: labels, Kind: Counter, Value: 20}}})
	res, err := r.Query(0, stepMs, 15, []string{agenttypes.SeriesDiskUtilPct})
	if err != nil {
		t.Fatal(err)
	}
	if v := float64(res.Series[0].Values[0]); v != 100 {
		t.Fatalf("util must clamp to 100, got %v", v)
	}
}

func close(a, b float64) bool {
	d := a - b
	return d < 0.001 && d > -0.001
}

// The UI's "last hour at 15s" preset: from is the caller's now minus one
// hour, and the newest sample lags that now by up to a sampling period.
// An exact coverage test silently downgraded exactly this request to the
// coarse tier -- the flagship window was the one window that never got
// its resolution.
func TestQueryLastHourPresetStaysFine(t *testing.T) {
	r := NewRing(0)
	base := int64(2) * 3600 * 1000
	r.Add(Sample{AtMs: base, Points: []Point{{Name: mLoad1, Kind: Gauge, Value: 1}}})

	callerNow := base + 9_000 // the sample lags the browser's clock
	res, err := r.Query(callerNow-3_600_000, callerNow, 15, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StepSec != 15 {
		t.Fatalf("the 1h preset must answer at 15s, got %ds", res.StepSec)
	}
}
