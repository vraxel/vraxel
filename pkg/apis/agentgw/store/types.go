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
	// ConflictAt is when two live processes were last caught claiming
	// this identity. Nil once a clean session gets through.
	ConflictAt *time.Time

	// --- machine fingerprint ---
	// What the machine holding this row last reported about itself. See
	// the 20260814075808 migration for the A/B split these divide into.
	ProductUUID string
	MachineID   string
	MACs        []string
	// IdentitySource names the class that claimed this row:
	// IdentitySourceProductUUID, or IdentitySourceNone when nothing
	// identified the machine and only an operator can bind it.
	IdentitySource string
	BootAt         *time.Time
}

// Identity sources, stored in host_agents.identity_source.
const (
	// IdentitySourceProductUUID: the row is claimable by a machine
	// presenting the same SMBIOS UUID.
	IdentitySourceProductUUID = "product_uuid"
	// IdentitySourceNone: the machine offered nothing that can identify
	// it (no DMI, or firmware junk). Re-registering after losing its
	// state file produces a second host, which an operator merges.
	IdentitySourceNone = "none"
)

// FingerprintInput is what a machine reported about itself, normalised
// by the business layer. Written on register and refreshed on reconnect.
type FingerprintInput struct {
	ProductUUID string
	MachineID   string
	MACs        []string
	Source      string
	BootAt      *time.Time
}

// BindInput is one machine claiming one host row.
type BindInput struct {
	HostID  int64
	Version string
	// AgentID names the row to rebind, or "" to allocate a new identity.
	// Allocated, never derived: a derived id changes whenever the signal
	// it came from changes, which is how a cloned disk used to arrive as
	// an existing agent.
	AgentID     string
	Fingerprint FingerprintInput
	// ClaimUUIDs is the double-check the write side performs under its
	// lock: if a row already claims one of these, bind to that row
	// instead of creating a second one. Set only when claiming such a
	// row would be correct -- a caller that deliberately refused a
	// candidate (an untrustworthy UUID, say) passes nil, so this cannot
	// re-admit a decision the caller already made.
	//
	// It closes the window between the caller's lookup and this write,
	// which two concurrent installs of the same machine would otherwise
	// both pass through.
	ClaimUUIDs []string
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
