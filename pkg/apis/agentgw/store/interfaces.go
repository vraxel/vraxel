package store

import (
	"context"
	"time"

	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
	"vraxel.io/vraxel/pkg/db"
)

// AgentStore is the host_agents surface: identity upsert at register
// time plus control-channel session bookkeeping.
type AgentStore interface {
	// Bind is the register-time write: it attaches a machine to a host
	// and bumps token_version, revoking every token issued before now.
	//
	// It replaces an Upsert keyed on a deterministic agent_id. Identity
	// is now decided by the caller from the machine's fingerprint, and
	// agent_id is allocated once and never re-derived -- so this takes
	// the id to rebind, or "" to allocate a fresh one.
	Bind(ctx context.Context, in BindInput) (*AgentRow, error)
	// FindByProductUUID returns the rows claiming any of these SMBIOS
	// UUIDs. Plural in both directions: one machine can spell its UUID
	// two ways, and one UUID can (on junk firmware) be shared by a whole
	// batch of machines -- which the caller must detect rather than
	// merge.
	FindByProductUUID(ctx context.Context, productUUIDs []string) ([]AgentRow, error)
	// FindByMachineID returns every row built from one disk image. Not
	// an identity lookup: it answers "what else came from this
	// template", which is what turns a clone into an operator-actionable
	// finding instead of a silent collision.
	FindByMachineID(ctx context.Context, machineID string) ([]AgentRow, error)
	// RefreshFingerprint records the mutable half of a fingerprint seen
	// on reconnect, and clears any conflict flag: the only change it
	// accepts is a machine that kept its hardware identity and reset
	// /etc/machine-id, which is the fix we ask cloned hosts to apply.
	RefreshFingerprint(ctx context.Context, hostID int64, fp FingerprintInput) error
	GetByAgentID(ctx context.Context, agentID string) (*AgentRow, error)
	GetByHostID(ctx context.Context, hostID int64) (*AgentRow, error)

	// CheckIdentity records this connection's boot nonce and reports
	// whether the agent id is contended -- two live agent processes
	// claiming it, which a disk clone of an onboarded host produces.
	// Called before the session is registered; a contended id must not
	// reach MarkOnline.
	CheckIdentity(ctx context.Context, hostID int64, bootNonce string, cooldown time.Duration) (contended bool, err error)
	// MarkOnline flips the row online under instanceID and returns the
	// connected_at it wrote -- the session token's connEpoch, which a
	// later reconnect changes to invalidate stale tokens (design §4.1).
	MarkOnline(ctx context.Context, hostID int64, instanceID, version string, clockSkewMs int64) (time.Time, error)
	// Touch records a heartbeat against the row this instance owns and
	// holds online. It reports whether that row was found: false means
	// another instance holds the claim, MarkOnline never landed, or a
	// stale-sweep misfire marked a live agent offline. All three are
	// repaired by taking the row back with MarkOnline, which the caller
	// must do -- every further beat would otherwise be a no-op.
	//
	// The heartbeat deliberately does not restore status itself. It runs
	// once per agent per 15s, and a status write there would be
	// indistinguishable from a real online transition on the watch
	// channel.
	Touch(ctx context.Context, hostID int64, instanceID string, clockSkewMs int64) (claimed bool, err error)
	MarkOffline(ctx context.Context, hostID int64, instanceID string) error
	// MarkOrphansOffline clears rows left online by instances that no
	// longer hold a lease, including this process's own previous life.
	MarkOrphansOffline(ctx context.Context, staleAfter time.Duration) error
	// MarkStaleOffline sweeps rows with no heartbeat for staleAfter. The
	// cutoff is applied against the DB clock, not the caller's.
	MarkStaleOffline(ctx context.Context, staleAfter time.Duration) error
}

// JoinTokenStore is the host_agent_join_tokens surface. The CRUD half is
// consumed by compute's agent-join-tokens resource; Consume is the
// gateway's own /register path.
type JoinTokenStore interface {
	Create(ctx context.Context, in JoinTokenCreateInput) (*JoinTokenRow, error)
	GetByID(ctx context.Context, id int64, sf scope.Filter) (*JoinTokenRow, error)
	List(ctx context.Context, q list.Query) (*list.Result[JoinTokenRow], error)
	Delete(ctx context.Context, id int64, sf scope.Filter) error
	// Peek reports whether a token is live without claiming a use, so
	// /register can reject an unauthenticated caller before doing
	// anything else. Expired / exhausted / unknown hashes yield
	// pgerrors.ErrNotFound.
	Peek(ctx context.Context, tokenHash []byte) (*JoinTokenRow, error)
	// Consume claims one use of a live token, returning the claimed row.
	// Expired / exhausted / unknown hashes yield pgerrors.ErrNotFound.
	Consume(ctx context.Context, tokenHash []byte) (*JoinTokenRow, error)
	// Refund returns a claimed use after a registration failed downstream.
	Refund(ctx context.Context, id int64) error
	// BindTarget records which host a token onboarded, once it has. A
	// token minted against a specific host already names one and is left
	// alone.
	BindTarget(ctx context.Context, id, hostID int64) error
}

// Stores aggregates the gateway's stores. server_instances is
// deliberately absent: instance leasing is a platform concern shared with
// the run orchestrator, and lives in lib/serverinstance.
//
// RunStore (the (play, host) job pipeline) is not part of the register /
// control-channel slice and lands with the jobs slice.
type Stores struct {
	Agent     AgentStore
	JoinToken JoinTokenStore
}

// NewStores builds the pg-backed store set.
func NewStores(d *db.DB) Stores {
	return Stores{
		Agent:     NewPGAgentStore(d),
		JoinToken: NewPGJoinTokenStore(d),
	}
}
