package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	apierrors "vraxel.io/vraxel/lib/api/errors"
)

// vmQueryTimeout bounds one whole Series call -- every range query it
// makes, together. VictoriaMetrics answers these in milliseconds; a
// backend that cannot is one the operator needs to hear about sooner
// than a browser gives up.
const vmQueryTimeout = 15 * time.Second

// vmMaxBody caps one query_range response read.
const vmMaxBody = 16 << 20

// vmRateWindow is the range every rate() uses. Fixed at 1m rather than
// derived from the chart step: the underlying samples arrive on the
// push interval (~15s), so a minute holds the >=2 samples rate needs at
// any step the API allows, and deriving it from step would smear fine
// charts at coarse steps for no reader-visible gain.
const vmRateWindow = "1m"

// vmExpr is one chart series family translated into PromQL. dim is the
// label kept on the way out (mode / device / mountpoint); everything
// else VictoriaMetrics carries (job, host_id, instance) is backend
// plumbing the chart vocabulary promises to hide.
type vmExpr struct {
	expr string
	dim  string
	// dim2 is the second identifying label, for the one series that needs
	// two: an hwmon reading is a (chip, sensor) pair, and keeping only one
	// of them would collapse a board's several temperatures onto one line
	// -- while the agent backend keeps them apart. Empty for everything
	// else.
	dim2 string
}

// vmVocabulary is the translation table: the same chart names the
// agent's ring derives locally, expressed over the node_exporter series
// the full tier pushes. The two backends answering identically for the
// same window is the "lite to full with no seam" promise, so a change
// on either side of this table is a change to both. That includes the
// clamps: every percentage the agent backend runs through clampPct is
// clamped here too, or a deep disk queue reads 100.4% busy on one tier
// and 100% on the other.
// fsFilter / netFilter are the PromQL spellings of RealFSType and
// RealNetDevice, generated from the very lists those predicates use. The
// agent backend applies them by calling the predicates; expressing the
// same rule twice by hand is how the two tiers would quietly stop
// agreeing about what a host's disk is.
var (
	fsFilter  = `fstype!~"` + strings.Join(agenttypes.NonLocalFSTypes(), "|") + `"`
	netFilter = buildNetFilter()
)

func buildNetFilter() string {
	prefixes, exact := agenttypes.VirtualNetPrefixes()
	alts := make([]string, 0, len(prefixes)+len(exact))
	alts = append(alts, exact...)
	for _, p := range prefixes {
		// Anchored implicitly by PromQL (=~ matches the whole label), so a
		// prefix rule needs its own trailing wildcard.
		alts = append(alts, regexp.QuoteMeta(p)+".*")
	}
	return `device!~"` + strings.Join(alts, "|") + `"`
}

// rate wraps a counter in the shared rate window, so no expression below
// spells the window out and none can drift from the others.
func rate(metric string) string { return `rate(` + metric + `[` + vmRateWindow + `])` }

