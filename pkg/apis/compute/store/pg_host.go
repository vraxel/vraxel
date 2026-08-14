package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/pkg/apis/shared/hostevent"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

// HostRow is one host as the REST layer sees it: the record plus the
// agent session joined onto it.
//
// The agent fields are pointers because their absence is meaningful.
// A nil AgentStatus means no agent has ever bound to this host, which is
// a different state from an agent that is not answering -- one is
// waiting for an install, the other for a machine to come back. Flatten
// them to "" / "offline" here and the distinction is gone before the API
// ever sees it.
type HostRow struct {
	ID          int64
	Name        string
	DisplayName string
	Description string
	Hostname    string
	OS          string
	Arch        string
	CPUCores    int32
	MemoryMB    int64
	DiskGB      int64
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
	SSHPort     int32
	Origin      string
	// ConnectivityMode is how the control plane reaches the host today
	// and changes over the row's life; Origin is how the row came into
	// existence and never changes. Nothing may infer one from the other.
	ConnectivityMode  string
	ReportedPrimaryIP string
	PrimaryIPOverride string
	CreatedBy         *int64
	CreatorName       string
	WorkspaceName     string
	NamespaceName     string
	CreatedAt         time.Time
	UpdatedAt         time.Time

	AgentID          string
	AgentStatus      *string
	AgentVersion     *string
	AgentConnectedAt *time.Time
	AgentLastSeenAt  *time.Time
	AgentConflictAt  *time.Time
}

// HostCreateInput is a host recorded by hand.
type HostCreateInput struct {
	Name        string
	DisplayName string
	Description string
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
	SSHPort     int32
	// PrimaryIP is optional: a host reached only through an outbound
	// agent has no address the control plane would ever dial.
	PrimaryIP string
	CreatedBy *int64
}

// HostUpdateInput is the pair of fields a human owns. Everything else on
// the row is agent-reported or structural.
type HostUpdateInput struct {
	DisplayName string
	Description string
}

// HostScope is the tenancy of one host.
type HostScope struct {
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
}

// HostStore is the operator-facing hosts surface. Separate from
// AgentHostStore, which is the narrow set of writes the agent gateway
// performs during registration.
type HostStore interface {
	List(ctx context.Context, q list.Query) (*list.Result[HostRow], error)
	GetByID(ctx context.Context, id int64, sf scope.Filter) (*HostRow, error)
	Create(ctx context.Context, in HostCreateInput) (int64, error)
	Update(ctx context.Context, id int64, sf scope.Filter, in HostUpdateInput) error
	Delete(ctx context.Context, id int64, sf scope.Filter) error
	// GetScope reads a host's tenancy, so a token minted for it can be
	// stamped with the host's own scope rather than the caller's.
	GetScope(ctx context.Context, id int64) (*HostScope, error)
}

type pgHostStore struct {
	db.Store
}

// NewPGHostStore creates a PostgreSQL-backed HostStore.
func NewPGHostStore(d *db.DB) HostStore { return &pgHostStore{Store: db.Store{DB: d}} }

type hostFilters struct {
	Scope       *string `filter:"scope"`
	WorkspaceID *int64  `filter:"workspace_id"`
	NamespaceID *int64  `filter:"namespace_id"`
	Origin      *string `filter:"origin"`
	AgentStatus *string `filter:"agent_status"`
	Search      *string `filter:"search"`
}

