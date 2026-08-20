package agentgw

import (
	"context"
	"sync"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	"vraxel.io/vraxel/lib/logger"
	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

// alertRulesTTL is how stale the in-process rule cache may be. Rules
// change at human speed and tolerate this the same way scrape targets
// tolerate their 30s cache; per-instance caches need no coordination
// because a rule edit merely takes up to a TTL to be enforced
// everywhere.
const alertRulesTTL = 30 * time.Second

// HostScopes is the one thing the evaluator needs from the host module:
// which tenancy a host belongs to, so a workspace rule stays inside its
// workspace. A dependency interface (like HostRegistrar) because agentgw
// must not import compute.
type HostScopes interface {
	GetHostScope(ctx context.Context, hostID int64) (scope string, workspaceID, namespaceID *int64, err error)
}

// alertEvaluator applies threshold rules to heartbeat snapshots.
//
// It runs HERE -- server side, on the instance holding the host's
// control channel -- and not on the agent, because the numbers are
// already here, a rule edit must not require touching a fleet with no
// self-upgrade, and single-writer-per-host falls out of channel
// ownership with no distributed locking.
//
// The debounce is wall clock, not a beat count: for_seconds means "the
// breach held this long", which stays true across missed beats and
// reconnects, where a counter of consecutive beats would quietly reset.
type alertEvaluator struct {
	store  gwstore.AlertStore
	scopes HostScopes

	mu        sync.Mutex
	rules     []gwstore.AlertRule
	fetchedAt time.Time
}

func newAlertEvaluator(store gwstore.AlertStore, scopes HostScopes) *alertEvaluator {
	return &alertEvaluator{store: store, scopes: scopes}
}

// Evaluate applies every applicable rule to one heartbeat's snapshot.
// Failures are logged and swallowed for the same reason recordMetrics
// swallows them: alerting must never be able to take a control channel
// down, and a beat lost to a blip is repaired by the next one.
func (e *alertEvaluator) Evaluate(ctx context.Context, hostID int64, m *agenttypes.MetricsSummary) {
	if m == nil || e == nil {
		return
	}
	rules := e.enabledRules(ctx)
	if len(rules) == 0 {
		// The common case for most deployments, and it must stay free:
		// no scope lookup, no states query, nothing.
		return
	}

	_, wsID, nsID, err := e.scopes.GetHostScope(ctx, hostID)
	if err != nil {
		logger.Warnf("agentgw: alert evaluation for host %d: resolve scope: %v", hostID, err)
		return
	}
	applicable := applicableRules(rules, wsID, nsID)
	if len(applicable) == 0 {
		return
	}

	states, err := e.store.States(ctx, hostID)
	if err != nil {
		logger.Warnf("agentgw: alert evaluation for host %d: read states: %v", hostID, err)
		return
	}
	byRule := make(map[int64]gwstore.AlertState, len(states))
	for _, s := range states {
		byRule[s.RuleID] = s
	}

	now := time.Now()
	for _, r := range applicable {
		value, ok := summaryValue(m, r.Metric)
		if !ok {
			continue
		}
		state, seen := byRule[r.ID]
		if !breaches(value, r.Op, r.Threshold) {
			if seen {
				if wasFiring, err := e.store.Clear(ctx, hostID, r.ID); err != nil {
					logger.Warnf("agentgw: clear alert %d on host %d: %v", r.ID, hostID, err)
				} else if wasFiring {
					logger.Infof("agentgw: alert %q resolved on host %d (%s %s %.4g, now %.4g)",
						r.Name, hostID, r.Metric, r.Op, r.Threshold, value)
				}
			}
			continue
		}
		if !seen {
			if err := e.store.RecordBreach(ctx, hostID, r.ID, value); err != nil {
				logger.Warnf("agentgw: record breach of alert %d on host %d: %v", r.ID, hostID, err)
			}
			// Zero-debounce rules still wait one beat before firing.
			// Deliberate: firing off a single observation is what
			// for_seconds exists to prevent, and one beat is its floor.
			continue
		}
		if !state.Firing && now.Sub(state.BreachedSince) >= time.Duration(r.ForSeconds)*time.Second {
			if flipped, err := e.store.MarkFiring(ctx, hostID, r.ID, value); err != nil {
				logger.Warnf("agentgw: fire alert %d on host %d: %v", r.ID, hostID, err)
			} else if flipped {
				logger.Warnf("agentgw: alert %q firing on host %d (%s %s %.4g, at %.4g for %s)",
					r.Name, hostID, r.Metric, r.Op, r.Threshold, value, now.Sub(state.BreachedSince).Round(time.Second))
			}
		}
	}

	// Rules disabled (or moved out of this host's tenancy) while a state
	// row existed: the loop above never visits them, so the row would sit
	// there as a firing alert for a rule nobody can see. Deletion is
	// covered by the schema's cascade; this covers the rest.
	for id := range byRule {
		if !ruleStillApplies(applicable, id) {
			if _, err := e.store.Clear(ctx, hostID, id); err != nil {
				logger.Warnf("agentgw: clear orphaned alert %d on host %d: %v", id, hostID, err)
			}
		}
	}
}

// enabledRules serves from the cache, refetching at most once per TTL.
func (e *alertEvaluator) enabledRules(ctx context.Context) []gwstore.AlertRule {
	e.mu.Lock()
	defer e.mu.Unlock()
	if time.Since(e.fetchedAt) < alertRulesTTL {
		return e.rules
	}
	rules, err := e.store.EnabledRules(ctx)
	if err != nil {
		logger.Warnf("agentgw: refresh alert rules: %v", err)
		// Keep serving the stale set: enforcement with old rules beats
		// no enforcement during a database blip.
		return e.rules
	}
	e.rules, e.fetchedAt = rules, time.Now()
	return e.rules
}

// applicableRules narrows the full set to one host's tenancy: platform
// rules see everything, workspace rules their workspace, namespace rules
// their project. Matching is by ids alone -- a namespace host carries
// both ids, so its workspace's rules reach it too.
func applicableRules(rules []gwstore.AlertRule, wsID, nsID *int64) []gwstore.AlertRule {
	var out []gwstore.AlertRule
	for _, r := range rules {
		switch r.Scope {
		case "platform":
			out = append(out, r)
		case "workspace":
			if r.WorkspaceID != nil && wsID != nil && *r.WorkspaceID == *wsID {
				out = append(out, r)
			}
		case "namespace":
			if r.NamespaceID != nil && nsID != nil && *r.NamespaceID == *nsID {
				out = append(out, r)
			}
		}
	}
	return out
}

func ruleStillApplies(applicable []gwstore.AlertRule, id int64) bool {
	for _, r := range applicable {
		if r.ID == id {
			return true
		}
	}
	return false
}

// summaryValue reads one metric off the snapshot by its rule key. The
// keys are the API's snake_case field names, validated at rule creation;
// an unknown key (a newer server wrote a rule this one does not know)
// reads as absent rather than as zero.
func summaryValue(m *agenttypes.MetricsSummary, metric string) (float64, bool) {
	switch metric {
	case "cpu_used_pct":
		return m.CPUUsedPct, true
	case "mem_used_pct":
		return m.MemUsedPct, true
	case "disk_used_pct":
		return m.DiskUsedPct, true
	case "load1":
		return m.Load1, true
	case "load5":
		return m.Load5, true
	case "load15":
		return m.Load15, true
	case "net_rx_bps":
		return m.NetRxBps, true
	case "net_tx_bps":
		return m.NetTxBps, true
	}
	return 0, false
}

// AlertMetrics is the rule-writable metric set, shared with the compute
// module's validation so a rule that passes create is a rule the
// evaluator can read.
var AlertMetrics = []string{
	"cpu_used_pct", "mem_used_pct", "disk_used_pct",
	"load1", "load5", "load15", "net_rx_bps", "net_tx_bps",
}

func breaches(value float64, op string, threshold float64) bool {
	switch op {
	case "gt":
		return value > threshold
	case "ge":
		return value >= threshold
	case "lt":
		return value < threshold
	case "le":
		return value <= threshold
	}
	return false
}
