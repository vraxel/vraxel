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
  // How the record came into existence. Fixed at creation and never
  // changed afterwards -- distinct from how the control plane reaches
  // the host today, which does change (an imported host that installs an
  // agent later has both an SSH address and an agent channel).
  origin?: "agent" | "manual" | "cloud"
  // Only meaningful for a record that was not created by an agent.
  sshPort?: number
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
  // The host this token attaches its agent to, when it has one. Set for
  // a record that already exists (imported by hand, or provisioned from
  // a cloud pool): the registering agent adopts that row instead of
  // creating a second one, and max uses is pinned to 1 -- one host, one
  // machine. Empty for the agent-onboarding path, where the record does
  // not exist until the agent brings it into being.
  //
  // Binding on id rather than reserving a name: the row is already there
  // in every case that needs this, so an id says exactly what a
  // name-shaped placeholder only approximates.
  targetHostId?: string
  targetHostName?: string
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