func (s *pgHostStore) List(ctx context.Context, q list.Query) (*list.Result[HostRow], error) {
	offset, limit := q.OffsetLimit()
	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}
	f := list.Parse[hostFilters](q.Filters)

	count, err := s.Q().CountHosts(ctx, generated.CountHostsParams{
		Scope:       f.Scope,
		WorkspaceID: f.WorkspaceID,
		NamespaceID: f.NamespaceID,
		Origin:      f.Origin,
		AgentStatus: f.AgentStatus,
		Search:      f.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("count hosts: %w", err)
	}

	rows, err := s.Q().ListHosts(ctx, generated.ListHostsParams{
		Scope:       f.Scope,
		WorkspaceID: f.WorkspaceID,
		NamespaceID: f.NamespaceID,
		Origin:      f.Origin,
		AgentStatus: f.AgentStatus,
		Search:      f.Search,
		SortField:   q.SortBy,
		SortOrder:   sortOrder,
		PageOffset:  offset,
		PageSize:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list hosts: %w", err)
	}

	items := make([]HostRow, len(rows))
	for i := range rows {
		items[i] = listRowToDomain(&rows[i])
	}
	return &list.Result[HostRow]{Items: items, TotalCount: count}, nil
}

func (s *pgHostStore) GetByID(ctx context.Context, id int64, sf scope.Filter) (*HostRow, error) {
	row, err := s.Q().GetHostByID(ctx, generated.GetHostByIDParams{
		ID:                id,
		WorkspaceIDFilter: sf.WorkspaceID,
		NamespaceIDFilter: sf.NamespaceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("host %d: %w", id, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get host: %w", err)
	}
	out := getRowToDomain(&row)
	return &out, nil
}

func (s *pgHostStore) Create(ctx context.Context, in HostCreateInput) (int64, error) {
	id, err := s.Q().CreateHost(ctx, generated.CreateHostParams{
		Name:              in.Name,
		DisplayName:       in.DisplayName,
		Description:       in.Description,
		Scope:             in.Scope,
		WorkspaceID:       in.WorkspaceID,
		NamespaceID:       in.NamespaceID,
		SshPort:           in.SSHPort,
		PrimaryIpOverride: in.PrimaryIP,
		CreatedBy:         in.CreatedBy,
	})
	if err != nil {
		return 0, fmt.Errorf("create host: %w", pgerrors.CheckPG(err))
	}
	// Announced from the store rather than the handler because a host row
	// is written from more than one entry point (this one, and the agent
	// registrar next door), and this is the layer they have in common.
	hostevent.Channel.Publish(ctx, s.DB.GetPool(), hostevent.Event{
		HostID: id, Scope: in.Scope, WorkspaceID: in.WorkspaceID, NamespaceID: in.NamespaceID,
	})
	return id, nil
}

func (s *pgHostStore) Update(ctx context.Context, id int64, sf scope.Filter, in HostUpdateInput) error {
	n, err := s.Q().UpdateHost(ctx, generated.UpdateHostParams{
		ID:                id,
		DisplayName:       in.DisplayName,
		Description:       in.Description,
		WorkspaceIDFilter: sf.WorkspaceID,
		NamespaceIDFilter: sf.NamespaceID,
	})
	if err != nil {
		return fmt.Errorf("update host: %w", pgerrors.CheckPG(err))
	}
	if n == 0 {
		return fmt.Errorf("host %d: %w", id, pgerrors.ErrNotFound)
	}
	// No scope: the row is still there, so the subscriber can read the
	// tenancy itself, and this statement does not have it at hand.
	hostevent.Channel.Publish(ctx, s.DB.GetPool(), hostevent.Event{HostID: id})
	return nil
}

func (s *pgHostStore) Delete(ctx context.Context, id int64, sf scope.Filter) error {
	row, err := s.Q().DeleteHost(ctx, generated.DeleteHostParams{
		ID:                id,
		WorkspaceIDFilter: sf.WorkspaceID,
		NamespaceIDFilter: sf.NamespaceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("host %d: %w", id, pgerrors.ErrNotFound)
		}
		return fmt.Errorf("delete host: %w", err)
	}
	// The deleted row's own tenancy, which nothing can look up afterwards.
	hostevent.Channel.Publish(ctx, s.DB.GetPool(), hostevent.Event{
		HostID: id, Scope: row.Scope, WorkspaceID: row.WorkspaceID, NamespaceID: row.NamespaceID,
		Deleted: true,
	})
	return nil
}

func (s *pgHostStore) GetScope(ctx context.Context, id int64) (*HostScope, error) {
	row, err := s.Q().HostScopeByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("host %d: %w", id, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get host scope: %w", err)
	}
	return &HostScope{Scope: row.Scope, WorkspaceID: row.WorkspaceID, NamespaceID: row.NamespaceID}, nil
}

// uuidString renders the joined agent id, or "" when the LEFT JOIN found
// no agent. pgtype.UUID's zero value is a valid all-zero uuid, so the
// Valid flag is the only thing that tells "no agent" from one.
func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}

// The two generated row types are structurally identical but distinct
// Go types, so the mapping is written once per query rather than shared
// through an interface nobody else would implement.

func listRowToDomain(r *generated.ListHostsRow) HostRow {
	return HostRow{
		ID: r.ID, Name: r.Name, DisplayName: r.DisplayName, Description: r.Description,
		Hostname: r.Hostname, OS: r.Os, Arch: r.Arch,
		CPUCores: r.CpuCores, MemoryMB: r.MemoryMb, DiskGB: r.DiskGb,
		Scope: r.Scope, WorkspaceID: r.WorkspaceID, NamespaceID: r.NamespaceID,
		SSHPort: r.SshPort, Origin: r.Origin, ConnectivityMode: r.ConnectivityMode,
		ReportedPrimaryIP: r.ReportedPrimaryIp, PrimaryIPOverride: r.PrimaryIpOverride,
		CreatedBy: r.CreatedBy, CreatorName: r.CreatorName,
		WorkspaceName: r.WorkspaceName, NamespaceName: r.NamespaceName,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		AgentID:          uuidString(r.AgentID),
		AgentStatus:      r.AgentStatus,
		AgentVersion:     r.AgentVersion,
		AgentConnectedAt: r.AgentConnectedAt,
		AgentLastSeenAt:  r.AgentLastSeenAt,
		AgentConflictAt:  r.AgentConflictAt,
	}
}

func getRowToDomain(r *generated.GetHostByIDRow) HostRow {
	return HostRow{
		ID: r.ID, Name: r.Name, DisplayName: r.DisplayName, Description: r.Description,
		Hostname: r.Hostname, OS: r.Os, Arch: r.Arch,
		CPUCores: r.CpuCores, MemoryMB: r.MemoryMb, DiskGB: r.DiskGb,
		Scope: r.Scope, WorkspaceID: r.WorkspaceID, NamespaceID: r.NamespaceID,
		SSHPort: r.SshPort, Origin: r.Origin, ConnectivityMode: r.ConnectivityMode,
		ReportedPrimaryIP: r.ReportedPrimaryIp, PrimaryIPOverride: r.PrimaryIpOverride,
		CreatedBy: r.CreatedBy, CreatorName: r.CreatorName,
		WorkspaceName: r.WorkspaceName, NamespaceName: r.NamespaceName,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		AgentID:          uuidString(r.AgentID),
		AgentStatus:      r.AgentStatus,
		AgentVersion:     r.AgentVersion,
		AgentConnectedAt: r.AgentConnectedAt,
		AgentLastSeenAt:  r.AgentLastSeenAt,
		AgentConflictAt:  r.AgentConflictAt,
	}
}
