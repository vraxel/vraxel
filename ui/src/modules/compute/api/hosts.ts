import { defineAction, defineResourceApi, defineSubApi } from "@/core/api/resource-api"
import type { ListParams } from "@/core/api/types"
import { hostsDef } from "../defs"
import type { Host, HostList } from "./types"
import type { HostMergeRequest, HostMergeResponse } from "@/generated/compute"

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
// defineSubApi rather than defineVerb: the backend registers verbs at
// /{resource}/{id}/{verb} (apiserver resource.go), while defineVerb
// builds the colon form no route matches.
export const hostImageSiblingsApi = defineSubApi<Host>(hostsDef, "image-siblings")
