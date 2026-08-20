package nodemetrics

import (
	dto "github.com/prometheus/client_model/go"
)

// familiesToSample converts one Gather into ring points.
//
// Platform-neutral on purpose: the embedded collector set only builds on
// Linux, but this conversion -- and therefore everything downstream of
// it -- is testable from fixture families on the macOS machines that run
// make check.
func familiesToSample(atMs int64, fams []*dto.MetricFamily) Sample {
	out := Sample{AtMs: atMs, Points: make([]Point, 0, 512)}
	for _, f := range fams {
		var kind Kind
		switch f.GetType() {
		case dto.MetricType_COUNTER:
			kind = Counter
		// Untyped is read as a gauge, the same guess Prometheus makes.
		case dto.MetricType_GAUGE, dto.MetricType_UNTYPED:
			kind = Gauge
		default:
			// Histograms and summaries do not fit a one-value-per-slot
			// ring. node_exporter emits none by default; if an enabled
			// collector ever does, its other series still land.
			continue
		}
		name := f.GetName()
		for _, m := range f.GetMetric() {
			var v float64
			switch f.GetType() {
			case dto.MetricType_COUNTER:
				v = m.GetCounter().GetValue()
			case dto.MetricType_GAUGE:
				v = m.GetGauge().GetValue()
			default:
				v = m.GetUntyped().GetValue()
			}
			var labels []Label
			if lp := m.GetLabel(); len(lp) > 0 {
				labels = make([]Label, 0, len(lp))
				for _, l := range lp {
					labels = append(labels, Label{Name: l.GetName(), Value: l.GetValue()})
				}
			}
			out.Points = append(out.Points, Point{Name: name, Labels: labels, Kind: kind, Value: v})
		}
	}
	return out
}
