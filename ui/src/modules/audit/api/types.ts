import type { TypeMeta } from "@/core/api/types"

// --- AuditLog ---

export interface AuditLogSpec {
  id: string
  userId?: string
  username: string
  eventType: "api_operation" | "authentication"
  action: string
  resourceType?: string
  resourceId?: string
  module?: string
  scope: "platform" | "workspace" | "namespace"
  workspaceId?: string
  namespaceId?: string
  httpMethod?: string
  httpPath?: string
  statusCode?: number
  clientIp?: string
  userAgent?: string
  durationMs?: number
  success: boolean
  detail?: Record<string, unknown>
  responseDetail?: Record<string, unknown>
  createdAt: string
}

export interface AuditLog extends TypeMeta {
  spec: AuditLogSpec
}

export interface AuditLogList extends TypeMeta {
  items: AuditLog[]
  totalCount: number
}
