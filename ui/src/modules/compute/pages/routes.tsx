import { Navigate, type RouteObject } from "react-router"
import HostListPage from "./hosts/list"
import HostDetailPage from "./hosts/detail"
import HostOnboardPage from "./hosts/onboard"

// Every resource declared in NAV_ITEMS with workspace / namespace scopes
// must be routable at all three depths: buildScopedPath turns the scope
// selector into `/compute/workspaces/{ws}/hosts`, and a missing route
// falls through to the `*` catch-all, which redirects to the default
// (platform) path -- so picking a workspace would silently bounce the
// selector back to "all".
//
// "hosts/onboard" precedes "hosts/:hostId" so it is not swallowed as an id.
const hostRoutes = (prefix: string): RouteObject[] => [
  { path: `${prefix}hosts`, element: <HostListPage /> },
  { path: `${prefix}hosts/onboard`, element: <HostOnboardPage /> },
  { path: `${prefix}hosts/:hostId`, element: <HostDetailPage /> },
]

export const computeRoutes: RouteObject[] = [
  { index: true, element: <Navigate to="/compute/hosts" replace /> },
  ...hostRoutes(""),
  ...hostRoutes("workspaces/:workspaceId/"),
  ...hostRoutes("workspaces/:workspaceId/namespaces/:namespaceId/"),
]
