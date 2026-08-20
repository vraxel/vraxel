package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
)

// metricsOpenTimeout bounds the whole exchange: opening the stream on the
// agent's data channel and reading the one JSON body back. The agent is
// answering from memory, so a healthy exchange is milliseconds plus one
// channel establishment; anything past this is a host that is not going
// to answer.
const metricsOpenTimeout = 15 * time.Second

// metricsMaxBody caps the response read. The agent bounds its own
// responses (MetricsMaxPoints), so this guards against a confused or
// hostile peer, not a legitimate answer.
const metricsMaxBody = 4 << 20

// MetricsRequest is one windowed read of a host's chart series.
type MetricsRequest struct {
	FromMs  int64
	ToMs    int64
	StepSec int
	// Series filters by chart-series name; empty means all of them.
	Series []string
}

// MetricsBackend answers chart queries for one host.
//
// An interface with one method because the two tiers answer differently
// and the frontend must not know which one it is talking to: the lite
// tier reads the agent's in-memory ring over the data channel
// (agentLiveMetrics, below); the full tier will translate the same
// vocabulary into PromQL against VictoriaMetrics. Same request, same
// response shape, different provenance.
type MetricsBackend interface {
	Series(ctx context.Context, hostID int64, req MetricsRequest) (*HostMetrics, error)
}

// agentLiveMetrics reads the host's own ring buffer over its data
// channel. The server holds no metric history at all on this path --
// which is the point: nothing is transmitted or stored while nobody is
// looking at a chart.
type agentLiveMetrics struct {
	dialer *AgentDialerHolder
}

// NewAgentLiveMetrics is the lite-tier backend.
func NewAgentLiveMetrics(dialer *AgentDialerHolder) MetricsBackend {
	return agentLiveMetrics{dialer: dialer}
}

func (b agentLiveMetrics) Series(ctx context.Context, hostID int64, req MetricsRequest) (*HostMetrics, error) {
	d := b.dialer.Get()
	if d == nil {
		return nil, apierrors.NewServiceUnavailable("the agent data plane is not wired on this server")
	}

	dctx, cancel := context.WithTimeout(ctx, metricsOpenTimeout)
	defer cancel()
	stream, err := d.StreamFor(dctx, hostID, agenttypes.StreamOpen{
		Kind:    agenttypes.StreamKindMetrics,
		FromMs:  req.FromMs,
		ToMs:    req.ToMs,
		StepSec: req.StepSec,
		Series:  req.Series,
	})
	if err != nil {
		return nil, metricsFailure(err)
	}
	defer stream.Close()

	// The context deadline does not reach into a blocked net.Conn read;
	// the stream's own deadline is what does.
	_ = stream.SetReadDeadline(time.Now().Add(metricsOpenTimeout))
	var res agenttypes.MetricsResult
	if err := json.NewDecoder(io.LimitReader(stream, metricsMaxBody)).Decode(&res); err != nil {
		return nil, apierrors.NewServiceUnavailable("the host's agent did not return a readable answer")
	}
	return hostMetricsToAPI(&res), nil
}

// metricsFailure turns a stream-open failure into something the operator
// can act on, with the REST status carrying the same distinction the
// terminal's WS status frames do: unreachable-shaped problems are 503
// (try again / start the agent), agent-shaped refusals are 409 (the host
// needs an operator's hand first).
func metricsFailure(err error) error {
	switch {
	case errors.Is(err, agentgw.ErrHostUnreachable):
		return apierrors.NewServiceUnavailable("this host's agent is not connected")
	case errors.Is(err, agentgw.ErrHostOnAnotherInstance):
		return apierrors.NewServiceUnavailable("this host is connected to another server replica, which cannot be reached yet")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return apierrors.NewServiceUnavailable("the host did not open its data channel in time")
	}
	var rejected *agentgw.StreamRejected
	if errors.As(err, &rejected) {
		// An agent that predates the metrics stream kind answers exactly
		// as if the kind were misspelled -- from its point of view it is.
		// Naming the fix matters: this is the one refusal an operator
		// resolves by reinstalling, not by waiting.
		if rejected.Code == agenttypes.StreamErrUnknownKind {
			return apierrors.NewConflictMessage("the agent on this host predates metrics support; reinstall the agent to update it")
		}
		return apierrors.NewConflictMessage("the agent declined: " + rejected.Message)
	}
	return apierrors.NewServiceUnavailable("could not read metrics from this host")
}

// hostMetricsToAPI re-types the wire result. MetricValue's NaN-is-null
// convention stops at this boundary: the API speaks *float64, which is
// what the schema generators and the frontend see.
func hostMetricsToAPI(r *agenttypes.MetricsResult) *HostMetrics {
	out := &HostMetrics{
		FromMs:  r.FromMs,
		StepSec: r.StepSec,
		Count:   r.Count,
		Series:  make([]HostMetricsSeries, len(r.Series)),
	}
	for i, s := range r.Series {
		vs := make([]*float64, len(s.Values))
		for j, v := range s.Values {
			if !v.IsNone() {
				f := float64(v)
				vs[j] = &f
			}
		}
		out.Series[i] = HostMetricsSeries{Name: s.Name, Labels: s.Labels, Values: vs}
	}
	return out
}

// hostMetricsOps serves GET /hosts/{id}:metrics.
type hostMetricsOps struct {
	hosts   modstore.HostStore
	backend MetricsBackend
}

// metricsParams are the verb's query parameters, snake_case per the API
// convention.
type metricsParams struct {
	FromMs  *int64  `filter:"from_ms"`
	ToMs    *int64  `filter:"to_ms"`
	StepSec *int64  `filter:"step_sec"`
	Series  *string `filter:"series"`
}

// +openapi:summary=查询主机监控指标
// +openapi:summary.workspaces.hosts=查询工作空间下主机监控指标
// +openapi:summary.workspaces.namespaces.hosts=查询项目下主机监控指标
func (o hostMetricsOps) series(ctx apiserver.Ctx, id int64, q list.Query) (any, error) {
	sf := scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID)
	// Reading the host first is the authorisation: it applies the scope
	// filter, so an id from another tenant is a 404 before anything else
	// happens.
	if _, err := o.hosts.GetByID(ctx, id, sf); err != nil {
		return nil, domainErr(err)
	}

	p := list.Parse[metricsParams](q.Filters)
	req := MetricsRequest{StepSec: 60}
	now := time.Now().UnixMilli()
	req.ToMs, req.FromMs = now, now-time.Hour.Milliseconds()
	if p.ToMs != nil {
		req.ToMs = *p.ToMs
	}
	if p.FromMs != nil {
		req.FromMs = *p.FromMs
	}
	if p.StepSec != nil {
		req.StepSec = int(*p.StepSec)
	}
	if p.Series != nil && *p.Series != "" {
		req.Series = splitCSV(*p.Series)
	}

	// The same bounds the agent enforces, applied before anything goes on
	// the wire: a request the agent would refuse should not cost a
	// data-channel establishment to find out.
	if req.ToMs <= req.FromMs {
		return nil, apierrors.NewBadRequest("to_ms must be after from_ms", nil)
	}
	if req.StepSec < 1 {
		return nil, apierrors.NewBadRequest("step_sec must be at least 1", nil)
	}
	if points := (req.ToMs - req.FromMs) / 1000 / int64(req.StepSec); points > agenttypes.MetricsMaxPoints {
		return nil, apierrors.NewBadRequest(
			fmt.Sprintf("the window at this step is %d points; the limit is %d", points, agenttypes.MetricsMaxPoints), nil)
	}

	res, err := o.backend.Series(ctx, id, req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func splitCSV(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	return out
}
