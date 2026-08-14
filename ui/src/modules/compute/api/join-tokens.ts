import { defineResourceApi } from "@/core/api/resource-api"
import { agentJoinTokensDef } from "../defs"
import type { AgentJoinToken, AgentJoinTokenList } from "./types"

// Minting a token returns the only copy of its plaintext. There is no
// second chance and no recovery path: the server stores a SHA-256 hash.
export interface CreateJoinTokenBody {
  metadata?: { name?: string }
  spec?: {
    /** Binds the token to a host that already exists, so the agent that
     *  redeems it adopts that row instead of creating a second one. */
    targetHostId?: string
    ttlHours?: number
  }
}

// Scope is deliberately not in the body: the backend derives it from the
// route (ctx.Scope) and treats the body's scope fields as read-only. A
// scope argument here would invite the frontend to think it decides
// placement, and it does not -- whichever scope the operator is standing
// in is the scope the machine lands in.
export const joinTokensApi = defineResourceApi<
  AgentJoinToken,
  AgentJoinTokenList,
  Record<string, never>,
  CreateJoinTokenBody
>(agentJoinTokensDef)
