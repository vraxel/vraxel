import { defineResource } from "@/core/registry/resource"

// Audit logs are read-only and platform-only (pages/routes.tsx mounts
// /audit/logs with no workspace/namespace variants). No detailParam:
// the row detail is an inline dialog, not a route.
export const auditLogsDef = defineResource({
  module: "audit",
  name: "logs",
  scopes: ["platform"],
})
