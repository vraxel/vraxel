// Package store holds the DB access paths for the agent gateway:
// host_agents and host_agent_join_tokens.
//
// The gateway owns these tables. hosts belongs to the host module and is
// written through the injected HostRegistrar interface, never from here.
// server_instances is a platform concern shared with the run orchestrator,
// so its leasing lives in lib/serverinstance, not here. The (play, host)
// job pipeline (runs / jobs / locks) lands with the jobs slice.
package store

import (
	"time"
)

// AgentRow is one agent identity plus its current control-channel session
// state. agent_id is a uuid in the DB; the domain type carries the
// canonical string form so nothing above the store layer needs pgtype.
type AgentRow struct {
	HostID       int64
	AgentID      string
	TokenVersion int32
	Version      string
	InstanceID   string
	Status       string
	ConnectedAt  *time.Time
	LastSeenAt   *time.Time
	ClockSkewMs  int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// JoinTokenRow is a registration token. TokenHash is the SHA-256 of the
// plaintext; the plaintext itself exists only in the create response.
type JoinTokenRow struct {
	ID          int64
	Name        string
	TokenHash   []byte
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
	MaxUses     int32
	UsedCount   int32
	ExpiresAt   time.Time
	CreatedBy   *int64
	CreatedAt   time.Time
	CreatorName string
	// TargetHostID binds the token to a host that already exists; the
	// agent redeeming it adopts that row instead of creating one. Nil
	// for the onboarding path.
	TargetHostID *int64
}

// JoinTokenCreateInput is the create payload for a join token.
type JoinTokenCreateInput struct {
	Name         string
	TokenHash    []byte
	Scope        string
	WorkspaceID  *int64
	NamespaceID  *int64
	MaxUses      int32
	ExpiresAt    time.Time
	CreatedBy    *int64
	TargetHostID *int64
}
