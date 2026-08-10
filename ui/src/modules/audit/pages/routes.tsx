import type { RouteObject } from "react-router"
import AuditLogListPage from "./logs"

export const auditRoutes: RouteObject[] = [
  { path: "logs", element: <AuditLogListPage /> },
]
