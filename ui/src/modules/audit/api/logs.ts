import { auditApi } from "./client"
import { apiRequest } from "@/core/api/client"
import type { ListParams } from "@/core/api/types"
import type { ScopeRef } from "@/core/registry/resource"
import type { AuditLog, AuditLogList } from "./types"

export async function listAuditLogs(params?: ListParams): Promise<AuditLogList> {
  return apiRequest(auditApi.get("logs", { searchParams: params as Record<string, string> }).json())
}

export async function getAuditLog(id: string): Promise<AuditLog> {
  return apiRequest(auditApi.get(`logs/${id}`).json())
}

// Audit rows carry their id in spec.id -- there is no ObjectMeta on the
// wire, so defineResourceApi would type-lie about a metadata the backend
// never sends. frameworks/list keys rows by metadata.id; this adapter
// stamps a minimal metadata onto each row instead of teaching the
// framework a second id convention. Scope is ignored: logs are
// platform-only.
export type AuditLogRow = AuditLog & { metadata: { id: string } }

export const auditLogsApi = {
  list: async (
    _s: ScopeRef,
    params?: ListParams,
  ): Promise<{ items: AuditLogRow[]; totalCount: number }> => {
    const data = await listAuditLogs(params)
    return {
      items: (data.items ?? []).map((l) => ({ ...l, metadata: { id: l.spec.id } })),
      totalCount: data.totalCount,
    }
  },
}
