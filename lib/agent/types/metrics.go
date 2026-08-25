package types

import (
	"encoding/json"
	"math"
)

// MetricsSummary is the current-utilisation snapshot the agent attaches
// to every heartbeat. It is display-ready on purpose: the agent turns
// counters into percentages and rates so the server stores the numbers
// without interpreting them, which is what lets the server keep one row
// per host instead of a time series.
//
// The agent sends NOTHING until it can fill every field. Percentages and
// rates need two consecutive samples, so the first summary is one sample
// interval after the collector starts; a partially-filled snapshot would
// put a real-looking 0% CPU on the host list.
type MetricsSummary struct {
	// SampledAtMs is when the newer of the two samples was taken, by the
	// AGENT's clock. The server stores it as sampled_at and compares it
	// against its own now() to grey out a stale row, so a host with a
	// broken clock shows as stale rather than as fresh-with-wrong-numbers.
	SampledAtMs int64 `json:"sampledAtMs"`

	// CPUUsedPct is 100 * (1 - (idle+iowait)/total) over the interval
	// between the two samples. iowait counts as idle here: the CPU is
	// available, and counting a slow disk as CPU load misdirects whoever
	// reads the list.
	CPUUsedPct float64 `json:"cpuUsedPct"`

	// MemUsedPct is 100 * (1 - MemAvailable/MemTotal). MemAvailable, not
	// MemFree: page cache is reclaimable, and MemFree reports every Linux
	// box that has been up for a day as nearly out of memory.
	MemUsedPct float64 `json:"memUsedPct"`

	// DiskUsedPct is the FULLEST real filesystem, and DiskUsedPath names
	// it. The maximum rather than the root filesystem, because the
	// actionable signal is "some disk is filling up" -- reporting root at
	// 20% while a data mount sits at 95% is worse than reporting nothing.
	//
	// "Real" excludes tmpfs and ramfs: a full /run is not a full disk,
	// and it would pin this field at a number no operator can act on.
	// The fs.* chart series keep node_exporter's own filter instead, so
	// the two answer different questions on purpose.
	DiskUsedPct  float64 `json:"diskUsedPct"`
	DiskUsedPath string  `json:"diskUsedPath,omitempty"`

	// DiskUsedBytes / DiskTotalBytes are the host's whole disk footprint:
	// used and total summed over the SAME real filesystems DiskUsedPct
	// chooses its maximum from. The pair answers "how much disk does this
	// machine have, and how much of it is gone" -- inventory, where
	// DiskUsedPct is a warning. Both are kept because neither can stand
	// in for the other: a full 1 GiB /boot vanishes inside a 600 GiB
	// total, and a 92% /data says nothing about how big the host is.
	//
	// Zero means the agent found no real filesystem to measure, which on
	// Linux does not happen; an agent too old to send these fields omits
	// them and the server stores NULL rather than a fabricated zero.
	DiskUsedBytes  int64 `json:"diskUsedBytes,omitempty"`
	DiskTotalBytes int64 `json:"diskTotalBytes,omitempty"`

	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`

	// NetRxBps / NetTxBps are BYTES per second summed over the physical
	// interfaces, over the same interval as CPUUsedPct.
	NetRxBps float64 `json:"netRxBps"`
	NetTxBps float64 `json:"netTxBps"`

	// CPUTrend is CPUUsedPct over roughly the last 24 hours, in
	// half-hour buckets, oldest first, ending at the bucket that holds
	// SampledAtMs; null entries are buckets the agent has nothing for.
	//
	// It rides the heartbeat so the host LIST can draw a per-row
	// sparkline out of the single SQL query it already makes. The
	// alternative -- fanning a data-channel read out to every listed
	// agent on each page view -- prices the page by its slowest host,
	// multiplies with the refresh interval, and goes dark for hosts
	// whose channel lives on another server instance. Fixed shape, not
	// negotiated: its one consumer is a mini chart that is not
	// interactive. About 300 bytes a beat.
	CPUTrend []MetricValue `json:"cpuTrend,omitempty"`
}

// Chart series names: the vocabulary of the metrics query API.
//
// This is deliberately NOT the node_exporter vocabulary, and the two
// layers must not be conflated:
//
//   - Storage and push speak node_exporter names and labels. That is
//     where the compatibility promise lives: when a deployment turns on
//     the full metric set and pushes to VictoriaMetrics, existing
//     dashboards, PromQL and alert rules keep working unchanged.
//   - This API speaks chart series: already derived, already a
//     percentage or a per-second rate, one number per bucket. The UI
//     draws what it is handed and never sees a counter or writes rate().
//
// The split is what makes the lite and full tiers interchangeable behind
// one frontend: each backend translates into this vocabulary its own way
// (the agent derives from its ring buffer, the VictoriaMetrics backend
// evaluates a PromQL expression), and the charts cannot tell which
// answered.
//
// Names are frozen at v1. Adding one is additive; changing the meaning
// of one is not, because agents in the field keep the old meaning until
// they are re-installed.
const (
	// No labels.
	SeriesCPUUsedPct = "cpu.used_pct"
	SeriesMemUsedPct = "mem.used_pct"
	SeriesMemUsed    = "mem.used_bytes"
	SeriesSwapUsed   = "swap.used_pct"
	SeriesLoad1      = "load.1"
	SeriesLoad5      = "load.5"
	SeriesLoad15     = "load.15"

	// Labelled "mode".
	SeriesCPUModePct = "cpu.mode_pct"

	// Labelled "mountpoint".
	SeriesFSUsedPct = "fs.used_pct"
	SeriesFSSize    = "fs.size_bytes"

	// Labelled "device".
	SeriesDiskReadBps  = "disk.read_bps"
	SeriesDiskWriteBps = "disk.write_bps"
	SeriesDiskUtilPct  = "disk.util_pct"
	SeriesNetRxBps     = "net.rx_bps"
	SeriesNetTxBps     = "net.tx_bps"

	// --- pressure stall information ---
	// The share of wall clock in which at least one task was stalled on
	// the resource. Reported because load average cannot separate "waiting
	// for CPU" from "waiting for a disk" -- it counts both as load -- and
	// because a host can be at 100% CPU while nothing is actually
	// contended. PSI is what says whether the machine is short of the
	// thing, rather than merely busy with it.
	SeriesPSICPUPct = "psi.cpu_pct"
	SeriesPSIMemPct = "psi.mem_pct"
	SeriesPSIIOPct  = "psi.io_pct"

	// --- memory, unlabelled ---
	// The composition behind mem.used_pct: a host "at 90%" that is 70%
	// page cache is not short of memory, and the percentage alone cannot
	// tell an operator which of the two they are looking at.
	SeriesMemTotal   = "mem.total_bytes"
	SeriesMemFree    = "mem.free_bytes"
	SeriesMemBuffers = "mem.buffers_bytes"
	SeriesMemCached  = "mem.cached_bytes"
	// Pages swapped per second. Swap USAGE can sit still at a high value
	// for months on a healthy host; swap TRAFFIC is what says the machine
	// is thrashing right now, and it is the pair that gets confused.
	SeriesSwapInPps  = "swap.in_pps"
	SeriesSwapOutPps = "swap.out_pps"
	// Kills per bucket, not per second: at one kill in a 60s bucket a rate
	// reads 0.016, which rounds to nothing on a chart. The whole value of
	// this series is that a single event is visible.
	SeriesOOMKills = "mem.oom_kills"

	// --- cpu / system counters, unlabelled ---
	SeriesCtxSwitches = "sys.ctx_switches"
	SeriesInterrupts  = "sys.interrupts"

	// --- system state, unlabelled ---
	SeriesUptimeSec    = "sys.uptime_sec"
	SeriesProcsRunning = "sys.procs_running"
	SeriesProcsBlocked = "sys.procs_blocked"
	SeriesFDUsedPct    = "sys.fd_used_pct"
	// Offset from true time in seconds, and whether the kernel considers
	// itself synchronised (1 / 0). A host whose clock has drifted writes
	// metric timestamps nothing can line up -- observed on this platform
	// as charts that were simply empty, with no other field able to say
	// why.
	SeriesTimeDriftSec = "sys.time_drift_sec"
	SeriesTimeSynced   = "sys.time_synced"

	// --- sockets, unlabelled ---
	// Retransmits per second is the single best "is the network healthy"
	// signal: throughput and packet rate both look fine while a link is
	// quietly losing and resending.
	SeriesTCPRetrans  = "tcp.retrans"
	SeriesTCPInUse    = "tcp.in_use"
	SeriesSocketsUsed = "sockets.used"
	// Conntrack fills silently: past the limit the kernel drops packets
	// and logs nothing an application can see.
	SeriesConntrackPct = "conntrack.used_pct"

	// Labelled "kind" (minor / major). Major faults are the ones that
	// actually went to disk.
	SeriesPageFaults = "mem.page_faults"

	// Labelled "mountpoint". A filesystem out of inodes reports plenty of
	// free bytes and refuses to create a file.
	SeriesFSInodesPct = "fs.inodes_used_pct"

	// Labelled "device".
	// Operations per second, distinct from bytes per second: a device can
	// be saturated on either without the other moving.
	SeriesDiskReadIOPS  = "disk.read_iops"
	SeriesDiskWriteIOPS = "disk.write_iops"
	// Mean seconds per operation over the bucket -- the direct answer to
	// "is the disk slow", which utilisation and throughput only imply.
	SeriesDiskReadWait  = "disk.read_wait_sec"
	SeriesDiskWriteWait = "disk.write_wait_sec"
	SeriesNetRxPps      = "net.rx_pps"
	SeriesNetTxPps      = "net.tx_pps"
	SeriesNetRxErrs     = "net.rx_errs"
	SeriesNetTxErrs     = "net.tx_errs"
	SeriesNetRxDrops    = "net.rx_drops"
	SeriesNetTxDrops    = "net.tx_drops"

	// Labelled "chip" and "sensor".
	SeriesTempCelsius = "sys.temp_celsius"
)

// MetricsMaxPoints caps the number of buckets one query may ask for.
//
// Enforced on BOTH ends: the gateway rejects an over-wide request before
// opening a stream, and the agent rejects it again on arrival. The
// gateway check is what keeps a bad request off the wire; the agent
// check is what keeps the bound true regardless of which gateway
// version asked. The same constant is the only thing that keeps the two
// from drifting apart.
//
// 2000 is above any chart's pixel width and below anything that would
// make one response expensive to build or send.
const MetricsMaxPoints = 2000

// MetricsResult is the body of a metrics stream: one fixed grid, one
// value per bucket per series, ready to plot.
//
// The grid is echoed back rather than assumed, because the agent snaps
// the request to the resolution it actually holds -- a caller asking for
// 10-second buckets over 24 hours gets 60-second ones, and StepSec is
// how it learns that.
type MetricsResult struct {
	FromMs  int64           `json:"fromMs"`
	StepSec int             `json:"stepSec"`
	Count   int             `json:"count"`
	Series  []MetricsSeries `json:"series"`
}

// MetricsSeries is one line on a chart. Values has exactly Count
// entries, one per bucket, oldest first.
type MetricsSeries struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
	Values []MetricValue     `json:"values"`
}

// MetricValue is a bucket that may hold nothing.
//
// A gap -- the agent was not running, the disk had not been mounted yet,
// a counter reset left an interval unattributable -- must reach the
// chart as a gap. Encoded as a float64 whose NaN marshals to JSON null
// rather than as *float64, because a 24h response carries tens of
// thousands of these and the pointers would all be garbage.
type MetricValue float64

// MetricValueNone is the absent value.
var MetricValueNone = MetricValue(math.NaN())

// IsNone reports whether the bucket holds no value.
func (v MetricValue) IsNone() bool {
	f := float64(v)
	return math.IsNaN(f) || math.IsInf(f, 0)
}

// MarshalJSON writes null for an absent value, the number otherwise.
func (v MetricValue) MarshalJSON() ([]byte, error) {
	if v.IsNone() {
		return []byte("null"), nil
	}
	return json.Marshal(float64(v))
}

// UnmarshalJSON reads null back as the absent value. Present so the
// gateway can decode a result it forwards, and so a round trip through
// JSON is the identity.
func (v *MetricValue) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*v = MetricValueNone
		return nil
	}
	var f float64
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	*v = MetricValue(f)
	return nil
}