var vmVocabulary = map[string]vmExpr{
	agenttypes.SeriesCPUUsedPct: {
		expr: `clamp(100 * (1 - sum(rate(node_cpu_seconds_total{mode=~"idle|iowait"}[` + vmRateWindow + `])) / sum(rate(node_cpu_seconds_total[` + vmRateWindow + `]))), 0, 100)`,
	},
	agenttypes.SeriesCPUModePct: {
		expr: `clamp(100 * sum by (mode) (rate(node_cpu_seconds_total[` + vmRateWindow + `])) / on () group_left () sum(rate(node_cpu_seconds_total[` + vmRateWindow + `])), 0, 100)`,
		dim:  "mode",
	},
	agenttypes.SeriesMemUsedPct: {
		expr: `clamp(100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes), 0, 100)`,
	},
	agenttypes.SeriesMemUsed: {
		expr: `node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes`,
	},
	// A swapless host has SwapTotal 0 and this expression yields no
	// points, where the agent backend reports a flat 0. Divergence
	// accepted: rendering it as 0 would need per-host or() gymnastics to
	// keep the vector matched, for a line nobody reads on such hosts.
	agenttypes.SeriesSwapUsed: {
		expr: `clamp(100 * (1 - node_memory_SwapFree_bytes / node_memory_SwapTotal_bytes), 0, 100)`,
	},
	agenttypes.SeriesLoad1:  {expr: `node_load1`},
	agenttypes.SeriesLoad5:  {expr: `node_load5`},
	agenttypes.SeriesLoad15: {expr: `node_load15`},
	agenttypes.SeriesFSUsedPct: {
		expr: `clamp(100 * (1 - node_filesystem_avail_bytes{` + fsFilter + `} / node_filesystem_size_bytes{` + fsFilter + `}), 0, 100)`,
		dim:  "mountpoint",
	},
	agenttypes.SeriesFSSize: {
		expr: `node_filesystem_size_bytes{` + fsFilter + `}`,
		dim:  "mountpoint",
	},
	agenttypes.SeriesFSInodesPct: {
		expr: `clamp(100 * (1 - node_filesystem_files_free{` + fsFilter + `} / node_filesystem_files{` + fsFilter + `}), 0, 100)`,
		dim:  "mountpoint",
	},
	agenttypes.SeriesDiskReadBps: {
		expr: `rate(node_disk_read_bytes_total[` + vmRateWindow + `])`,
		dim:  "device",
	},
	agenttypes.SeriesDiskWriteBps: {
		expr: `rate(node_disk_written_bytes_total[` + vmRateWindow + `])`,
		dim:  "device",
	},
	agenttypes.SeriesDiskUtilPct: {
		expr: `clamp(100 * rate(node_disk_io_time_seconds_total[` + vmRateWindow + `]), 0, 100)`,
		dim:  "device",
	},
	agenttypes.SeriesNetRxBps:   {expr: rate(`node_network_receive_bytes_total{` + netFilter + `}`), dim: "device"},
	agenttypes.SeriesNetTxBps:   {expr: rate(`node_network_transmit_bytes_total{` + netFilter + `}`), dim: "device"},
	agenttypes.SeriesNetRxPps:   {expr: rate(`node_network_receive_packets_total{` + netFilter + `}`), dim: "device"},
	agenttypes.SeriesNetTxPps:   {expr: rate(`node_network_transmit_packets_total{` + netFilter + `}`), dim: "device"},
	agenttypes.SeriesNetRxErrs:  {expr: rate(`node_network_receive_errs_total{` + netFilter + `}`), dim: "device"},
	agenttypes.SeriesNetTxErrs:  {expr: rate(`node_network_transmit_errs_total{` + netFilter + `}`), dim: "device"},
	agenttypes.SeriesNetRxDrops: {expr: rate(`node_network_receive_drop_total{` + netFilter + `}`), dim: "device"},
	agenttypes.SeriesNetTxDrops: {expr: rate(`node_network_transmit_drop_total{` + netFilter + `}`), dim: "device"},

	// --- memory composition ---
	agenttypes.SeriesMemTotal:   {expr: `node_memory_MemTotal_bytes`},
	agenttypes.SeriesMemFree:    {expr: `node_memory_MemFree_bytes`},
	agenttypes.SeriesMemBuffers: {expr: `node_memory_Buffers_bytes`},
	agenttypes.SeriesMemCached:  {expr: `node_memory_Cached_bytes`},
	agenttypes.SeriesSwapInPps:  {expr: rate(`node_vmstat_pswpin`)},
	agenttypes.SeriesSwapOutPps: {expr: rate(`node_vmstat_pswpout`)},
	// label_replace rather than two series, so the "kind" dimension the
	// agent backend attaches survives on this tier too.
	agenttypes.SeriesPageFaults: {
		expr: `label_replace(` + rate(`node_vmstat_pgfault`) + `, "kind", "minor", "", "") or ` +
			`label_replace(` + rate(`node_vmstat_pgmajfault`) + `, "kind", "major", "", "")`,
		dim: "kind",
	},
	// increase() over the step, not rate(): one kill has to read as 1.
	agenttypes.SeriesOOMKills: {expr: `increase(node_vmstat_oom_kill[` + vmRateWindow + `])`},

	// --- pressure ---
	agenttypes.SeriesPSICPUPct: {expr: `clamp(100 * ` + rate(`node_pressure_cpu_waiting_seconds_total`) + `, 0, 100)`},
	agenttypes.SeriesPSIMemPct: {expr: `clamp(100 * ` + rate(`node_pressure_memory_waiting_seconds_total`) + `, 0, 100)`},
	agenttypes.SeriesPSIIOPct:  {expr: `clamp(100 * ` + rate(`node_pressure_io_waiting_seconds_total`) + `, 0, 100)`},

	// --- cpu / system counters ---
	agenttypes.SeriesCtxSwitches: {expr: rate(`node_context_switches_total`)},
	agenttypes.SeriesInterrupts:  {expr: rate(`node_intr_total`)},

	// --- disk ops and latency ---
	agenttypes.SeriesDiskReadIOPS:  {expr: rate(`node_disk_reads_completed_total`), dim: "device"},
	agenttypes.SeriesDiskWriteIOPS: {expr: rate(`node_disk_writes_completed_total`), dim: "device"},
	// Mean seconds per operation. The `> 0` guard is what the agent
	// backend's "an idle device has no latency" rule looks like in PromQL:
	// without it the division is 0/0 and the chart draws NaN as a break
	// anyway, but with it an idle device drops out cleanly instead.
	agenttypes.SeriesDiskReadWait: {
		expr: rate(`node_disk_read_time_seconds_total`) + ` / (` + rate(`node_disk_reads_completed_total`) + ` > 0)`,
		dim:  "device",
	},
	agenttypes.SeriesDiskWriteWait: {
		expr: rate(`node_disk_write_time_seconds_total`) + ` / (` + rate(`node_disk_writes_completed_total`) + ` > 0)`,
		dim:  "device",
	},

	// --- sockets ---
	agenttypes.SeriesTCPRetrans:  {expr: rate(`node_netstat_Tcp_RetransSegs`)},
	agenttypes.SeriesTCPInUse:    {expr: `node_sockstat_TCP_inuse`},
	agenttypes.SeriesSocketsUsed: {expr: `node_sockstat_sockets_used`},
	agenttypes.SeriesConntrackPct: {
		expr: `clamp(100 * node_nf_conntrack_entries / (node_nf_conntrack_entries_limit > 0), 0, 100)`,
	},

	// --- system state ---
	agenttypes.SeriesUptimeSec:    {expr: `node_time_seconds - node_boot_time_seconds`},
	agenttypes.SeriesProcsRunning: {expr: `node_procs_running`},
	agenttypes.SeriesProcsBlocked: {expr: `node_procs_blocked`},
	agenttypes.SeriesFDUsedPct:    {expr: `clamp(100 * node_filefd_allocated / (node_filefd_maximum > 0), 0, 100)`},
	agenttypes.SeriesTimeDriftSec: {expr: `node_timex_offset_seconds`},
	agenttypes.SeriesTimeSynced:   {expr: `node_timex_sync_status`},
	agenttypes.SeriesTempCelsius:  {expr: `node_hwmon_temp_celsius`, dim: "chip", dim2: "sensor"},
}

