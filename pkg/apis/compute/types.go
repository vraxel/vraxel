package compute

import (
	"time"

	apitypes "vraxel.io/vraxel/lib/api/types"
	"vraxel.io/vraxel/lib/runtime"
)

// HostSpec is what an operator sees of a host.
//
// There is no CPU / memory / disk form anywhere: those arrive from the
// agent and a hand-typed copy would be stale within a quarter. Only
// DisplayName and Description are writable; everything else is either
// reported by the machine or fixed when the record was created.
type HostSpec struct {
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`

	// --- reported by the agent, read-only ---
	Hostname          string `json:"hostname,omitempty"`
	OS                string `json:"os,omitempty"`
	Arch              string `json:"arch,omitempty"`
	CPUCores          int32  `json:"cpuCores,omitempty"`
	MemoryMB          int64  `json:"memoryMb,omitempty"`
	DiskGB            int64  `json:"diskGb,omitempty"`
	ReportedPrimaryIP string `json:"reportedPrimaryIp,omitempty"`

	// --- set at creation, read-only afterwards ---
	// Origin is how this record came into existence: "agent" (the machine
	// onboarded itself) or "manual" (a human entered it). It never
	// changes. ConnectivityMode is how the control plane reaches the host
	// today and does change -- an imported host that installs an agent
	// later keeps origin "manual" and flips mode to "agent". Nothing may
	// infer one from the other.
	Origin           string `json:"origin,omitempty"`
	ConnectivityMode string `json:"connectivityMode,omitempty"`

	// IP is the address an operator recorded, dialable by the control
	// plane. Optional, and absent for a host reached only through its
	// agent. Distinct from ReportedPrimaryIP, which the agent observed
	// from inside the host and which may be unroutable from here.
	IP      string `json:"ip,omitempty"`
	SSHPort int32  `json:"sshPort,omitempty"`

	Scope       string `json:"scope,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	NamespaceID string `json:"namespaceId,omitempty"`
	// CLAUDE.md: any listable resource shows its creator.
	CreatedByName string `json:"createdByName,omitempty"`

	// --- agent session ---
	// AgentStatus is "online", "offline", or empty when no agent has ever
	// bound to this host. The empty case is not a missing value: it is
	// the state an imported host sits in until someone installs one, and
	// it is what the list renders as "not installed".
	AgentStatus      string     `json:"agentStatus,omitempty"`
	AgentID          string     `json:"agentId,omitempty"`
	AgentVersion     string     `json:"agentVersion,omitempty"`
	AgentConnectedAt *time.Time `json:"agentConnectedAt,omitempty"`
	AgentLastSeenAt  *time.Time `json:"agentLastSeenAt,omitempty"`
	// AgentConflictAt is set while two live agent processes claim this
	// host's identity, which is what a cloned disk produces. The gateway
	// refuses every channel for the host until it clears.
	AgentConflictAt *time.Time `json:"agentConflictAt,omitempty"`
}

// Host is a managed machine.
// +openapi:description=主机：通过 agent 纳管或手工录入的机器
type Host struct {
	runtime.TypeMeta `json:",inline"`
	Metadata         apitypes.ObjectMeta `json:"metadata"`
	Spec             HostSpec            `json:"spec"`
}

func (h *Host) GetTypeMeta() *runtime.TypeMeta { return &h.TypeMeta }

// AgentJoinTokenSpec is a one-shot registration credential.
//
// The plaintext is returned once, by create, and never again: only its
// SHA-256 hash is stored. The resource is marked Sensitive so the audit
// log does not capture the create response body.
type AgentJoinTokenSpec struct {
	Scope       string `json:"scope,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	NamespaceID string `json:"namespaceId,omitempty"`

	// TargetHostID binds the token to a host that already exists: the
	// agent redeeming it adopts that row instead of creating one. Empty
	// for the onboarding path, where the record does not exist until the
	// agent brings it into being. A bound token is always single-use --
	// one host, one machine.
	TargetHostID   string `json:"targetHostId,omitempty"`
	TargetHostName string `json:"targetHostName,omitempty"`

	MaxUses   int32      `json:"maxUses,omitempty"`
	UsedCount int32      `json:"usedCount,omitempty"`
	TTLHours  int32      `json:"ttlHours,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`

	// Token is the plaintext, present only in a create response.
	Token         string `json:"token,omitempty"`
	CreatedByName string `json:"createdByName,omitempty"`
}

// AgentJoinToken is a pending onboarding.
// +openapi:description=Agent 接入令牌：主机侧执行 install-agent.sh 时携带，用于换取长期 agent token。明文只在创建响应中返回一次，库内仅存 SHA-256 哈希。
type AgentJoinToken struct {
	runtime.TypeMeta `json:",inline"`
	Metadata         apitypes.ObjectMeta `json:"metadata"`
	Spec             AgentJoinTokenSpec  `json:"spec"`
}

func (t *AgentJoinToken) GetTypeMeta() *runtime.TypeMeta { return &t.TypeMeta }
