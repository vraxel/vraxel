// Wire types for the compute module. Mirrors what pkg/apis/compute will
// return once the host slice lands (docs/agent/onboarding-design.md §9);
// the shapes are fixed by that design, so pages written against them do
// not change when the demo fixtures are swapped for real requests.

export interface ObjectMeta {
  id: string
  name: string
  createdAt?: string
  updatedAt?: string
}

// HostSpec carries only what an agent reports plus what an operator may
// edit. No SSH port, no credentials, no disks/NICs: an agent-managed host
// is described by the machine itself.
export interface HostSpec {
  displayName?: string
  description?: string
  hostname?: string
  os?: string
  arch?: string
  cpuCores?: number
  memoryMb?: number
  diskGb?: number
  reportedPrimaryIp?: string
  scope?: string
  workspaceId?: string
  namespaceId?: string
  createdByName?: string
  // --- agent session (host_agents) ---
  // The list's primary status column. `hosts.status` is deliberately not
  // shown: under agent-only management it carries no information the
  // agent's online/offline does not already carry, and two status columns
  // side by side only invite "running but offline" confusion.
  agentStatus?: "online" | "offline"
  agentVersion?: string
  agentId?: string
  agentLastSeenAt?: string
  agentConnectedAt?: string
  // Set while two live agent processes are claiming this host's identity
  // (a cloned disk); the gateway refuses both until it clears.
  agentConflictAt?: string
}

export interface Host {
  metadata: ObjectMeta
  spec: HostSpec
}

export interface HostList {
  items: Host[]
  totalCount: number
}

export interface AgentJoinTokenSpec {
  scope?: string
  workspaceId?: string
  namespaceId?: string
  // Optional host name reserved by this token. When set the registering
  // agent takes it instead of deriving one from its reported hostname,
  // and max uses is pinned to 1 -- one name, one machine.
  hostName?: string
  maxUses?: number
  usedCount?: number
  ttlHours?: number
  expiresAt?: string
  // Returned by create only, once and never again.
  token?: string
  createdByName?: string
}

export interface AgentJoinToken {
  metadata: ObjectMeta
  spec: AgentJoinTokenSpec
}

export interface AgentJoinTokenList {
  items: AgentJoinToken[]
  totalCount: number
}
