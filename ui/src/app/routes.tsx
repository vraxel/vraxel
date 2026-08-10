import { Navigate, type RouteObject } from "react-router"
import RootLayout from "@/app/layouts/root-layout"
import LoginPage from "@/app/pages/login"
import ApiDocsPage from "@/app/pages/api-docs"
import AuthCallbackPage from "@/app/pages/auth-callback"
import ErrorPage from "@/app/pages/error"
import { iamRoutes } from "@/modules/iam/pages/routes"
import { auditRoutes } from "@/modules/audit/pages/routes"
import { usePermissionStore } from "@/core/permission/permission-store"
import { getDefaultPath } from "@/core/permission/use-permission"
import { DEFAULT_PATH } from "@/core/registry/nav-config"

// eslint-disable-next-line react-refresh/only-export-components
function DefaultRedirect() {
  const permissions = usePermissionStore((s) => s.permissions)
  const target = permissions ? getDefaultPath(permissions) : DEFAULT_PATH
  return <Navigate to={target} replace />
}

export const routes: RouteObject[] = [
  {
    path: "/login",
    element: <LoginPage />,
  },
  {
    path: "/api-docs",
    element: <ApiDocsPage />,
  },
  {
    path: "/auth/callback",
    element: <AuthCallbackPage />,
  },
  {
    path: "/error",
    element: <ErrorPage />,
  },
  {
    path: "/",
    element: <RootLayout />,
    children: [
      { index: true, element: <DefaultRedirect /> },
      {
        path: "iam",
        children: iamRoutes,
      },
      {
        path: "audit",
        children: [
          { index: true, element: <Navigate to="/audit/logs" replace /> },
          ...auditRoutes,
        ],
      },
    ],
  },
  {
    path: "*",
    element: <DefaultRedirect />,
  },
]
