package nodemetrics

import (
	"encoding/json"
	"testing"

	dto "github.com/prometheus/client_model/go"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

func fp(v float64) *float64 { return &v }
func sp(s string) *string   { return &s }
func tp(t dto.MetricType) *dto.MetricType {
	return &t
}

func TestFamiliesToSample(t *testing.T) {
	fams := []*dto.MetricFamily{
		{
			Name: sp("node_cpu_seconds_total"),
			Type: tp(dto.MetricType_COUNTER),
			Metric: []*dto.Metric{{
				Label: []*dto.LabelPair{
					{Name: sp("cpu"), Value: sp("0")},
					{Name: sp("mode"), Value: sp("idle")},
				},
				Counter: &dto.Counter{Value: fp(12.5)},
			}},
		},
		{
			Name:   sp("node_load1"),
			Type:   tp(dto.MetricType_GAUGE),
			Metric: []*dto.Metric{{Gauge: &dto.Gauge{Value: fp(0.7)}}},
		},
		{
			Name:   sp("some_untyped"),
			Type:   tp(dto.MetricType_UNTYPED),
			Metric: []*dto.Metric{{Untyped: &dto.Untyped{Value: fp(3)}}},
		},
		{
			// No slot shape for these: dropped, others unaffected.
			Name:   sp("some_histogram"),
			Type:   tp(dto.MetricType_HISTOGRAM),
			Metric: []*dto.Metric{{Histogram: &dto.Histogram{}}},
		},
	}

	s := familiesToSample(42, fams)
	if s.AtMs != 42 || len(s.Points) != 3 {
		t.Fatalf("points: %d at %d", len(s.Points), s.AtMs)
	}
	cpu := s.Points[0]
	if cpu.Kind != Counter || cpu.Value != 12.5 ||
		len(cpu.Labels) != 2 || cpu.Labels[1].Value != "idle" {
		t.Fatalf("counter point mangled: %+v", cpu)
	}
	if s.Points[1].Kind != Gauge || s.Points[2].Kind != Gauge {
		t.Fatalf("gauge/untyped must both read back as gauges")
	}
}

func TestMetricValueJSONRoundTrip(t *testing.T) {
	in := []agenttypes.MetricValue{1.5, agenttypes.MetricValueNone}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[1.5,null]" {
		t.Fatalf("marshal: %s", raw)
	}
	var out []agenttypes.MetricValue
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out[0] != 1.5 || !out[1].IsNone() {
		t.Fatalf("round trip: %v", out)
	}
}
