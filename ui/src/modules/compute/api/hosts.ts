import { defineAction, defineResourceApi, defineSubApi, defineVerb } from "@/core/api/resource-api"
import type { ListParams } from "@/core/api/types"
import { hostsDef } from "../defs"
import type { Host, HostList } from "./types"
import type {
  HostAccounts,
  HostMergeRequest,
  HostMergeResponse,
  HostMetrics,
  HostProcesses,
} from "@/generated/compute"

// Params the list route understands beyond the standard ones. Both are
// server-side filters, so the toolbar does not have to hold the whole
// table to narrow it.
export interface HostListParams extends ListParams {
  /** "online" | "offline" | "none" -- none selects hosts with no agent
   *  bound at all, which is where an imported host starts. */
  agentStatus?: string
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

// Fold a duplicate record into the host in the URL, which survives.
//
// Needed because the backend splits rather than merges whenever it
// cannot prove two machines are one: a replaced motherboard and a fresh
// clone leave identical evidence, and only the operator knows which
// happened. This is how they say so.
export const mergeHost = defineAction<HostMergeRequest, HostMergeResponse>(hostsDef, "merge")

// Hosts built from the same disk image as this one -- the candidates for
// a merge, and the evidence for deciding one.
//
// defineSubApi rather than defineVerb because the answer is a list
// shape; both build the same /{id}/{segment} URL.
export const hostImageSiblingsApi = defineSubApi<Host>(hostsDef, "image-siblings")

// One windowed read of the host's utilisation charts. The window rides
// as snake_case query params; the answer is a fixed grid of derived
// chart series (see HostMetrics in the generated types), so the caller
// plots what it gets and never computes a rate.
export const hostMetricsApi = defineVerb<HostMetrics>(hostsDef, "metrics")

// What the host is running: a grouped workload snapshot with the ports
// each one listens on. Same verb shape as metrics, and under the same
// permission -- what a machine runs is the same class of fact as how
// loaded it is.
export const hostProcessesApi = defineVerb<HostProcesses>(hostsDef, "processes")

// Who can use the host and what they can do on it.
//
// defineVerb builds the same /{id}/accounts URL, but this one is NOT a
// verb on the server: it is a nested resource with a permission code of
// its own (compute:hosts:accounts:list), because "may read this host's
// details" should not automatically carry which accounts exist, which
// can log in and which are root. A caller without that code gets a 403
// here while the rest of the page keeps working.
export const hostAccountsApi = defineVerb<HostAccounts>(hostsDef, "accounts")
