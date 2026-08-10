import { api } from "@/core/api/client"

export const auditApi = api.extend({ prefix: "/api/audit/v1" })
