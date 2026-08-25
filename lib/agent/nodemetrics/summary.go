package nodemetrics

import (
	"math"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// What counts as a filesystem and what counts as an interface is decided
// by agenttypes.RealFSType / RealNetDevice, not here.
//
// Those predicates also shape the facts the agent reports for the detail
// page, and the two views have to agree: a page listing a tmpfs that the
// host list's disk gauge excluded is a page whose rows do not add up to
// the number printed beside them. One definition, in the package both
// sides already import.
//
// This shapes only the summary. The fs.* and net.* chart series keep
// node_exporter's full surface -- a full /run is data; it just must not
// be the number an operator is asked to act on.

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

	out.DiskUsedPct, out.DiskUsedPath, out.DiskUsedBytes, out.DiskTotalBytes = r.summaryDisk(g)
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

// summaryDisk walks the real filesystems once and answers both disk
// questions from that walk: the fullest one and where it is mounted (the
// warning), and used/total summed across all of them (the inventory).
//
// One walk rather than two because the two answers must agree on what
// counts as a filesystem: if the sum included a tmpfs the maximum
// excludes, a host could show more disk than it has.
//
// The sum counts each DEVICE once, the maximum every mountpoint. A bind
// mount and a btrfs subvolume appear as separate mountpoints backed by
// the same device and the same blocks, so adding them would report a
// host with more disk than it owns -- while for "is anything filling
// up" seeing the same filesystem twice changes nothing. Mountpoints
// arrive sorted, so the survivor is the shortest path on each device,
// which is the one an operator would name.
func (r *Ring) summaryDisk(g grid) (worst float64, at string, usedBytes, totalBytes int64) {
	sizes, mounts := r.group(mFSSize, lMountpoint)
	avails, _ := r.group(mFSAvail, lMountpoint)
	usedSum, totalSum := 0.0, 0.0
	counted := map[string]struct{}{}
	for _, mp := range mounts {
		size := sizes[mp]
		if !agenttypes.RealFSType(size.Label(lFSType)) {
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
		dev := size.Label(lDevice)
		if _, seen := counted[dev]; !seen || dev == "" {
			counted[dev] = struct{}{}
			usedSum += t - a
			totalSum += t
		}
		if pct := clampPct(100 * (t - a) / t); pct > worst || at == "" {
			worst, at = pct, mp
		}
	}
	return worst, at, int64(usedSum), int64(totalSum)
}

// summaryNet is the per-second byte rate summed over the real
// interfaces.
func (r *Ring) summaryNet(g grid, metric string) float64 {
	devs, names := r.group(metric, lDevice)
	sum := 0.0
	for _, d := range names {
		if !agenttypes.RealNetDevice(d) {
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
