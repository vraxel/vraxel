import { Navigate, type RouteObject } from "react-router"
import WorkspaceLayout from "@/app/layouts/workspace-layout"
import WorkspaceListPage from "./workspaces/list"
import WorkspaceDetailPage from "./workspaces/detail"
import WorkspaceUsersPage from "./workspaces/users"
import WorkspaceNamespacesPage from "./workspaces/namespaces-tab"
import NamespaceListPage from "./namespaces/list"
import NamespaceDetailPage from "./namespaces/detail"
import NamespaceUsersPage from "./namespaces/users"
import UserListPage from "./users/list"
import UserDetailPage from "./users/detail"
import RoleListPage from "./roles/list"
import RoleDetailPage from "./roles/detail"
import WorkspaceRolesTab from "./workspaces/roles-tab"
import NamespaceRolesTab from "./namespaces/roles-tab"
import ScopedRoleDetailPage from "./roles/scoped-detail"
import ScopedUserDetailPage from "./users/scoped-detail"

export const iamRoutes: RouteObject[] = [
  { index: true, element: <Navigate to="/iam/workspaces" replace /> },
  { path: "workspaces", element: <WorkspaceListPage /> },
  {
    path: "workspaces/:workspaceId",
    element: <WorkspaceLayout />,
    children: [
      { index: true, element: <WorkspaceDetailPage /> },
      { path: "users", element: <WorkspaceUsersPage /> },
      { path: "users/:userId", element: <ScopedUserDetailPage /> },
      { path: "namespaces", element: <WorkspaceNamespacesPage /> },
      { path: "roles", element: <WorkspaceRolesTab /> },
      { path: "roles/:roleId", element: <ScopedRoleDetailPage /> },
      { path: "namespaces/:namespaceId", element: <NamespaceDetailPage /> },
      { path: "namespaces/:namespaceId/users", element: <NamespaceUsersPage /> },
      { path: "namespaces/:namespaceId/users/:userId", element: <ScopedUserDetailPage /> },
      { path: "namespaces/:namespaceId/roles", element: <NamespaceRolesTab /> },
      { path: "namespaces/:namespaceId/roles/:roleId", element: <ScopedRoleDetailPage /> },
    ],
  },
  { path: "namespaces", element: <NamespaceListPage /> },
  { path: "namespaces/:namespaceId", element: <NamespaceDetailPage /> },
  { path: "users", element: <UserListPage /> },
  { path: "users/:userId", element: <UserDetailPage /> },
  { path: "roles", element: <RoleListPage /> },
  { path: "roles/:roleId", element: <RoleDetailPage /> },
]
