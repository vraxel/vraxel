import { Navigate, type RouteObject } from "react-router"
import HostListPage from "./hosts/list"
import HostDetailPage from "./hosts/detail"
import HostOnboardPage from "./hosts/onboard"

// The onboarding wizard is a sibling route of the list rather than a
// dialog inside it, and it is declared before the :hostId detail route so
// "onboard" is not swallowed as an id.
export const computeRoutes: RouteObject[] = [
  { index: true, element: <Navigate to="/compute/hosts" replace /> },
  { path: "hosts", element: <HostListPage /> },
  { path: "hosts/onboard", element: <HostOnboardPage /> },
  { path: "hosts/:hostId", element: <HostDetailPage /> },
]