// vmVocabularyOrder keeps responses deterministic: map iteration order
// must not decide which line is drawn first.
//
// It must also list EVERY key in vmVocabulary, because an unlisted one is
// simply never queried when the caller asks for all series -- a chart
// that silently stays empty on this tier and works on the other.
// TestVMVocabularyOrderIsComplete holds the two together.
var vmVocabularyOrder = []string{
	agenttypes.SeriesCPUUsedPct, agenttypes.SeriesCPUModePct,
	agenttypes.SeriesPSICPUPct, agenttypes.SeriesCtxSwitches, agenttypes.SeriesInterrupts,
	agenttypes.SeriesLoad1, agenttypes.SeriesLoad5, agenttypes.SeriesLoad15,

	agenttypes.SeriesMemUsedPct, agenttypes.SeriesMemUsed, agenttypes.SeriesSwapUsed,
	agenttypes.SeriesMemTotal, agenttypes.SeriesMemFree,
	agenttypes.SeriesMemBuffers, agenttypes.SeriesMemCached,
	agenttypes.SeriesSwapInPps, agenttypes.SeriesSwapOutPps,
	agenttypes.SeriesPageFaults, agenttypes.SeriesOOMKills, agenttypes.SeriesPSIMemPct,

	agenttypes.SeriesFSUsedPct, agenttypes.SeriesFSSize, agenttypes.SeriesFSInodesPct,
	agenttypes.SeriesDiskReadBps, agenttypes.SeriesDiskWriteBps, agenttypes.SeriesDiskUtilPct,
	agenttypes.SeriesDiskReadIOPS, agenttypes.SeriesDiskWriteIOPS,
	agenttypes.SeriesDiskReadWait, agenttypes.SeriesDiskWriteWait,
	agenttypes.SeriesPSIIOPct,

	agenttypes.SeriesNetRxBps, agenttypes.SeriesNetTxBps,
	agenttypes.SeriesNetRxPps, agenttypes.SeriesNetTxPps,
	agenttypes.SeriesNetRxErrs, agenttypes.SeriesNetTxErrs,
	agenttypes.SeriesNetRxDrops, agenttypes.SeriesNetTxDrops,
	agenttypes.SeriesTCPRetrans, agenttypes.SeriesTCPInUse,
	agenttypes.SeriesSocketsUsed, agenttypes.SeriesConntrackPct,

	agenttypes.SeriesUptimeSec, agenttypes.SeriesProcsRunning, agenttypes.SeriesProcsBlocked,
	agenttypes.SeriesFDUsedPct, agenttypes.SeriesTimeDriftSec, agenttypes.SeriesTimeSynced,
	agenttypes.SeriesTempCelsius,
}

