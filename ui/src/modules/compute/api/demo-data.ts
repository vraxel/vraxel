// DEMO FIXTURES -- delete this whole file when pkg/apis/compute exposes
// the host slice (docs/agent/onboarding-design.md §9).
//
// It exists so the onboarding flow can be reviewed as a running screen
// before the backend lands. Everything it fakes is confined here: the
// pages import only api/hosts.ts and api/join-tokens.ts, whose signatures
// already match the real endpoints, so swapping them over is a change to
// two files and a deletion of this one.

import type { AgentJoinToken, Host } from "./types"

const now = Date.now()
const minutesAgo = (m: number) => new Date(now - m * 60_000).toISOString()
const daysAgo = (d: number) => new Date(now - d * 86_400_000).toISOString()

export const demoHosts: Host[] = [
  {
    metadata: { id: "1", name: "node-12", createdAt: daysAgo(31) },
    spec: {
      displayName: "node-12",
      description: "中间件测试机",
      hostname: "node-12",
      os: "Rocky Linux 8.9",
      arch: "amd64",
      cpuCores: 8,
      memoryMb: 16384,
      diskGb: 200,
      reportedPrimaryIp: "10.1.1.12",
      scope: "platform",
      createdByName: "张磊",
      agentStatus: "online",
      agentVersion: "v0.4.1",
      agentId: "3f2a9c14-8d5e-4b71-9e02-6c1f8a7d4b33",
      agentLastSeenAt: minutesAgo(0),
      agentConnectedAt: daysAgo(6),
    },
  },
  {
    metadata: { id: "2", name: "node-13", createdAt: daysAgo(31) },
    spec: {
      displayName: "node-13",
      hostname: "node-13",
      os: "Ubuntu 22.04.4 LTS",
      arch: "amd64",
      cpuCores: 16,
      memoryMb: 32768,
      diskGb: 500,
      reportedPrimaryIp: "10.1.1.13",
      scope: "platform",
      createdByName: "张磊",
      agentStatus: "online",
      agentVersion: "v0.4.1",
      agentId: "8b1d0e57-2a44-49f3-bb60-1d9e5c07a218",
      agentLastSeenAt: minutesAgo(0),
      agentConnectedAt: daysAgo(6),
    },
  },
  {
    metadata: { id: "3", name: "db-primary", createdAt: daysAgo(12) },
    spec: {
      displayName: "订单库主节点",
      description: "PostgreSQL 16 主库",
      hostname: "db-primary",
      os: "Debian 12",
      arch: "amd64",
      cpuCores: 32,
      memoryMb: 131072,
      diskGb: 2048,
      reportedPrimaryIp: "10.1.2.20",
      scope: "platform",
      createdByName: "李伟",
      agentStatus: "offline",
      agentVersion: "v0.4.0",
      agentId: "c07e4a91-5f38-4d2b-8a16-77b3e9d05c4a",
      agentLastSeenAt: minutesAgo(47),
      agentConnectedAt: daysAgo(11),
    },
  },
  {
    metadata: { id: "4", name: "edge-arm-01", createdAt: daysAgo(3) },
    spec: {
      displayName: "边缘节点 01",
      hostname: "edge-arm-01",
      os: "openEuler 22.03",
      arch: "arm64",
      cpuCores: 4,
      memoryMb: 8192,
      diskGb: 128,
      reportedPrimaryIp: "10.2.7.5",
      scope: "platform",
      createdByName: "王芳",
      agentStatus: "online",
      agentVersion: "v0.4.1",
      agentId: "1a5c8f30-9b27-4e6d-a04f-2e8b6c13d975",
      agentLastSeenAt: minutesAgo(1),
      agentConnectedAt: daysAgo(3),
    },
  },
  {
    // Two live agent processes claiming one identity -- what a cloned disk
    // produces. Present so the list's conflict state is reviewable.
    metadata: { id: "5", name: "app-tpl-clone", createdAt: daysAgo(1) },
    spec: {
      displayName: "app-tpl-clone",
      hostname: "app-tpl",
      os: "Rocky Linux 9.3",
      arch: "amd64",
      cpuCores: 8,
      memoryMb: 16384,
      diskGb: 200,
      reportedPrimaryIp: "10.1.5.31",
      scope: "platform",
      createdByName: "王芳",
      agentStatus: "offline",
      agentVersion: "v0.4.1",
      agentId: "44e9b2c8-6013-4a7f-9d52-b8f10c3e6a27",
      agentLastSeenAt: minutesAgo(4),
      agentConnectedAt: daysAgo(1),
      agentConflictAt: minutesAgo(4),
    },
  },
]

export const demoJoinTokens: AgentJoinToken[] = [
  {
    metadata: { id: "9", name: "机房 B 扩容", createdAt: minutesAgo(35) },
    spec: {
      scope: "platform",
      maxUses: 1,
      usedCount: 0,
      expiresAt: new Date(now + 23 * 3600_000).toISOString(),
      createdByName: "张磊",
    },
  },
  {
    metadata: { id: "10", name: "", createdAt: minutesAgo(210) },
    spec: {
      scope: "platform",
      hostName: "cache-02",
      maxUses: 1,
      usedCount: 0,
      expiresAt: new Date(now + 20 * 3600_000).toISOString(),
      createdByName: "李伟",
    },
  },
]

// demoRegisteredHost is what the wizard's final step reveals once the
// pasted command has run: the host row the agent's registration created.
export const demoRegisteredHost: Host = {
  metadata: { id: "99", name: "cache-02", createdAt: new Date(now).toISOString() },
  spec: {
    displayName: "cache-02",
    hostname: "cache-02",
    os: "Rocky Linux 9.3",
    arch: "amd64",
    cpuCores: 4,
    memoryMb: 8192,
    diskGb: 100,
    reportedPrimaryIp: "10.1.5.44",
    scope: "platform",
    createdByName: "张磊",
    agentStatus: "online",
    agentVersion: "v0.4.1",
    agentLastSeenAt: new Date(now).toISOString(),
  },
}
