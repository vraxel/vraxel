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
	// AgentID is the machine's stable agent identity. Used only to derive
	// a deterministic suffix when the preferred host name is taken.
	AgentID string

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
	// RegisterAgentHost returns the host id bound to this agent. It must
	// be idempotent for a given ExistingHostID: repeated registrations of
	// one machine converge on one row.
	RegisterAgentHost(ctx context.Context, spec AgentHostSpec) (int64, error)
	// UnregisterAgentHost deletes a host row this registration created but
	// failed to finish binding. Only ever called with an id
	// RegisterAgentHost just returned from its create branch.
	UnregisterAgentHost(ctx context.Context, hostID int64) error
}
