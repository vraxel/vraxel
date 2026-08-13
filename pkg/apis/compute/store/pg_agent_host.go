package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

// AgentHostCreateInput is the host row an the agent self-registration
// produces. Deliberately narrower than HostCreateInput: the agent path
// has no IPAM, no cloud origin columns and no credential binding.
type AgentHostCreateInput struct {
	Name        string
	DisplayName string
	Hostname    string
	OS          string
	Arch        string
	CPUCores    int32
	MemoryMB    int64
	DiskGB      int64
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
	PrimaryIP   string
	CreatedBy   *int64
}

// AgentHostFactsInput refreshes the reported hardware facts of an
// already-onboarded agent host.
type AgentHostFactsInput struct {
	Hostname  string
	OS        string
	Arch      string
	CPUCores  int32
	MemoryMB  int64
	DiskGB    int64
	PrimaryIP string
}

// AgentHostScope is the tenancy of an existing agent host.
type AgentHostScope struct {
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
}

// AgentHostStore is the hosts-table surface the agent gateway needs.
//
// A dedicated narrow store rather than two more methods on HostStore:
// HostStore is a wide interface with several implementors, and the agent
// path shares none of its IPAM / label / lifecycle machinery.
type AgentHostStore interface {
	// Create inserts a host row with connectivity_mode='agent'. A name
	// collision surfaces as pgerrors.ErrConflict so the caller can retry
	// with a disambiguated name.
	Create(ctx context.Context, in AgentHostCreateInput) (int64, error)
	// UpdateFacts refreshes an existing agent host. Returns
	// pgerrors.ErrNotFound when the row is gone.
	UpdateFacts(ctx context.Context, hostID int64, in AgentHostFactsInput) error
	// GetScope reads a host's tenancy. Returns pgerrors.ErrNotFound when
	// the row is gone.
	GetScope(ctx context.Context, hostID int64) (*AgentHostScope, error)
}

type pgAgentHostStore struct {
	db.Store
}

// NewPGAgentHostStore creates a PostgreSQL-backed AgentHostStore.
func NewPGAgentHostStore(d *db.DB) AgentHostStore { return &pgAgentHostStore{Store: db.Store{DB: d}} }

func (s *pgAgentHostStore) Create(ctx context.Context, in AgentHostCreateInput) (int64, error) {
	id, err := s.Q().CreateAgentHost(ctx, generated.CreateAgentHostParams{
		Name:              in.Name,
		DisplayName:       in.DisplayName,
		Hostname:          in.Hostname,
		Os:                in.OS,
		Arch:              in.Arch,
		CpuCores:          in.CPUCores,
		MemoryMb:          in.MemoryMB,
		DiskGb:            in.DiskGB,
		Scope:             in.Scope,
		WorkspaceID:       in.WorkspaceID,
		NamespaceID:       in.NamespaceID,
		ReportedPrimaryIp: in.PrimaryIP,
		CreatedBy:         in.CreatedBy,
	})
	if err != nil {
		return 0, fmt.Errorf("create agent host: %w", pgerrors.CheckPG(err))
	}
	return id, nil
}

func (s *pgAgentHostStore) GetScope(ctx context.Context, hostID int64) (*AgentHostScope, error) {
	row, err := s.Q().GetAgentHostScope(ctx, hostID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("host %d: %w", hostID, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get agent host scope: %w", err)
	}
	return &AgentHostScope{
		Scope:       row.Scope,
		WorkspaceID: row.WorkspaceID,
		NamespaceID: row.NamespaceID,
	}, nil
}

func (s *pgAgentHostStore) UpdateFacts(ctx context.Context, hostID int64, in AgentHostFactsInput) error {
	n, err := s.Q().UpdateAgentHostFacts(ctx, generated.UpdateAgentHostFactsParams{
		ID:                hostID,
		Hostname:          in.Hostname,
		Os:                in.OS,
		Arch:              in.Arch,
		CpuCores:          in.CPUCores,
		MemoryMb:          in.MemoryMB,
		DiskGb:            in.DiskGB,
		ReportedPrimaryIp: in.PrimaryIP,
	})
	if err != nil {
		return fmt.Errorf("update agent host facts: %w", pgerrors.CheckPG(err))
	}
	if n == 0 {
		return fmt.Errorf("host %d: %w", hostID, pgerrors.ErrNotFound)
	}
	return nil
}
