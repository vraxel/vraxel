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
  list: async (_scope: ScopeRef, params?: ListParams): Promise<{ items: Host[]; totalCount: number }> => {
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
