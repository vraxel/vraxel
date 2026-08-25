package nodemetrics

import (
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// two writes one sample at t=0 and one at t=stepMs, which is the minimum
// a counter needs a rise from.
func two(r *Ring, pts0, pts1 []Point) {
	r.Add(Sample{AtMs: 0, Points: pts0})
	r.Add(Sample{AtMs: stepMs, Points: pts1})
}

func firstValue(t *testing.T, res agenttypes.MetricsResult, name string) float64 {
	t.Helper()
	for _, s := range res.Series {
		if s.Name == name {
			return float64(s.Values[0])
		}
	}
	t.Fatalf("series %q not derived; got %+v", name, res.Series)
	return 0
}

// PSI is a counter of stalled seconds, so its rise per second of wall
// clock IS the stalled fraction. 1.5s stalled in a 15s bucket is 10%.
func TestQueryPressureIsAShareOfWallClock(t *testing.T) {
	r := NewRing(0)
	two(r,
		[]Point{{Name: mPSICPU, Kind: Counter, Value: 0}},
		[]Point{{Name: mPSICPU, Kind: Counter, Value: 1.5}},
	)
	res, err := r.Query(0, stepMs, 15, []string{agenttypes.SeriesPSICPUPct})
	if err != nil {
		t.Fatal(err)
	}
	if v := firstValue(t, res, agenttypes.SeriesPSICPUPct); !close(v, 10) {
		t.Fatalf("psi = %v, want 10", v)
	}
}

// Mean service time is accumulated seconds divided by completed
// operations over the SAME bucket -- 0.4s over 200 ops is 2ms -- and
// neither counter alone can produce it.
func TestQueryDiskLatencyIsPerOperation(t *testing.T) {
	r := NewRing(0)
	labels := []Label{{Name: lDevice, Value: "sda"}}
	two(r,
		[]Point{
			{Name: mDiskReadTime, Labels: labels, Kind: Counter, Value: 0},
			{Name: mDiskReadOps, Labels: labels, Kind: Counter, Value: 0},
		},
		[]Point{
			{Name: mDiskReadTime, Labels: labels, Kind: Counter, Value: 0.4},
			{Name: mDiskReadOps, Labels: labels, Kind: Counter, Value: 200},
		},
	)
	res, err := r.Query(0, stepMs, 15, []string{agenttypes.SeriesDiskReadWait})
	if err != nil {
		t.Fatal(err)
	}
	if v := firstValue(t, res, agenttypes.SeriesDiskReadWait); !close(v, 0.002) {
		t.Fatalf("wait = %v, want 0.002", v)
	}
}

// An idle device did zero operations, and its latency is undefined --
// not zero, which would draw a flat line reading "instant".
func TestQueryDiskLatencyIdleIsAbsentNotZero(t *testing.T) {
	r := NewRing(0)
	labels := []Label{{Name: lDevice, Value: "sda"}}
	two(r,
		[]Point{
			{Name: mDiskReadTime, Labels: labels, Kind: Counter, Value: 7},
			{Name: mDiskReadOps, Labels: labels, Kind: Counter, Value: 42},
		},
		[]Point{
			{Name: mDiskReadTime, Labels: labels, Kind: Counter, Value: 7},
			{Name: mDiskReadOps, Labels: labels, Kind: Counter, Value: 42},
		},
	)
	res, err := r.Query(0, stepMs, 15, []string{agenttypes.SeriesDiskReadWait})
	if err != nil {
		t.Fatal(err)
	}
	// build() drops a series with nothing in the window, so an idle
	// device produces no series at all rather than a line of zeroes.
	for _, s := range res.Series {
		if s.Name == agenttypes.SeriesDiskReadWait {
			t.Fatalf("idle device must not report a latency: %v", s.Values)
		}
	}
}

func TestQueryInodes(t *testing.T) {
	r := NewRing(0)
	real := []Label{{Name: lFSType, Value: "ext4"}, {Name: lMountpoint, Value: "/"}}
	// A tmpfs has inodes too, and the disk gauge excludes it. Charting it
	// here would give the page two disk answers that disagree.
	tmp := []Label{{Name: lFSType, Value: "tmpfs"}, {Name: lMountpoint, Value: "/run"}}
	two(r,
		[]Point{
			{Name: mFSFiles, Labels: real, Kind: Gauge, Value: 1000},
			{Name: mFSFilesFree, Labels: real, Kind: Gauge, Value: 250},
			{Name: mFSFiles, Labels: tmp, Kind: Gauge, Value: 1000},
			{Name: mFSFilesFree, Labels: tmp, Kind: Gauge, Value: 900},
		},
		[]Point{
			{Name: mFSFiles, Labels: real, Kind: Gauge, Value: 1000},
			{Name: mFSFilesFree, Labels: real, Kind: Gauge, Value: 250},
			{Name: mFSFiles, Labels: tmp, Kind: Gauge, Value: 1000},
			{Name: mFSFilesFree, Labels: tmp, Kind: Gauge, Value: 900},
		},
	)
	res, err := r.Query(0, stepMs, 15, []string{agenttypes.SeriesFSInodesPct})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 1 {
		t.Fatalf("tmpfs must not be charted; got %+v", res.Series)
	}
	if v := float64(res.Series[0].Values[0]); !close(v, 75) {
		t.Fatalf("inodes = %v, want 75", v)
	}
}

// Container and bridge plumbing is dropped by the same predicate the
// summary and the reported NIC list use.
func TestQueryNetworkDropsVirtualDevices(t *testing.T) {
	r := NewRing(0)
	var pts0, pts1 []Point
	for _, d := range []string{"ens33", "lo", "docker0", "veth1234", "br-abc"} {
		l := []Label{{Name: lDevice, Value: d}}
		pts0 = append(pts0, Point{Name: mNetRxPkts, Labels: l, Kind: Counter, Value: 0})
		pts1 = append(pts1, Point{Name: mNetRxPkts, Labels: l, Kind: Counter, Value: 150})
	}
	two(r, pts0, pts1)
	res, err := r.Query(0, stepMs, 15, []string{agenttypes.SeriesNetRxPps})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 1 || res.Series[0].Labels[lDevice] != "ens33" {
		t.Fatalf("only real interfaces belong on the chart; got %+v", res.Series)
	}
	if v := float64(res.Series[0].Values[0]); !close(v, 10) {
		t.Fatalf("pps = %v, want 10", v)
	}
}

// Uptime comes from the two clock gauges in the same sample, so it needs
// no clock from the server and survives a host whose wall clock is wrong.
func TestQueryUptime(t *testing.T) {
	r := NewRing(0)
	two(r,
		[]Point{
			{Name: mTime, Kind: Gauge, Value: 1_000_000},
			{Name: mBootTime, Kind: Gauge, Value: 900_000},
		},
		[]Point{
			{Name: mTime, Kind: Gauge, Value: 1_000_015},
			{Name: mBootTime, Kind: Gauge, Value: 900_000},
		},
	)
	res, err := r.Query(0, stepMs, 15, []string{agenttypes.SeriesUptimeSec})
	if err != nil {
		t.Fatal(err)
	}
	// 100_015, not 100_000: a bucket is half-open and closed at the top,
	// so the sample at stepMs is the one bucket 0 reports, and a gauge
	// reads the LAST value in its bucket.
	if v := firstValue(t, res, agenttypes.SeriesUptimeSec); !close(v, 100_015) {
		t.Fatalf("uptime = %v, want 100015", v)
	}
}

// One kill in a bucket has to be visible as 1. Divided by a 60s bucket a
// rate would be 0.016 and round away on the chart.
func TestQueryOOMKillsCountPerBucket(t *testing.T) {
	r := NewRing(0)
	two(r,
		[]Point{{Name: mOOMKill, Kind: Counter, Value: 3}},
		[]Point{{Name: mOOMKill, Kind: Counter, Value: 4}},
	)
	res, err := r.Query(0, stepMs, 15, []string{agenttypes.SeriesOOMKills})
	if err != nil {
		t.Fatal(err)
	}
	if v := firstValue(t, res, agenttypes.SeriesOOMKills); !close(v, 1) {
		t.Fatalf("oom kills = %v, want 1", v)
	}
}

func TestQueryRatioSeries(t *testing.T) {
	r := NewRing(0)
	two(r,
		[]Point{
			{Name: mFDAllocated, Kind: Gauge, Value: 512},
			{Name: mFDMaximum, Kind: Gauge, Value: 2048},
			{Name: mConntrack, Kind: Gauge, Value: 10},
			{Name: mConntrackMax, Kind: Gauge, Value: 0},
		},
		[]Point{
			{Name: mFDAllocated, Kind: Gauge, Value: 512},
			{Name: mFDMaximum, Kind: Gauge, Value: 2048},
			{Name: mConntrack, Kind: Gauge, Value: 10},
			{Name: mConntrackMax, Kind: Gauge, Value: 0},
		},
	)
	res, err := r.Query(0, stepMs, 15, []string{
		agenttypes.SeriesFDUsedPct, agenttypes.SeriesConntrackPct,
	})
	if err != nil {
		t.Fatal(err)
	}
	if v := firstValue(t, res, agenttypes.SeriesFDUsedPct); !close(v, 25) {
		t.Fatalf("fd = %v, want 25", v)
	}
	// A zero limit means conntrack is not loaded. That is "no answer",
	// not a division by zero and not 100% full.
	for _, s := range res.Series {
		if s.Name == agenttypes.SeriesConntrackPct {
			t.Fatalf("a zero limit must not produce a percentage: %v", s.Values)
		}
	}
}

// Temperature is identified by chip AND sensor: either alone collapses a
// board's several readings onto one key.
func TestQueryTemperatureKeepsEverySensor(t *testing.T) {
	r := NewRing(0)
	var pts []Point
	for _, s := range []struct{ chip, sensor string }{
		{"platform_coretemp_0", "temp1"},
		{"platform_coretemp_0", "temp2"},
		{"platform_nvme", "temp1"},
	} {
		pts = append(pts, Point{
			Name:   mHwmonTemp,
			Labels: []Label{{Name: lChip, Value: s.chip}, {Name: lSensor, Value: s.sensor}},
			Kind:   Gauge, Value: 42,
		})
	}
	two(r, pts, pts)
	res, err := r.Query(0, stepMs, 15, []string{agenttypes.SeriesTempCelsius})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 3 {
		t.Fatalf("want 3 sensors, got %d: %+v", len(res.Series), res.Series)
	}
}
