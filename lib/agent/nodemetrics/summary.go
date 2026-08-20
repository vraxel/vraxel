package nodemetrics

import (
	"math"
	"strings"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// summaryFSTypes that must not decide DiskUsedPct.
//
// A full /run or /dev/shm is a memory problem wearing a filesystem's
// clothes, and it would pin the host list's disk column at a number no
// operator can act on. They stay in the fs.* chart series -- that is
// data, and node_exporter reports them too -- but they are kept out of
// the one field that is meant to be an operational signal.
var summaryFSTypes = map[string]struct{}{
	"tmpfs": {}, "ramfs": {}, "devtmpfs": {},
}

// summaryNetPrefixes are the per-container and per-bridge interface
// families that must not count toward the summary's rx/tx figure: a
// packet crossing a bridge is counted again on every veth it traverses,
// so summing them reports a host pushing multiples of its real traffic.
// The net.* chart series keep node_exporter's full surface; like
// summaryFSTypes, this shapes only the one number the host list sorts
// on.
var summaryNetPrefixes = []string{
	"veth", "docker", "br-", "virbr", "cni", "flannel", "cali", "tunl",
	"nodelocaldns", "kube-ipvs", "dummy", "lxc", "tap",
}

func summaryNetDevice(name string) bool {
	if name == "lo" {
		return false
	}
	for _, p := range summaryNetPrefixes {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	return true
}

// trendStep / trendPoints shape the CPU sparkline the heartbeat carries:
// the last 24 hours in half-hour buckets, ~300 bytes a beat. Fixed
// rather than negotiated because its one consumer is the host list's
// per-row mini chart, which is not interactive; an operator who wants a
// window picks the host and gets the query API.
const (
	trendStep   = 30 * time.Minute
	trendPoints = 48
)

// Summary derives the current-utilisation snapshot from the last
// sampling interval, or nil if it cannot be derived in full.
//
// All-or-nothing on purpose. CPU and network are rates and need two
// consecutive samples, so for the first interval after the collector
// starts there is nothing honest to say about them -- and a snapshot
// with a real-looking 0% CPU next to a real memory figure is worse than
// no snapshot, because nothing downstream can tell the two apart.
func (r *Ring) Summary() *agenttypes.MetricsSummary {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.newestMs == 0 {
		return nil
	}
	stepMs := FineStep.Milliseconds()
	end := r.newestMs / stepMs * stepMs
	g := grid{fine: true, fromMs: end - stepMs, stepMs: stepMs, count: 1}

	cpu, ok := r.summaryCPU(g)
	if !ok {
		return nil
	}
	out := &agenttypes.MetricsSummary{SampledAtMs: r.newestMs, CPUUsedPct: cpu}

	total, avail := r.find(mMemTotal), r.find(mMemAvail)
	if total == nil || avail == nil {
		return nil
	}
	t, a := total.gauge(g, 0), avail.gauge(g, 0)
	if math.IsNaN(t) || math.IsNaN(a) || t <= 0 {
		return nil
	}
	out.MemUsedPct = clampPct(100 * (t - a) / t)

	out.DiskUsedPct, out.DiskUsedPath = r.summaryDisk(g)
	out.Load1 = zeroNaN(gaugeOf(r.find(mLoad1), g))
	out.Load5 = zeroNaN(gaugeOf(r.find(mLoad5), g))
	out.Load15 = zeroNaN(gaugeOf(r.find(mLoad15), g))
	out.NetRxBps = r.summaryNet(g, mNetRx)
	out.NetTxBps = r.summaryNet(g, mNetTx)
	out.CPUTrend = r.cpuTrend()
	return out
}

// summaryCPU is the busy fraction over the interval, and whether there
// were two samples to compute it from.
func (r *Ring) summaryCPU(g grid) (float64, bool) {
	rises, _, totals := r.cpuModeRises(g)
	if rises == nil || totals[0] <= 0 {
		return 0, false
	}
	idle := nonNaN(rises[modeIdle], 0) + nonNaN(rises[modeIOWait], 0)
	return clampPct(100 * (1 - idle/totals[0])), true
}

// cpuTrend is the sparkline: cpu.used_pct over the trend window, ending
// at the bucket that holds the newest sample. Buckets older than the
// ring's coverage come back null, which is how a freshly started agent's
// sparkline honestly starts short. Callers hold r.mu.
func (r *Ring) cpuTrend() []agenttypes.MetricValue {
	stepMs := trendStep.Milliseconds()
	from := r.newestMs/stepMs*stepMs - int64(trendPoints-1)*stepMs
	g := grid{fromMs: from, stepMs: stepMs, count: trendPoints}
	rises, _, totals := r.cpuModeRises(g)
	if rises == nil {
		return nil
	}
	return cpuUsedValues(g.count, rises, totals)
}

// summaryDisk is the fullest real filesystem and where it is mounted.
func (r *Ring) summaryDisk(g grid) (float64, string) {
	sizes, mounts := r.group(mFSSize, lMountpoint)
	avails, _ := r.group(mFSAvail, lMountpoint)
	worst, at := 0.0, ""
	for _, mp := range mounts {
		size := sizes[mp]
		if _, skip := summaryFSTypes[size.Label(lFSType)]; skip {
			continue
		}
		avail, ok := avails[mp]
		if !ok {
			continue
		}
		t, a := size.gauge(g, 0), avail.gauge(g, 0)
		if math.IsNaN(t) || math.IsNaN(a) || t <= 0 {
			continue
		}
		if pct := clampPct(100 * (t - a) / t); pct > worst || at == "" {
			worst, at = pct, mp
		}
	}
	return worst, at
}

// summaryNet is the per-second byte rate summed over the real
// interfaces.
func (r *Ring) summaryNet(g grid, metric string) float64 {
	devs, names := r.group(metric, lDevice)
	sum := 0.0
	for _, d := range names {
		if !summaryNetDevice(d) {
			continue
		}
		if v := devs[d].rate(g, 0); !math.IsNaN(v) {
			sum += v
		}
	}
	return sum
}

func gaugeOf(s *Series, g grid) float64 {
	if s == nil {
		return math.NaN()
	}
	return s.gauge(g, 0)
}

// zeroNaN reports an unreadable gauge as zero.
//
// Only used for load average, and only because the summary is a fixed
// struct of numbers with no way to say "absent". Load is present on
// every Linux host, so this converts an impossibility rather than
// hiding a real gap; the fields that CAN legitimately be absent make
// the whole summary nil instead.
func zeroNaN(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return v
}
