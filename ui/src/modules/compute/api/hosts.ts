import type { ListParams } from "@/core/api/types"
import type { ScopeRef } from "@/core/registry/resource"
import type { Host } from "./types"
import { demoHosts } from "./demo-data"

// DEMO: served from fixtures until pkg/apis/compute exposes the host
// slice. The signatures below are the ones the real client will keep
// (scope first, then params -- the shape frameworks/list expects), so
// switching over replaces the bodies and nothing else.
//
// Real bodies will be:
//   listHosts:  apiRequest(computeApi.get(pathFor(hostsDef, scope), {...}).json())
//   getHost:    apiRequest(computeApi.get(pathFor(hostsDef, scope, id)).json())

const LATENCY_MS = 220

function delay<T>(value: T): Promise<T> {
  return new Promise((resolve) => setTimeout(() => resolve(value), LATENCY_MS))
}

function matches(host: Host, search: string): boolean {
  if (!search) return true
  const needle = search.toLowerCase()
  return [
    host.metadata.name,
    host.spec.displayName,
    host.spec.reportedPrimaryIp,
    host.spec.os,
  ].some((v) => (v ?? "").toLowerCase().includes(needle))
}

export const hostsApi = {
  list: async (
    _scope: ScopeRef,
    params?: ListParams,
  ): Promise<{ items: Host[]; totalCount: number }> => {
    const search = String(params?.search ?? "")
    // useListQuery drops a filter set to "all" before it reaches here, so
    // an empty string is the only "unset" this has to handle.
    const status = String(params?.agentStatus ?? "")
    const filtered = demoHosts
      .filter((h) => matches(h, search))
      .filter((h) => !status || h.spec.agentStatus === status)
    return delay({ items: filtered, totalCount: filtered.length })
  },

  get: async (_scope: ScopeRef, id: string): Promise<Host> => {
    const found = demoHosts.find((h) => h.metadata.id === id)
    if (!found) throw new Error(`host ${id} not found`)
    return delay(found)
  },
}

export interface CreateHostInput {
  name: string
  description?: string
  /** Optional: a host reached only through an outbound agent has no
   *  address the control plane can dial. */
  ip?: string
  sshPort?: number
}

/**
 * DEMO: creates the record for a host that already exists somewhere.
 *
 * The row is written here and not at the end of the wizard, which is the
 * whole reason installing the agent can be a separate step, skipped, and
 * done days later from the host detail page. agentStatus starts empty
 * rather than "offline": no agent has ever been bound, which is a
 * different thing from one that is not answering.
 */
export async function createHost(_scope: ScopeRef, input: CreateHostInput): Promise<Host> {
  return delay({
    metadata: {
      id: String(Date.now()),
      name: input.name,
      createdAt: new Date().toISOString(),
    },
    spec: {
      description: input.description,
      reportedPrimaryIp: input.ip,
      sshPort: input.sshPort,
      origin: "manual",
    },
  })
}
