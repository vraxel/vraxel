package datachan

import (
	"encoding/json"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

type fakeQuerier struct {
	res agenttypes.MetricsResult
	err error

	gotFrom, gotTo int64
	gotStep        int
	gotSeries      []string
}

func (f *fakeQuerier) Query(fromMs, toMs int64, stepSec int, names []string) (agenttypes.MetricsResult, error) {
	f.gotFrom, f.gotTo, f.gotStep, f.gotSeries = fromMs, toMs, stepSec, names
	return f.res, f.err
}

func TestMetricsStreamRoundTrip(t *testing.T) {
	q := &fakeQuerier{res: agenttypes.MetricsResult{
		FromMs: 0, StepSec: 60, Count: 2,
		Series: []agenttypes.MetricsSeries{{
			Name:   agenttypes.SeriesCPUUsedPct,
			Values: []agenttypes.MetricValue{12.5, agenttypes.MetricValueNone},
		}},
	}}
	g := newGateway(t, NewGuard(nil), func(c *Config) { c.Metrics = q })
	sess := g.session(t)

	stream, acc := open(t, sess, agenttypes.StreamOpen{
		Kind:    agenttypes.StreamKindMetrics,
		FromMs:  1000,
		ToMs:    121000,
		StepSec: 60,
		Series:  []string{agenttypes.SeriesCPUUsedPct},
	})
	defer stream.Close()
	if !acc.Ok {
		t.Fatalf("stream rejected: %+v", acc)
	}

	var res agenttypes.MetricsResult
	if err := json.NewDecoder(stream).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if q.gotFrom != 1000 || q.gotTo != 121000 || q.gotStep != 60 || len(q.gotSeries) != 1 {
		t.Fatalf("request not forwarded: %+v", q)
	}
	if len(res.Series) != 1 || res.Series[0].Values[0] != 12.5 || !res.Series[0].Values[1].IsNone() {
		t.Fatalf("result mangled: %+v", res)
	}
}

func TestMetricsStreamWithoutCollectorRejected(t *testing.T) {
	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)

	stream, acc := open(t, sess, agenttypes.StreamOpen{Kind: agenttypes.StreamKindMetrics})
	defer stream.Close()
	if acc.Ok || acc.Code != agenttypes.StreamErrOpFailed {
		t.Fatalf("a host without a collector must reject, got %+v", acc)
	}
}

func TestMetricsStreamQueryErrorRejected(t *testing.T) {
	q := &fakeQuerier{err: errTooWide}
	g := newGateway(t, NewGuard(nil), func(c *Config) { c.Metrics = q })
	sess := g.session(t)

	stream, acc := open(t, sess, agenttypes.StreamOpen{Kind: agenttypes.StreamKindMetrics})
	defer stream.Close()
	if acc.Ok || acc.Error != errTooWide.Error() {
		t.Fatalf("query errors must reach the gateway verbatim, got %+v", acc)
	}
}

var errTooWide = errTest("metrics: too many points for one query")

type errTest string

func (e errTest) Error() string { return string(e) }
