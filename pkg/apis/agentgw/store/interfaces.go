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
	// Upsert binds agentID to hostID and bumps token_version, revoking
	// every token issued to this agent before now.
	Upsert(ctx context.Context, hostID int64, agentID, version string) (*AgentRow, error)
	GetByAgentID(ctx context.Context, agentID string) (*AgentRow, error)
	GetByHostID(ctx context.Context, hostID int64) (*AgentRow, error)

	// MarkOnline flips the row online under instanceID and returns the
	// connected_at it wrote -- the session token's connEpoch, which a
	// later reconnect changes to invalidate stale tokens (design §4.1).
	MarkOnline(ctx context.Context, hostID int64, instanceID, version string, clockSkewMs int64) (time.Time, error)
	// Touch records a heartbeat and restores status='online' for the row
	// this instance owns, so a stale-sweep misfire heals on the next beat.
	// It reports whether the row was ours: false means another instance
	// (or a failed MarkOnline) holds the claim and the caller must take it
	// back, since every further beat would be a no-op.
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
