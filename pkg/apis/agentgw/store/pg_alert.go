package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"vraxel.io/vraxel/pkg/apis/shared/hostevent"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
)

// AlertRule is one enabled rule as the evaluator consumes it.
type AlertRule struct {
	ID          int64
	Name        string
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
	Metric      string
	Op          string
	Threshold   float64
	ForSeconds  int32
	Severity    string
}

// AlertState is one live breach -- pending or firing -- for a host.
type AlertState struct {
	RuleID        int64
	BreachedSince time.Time
	Firing        bool
}

// AlertStore is the evaluator's surface over host_alert_rules and
// host_alert_states. Every write is a TRANSITION -- first breach, the
// pending-to-firing flip, recovery -- so a steady state, healthy or
// steadily broken, costs zero writes per beat.
type AlertStore interface {
	// EnabledRules is every enabled rule, unscoped: the caller filters
	// per host. It is the cache-refill read, so it must stay one cheap
	// statement.
	EnabledRules(ctx context.Context) ([]AlertRule, error)
	// States are the host's live breaches.
	States(ctx context.Context, hostID int64) ([]AlertState, error)
	// RecordBreach starts the debounce clock. Idempotent, and it must
	// not touch an existing row -- re-recording on every breaching beat
	// would reset breached_since and nothing would ever fire.
	RecordBreach(ctx context.Context, hostID, ruleID int64, value float64) error
	// MarkFiring flips pending to firing and reports whether THIS call
	// did the flipping. A true return has already published the host
	// event; callers add nothing.
	MarkFiring(ctx context.Context, hostID, ruleID int64, value float64) (flipped bool, err error)
	// Clear removes the state on recovery and reports whether it had
	// been firing. A true return has already published the host event.
	Clear(ctx context.Context, hostID, ruleID int64) (wasFiring bool, err error)
	// SweepDisabled removes every state row whose rule is disabled --
	// the backstop for the two paths the per-beat evaluator cannot
	// cover: a deployment whose LAST enabled rule was just switched off
	// (the zero-rules fast path then never reads states again), and an
	// instance that re-created rows off a stale rule cache within the
	// TTL after the disable. Runs on the lease tick; publishes for the
	// rows that were firing.
	SweepDisabled(ctx context.Context) error
}

type pgAlertStore struct {
	db.Store
}

// NewPGAlertStore creates a PostgreSQL-backed AlertStore.
func NewPGAlertStore(d *db.DB) AlertStore { return &pgAlertStore{Store: db.Store{DB: d}} }

func (s *pgAlertStore) EnabledRules(ctx context.Context) ([]AlertRule, error) {
	rows, err := s.Q().ListEnabledHostAlertRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled alert rules: %w", err)
	}
	out := make([]AlertRule, len(rows))
	for i, r := range rows {
		out[i] = AlertRule{
			ID: r.ID, Name: r.Name,
			Scope: r.Scope, WorkspaceID: r.WorkspaceID, NamespaceID: r.NamespaceID,
			Metric: r.Metric, Op: r.Op, Threshold: float64(r.Threshold),
			ForSeconds: r.ForSeconds, Severity: r.Severity,
		}
	}
	return out, nil
}

func (s *pgAlertStore) States(ctx context.Context, hostID int64) ([]AlertState, error) {
	rows, err := s.Q().ListHostAlertStates(ctx, hostID)
	if err != nil {
		return nil, fmt.Errorf("list alert states: %w", err)
	}
	out := make([]AlertState, len(rows))
	for i, r := range rows {
		out[i] = AlertState{RuleID: r.RuleID, BreachedSince: r.BreachedSince, Firing: r.Firing}
	}
	return out, nil
}

func (s *pgAlertStore) RecordBreach(ctx context.Context, hostID, ruleID int64, value float64) error {
	err := s.Q().UpsertHostAlertBreach(ctx, generated.UpsertHostAlertBreachParams{
		HostID: hostID, RuleID: ruleID, Value: float32(value),
	})
	if err != nil {
		return fmt.Errorf("record alert breach: %w", err)
	}
	return nil
}

// The two transition writes publish the host event themselves, because
// only they know a transition happened: publishing from the evaluator
// would either fire on every breaching beat or need the store's answer
// anyway. Transitions only -- the flood the metrics upsert deliberately
// avoids must stay avoided here.

func (s *pgAlertStore) MarkFiring(ctx context.Context, hostID, ruleID int64, value float64) (bool, error) {
	n, err := s.Q().MarkHostAlertFiring(ctx, generated.MarkHostAlertFiringParams{
		HostID: hostID, RuleID: ruleID, Value: float32(value),
	})
	if err != nil {
		return false, fmt.Errorf("mark alert firing: %w", err)
	}
	if n == 0 {
		return false, nil
	}
	hostevent.Channel.Publish(ctx, s.DB.GetPool(), hostevent.Event{HostID: hostID})
	return true, nil
}

func (s *pgAlertStore) SweepDisabled(ctx context.Context) error {
	rows, err := s.Q().SweepDisabledAlertStates(ctx)
	if err != nil {
		return fmt.Errorf("sweep disabled alert states: %w", err)
	}
	for _, r := range rows {
		if r.Firing {
			hostevent.Channel.Publish(ctx, s.DB.GetPool(), hostevent.Event{HostID: r.HostID})
		}
	}
	return nil
}

func (s *pgAlertStore) Clear(ctx context.Context, hostID, ruleID int64) (bool, error) {
	firing, err := s.Q().ClearHostAlertState(ctx, generated.ClearHostAlertStateParams{
		HostID: hostID, RuleID: ruleID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("clear alert state: %w", err)
	}
	if firing {
		hostevent.Channel.Publish(ctx, s.DB.GetPool(), hostevent.Event{HostID: hostID})
	}
	return firing, nil
}