// vmMetrics answers chart queries from VictoriaMetrics: the full tier.
// Same request, same response shape as agentLiveMetrics -- the frontend
// cannot tell which one answered, which is the point of the interface.
type vmMetrics struct {
	base   string
	client *http.Client
}

// NewVMMetrics is the full-tier backend, querying the VictoriaMetrics
// at queryURL.
func NewVMMetrics(queryURL string) MetricsBackend {
	return vmMetrics{
		base:   queryURL,
		client: &http.Client{Timeout: vmQueryTimeout},
	}
}

func (b vmMetrics) Series(ctx context.Context, hostID int64, req MetricsRequest) (*HostMetrics, error) {
	stepMs := int64(req.StepSec) * 1000
	fromMs := req.FromMs / stepMs * stepMs
	count := int(((req.ToMs+stepMs-1)/stepMs*stepMs - fromMs) / stepMs)
	if count <= 0 {
		return nil, apierrors.NewBadRequest("to_ms must be after from_ms", nil)
	}

	names := req.Series
	if len(names) == 0 {
		names = vmVocabularyOrder
	}

	qctx, cancel := context.WithTimeout(ctx, vmQueryTimeout)
	defer cancel()

	// Series starts non-nil so an empty answer marshals as [] -- the
	// agentlive backend's shape, and what the frontend iterates without
	// a null check. The two backends being byte-interchangeable is the
	// contract; null-vs-[] is exactly the kind of seam it forbids.
	out := &HostMetrics{FromMs: fromMs, StepSec: req.StepSec, Count: count, Series: []HostMetricsSeries{}}
	for _, name := range names {
		v, ok := vmVocabulary[name]
		if !ok {
			continue
		}
		series, err := b.rangeQuery(qctx, hostID, v, fromMs, stepMs, count, name)
		if err != nil {
			return nil, err
		}
		out.Series = append(out.Series, series...)
	}
	return out, nil
}

func (b vmMetrics) rangeQuery(ctx context.Context, hostID int64, v vmExpr, fromMs, stepMs int64, count int, name string) ([]HostMetricsSeries, error) {
	params := url.Values{}
	params.Set("query", v.expr)
	params.Set("start", strconv.FormatFloat(float64(fromMs)/1000, 'f', 3, 64))
	params.Set("end", strconv.FormatFloat(float64(fromMs+int64(count-1)*stepMs)/1000, 'f', 3, 64))
	params.Set("step", strconv.FormatInt(stepMs/1000, 10)+"s")
	// The host filter is VictoriaMetrics' extra_filters, applied to
	// every selector in the expression -- which is what makes one
	// translation table serve every host instead of interpolating ids
	// into query strings.
	params.Add("extra_filters[]", `{host_id="`+strconv.FormatInt(hostID, 10)+`",job="vraxel-agent"}`)

	reqURL := b.base + "/api/v1/query_range?" + params.Encode()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, apierrors.NewServiceUnavailable("the metrics backend did not answer")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, vmMaxBody))
	if err != nil {
		return nil, apierrors.NewServiceUnavailable("the metrics backend did not answer")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apierrors.NewServiceUnavailable(fmt.Sprintf("the metrics backend returned %d", resp.StatusCode))
	}

	var parsed struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Values [][2]json.RawMessage
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Status != "success" {
		return nil, apierrors.NewServiceUnavailable("the metrics backend returned an unreadable answer")
	}

	out := make([]HostMetricsSeries, 0, len(parsed.Data.Result))
	for _, r := range parsed.Data.Result {
		values := make([]*float64, count)
		for _, p := range r.Values {
			var ts float64
			var raw string
			if json.Unmarshal(p[0], &ts) != nil || json.Unmarshal(p[1], &raw) != nil {
				continue
			}
			f, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				// "NaN" and friends: a bucket with no honest number is a
				// gap, exactly as the agent backend renders it.
				continue
			}
			idx := (int64(ts*1000) - fromMs) / stepMs
			if idx >= 0 && idx < int64(count) {
				values[idx] = &f
			}
		}
		var labels map[string]string
		if v.dim != "" {
			if d, ok := r.Metric[v.dim]; ok {
				labels = map[string]string{v.dim: d}
				if v.dim2 != "" {
					if d2, ok := r.Metric[v.dim2]; ok {
						labels[v.dim2] = d2
					}
				}
			}
		}
		out = append(out, HostMetricsSeries{Name: name, Labels: labels, Values: values})
	}
	return out, nil
}
