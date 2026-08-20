package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

// AlertRuleRow is one threshold rule as the REST layer sees it.
type AlertRuleRow struct {
	ID          int64
	Name        string
	Description string
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
	Metric      string
	Op          string
	Threshold   float64
	ForSeconds  int32
	Severity    string
	Enabled     bool
	CreatedBy   *int64
	CreatorName string
	// FiringCount is how many hosts this rule is firing on right now.
	FiringCount   int64
	WorkspaceName string
	NamespaceName string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// AlertRuleInput carries the writable fields; Create additionally fixes
// the tenancy, which Update never touches -- moving a rule between
// tenants would silently re-aim it at a different fleet.
type AlertRuleInput struct {
	Name        string
	Description string
	Metric      string
	Op          string
	Threshold   float64
	ForSeconds  int32
	Severity    string
	Enabled     bool
}

// AlertRuleStore is the operator-facing rules surface. The evaluator
// reads the same table through the gateway's own store; the two sides
// meet only at the schema, like hosts and host_agents.
type AlertRuleStore interface {
	List(ctx context.Context, q list.Query) (*list.Result[AlertRuleRow], error)
	GetByID(ctx context.Context, id int64, sf scope.Filter) (*AlertRuleRow, error)
	Create(ctx context.Context, sc string, wsID, nsID, createdBy *int64, in AlertRuleInput) (int64, error)
	Update(ctx context.Context, id int64, sf scope.Filter, in AlertRuleInput) error
	Delete(ctx context.Context, id int64, sf scope.Filter) error
}

type pgAlertRuleStore struct {
	db.Store
}

// NewPGAlertRuleStore creates a PostgreSQL-backed AlertRuleStore.
func NewPGAlertRuleStore(d *db.DB) AlertRuleStore { return &pgAlertRuleStore{Store: db.Store{DB: d}} }

type alertRuleFilters struct {
	Scope       *string `filter:"scope"`
	WorkspaceID *int64  `filter:"workspace_id"`
	NamespaceID *int64  `filter:"namespace_id"`
	Metric      *string `filter:"metric"`
	Severity    *string `filter:"severity"`
	Enabled     *bool   `filter:"enabled"`
	Search      *string `filter:"search"`
}

func (s *pgAlertRuleStore) List(ctx context.Context, q list.Query) (*list.Result[AlertRuleRow], error) {
	offset, limit := q.OffsetLimit()
	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}
	f := list.Parse[alertRuleFilters](q.Filters)

	count, err := s.Q().CountHostAlertRules(ctx, generated.CountHostAlertRulesParams{
		Scope: f.Scope, WorkspaceID: f.WorkspaceID, NamespaceID: f.NamespaceID,
		Metric: f.Metric, Severity: f.Severity, Enabled: f.Enabled, Search: f.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("count alert rules: %w", err)
	}
	rows, err := s.Q().ListHostAlertRules(ctx, generated.ListHostAlertRulesParams{
		Scope: f.Scope, WorkspaceID: f.WorkspaceID, NamespaceID: f.NamespaceID,
		Metric: f.Metric, Severity: f.Severity, Enabled: f.Enabled, Search: f.Search,
		SortField: q.SortBy, SortOrder: sortOrder,
		PageOffset: offset, PageSize: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list alert rules: %w", err)
	}
	items := make([]AlertRuleRow, len(rows))
	for i := range rows {
		items[i] = AlertRuleRow{
			ID: rows[i].ID, Name: rows[i].Name, Description: rows[i].Description,
			Scope: rows[i].Scope, WorkspaceID: rows[i].WorkspaceID, NamespaceID: rows[i].NamespaceID,
			Metric: rows[i].Metric, Op: rows[i].Op, Threshold: float64(rows[i].Threshold),
			ForSeconds: rows[i].ForSeconds, Severity: rows[i].Severity, Enabled: rows[i].Enabled,
			CreatedBy: rows[i].CreatedBy, CreatorName: rows[i].CreatorName,
			FiringCount:   rows[i].FiringCount,
			WorkspaceName: rows[i].WorkspaceName, NamespaceName: rows[i].NamespaceName,
			CreatedAt: rows[i].CreatedAt, UpdatedAt: rows[i].UpdatedAt,
		}
	}
	return &list.Result[AlertRuleRow]{Items: items, TotalCount: count}, nil
}

func (s *pgAlertRuleStore) GetByID(ctx context.Context, id int64, sf scope.Filter) (*AlertRuleRow, error) {
	r, err := s.Q().GetHostAlertRule(ctx, generated.GetHostAlertRuleParams{
		ID: id, WorkspaceIDFilter: sf.WorkspaceID, NamespaceIDFilter: sf.NamespaceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("alert rule %d: %w", id, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get alert rule: %w", err)
	}
	out := AlertRuleRow{
		ID: r.ID, Name: r.Name, Description: r.Description,
		Scope: r.Scope, WorkspaceID: r.WorkspaceID, NamespaceID: r.NamespaceID,
		Metric: r.Metric, Op: r.Op, Threshold: float64(r.Threshold),
		ForSeconds: r.ForSeconds, Severity: r.Severity, Enabled: r.Enabled,
		CreatedBy: r.CreatedBy, CreatorName: r.CreatorName,
		FiringCount:   r.FiringCount,
		WorkspaceName: r.WorkspaceName, NamespaceName: r.NamespaceName,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	return &out, nil
}

func (s *pgAlertRuleStore) Create(ctx context.Context, sc string, wsID, nsID, createdBy *int64, in AlertRuleInput) (int64, error) {
	id, err := s.Q().CreateHostAlertRule(ctx, generated.CreateHostAlertRuleParams{
		Name: in.Name, Description: in.Description,
		Scope: sc, WorkspaceID: wsID, NamespaceID: nsID,
		Metric: in.Metric, Op: in.Op, Threshold: float32(in.Threshold),
		ForSeconds: in.ForSeconds, Severity: in.Severity, Enabled: in.Enabled,
		CreatedBy: createdBy,
	})
	if err != nil {
		return 0, fmt.Errorf("create alert rule: %w", pgerrors.CheckPG(err))
	}
	return id, nil
}

func (s *pgAlertRuleStore) Update(ctx context.Context, id int64, sf scope.Filter, in AlertRuleInput) error {
	n, err := s.Q().UpdateHostAlertRule(ctx, generated.UpdateHostAlertRuleParams{
		ID: id, Name: in.Name, Description: in.Description,
		Metric: in.Metric, Op: in.Op, Threshold: float32(in.Threshold),
		ForSeconds: in.ForSeconds, Severity: in.Severity, Enabled: in.Enabled,
		WorkspaceIDFilter: sf.WorkspaceID, NamespaceIDFilter: sf.NamespaceID,
	})
	if err != nil {
		return fmt.Errorf("update alert rule: %w", pgerrors.CheckPG(err))
	}
	if n == 0 {
		return fmt.Errorf("alert rule %d: %w", id, pgerrors.ErrNotFound)
	}
	return nil
}

func (s *pgAlertRuleStore) Delete(ctx context.Context, id int64, sf scope.Filter) error {
	n, err := s.Q().DeleteHostAlertRule(ctx, generated.DeleteHostAlertRuleParams{
		ID: id, WorkspaceIDFilter: sf.WorkspaceID, NamespaceIDFilter: sf.NamespaceID,
	})
	if err != nil {
		return fmt.Errorf("delete alert rule: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("alert rule %d: %w", id, pgerrors.ErrNotFound)
	}
	return nil
}
