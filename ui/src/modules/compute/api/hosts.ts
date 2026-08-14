import { defineResourceApi } from "@/core/api/resource-api"
import type { ListParams } from "@/core/api/types"
import { hostsDef } from "../defs"
import type { Host, HostList } from "./types"

// Params the list route understands beyond the standard ones. Both are
// server-side filters, so the toolbar does not have to hold the whole
// table to narrow it.
export interface HostListParams extends ListParams {
  /** "online" | "offline" | "none" -- none selects hosts with no agent
   *  bound at all, which is where an imported host starts. */
  agentStatus?: string
  /** "agent" | "manual" -- how the record came into existence. */
  origin?: string
}

// Create sends only what an operator can decide. Everything else on the
// row is reported by the agent or fixed by the server.
export interface CreateHostBody {
  metadata: { name: string }
  spec: {
    displayName?: string
    description?: string
    ip?: string
    sshPort?: number
  }
}

// Update is narrower still: the backend accepts these two and ignores
// the rest, because anything agent-reported would be overwritten on the
// next heartbeat and the name is the host's stable identifier.
export interface UpdateHostBody {
  spec: {
    displayName?: string
    description?: string
  }
}

export const hostsApi = defineResourceApi<
  Host,
  HostList,
  HostListParams,
  CreateHostBody,
  UpdateHostBody
>(hostsDef)
