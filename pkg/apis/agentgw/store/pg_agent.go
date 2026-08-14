package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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

// bindLockNamespace keeps this lock's keys from colliding with any other
// advisory-lock user in the process. Advisory locks share one int64 key
// space across the whole database.
const bindLockNamespace = "agentgw.bind:"

// Bind attaches a machine to a host row.
//
// Serialised per machine, not per host: the race worth closing is one
// machine registering twice at once (a double-pasted install command, or
// a retry that overlaps its own first attempt). Both requests would
// otherwise find no row and each create one, and with agent_id now
// allocated rather than derived there is no unique constraint left to
// catch it -- the duplicate would be silent and permanent.
//
// Keyed on the machine's own identity so two DIFFERENT machines never
// wait on each other, which matters when a hundred clones of a template
// are onboarded at once.
func (s *pgAgentStore) Bind(ctx context.Context, in BindInput) (*AgentRow, error) {
	row, err := db.WithTxReturning(ctx, s.DB, func(ctx context.Context, q *generated.Queries) (generated.HostAgent, error) {
		var zero generated.HostAgent
		if _, err := db.AdvisoryLock(ctx, q, bindLockKey(in), db.AdvisoryLockBlocking); err != nil {
			return zero, err
		}

		agentID := in.AgentID
		// Re-check under the lock. A concurrent request may have created
		// the row between the caller's lookup and here; binding to it is
		// what keeps one machine to one host.
		if agentID == "" && len(in.ClaimUUIDs) > 0 {
			found, err := q.FindHostAgentsByProductUUID(ctx, in.ClaimUUIDs)
			if err != nil {
				return zero, fmt.Errorf("re-check product uuid: %w", err)
			}
			// Exactly one: more than one means the value identifies a
			// hardware batch rather than a machine, and the caller's own
			// lookup would have refused it too.
			if len(found) == 1 {
				agentID = uuid.UUID(found[0].AgentID.Bytes).String()
			}
		}

		if agentID != "" {
			uid, err := parseAgentUUID(agentID)
			if err != nil {
				return zero, err
			}
			out, err := q.RebindHostAgent(ctx, generated.RebindHostAgentParams{
				HostID:         in.HostID,
				Version:        clampVersion(in.Version),
				ProductUuid:    in.Fingerprint.ProductUUID,
				MachineID:      in.Fingerprint.MachineID,
				Macs:           in.Fingerprint.MACs,
				IdentitySource: in.Fingerprint.Source,
				BootAt:         in.Fingerprint.BootAt,
				AgentID:        uid,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return zero, fmt.Errorf("host agent %s: %w", agentID, pgerrors.ErrNotFound)
				}
				return zero, fmt.Errorf("rebind host agent: %w", pgerrors.CheckPG(err))
			}
			return out, nil
		}

		// A host holds at most one agent (host_agents is keyed by
		// host_id). Reaching here with the host already bound means a
		// join token minted against it was redeemed by a different
		// machine, which is an operator saying "this machine is that host
		// now" -- so the previous binding goes.
		if _, err := q.DeleteHostAgentByHostID(ctx, in.HostID); err != nil {
			return zero, fmt.Errorf("detach previous host agent: %w", err)
		}
		fresh, err := parseAgentUUID(uuid.NewString())
		if err != nil {
			return zero, err
		}
		out, err := q.InsertHostAgent(ctx, generated.InsertHostAgentParams{
			HostID:         in.HostID,
			AgentID:        fresh,
			Version:        clampVersion(in.Version),
			ProductUuid:    in.Fingerprint.ProductUUID,
			MachineID:      in.Fingerprint.MachineID,
			Macs:           in.Fingerprint.MACs,
			IdentitySource: in.Fingerprint.Source,
			BootAt:         in.Fingerprint.BootAt,
		})
		if err != nil {
			return zero, fmt.Errorf("insert host agent: %w", pgerrors.CheckPG(err))
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	// Binding an agent moves the host from "never installed" to
	// "offline", which is a state the list draws differently.
	s.notifyHost(ctx, row.HostID)
	return agentToDomain(&row), nil
}

// bindLockKey derives the per-machine lock key. Falls back through the
// same order the matcher trusts, and finally to the host id, so a machine
// that offers no identity at all still serialises against itself.
func bindLockKey(in BindInput) int64 {
	switch {
	case in.Fingerprint.ProductUUID != "":
		return db.HashLockKey(bindLockNamespace + in.Fingerprint.ProductUUID)
	case in.Fingerprint.MachineID != "":
		return db.HashLockKey(bindLockNamespace + in.Fingerprint.MachineID)
	default:
		return db.HashLockKey(bindLockNamespace + strconv.FormatInt(in.HostID, 10))
	}
}

func (s *pgAgentStore) FindByProductUUID(ctx context.Context, productUUIDs []string) ([]AgentRow, error) {
	if len(productUUIDs) == 0 {
		return nil, nil
	}
	rows, err := s.Q().FindHostAgentsByProductUUID(ctx, productUUIDs)
	if err != nil {
		return nil, fmt.Errorf("find host agents by product uuid: %w", err)
	}
	return agentsToDomain(rows), nil
}

func (s *pgAgentStore) FindByMachineID(ctx context.Context, machineID string) ([]AgentRow, error) {
	if machineID == "" {
		return nil, nil
	}
	rows, err := s.Q().FindHostAgentsByMachineID(ctx, machineID)
	if err != nil {
		return nil, fmt.Errorf("find host agents by machine id: %w", err)
	}
	return agentsToDomain(rows), nil
}

func (s *pgAgentStore) RefreshFingerprint(ctx context.Context, hostID int64, fp FingerprintInput) error {
	if _, err := s.Q().UpdateHostAgentFingerprint(ctx, generated.UpdateHostAgentFingerprintParams{
		MachineID: fp.MachineID,
		Macs:      fp.MACs,
		BootAt:    fp.BootAt,
		HostID:    hostID,
	}); err != nil {
		return fmt.Errorf("update host agent fingerprint: %w", err)
	}
	// The machine-id reset that lands here is the operator's fix for a
	// clone, and it clears conflict_at -- a badge state the list draws
	// above everything else.
	s.notifyHost(ctx, hostID)
	return nil
}

func (s *pgAgentStore) MoveBinding(ctx context.Context, fromHostID, toHostID int64) error {
	if _, err := s.Q().MoveHostAgentBinding(ctx, generated.MoveHostAgentBindingParams{
		ToHostID:   toHostID,
		FromHostID: fromHostID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("host agent for host %d: %w", fromHostID, pgerrors.ErrNotFound)
		}
		return fmt.Errorf("move host agent binding: %w", pgerrors.CheckPG(err))
	}
	// Both rows change what the list draws: one gains an agent, the other
	// is about to disappear.
	s.notifyHost(ctx, fromHostID)
	s.notifyHost(ctx, toHostID)
	return nil
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
		ConflictAt:   r.ConflictAt,

		ProductUUID:    r.ProductUuid,
		MachineID:      r.MachineID,
		MACs:           r.Macs,
		IdentitySource: r.IdentitySource,
		BootAt:         r.BootAt,
	}
}

func agentsToDomain(rows []generated.HostAgent) []AgentRow {
	out := make([]AgentRow, 0, len(rows))
	for i := range rows {
		out = append(out, *agentToDomain(&rows[i]))
	}
	return out
}
