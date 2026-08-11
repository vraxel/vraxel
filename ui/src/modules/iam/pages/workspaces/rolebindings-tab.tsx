import { useMemo } from "react"
import { useParams, Navigate } from "react-router"
import {
  listWorkspaceRoleBindings,
  createWorkspaceRoleBindings,
  deleteWorkspaceRoleBinding,
  deleteWorkspaceRoleBindings,
  listWorkspaceRoles,
} from "@/modules/iam/api/rbac"
import { listWorkspaceUsers } from "@/modules/iam/api/users"
import { usePermission } from "@/core/permission/use-permission"
import { usePermissionStore } from "@/core/permission/permission-store"
import {
  RoleBindingListView,
  type RoleBindingListConfig,
} from "@/modules/iam/components/rolebinding-list-view"

export default function WorkspaceRoleBindingsTab() {
  const workspaceId = useParams().workspaceId!
  const { hasPermission } = usePermission()
  const permissionsLoaded = usePermissionStore((s) => s.permissions) !== null

  const config = useMemo<RoleBindingListConfig>(
    () => ({
      listBindings: (params) => listWorkspaceRoleBindings(workspaceId, params),
      createBindings: (ids, roleId) => createWorkspaceRoleBindings(workspaceId, ids, roleId),
      deleteBinding: (id) => deleteWorkspaceRoleBinding(workspaceId, id),
      deleteBindings: (ids) => deleteWorkspaceRoleBindings(workspaceId, ids),
      listRoles: (params) => listWorkspaceRoles(workspaceId, params),
      listUsers: (params) => listWorkspaceUsers(workspaceId, { ...params, available: "true" }),
      permCreate: "iam:rolebindings:create",
      permDelete: "iam:rolebindings:delete",
      scope: "workspace",
      scopeParams: { workspaceId },
    }),
    [workspaceId],
  )

  if (permissionsLoaded && !hasPermission("iam:rolebindings:list", { workspaceId })) {
    return <Navigate to="/" replace />
  }

  return <RoleBindingListView config={config} />
}
