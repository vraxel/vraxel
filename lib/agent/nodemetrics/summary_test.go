package nodemetrics

import (
	"testing"
)

// feedFullHost writes six rounds, 15s apart, of everything the summary
// needs -- six rather than two so the samples cross a coarse-tier slot
// boundary and the 24h trend has a rate to show. Per interval: each of
// two cores adds idle 5s / user 10s; eth0 moves 15000 bytes in and
// 30000 out; lo and veth0 move huge amounts that must not count.
func feedFullHost(r *Ring) {
	fsRoot := []Label{{Name: lDevice, Value: "/dev/sda1"}, {Name: lFSType, Value: "ext4"}, {Name: lMountpoint, Value: "/"}}
	fsData := []Label{{Name: lDevice, Value: "/dev/sdb1"}, {Name: lFSType, Value: "xfs"}, {Name: lMountpoint, Value: "/data"}}
	fsRun := []Label{{Name: lDevice, Value: "tmpfs"}, {Name: lFSType, Value: "tmpfs"}, {Name: lMountpoint, Value: "/run"}}
	for i := 0; i <= 5; i++ {
		f := float64(i)
		var pts []Point
		for _, cpu := range []string{"0", "1"} {
			pts = append(pts,
				Point{Name: mCPU, Labels: []Label{{Name: "cpu", Value: cpu}, {Name: lMode, Value: modeIdle}}, Kind: Counter, Value: 5 * f},
				Point{Name: mCPU, Labels: []Label{{Name: "cpu", Value: cpu}, {Name: lMode, Value: "user"}}, Kind: Counter, Value: 10 * f},
			)
		}
		pts = append(pts,
			Point{Name: mMemTotal, Kind: Gauge, Value: 1000},
			Point{Name: mMemAvail, Kind: Gauge, Value: 250},
			Point{Name: mLoad1, Kind: Gauge, Value: 1.5},
			Point{Name: mLoad5, Kind: Gauge, Value: 1.0},
			Point{Name: mLoad15, Kind: Gauge, Value: 0.5},
			Point{Name: mFSSize, Labels: fsRoot, Kind: Gauge, Value: 100},
			Point{Name: mFSAvail, Labels: fsRoot, Kind: Gauge, Value: 20},
			Point{Name: mFSSize, Labels: fsData, Kind: Gauge, Value: 1000},
			Point{Name: mFSAvail, Labels: fsData, Kind: Gauge, Value: 900},
			// tmpfs at 100% -- the exact trap DiskUsedPct must not fall into.
			Point{Name: mFSSize, Labels: fsRun, Kind: Gauge, Value: 10},
			Point{Name: mFSAvail, Labels: fsRun, Kind: Gauge, Value: 0},
			Point{Name: mNetRx, Labels: []Label{{Name: lDevice, Value: "eth0"}}, Kind: Counter, Value: 15000 * f},
			Point{Name: mNetTx, Labels: []Label{{Name: lDevice, Value: "eth0"}}, Kind: Counter, Value: 30000 * f},
			Point{Name: mNetRx, Labels: []Label{{Name: lDevice, Value: "lo"}}, Kind: Counter, Value: 1e9 * f},
			Point{Name: mNetRx, Labels: []Label{{Name: lDevice, Value: "veth1234"}}, Kind: Counter, Value: 1e9 * f},
		)
		r.Add(Sample{AtMs: int64(i) * stepMs, Points: pts})
	}
}

func TestSummary(t *testing.T) {
	r := NewRing(0)
	feedFullHost(r)
	s := r.Summary()
	if s == nil {
		t.Fatal("summary must exist after two samples")
	}
	if s.SampledAtMs != 5*stepMs {
		t.Fatalf("sampledAt: %d", s.SampledAtMs)
	}
	if !close(s.CPUUsedPct, 100*(1-10.0/30.0)) {
		t.Fatalf("cpu: %v", s.CPUUsedPct)
	}
	if !close(s.MemUsedPct, 75) {
		t.Fatalf("mem: %v", s.MemUsedPct)
	}
	if !close(s.DiskUsedPct, 80) || s.DiskUsedPath != "/" {
		t.Fatalf("disk must be the fullest REAL mount: %v at %q", s.DiskUsedPct, s.DiskUsedPath)
	}
	if s.Load1 != 1.5 || s.Load5 != 1.0 || s.Load15 != 0.5 {
		t.Fatalf("load: %v %v %v", s.Load1, s.Load5, s.Load15)
	}
	if !close(s.NetRxBps, 1000) || !close(s.NetTxBps, 2000) {
		t.Fatalf("net must exclude lo and veth: rx=%v tx=%v", s.NetRxBps, s.NetTxBps)
	}
}

func TestSummaryNilUntilTwoSamples(t *testing.T) {
	r := NewRing(0)
	if r.Summary() != nil {
		t.Fatal("empty ring must yield nil")
	}
	r.Add(Sample{AtMs: 0, Points: []Point{
		{Name: mCPU, Labels: []Label{{Name: "cpu", Value: "0"}, {Name: lMode, Value: modeIdle}}, Kind: Counter, Value: 5},
		{Name: mMemTotal, Kind: Gauge, Value: 1000},
		{Name: mMemAvail, Kind: Gauge, Value: 500},
	}})
	if r.Summary() != nil {
		t.Fatal("one sample cannot derive a rate; the summary must be nil, not zero")
	}
}

func TestSummaryTrend(t *testing.T) {
	r := NewRing(0)
	feedFullHost(r)
	s := r.Summary()
	if len(s.CPUTrend) != trendPoints {
		t.Fatalf("trend length: %d", len(s.CPUTrend))
	}
	last := s.CPUTrend[trendPoints-1]
	if last.IsNone() || !close(float64(last), 100*(1-10.0/30.0)) {
		t.Fatalf("newest trend bucket: %v", last)
	}
	// A minute of history cannot fill 24 hours: everything older is null.
	for i := 0; i < trendPoints-1; i++ {
		if !s.CPUTrend[i].IsNone() {
			t.Fatalf("bucket %d must be null on a fresh agent, got %v", i, s.CPUTrend[i])
		}
	}
}
