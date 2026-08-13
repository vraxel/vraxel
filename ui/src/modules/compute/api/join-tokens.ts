import type { ScopeRef } from "@/core/registry/resource"
import type { AgentJoinToken } from "./types"
import { demoJoinTokens, demoRegisteredHost } from "./demo-data"

// DEMO: fixtures until pkg/apis/compute exposes agent-join-tokens.
//
// Note what the create signature does NOT take: scope. The backend
// derives it from the route (ctx.Scope.Parts()) and treats the body's
// scope fields as read-only, so a scope argument here would only invite
// the frontend to think it decides placement. It does not.

const LATENCY_MS = 260

function delay<T>(value: T): Promise<T> {
  return new Promise((resolve) => setTimeout(() => resolve(value), LATENCY_MS))
}

export interface CreateJoinTokenInput {
  /** Free-text label, for the pending-token list only. */
  name?: string
  /** Reserves the host name for whoever redeems this token. */
  hostName?: string
}

export const joinTokensApi = {
  list: async (_scope: ScopeRef): Promise<{ items: AgentJoinToken[]; totalCount: number }> =>
    delay({ items: demoJoinTokens, totalCount: demoJoinTokens.length }),
}

export async function createJoinToken(
  _scope: ScopeRef,
  input: CreateJoinTokenInput,
): Promise<AgentJoinToken> {
  const plaintext = `vra1.${randomSegment(22)}.${randomSegment(12)}`
  return delay({
    metadata: { id: String(Date.now()), name: input.name ?? "", createdAt: new Date().toISOString() },
    spec: {
      scope: "platform",
      hostName: input.hostName,
      // A token that reserves a name is pinned to one use: one name, one
      // machine. Without a name it is still one-use by default; max uses
      // only becomes a control when batch onboarding lands.
      maxUses: 1,
      usedCount: 0,
      ttlHours: 24,
      expiresAt: new Date(Date.now() + 24 * 3600_000).toISOString(),
      token: plaintext,
    },
  })
}

// DEMO ONLY: stands in for polling the host list until the agent shows
// up. The real page will watch the hosts query for a row carrying this
// token's reserved name (or simply refetch on an interval).
export async function pollForRegisteredHost(): Promise<typeof demoRegisteredHost> {
  return new Promise((resolve) => setTimeout(() => resolve(demoRegisteredHost), 6000))
}

function randomSegment(len: number): string {
  const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
  let out = ""
  for (let i = 0; i < len; i++) out += alphabet[Math.floor(Math.random() * alphabet.length)]
  return out
}
