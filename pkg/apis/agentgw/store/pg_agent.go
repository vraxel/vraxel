package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

type pgAgentStore struct {
	db.Store
}

// NewPGAgentStore creates a PostgreSQL-backed AgentStore.
func NewPGAgentStore(d *db.DB) AgentStore { return &pgAgentStore{Store: db.Store{DB: d}} }

func (s *pgAgentStore) Upsert(ctx context.Context, hostID int64, agentID, version string) (*AgentRow, error) {
	uid, err := parseAgentUUID(agentID)
	if err != nil {
		return nil, err
	}
	row, err := s.Q().UpsertHostAgent(ctx, generated.UpsertHostAgentParams{
		HostID:  hostID,
		AgentID: uid,
		Version: clampVersion(version),
	})
	if err != nil {
		return nil, fmt.Errorf("upsert host agent: %w", pgerrors.CheckPG(err))
	}
	return agentToDomain(&row), nil
}

func (s *pgAgentStore) GetByAgentID(ctx context.Context, agentID string) (*AgentRow, error) {
	uid, err := parseAgentUUID(agentID)
	if err != nil {
		return nil, err
	}
	row, err := s.Q().GetHostAgentByAgentID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("host agent %s: %w", agentID, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get host agent by agent id: %w", err)
	}
	return agentToDomain(&row), nil
}

func (s *pgAgentStore) GetByHostID(ctx context.Context, hostID int64) (*AgentRow, error) {
	row, err := s.Q().GetHostAgentByHostID(ctx, hostID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("host agent for host %d: %w", hostID, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get host agent by host id: %w", err)
	}
	return agentToDomain(&row), nil
}

func (s *pgAgentStore) MarkOnline(ctx context.Context, hostID int64, instanceID, version string, clockSkewMs int64) (time.Time, error) {
	connectedAt, err := s.Q().MarkHostAgentOnline(ctx, generated.MarkHostAgentOnlineParams{
		InstanceID:  instanceID,
		Version:     clampVersion(version),
		ClockSkewMs: clockSkewMs,
		HostID:      hostID,
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("mark host agent online: %w", err)
	}
	// connected_at was just SET to now() in the same statement, so the
	// RETURNING value is never NULL; the guard is defensive only. Callers
	// read the zero time as "no claim", which is also what the error path
	// above returns.
	if connectedAt == nil {
		return time.Time{}, nil
	}
	return *connectedAt, nil
}

func (s *pgAgentStore) Touch(ctx context.Context, hostID int64, instanceID string, clockSkewMs int64) (bool, error) {
	n, err := s.Q().TouchHostAgent(ctx, generated.TouchHostAgentParams{
		ClockSkewMs: clockSkewMs,
		HostID:      hostID,
		InstanceID:  instanceID,
	})
	if err != nil {
		return false, fmt.Errorf("touch host agent: %w", err)
	}
	return n > 0, nil
}

func (s *pgAgentStore) MarkOffline(ctx context.Context, hostID int64, instanceID string) error {
	if err := s.Q().MarkHostAgentOffline(ctx, generated.MarkHostAgentOfflineParams{
		HostID:     hostID,
		InstanceID: instanceID,
	}); err != nil {
		return fmt.Errorf("mark host agent offline: %w", err)
	}
	return nil
}

func (s *pgAgentStore) MarkOrphansOffline(ctx context.Context, staleAfter time.Duration) error {
	if err := s.Q().MarkOrphanedHostAgentsOffline(ctx, staleAfter.Seconds()); err != nil {
		return fmt.Errorf("mark orphaned host agents offline: %w", err)
	}
	return nil
}

func (s *pgAgentStore) MarkStaleOffline(ctx context.Context, staleAfter time.Duration) error {
	if err := s.Q().MarkStaleHostAgentsOffline(ctx, staleAfter.Seconds()); err != nil {
		return fmt.Errorf("mark stale host agents offline: %w", err)
	}
	return nil
}

// versionColumnWidth is host_agents.version's varchar width.
const versionColumnWidth = 32

// clampVersion truncates an agent-reported version to the column width.
//
// The value is attacker-controllable in the sense that it comes off the
// wire, and a dev build's ldflags version string already exceeds 32
// characters, so rejecting or overflowing would fail registration on a
// cosmetic field. Version is displayed, never compared.
func clampVersion(v string) string {
	if len(v) > versionColumnWidth {
		return v[:versionColumnWidth]
	}
	return v
}

// parseAgentUUID converts the canonical string form used above the store
// layer into the pgtype.UUID the generated code expects.
func parseAgentUUID(agentID string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(agentID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("agent id %q: %w", agentID, pgerrors.ErrBadRequest)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func agentToDomain(r *generated.HostAgent) *AgentRow {
	return &AgentRow{
		HostID:       r.HostID,
		AgentID:      uuid.UUID(r.AgentID.Bytes).String(),
		TokenVersion: r.TokenVersion,
		Version:      r.Version,
		InstanceID:   r.InstanceID,
		Status:       r.Status,
		ConnectedAt:  r.ConnectedAt,
		LastSeenAt:   r.LastSeenAt,
		ClockSkewMs:  r.ClockSkewMs,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}
