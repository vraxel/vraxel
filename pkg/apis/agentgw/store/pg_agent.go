package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"vraxel.io/vraxel/pkg/apis/shared/hostevent"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

type pgAgentStore struct {
	db.Store
}

// NewPGAgentStore creates a PostgreSQL-backed AgentStore.
func NewPGAgentStore(d *db.DB) AgentStore { return &pgAgentStore{Store: db.Store{DB: d}} }

// notifyHost announces that the agent bound to hostID changed, so every
// instance's host watchers refresh.
//
// Scope is left empty on purpose: this module owns host_agents, not
// hosts, and tenancy is a property of the host row -- compute's
// subscriber resolves it. See pkg/apis/shared/hostevent.
//
// Every caller sits behind a guard that makes the write worth showing,
// which is why a heartbeat (TouchHostAgent, one per agent per 15s) never
// reaches here.
func (s *pgAgentStore) notifyHost(ctx context.Context, hostID int64) {
	hostevent.Channel.Publish(ctx, s.DB.GetPool(), hostevent.Event{HostID: hostID})
}

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
	// Binding an agent moves the host from "never installed" to
	// "offline", which is a state the list draws differently.
	s.notifyHost(ctx, hostID)
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

func (s *pgAgentStore) CheckIdentity(ctx context.Context, hostID int64, bootNonce string, cooldown time.Duration) (bool, error) {
	contended, err := s.Q().CheckHostAgentIdentity(ctx, generated.CheckHostAgentIdentityParams{
		BootNonce:    clampBootNonce(bootNonce),
		HostID:       hostID,
		CooldownSecs: cooldown.Seconds(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, fmt.Errorf("host agent for host %d: %w", hostID, pgerrors.ErrNotFound)
		}
		return false, fmt.Errorf("check host agent identity: %w", err)
	}
	// The expression is NULL-free (conflict_at IS NOT NULL AND ...), so a
	// nil pointer cannot happen; treating it as "not contended" keeps the
	// admit path working if that ever changes.
	if contended != nil && *contended {
		// conflict_at is rendered -- it is the one badge state that
		// outranks online/offline -- and a contended agent never reaches
		// MarkOnline, so without this the page that most needs to update
		// is the one that never would.
		//
		// This fires per refused connection rather than once per conflict,
		// because a single statement cannot say whether it was the write
		// that stamped conflict_at. That is a clone reconnecting on its
		// backoff, not a hot path, and an operator staring at a flapping
		// conflict is better served by a live page than by a quiet one.
		s.notifyHost(ctx, hostID)
		return true, nil
	}
	return false, nil
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
	// Unguarded, so a re-claim of an already-online row publishes too.
	// That is one redundant refetch on a path that runs when a channel is
	// accepted or a drifted row is taken back -- neither is frequent, and
	// the alternative (reading the old status first) would race the write.
	s.notifyHost(ctx, hostID)
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
	n, err := s.Q().MarkHostAgentOffline(ctx, generated.MarkHostAgentOfflineParams{
		HostID:     hostID,
		InstanceID: instanceID,
	})
	if err != nil {
		return fmt.Errorf("mark host agent offline: %w", err)
	}
	// Zero rows means the guard held: the row is another instance's, or it
	// was already offline. Nothing changed, so nothing is announced.
	if n > 0 {
		s.notifyHost(ctx, hostID)
	}
	return nil
}

func (s *pgAgentStore) MarkOrphansOffline(ctx context.Context, staleAfter time.Duration) error {
	hostIDs, err := s.Q().MarkOrphanedHostAgentsOffline(ctx, staleAfter.Seconds())
	if err != nil {
		return fmt.Errorf("mark orphaned host agents offline: %w", err)
	}
	s.notifyHosts(ctx, hostIDs)
	return nil
}

func (s *pgAgentStore) MarkStaleOffline(ctx context.Context, staleAfter time.Duration) error {
	hostIDs, err := s.Q().MarkStaleHostAgentsOffline(ctx, staleAfter.Seconds())
	if err != nil {
		return fmt.Errorf("mark stale host agents offline: %w", err)
	}
	s.notifyHosts(ctx, hostIDs)
	return nil
}

// notifyHosts announces a sweep's worth of transitions. The sweeps are
// guarded on status='online', so the returned set is exactly the hosts
// that just went offline -- usually empty, occasionally one, and
// everything at once only when an instance died holding many channels.
func (s *pgAgentStore) notifyHosts(ctx context.Context, hostIDs []int64) {
	for _, id := range hostIDs {
		s.notifyHost(ctx, id)
	}
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

// bootNonceColumnWidth is host_agents.boot_nonce's varchar width.
const bootNonceColumnWidth = 64

// clampBootNonce truncates an agent-reported boot nonce to the column
// width. Off the wire and therefore attacker-controllable: an oversized
// value would fail the UPDATE and take the whole channel down with it,
// whereas a truncated one only ever weakens the comparison for the agent
// that sent it. The honest agent sends 32 hex characters.
func clampBootNonce(n string) string {
	if len(n) > bootNonceColumnWidth {
		return n[:bootNonceColumnWidth]
	}
	return n
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
