package agentgw

import (
	"context"

	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

// Type aliases re-export the gateway's store-layer interfaces and rows so
// other modules (compute's agent-join-tokens resource) consume them as
// agentgw.<Name> without importing agentgw/store, which the cross-module
// store rule forbids.
type (
	JoinTokenStore       = gwstore.JoinTokenStore
	JoinTokenRow         = gwstore.JoinTokenRow
	JoinTokenCreateInput = gwstore.JoinTokenCreateInput
	AgentRow             = gwstore.AgentRow
)

// AgentHostSpec is what an agent reports about the machine it runs on,
// normalised for host-row creation.
type AgentHostSpec struct {
	// ExistingHostID is non-zero when this agent id already has a host
	// row, i.e. the machine is re-registering. The registrar then
	// refreshes facts in place instead of inserting.
	ExistingHostID int64
	// NameSeed disambiguates the host name when the machine's preferred
	// one is taken. Any per-machine constant does; it is the machine's
	// strongest identity signal rather than its agent id, because an
	// agent id is now allocated fresh on every first registration. A
	// machine that lost its host binding and re-registers would otherwise
	// pick a different name each time, accumulating rows -- the exact
	// behaviour the deterministic suffix exists to prevent.
	NameSeed string

	Hostname string
	OS       string
	Arch     string
	CPUCores int32
	MemoryMB int64
	DiskGB   int64
	// PrimaryIP is the agent's default-route IPv4. Stored as
	// hosts.reported_primary_ip: display, log labels and PaaS peer
	// addresses only -- LCP never dials it (design §5.13).
	PrimaryIP string

	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
	CreatedBy   *int64
}

// HostRegistrar creates or refreshes the hosts row backing an agent.
//
// hosts belongs to compute, so the gateway never writes it directly; the
// implementation is injected at assembly time (pkg/apis/install.go). This
// is the same shape as every other cross-module data path in the tree:
// interface declared by the consumer, impl exposed by the owner.
type HostRegistrar interface {
	// RegisterAgentHost returns the host id bound to this agent, and
	// whether this call created that row rather than adopting one that
	// already existed. It must be idempotent for a given ExistingHostID:
	// repeated registrations of one machine converge on one row.
	//
	// created is what makes the rollback safe. Only the registrar knows
	// which branch it took, and the caller must not guess: today an
	// empty ExistingHostID happens to imply the create branch, but the
	// moment a join token can name a host to attach to, that inference
	// silently becomes "delete the row the operator just imported".
	RegisterAgentHost(ctx context.Context, spec AgentHostSpec) (hostID int64, created bool, err error)
	// UnregisterAgentHost deletes a host row this registration created
	// but failed to finish binding. Only ever called with an id
	// RegisterAgentHost just reported as created.
	UnregisterAgentHost(ctx context.Context, hostID int64) error
}
