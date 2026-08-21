package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
}

// vmVocabulary is the translation table: the same chart names the
// agent's ring derives locally, expressed over the node_exporter series
// the full tier pushes. The two backends answering identically for the
// same window is the "lite to full with no seam" promise, so a change
// on either side of this table is a change to both. That includes the
// clamps: every percentage the agent backend runs through clampPct is
// clamped here too, or a deep disk queue reads 100.4% busy on one tier
// and 100% on the other.
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
		expr: `clamp(100 * (1 - node_filesystem_avail_bytes / node_filesystem_size_bytes), 0, 100)`,
		dim:  "mountpoint",
	},
	agenttypes.SeriesFSSize: {
		expr: `node_filesystem_size_bytes`,
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
	agenttypes.SeriesNetRxBps: {
		expr: `rate(node_network_receive_bytes_total[` + vmRateWindow + `])`,
		dim:  "device",
	},
	agenttypes.SeriesNetTxBps: {
		expr: `rate(node_network_transmit_bytes_total[` + vmRateWindow + `])`,
		dim:  "device",
	},
}

// vmVocabularyOrder keeps responses deterministic: map iteration order
// must not decide which line is drawn first.
var vmVocabularyOrder = []string{
	agenttypes.SeriesCPUUsedPct, agenttypes.SeriesCPUModePct,
	agenttypes.SeriesMemUsedPct, agenttypes.SeriesMemUsed, agenttypes.SeriesSwapUsed,
	agenttypes.SeriesLoad1, agenttypes.SeriesLoad5, agenttypes.SeriesLoad15,
	agenttypes.SeriesFSUsedPct, agenttypes.SeriesFSSize,
	agenttypes.SeriesDiskReadBps, agenttypes.SeriesDiskWriteBps, agenttypes.SeriesDiskUtilPct,
	agenttypes.SeriesNetRxBps, agenttypes.SeriesNetTxBps,
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
			}
		}
		out = append(out, HostMetricsSeries{Name: name, Labels: labels, Values: values})
	}
	return out, nil
}
