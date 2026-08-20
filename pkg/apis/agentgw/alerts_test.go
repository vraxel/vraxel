package agentgw

import (
	"context"
	"testing"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

type fakeAlertStore struct {
	rules  []gwstore.AlertRule
	states map[int64]gwstore.AlertState

	breaches []int64
	fired    []int64
	cleared  []int64
}

func (f *fakeAlertStore) EnabledRules(context.Context) ([]gwstore.AlertRule, error) {
	return f.rules, nil
}

func (f *fakeAlertStore) States(context.Context, int64) ([]gwstore.AlertState, error) {
	var out []gwstore.AlertState
	for _, s := range f.states {
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeAlertStore) RecordBreach(_ context.Context, _ int64, ruleID int64, _ float64) error {
	f.breaches = append(f.breaches, ruleID)
	return nil
}

func (f *fakeAlertStore) MarkFiring(_ context.Context, _ int64, ruleID int64, _ float64) (bool, error) {
	f.fired = append(f.fired, ruleID)
	return true, nil
}

func (f *fakeAlertStore) SweepDisabled(context.Context) error { return nil }

func (f *fakeAlertStore) Clear(_ context.Context, _ int64, ruleID int64) (bool, error) {
	s, ok := f.states[ruleID]
	f.cleared = append(f.cleared, ruleID)
	return ok && s.Firing, nil
}

type fakeScopes struct{}

func (fakeScopes) GetHostScope(context.Context, int64) (string, *int64, *int64, error) {
	return "platform", nil, nil, nil
}

func cpuRule(id int64, threshold float64, forSecs int32) gwstore.AlertRule {
	return gwstore.AlertRule{
		ID: id, Name: "cpu-high", Scope: "platform",
		Metric: "cpu_used_pct", Op: "gt", Threshold: threshold, ForSeconds: forSecs,
	}
}

func beat(cpu float64) *agenttypes.MetricsSummary {
	return &agenttypes.MetricsSummary{CPUUsedPct: cpu}
}

func newTestEvaluator(store *fakeAlertStore) *alertEvaluator {
	return newAlertEvaluator(store, fakeScopes{})
}

func TestAlertLifecycle(t *testing.T) {
	store := &fakeAlertStore{rules: []gwstore.AlertRule{cpuRule(1, 90, 60)}, states: map[int64]gwstore.AlertState{}}
	e := newTestEvaluator(store)
	ctx := context.Background()

	// Healthy beat: nothing happens.
	e.Evaluate(ctx, 7, beat(10))
	if len(store.breaches)+len(store.fired)+len(store.cleared) != 0 {
		t.Fatalf("a healthy beat must write nothing: %+v", store)
	}

	// First breaching beat: the debounce clock starts, nothing fires.
	e.Evaluate(ctx, 7, beat(95))
	if len(store.breaches) != 1 || len(store.fired) != 0 {
		t.Fatalf("first breach must record and not fire: %+v", store)
	}

	// Still breaching but within for_seconds: no flip.
	store.states[1] = gwstore.AlertState{RuleID: 1, BreachedSince: time.Now().Add(-10 * time.Second)}
	e.Evaluate(ctx, 7, beat(96))
	if len(store.fired) != 0 {
		t.Fatalf("breach inside the debounce window must not fire")
	}

	// Breach held past for_seconds: fires.
	store.states[1] = gwstore.AlertState{RuleID: 1, BreachedSince: time.Now().Add(-2 * time.Minute)}
	e.Evaluate(ctx, 7, beat(97))
	if len(store.fired) != 1 {
		t.Fatalf("held breach must fire: %+v", store)
	}

	// Recovery clears.
	store.states[1] = gwstore.AlertState{RuleID: 1, BreachedSince: time.Now().Add(-2 * time.Minute), Firing: true}
	e.Evaluate(ctx, 7, beat(10))
	if len(store.cleared) != 1 {
		t.Fatalf("recovery must clear: %+v", store)
	}
}

func TestAlertNilSummaryDoesNothing(t *testing.T) {
	store := &fakeAlertStore{rules: []gwstore.AlertRule{cpuRule(1, 90, 0)}, states: map[int64]gwstore.AlertState{}}
	newTestEvaluator(store).Evaluate(context.Background(), 7, nil)
	if len(store.breaches) != 0 {
		t.Fatal("a beat without a snapshot must not evaluate")
	}
}

func TestAlertScopeMatching(t *testing.T) {
	ws1, ws2 := int64(1), int64(2)
	rules := []gwstore.AlertRule{
		{ID: 1, Scope: "platform", Metric: "cpu_used_pct", Op: "gt", Threshold: 50},
		{ID: 2, Scope: "workspace", WorkspaceID: &ws1, Metric: "cpu_used_pct", Op: "gt", Threshold: 50},
		{ID: 3, Scope: "workspace", WorkspaceID: &ws2, Metric: "cpu_used_pct", Op: "gt", Threshold: 50},
	}
	got := applicableRules(rules, &ws1, nil)
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("a ws1 host must see the platform rule and its own workspace's: %+v", got)
	}
	if got := applicableRules(rules, nil, nil); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("a platform host must see only platform rules: %+v", got)
	}
}

// A rule disabled while its state row lived must have the row cleared,
// or it would sit as a firing alert for a rule nobody can see.
func TestAlertOrphanedStateCleared(t *testing.T) {
	store := &fakeAlertStore{
		rules:  []gwstore.AlertRule{cpuRule(1, 90, 60)},
		states: map[int64]gwstore.AlertState{99: {RuleID: 99, Firing: true, BreachedSince: time.Now()}},
	}
	newTestEvaluator(store).Evaluate(context.Background(), 7, beat(10))
	if len(store.cleared) != 1 || store.cleared[0] != 99 {
		t.Fatalf("orphaned state must be cleared: %+v", store.cleared)
	}
}

func TestAlertZeroDebounceStillWaitsOneBeat(t *testing.T) {
	store := &fakeAlertStore{rules: []gwstore.AlertRule{cpuRule(1, 90, 0)}, states: map[int64]gwstore.AlertState{}}
	e := newTestEvaluator(store)
	e.Evaluate(context.Background(), 7, beat(95))
	if len(store.fired) != 0 {
		t.Fatal("even a zero-debounce rule must not fire off a single observation")
	}
	store.states[1] = gwstore.AlertState{RuleID: 1, BreachedSince: time.Now().Add(-time.Second)}
	e.Evaluate(context.Background(), 7, beat(95))
	if len(store.fired) != 1 {
		t.Fatal("the second breaching beat clears a zero debounce")
	}
}

func TestSummaryValueUnknownMetricIsAbsent(t *testing.T) {
	if _, ok := summaryValue(beat(50), "made_up"); ok {
		t.Fatal("an unknown metric key must read as absent, never as zero")
	}
	if v, ok := summaryValue(&agenttypes.MetricsSummary{Load5: 1.5}, "load5"); !ok || v != 1.5 {
		t.Fatalf("load5: %v %v", v, ok)
	}
}

func TestBreachOps(t *testing.T) {
	cases := []struct {
		v, th float64
		op    string
		want  bool
	}{
		{95, 90, "gt", true}, {90, 90, "gt", false}, {90, 90, "ge", true},
		{1, 5, "lt", true}, {5, 5, "le", true}, {6, 5, "le", false},
		{95, 90, "??", false},
	}
	for _, c := range cases {
		if got := breaches(c.v, c.op, c.th); got != c.want {
			t.Fatalf("%v %s %v = %v, want %v", c.v, c.op, c.th, got, c.want)
		}
	}
}
